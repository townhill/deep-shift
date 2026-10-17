package sim

import "fmt"

// Sim is one deterministic cave simulation. It is not safe for concurrent
// use; callers that render on another goroutine should hand over Snapshots.
type Sim struct {
	initial Cave // immutable copy of the starting layout
	seed    uint64
	exits   []Pos // exit cells in row-major order; terrain never changes in v1
	st      state
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
	s := &Sim{initial: c.Clone(), seed: seed}
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
// After the cave is complete Step does nothing and returns nil.
func (s *Sim) Step(in Input) []Event {
	if s.st.status != StatusPlaying {
		return nil
	}
	var events []Event
	events = s.stepPlayer(in, events)
	events = s.stepQuota(events)
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
			return append(events, Event{st.tick, EventExitSealed, to})
		}
	}

	// The move succeeds. The only player is the mover, so the destination
	// holds no occupant or a crystal.
	dug := dst.Terrain == TerrainEarth
	collected := dst.Occupant == OccupantCrystal
	if dug {
		dst.Terrain = TerrainEmpty
	}
	if collected {
		st.collected++
	}
	st.cells[s.initial.Index(st.player)].Occupant = OccupantNone
	dst.Occupant = OccupantPlayer
	st.player = to
	st.moveCooldown = MoveIntervalTicks - 1

	events = append(events, Event{st.tick, EventMoved, to})
	if dug {
		events = append(events, Event{st.tick, EventDug, to})
	}
	if collected {
		events = append(events, Event{st.tick, EventCollected, to})
	}
	if dst.Terrain == TerrainExit {
		st.status = StatusComplete
		events = append(events, Event{st.tick, EventCaveComplete, to})
	}
	return events
}

// stepQuota is the world phase: open the exit once the quota is met.
func (s *Sim) stepQuota(events []Event) []Event {
	st := &s.st
	if st.exitOpen || st.collected < s.initial.RequiredCrystals {
		return events
	}
	st.exitOpen = true
	for _, p := range s.exits {
		events = append(events, Event{st.tick, EventExitOpened, p})
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
