package sim

import "fmt"

// Sim is one deterministic cave simulation. It is not safe for concurrent
// use; callers that render on another goroutine should hand over Snapshots.
type Sim struct {
	initial Cave // immutable copy of the starting layout
	seed    uint64
	exits   []Pos // exit cells in row-major order; terrain never changes
	st      state
	updated []bool // gravity scratch: cells whose object already moved this step
	pushed  int    // index of the boulder pushed this tick, or -1
}

// state is everything Reset restores. Every field must be covered by
// StateHash.
type state struct {
	tick         uint64
	cells        []Cell
	player       Pos
	collected    int
	exitOpen     bool
	status       Status
	moveCooldown uint8
	rng          SplitMix64
}

// New validates c and returns a simulation at tick 0. The cave is copied, so
// later changes to c do not affect the simulation.
func New(c Cave, seed uint64) (*Sim, error) {
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("sim: invalid cave: %w", err)
	}
	s := &Sim{initial: c.Clone(), seed: seed, updated: make([]bool, len(c.Cells))}
	for i, cell := range s.initial.Cells {
		if cell.Terrain == TerrainExit {
			s.exits = append(s.exits, Pos{i % c.Width, i / c.Width})
		}
	}
	s.Reset()
	return s, nil
}

// Reset restores the initial cave, counters, tick and RNG seed exactly.
func (s *Sim) Reset() {
	s.st = state{
		cells: append([]Cell(nil), s.initial.Cells...),
		rng:   NewSplitMix64(s.seed),
	}
	for i, cell := range s.st.cells {
		if cell.Occupant == OccupantPlayer {
			s.st.player = Pos{i % s.initial.Width, i / s.initial.Width}
			break
		}
	}
	// A zero quota opens the exit before the first tick; no event is emitted
	// because no tick has happened.
	s.st.exitOpen = s.initial.RequiredCrystals == 0
}

// Step advances the simulation by exactly one logical tick using in, and
// returns the events produced during that tick in a deterministic order.
// Once the cave is complete or the player is dead, Step does nothing and
// returns nil.
func (s *Sim) Step(in Input) []Event {
	if s.st.status != StatusPlaying {
		return nil
	}
	var events []Event
	s.pushed = -1
	events = s.stepPlayer(in, events)
	// Entering the exit ends the cave at once: the world phase is skipped.
	if s.st.status == StatusPlaying {
		if s.st.tick%GravityIntervalTicks == 0 {
			events = s.stepGravity(events)
		}
		if s.st.status == StatusPlaying {
			events = s.stepQuota(events)
		}
	}
	s.st.tick++
	return events
}

// stepPlayer is the player phase: cooldown, then at most one move.
func (s *Sim) stepPlayer(in Input, events []Event) []Event {
	st := &s.st
	if st.moveCooldown > 0 {
		st.moveCooldown--
		return events
	}
	if in.Move < DirUp || in.Move > DirRight {
		return events
	}
	to := st.player.Add(in.Move)
	if !s.initial.InBounds(to) {
		return events // the cave edge blocks like a wall
	}
	dst := &st.cells[s.initial.Index(to)]
	switch dst.Terrain {
	case TerrainWall:
		return events
	case TerrainExit:
		if !st.exitOpen {
			return append(events, Event{Tick: st.tick, Kind: EventExitSealed, Pos: to})
		}
	}

	// A boulder can only be pushed: horizontally, while resting, one cell into
	// empty floor. Anything else (a second boulder, earth, a wall, a crystal,
	// the exit, the cave edge) blocks the push and the move.
	var pushedTo Pos
	pushed := false
	if dst.Occupant == OccupantBoulder {
		beyond := to.Add(in.Move)
		if (in.Move != DirLeft && in.Move != DirRight) || dst.Falling ||
			!s.initial.InBounds(beyond) || !st.cells[s.initial.Index(beyond)].Empty() {
			return events
		}
		st.cells[s.initial.Index(beyond)] = Cell{Terrain: TerrainEmpty, Occupant: OccupantBoulder}
		dst.Occupant = OccupantNone
		pushedTo, pushed = beyond, true
		s.pushed = s.initial.Index(beyond)
	}

	// The move succeeds. The only player is the mover and pushing emptied a
	// boulder cell, so the destination now holds no occupant or a crystal,
	// which may be falling: walking into it still collects it.
	from := st.player
	dug := dst.Terrain == TerrainEarth
	collected := dst.Occupant == OccupantCrystal
	if dug {
		dst.Terrain = TerrainEmpty
	}
	if collected {
		st.collected++
	}
	st.cells[s.initial.Index(from)].Occupant = OccupantNone
	dst.Occupant, dst.Falling = OccupantPlayer, false
	st.player = to
	st.moveCooldown = MoveIntervalTicks - 1

	events = append(events, Event{Tick: st.tick, Kind: EventMoved, Pos: to, From: from})
	if dug {
		events = append(events, Event{Tick: st.tick, Kind: EventDug, Pos: to})
	}
	if collected {
		events = append(events, Event{Tick: st.tick, Kind: EventCollected, Pos: to})
	}
	if pushed {
		events = append(events, Event{Tick: st.tick, Kind: EventBoulderPushed, Pos: pushedTo, From: to, Object: OccupantBoulder})
	}
	if dst.Terrain == TerrainExit {
		st.status = StatusComplete
		events = append(events, Event{Tick: st.tick, Kind: EventCaveComplete, Pos: to})
	}
	return events
}

// stepGravity is the gravity phase for boulders and crystals lying on open
// floor. Rows are scanned bottom to top and each row left to right, so a
// supported stack falls together. An object moves at most one cell per tick:
// cells an object moved into are marked updated and skipped if the scan
// reaches them again (an object that rolled right), and a boulder pushed
// this tick sits out this step.
func (s *Sim) stepGravity(events []Event) []Event {
	clear(s.updated)
	if s.pushed >= 0 {
		s.updated[s.pushed] = true
	}
	w := s.initial.Width
	for y := s.initial.Height - 1; y >= 0; y-- {
		for x := range w {
			i := y*w + x
			c := s.st.cells[i]
			if c.Occupant.Falls() && c.Terrain == TerrainEmpty && !s.updated[i] {
				events = s.updateFalling(Pos{x, y}, events)
			}
		}
	}
	return events
}

// updateFalling applies, in order: fall into empty floor below; crush the
// player below if already falling; roll off a rounded blocker (left side
// first, then right) when the side cell and the cell below it are both
// empty; otherwise come to rest.
func (s *Sim) updateFalling(p Pos, events []Event) []Event {
	st := &s.st
	o := &st.cells[s.initial.Index(p)]
	obj := o.Occupant
	if below := p.Add(DirDown); s.initial.InBounds(below) {
		bc := st.cells[s.initial.Index(below)]
		switch {
		case bc.Empty():
			s.moveObject(p, below)
			return append(events, Event{Tick: st.tick, Kind: EventFell, Pos: below, From: p, Object: obj})
		case bc.Occupant == OccupantPlayer && o.Falling && st.status == StatusPlaying:
			// The object stops on the player; the attempt ends this tick.
			o.Falling = false
			st.status = StatusDead
			return append(events, Event{Tick: st.tick, Kind: EventPlayerCrushed, Pos: below, From: p, Object: obj})
		case bc.Occupant.Rounded():
			for _, d := range [...]Dir{DirLeft, DirRight} {
				side := p.Add(d)
				diag := side.Add(DirDown)
				if s.initial.InBounds(diag) && st.cells[s.initial.Index(side)].Empty() && st.cells[s.initial.Index(diag)].Empty() {
					s.moveObject(p, side)
					return append(events, Event{Tick: st.tick, Kind: EventRolled, Pos: side, From: p, Object: obj})
				}
			}
		}
	}
	if o.Falling {
		o.Falling = false
		events = append(events, Event{Tick: st.tick, Kind: EventLanded, Pos: p, Object: obj})
	}
	return events
}

// moveObject moves the boulder or crystal at from to the empty cell to,
// marks it as falling and as updated for the rest of this gravity step.
func (s *Sim) moveObject(from, to Pos) {
	fi, ti := s.initial.Index(from), s.initial.Index(to)
	s.st.cells[ti] = Cell{Terrain: TerrainEmpty, Occupant: s.st.cells[fi].Occupant, Falling: true}
	s.st.cells[fi] = Cell{Terrain: TerrainEmpty}
	s.updated[ti] = true
}

// stepQuota opens the exit once the quota is met.
func (s *Sim) stepQuota(events []Event) []Event {
	st := &s.st
	if st.exitOpen || st.collected < s.initial.RequiredCrystals {
		return events
	}
	st.exitOpen = true
	for _, p := range s.exits {
		events = append(events, Event{Tick: st.tick, Kind: EventExitOpened, Pos: p})
	}
	return events
}

// Tick returns the number of ticks simulated since the last Reset.
func (s *Sim) Tick() uint64 { return s.st.tick }

// Seed returns the RNG seed this simulation was created with.
func (s *Sim) Seed() uint64 { return s.seed }

// Width returns the cave width in cells.
func (s *Sim) Width() int { return s.initial.Width }

// Height returns the cave height in cells.
func (s *Sim) Height() int { return s.initial.Height }

// Cell returns the cell at p. Positions outside the cave read as walls.
func (s *Sim) Cell(p Pos) Cell {
	if !s.initial.InBounds(p) {
		return Cell{Terrain: TerrainWall}
	}
	return s.st.cells[s.initial.Index(p)]
}

// Player returns the player's cell.
func (s *Sim) Player() Pos { return s.st.player }

// Collected returns how many crystals have been collected.
func (s *Sim) Collected() int { return s.st.collected }

// Required returns the crystal quota.
func (s *Sim) Required() int { return s.initial.RequiredCrystals }

// Boulders returns the number of boulders in the cave.
func (s *Sim) Boulders() int {
	n := 0
	for _, c := range s.st.cells {
		if c.Occupant == OccupantBoulder {
			n++
		}
	}
	return n
}

// ExitOpen reports whether the quota has been met.
func (s *Sim) ExitOpen() bool { return s.st.exitOpen }

// Status returns the attempt status.
func (s *Sim) Status() Status { return s.st.status }

// MoveReady reports whether a move Input given to the next Step will be
// attempted (the movement window is open and the cave is still in play).
func (s *Sim) MoveReady() bool {
	return s.st.status == StatusPlaying && s.st.moveCooldown == 0
}

// Snapshot is a read-only copy of the observable simulation state, for
// renderers, HUDs and tests. Mutating it has no effect on the Sim.
type Snapshot struct {
	Tick                uint64
	Width, Height       int
	Cells               []Cell
	Player              Pos
	Collected, Required int
	ExitOpen            bool
	Status              Status
	MoveReady           bool
}

// Snapshot returns a deep copy of the current observable state.
func (s *Sim) Snapshot() Snapshot {
	return Snapshot{
		Tick:      s.st.tick,
		Width:     s.initial.Width,
		Height:    s.initial.Height,
		Cells:     append([]Cell(nil), s.st.cells...),
		Player:    s.st.player,
		Collected: s.st.collected,
		Required:  s.initial.RequiredCrystals,
		ExitOpen:  s.st.exitOpen,
		Status:    s.st.status,
		MoveReady: s.MoveReady(),
	}
}

// Cell returns the cell at p. Positions outside the cave read as walls.
func (s Snapshot) Cell(p Pos) Cell {
	if p.X < 0 || p.Y < 0 || p.X >= s.Width || p.Y >= s.Height {
		return Cell{Terrain: TerrainWall}
	}
	return s.Cells[p.Y*s.Width+p.X]
}
