package sim

import "fmt"

// Sim is one deterministic cave simulation. It is not safe for concurrent
// use; callers that render on another goroutine should hand over Snapshots.
type Sim struct {
	initial Cave // immutable copy of the starting layout
	seed    uint64
	exits   []Pos // exit cells in row-major order; terrain never changes
	st      state
	updated []bool  // gravity and creature scratch: cells whose occupant already moved this step
	pushed  int     // index of the boulder pushed this tick, or -1
	level   []uint8 // water scratch: levels the spread is computed from
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
	s := &Sim{initial: c.Clone(), seed: seed, updated: make([]bool, len(c.Cells)), level: make([]uint8, len(c.Cells))}
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
			if s.st.tick%CrawlerIntervalTicks == 0 {
				events = s.stepCreatures(events)
			}
			s.stepWater()
			events = s.drownCreatures(events)
		}
		if s.st.status == StatusPlaying {
			events = s.stepQuota(events)
		}
	}
	s.st.tick++
	return events
}

// stepPlayer is the player phase: cooldown, then at most one action: turn
// an adjacent valve, or move.
func (s *Sim) stepPlayer(in Input, events []Event) []Event {
	st := &s.st
	if st.moveCooldown > 0 {
		st.moveCooldown--
		return events
	}
	if in.Interact {
		if m, ok := s.adjacentMechanism(); ok {
			return s.turnValve(m, events)
		}
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
	case TerrainWall, TerrainGateClosed, TerrainValveShut, TerrainValveOpen:
		return events
	case TerrainExit:
		if !st.exitOpen {
			return append(events, Event{Tick: st.tick, Kind: EventExitSealed, Pos: to})
		}
	}
	if dst.Deep() {
		return append(events, Event{Tick: st.tick, Kind: EventTooDeep, Pos: to})
	}
	if dst.Occupant.Creature() {
		// Walking into a creature is fatal; the player does not move.
		st.status = StatusDead
		return append(events, Event{Tick: st.tick, Kind: EventPlayerCaught, Pos: st.player, From: to, Object: dst.Occupant})
	}

	// A boulder can only be pushed: horizontally, while resting, one cell into
	// empty floor. Anything else (a second boulder, earth, a wall, a crystal,
	// the exit, a gate, the cave edge) blocks the push and the move. Water
	// beyond does not: it is displaced into the boulder's old cell.
	var pushedTo Pos
	pushed := false
	if dst.Occupant == OccupantBoulder {
		beyond := to.Add(in.Move)
		if (in.Move != DirLeft && in.Move != DirRight) || dst.Falling ||
			!s.initial.InBounds(beyond) || !st.cells[s.initial.Index(beyond)].Empty() {
			return events
		}
		s.shift(to, beyond, false)
		pushedTo, pushed = beyond, true
		s.pushed = s.initial.Index(beyond)
	}

	// The move succeeds. The only player is the mover and pushing emptied a
	// boulder cell, so the destination now holds no occupant or a crystal,
	// which may be falling: walking into it still collects it. Water in the
	// destination stays; the player wades in it.
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
			if c.Occupant.Falls() && c.Terrain.Floor() && !s.updated[i] {
				events = s.updateFalling(Pos{x, y}, events)
			}
		}
	}
	return events
}

// updateFalling applies, in order: fall into empty floor below; if already
// falling, crush the player or a crawler below, or shatter a glimmer below
// (a boulder turning to crystal as it does);
// roll off a rounded blocker (left side first, then right) when the side cell
// and the cell below it are both empty; otherwise come to rest.
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
		case bc.Occupant == OccupantCrawler && o.Falling:
			// A falling object crushes a crawler and falls on into its cell.
			c := &st.cells[s.initial.Index(below)]
			c.Occupant, c.Facing = OccupantNone, DirNone
			s.moveObject(p, below)
			return append(events,
				Event{Tick: st.tick, Kind: EventCrawlerCrushed, Pos: below, From: p, Object: obj},
				Event{Tick: st.tick, Kind: EventFell, Pos: below, From: p, Object: obj})
		case bc.Occupant == OccupantGlimmer && o.Falling:
			// A falling object shatters a glimmer into crystals and is part
			// of the shattering: a boulder turns to crystal with the rest of
			// the block, a crystal comes to rest among them.
			events = append(events, Event{Tick: st.tick, Kind: EventGlimmerShattered, Pos: below, From: p, Object: obj})
			o.Falling = false
			if obj == OccupantBoulder {
				o.Occupant = OccupantNone
			}
			return s.shatter(below, events)
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

// stepCreatures is the creature phase, after gravity on CrawlerIntervalTicks
// ticks. Crawlers and glimmers are scanned together, row by row from the top,
// each row left to right, and each moves at most once: a cell a creature
// moved into is skipped if the scan reaches it again.
//
// A crawler follows the wall on its left: it tries the directions left of
// its facing, ahead, right and behind, in that order. A glimmer is its mirror
// image and follows the wall on its right: right, ahead, left, behind. Each
// takes the first direction open to it (see Cell.OpenToCreature). If that
// cell holds the player, it catches them instead of moving. With every
// direction blocked it stays.
func (s *Sim) stepCreatures(events []Event) []Event {
	clear(s.updated)
	w := s.initial.Width
	for i := range s.st.cells {
		if !s.st.cells[i].Occupant.Creature() || s.updated[i] || s.st.status != StatusPlaying {
			continue
		}
		events = s.updateCreature(Pos{i % w, i / w}, events)
	}
	return events
}

// leftTurns lists, for each facing, the directions a crawler tries: its
// left, ahead, its right, behind.
var leftTurns = [...][4]Dir{
	DirUp:    {DirLeft, DirUp, DirRight, DirDown},
	DirDown:  {DirRight, DirDown, DirLeft, DirUp},
	DirLeft:  {DirDown, DirLeft, DirUp, DirRight},
	DirRight: {DirUp, DirRight, DirDown, DirLeft},
}

// rightTurns lists, for each facing, the directions a glimmer tries: its
// right, ahead, its left, behind.
var rightTurns = [...][4]Dir{
	DirUp:    {DirRight, DirUp, DirLeft, DirDown},
	DirDown:  {DirLeft, DirDown, DirRight, DirUp},
	DirLeft:  {DirUp, DirLeft, DirDown, DirRight},
	DirRight: {DirDown, DirRight, DirUp, DirLeft},
}

func (s *Sim) updateCreature(p Pos, events []Event) []Event {
	st := &s.st
	c := &st.cells[s.initial.Index(p)]
	kind, moved, turns := c.Occupant, EventCrawlerMoved, leftTurns[c.Facing]
	if kind == OccupantGlimmer {
		moved, turns = EventGlimmerMoved, rightTurns[c.Facing]
	}
	for _, d := range turns {
		to := p.Add(d)
		if !s.initial.InBounds(to) {
			continue
		}
		dst := &st.cells[s.initial.Index(to)]
		if !dst.OpenToCreature() {
			continue
		}
		c.Facing = d
		if dst.Occupant == OccupantPlayer {
			st.status = StatusDead
			return append(events, Event{Tick: st.tick, Kind: EventPlayerCaught, Pos: to, From: p, Object: kind})
		}
		dst.Occupant, dst.Facing = kind, d
		c.Occupant, c.Facing = OccupantNone, DirNone
		s.updated[s.initial.Index(to)] = true
		return append(events, Event{Tick: st.tick, Kind: moved, Pos: to, From: p, Object: kind})
	}
	return events
}

// drownCreatures removes every crawler and glimmer whose cell holds water
// after the water phase, in row-major order. A drowned glimmer leaves no
// crystals.
func (s *Sim) drownCreatures(events []Event) []Event {
	w := s.initial.Width
	for i := range s.st.cells {
		c := &s.st.cells[i]
		if !c.Occupant.Creature() || c.Water == 0 {
			continue
		}
		kind := EventCrawlerDrowned
		if c.Occupant == OccupantGlimmer {
			kind = EventGlimmerDrowned
		}
		c.Occupant, c.Facing = OccupantNone, DirNone
		events = append(events, Event{Tick: s.st.tick, Kind: kind, Pos: Pos{i % w, i / w}})
	}
	return events
}

// shatter removes the glimmer at g and leaves a crystal in its cell and in
// every empty, dry floor cell of the 3×3 block around it, in row-major
// order. A shattering boulder has already been removed from its cell above g,
// so that cell is among them. Nothing else in the block changes: earth,
// walls, the exit, gates, valves, other objects, creatures, the player and
// cells holding water are left as they are, so water is conserved. A
// glimmer's cell is always dry (water drowns it in the step it arrives), so
// at least one crystal forms. The new crystals rest and sit out the rest of
// this gravity step, like a pushed boulder.
func (s *Sim) shatter(g Pos, events []Event) []Event {
	c := &s.st.cells[s.initial.Index(g)]
	c.Occupant, c.Facing = OccupantNone, DirNone
	for y := g.Y - 1; y <= g.Y+1; y++ {
		for x := g.X - 1; x <= g.X+1; x++ {
			p := Pos{x, y}
			if !s.initial.InBounds(p) {
				continue
			}
			i := s.initial.Index(p)
			if cell := &s.st.cells[i]; cell.Empty() && cell.Water == 0 {
				cell.Occupant = OccupantCrystal
				s.updated[i] = true
				events = append(events, Event{Tick: s.st.tick, Kind: EventCrystalFormed, Pos: p})
			}
		}
	}
	return events
}

// moveObject moves the boulder or crystal at from to the empty cell to,
// marks it as falling and as updated for the rest of this gravity step.
func (s *Sim) moveObject(from, to Pos) {
	s.shift(from, to, true)
	s.updated[s.initial.Index(to)] = true
}

// shift moves the boulder or crystal at from into the empty cell to. Any
// water in to is displaced into from, which the object leaves holding no
// water, so the swap always fits and conserves water. Terrain stays put.
func (s *Sim) shift(from, to Pos, falling bool) {
	f, t := &s.st.cells[s.initial.Index(from)], &s.st.cells[s.initial.Index(to)]
	t.Occupant, t.Falling = f.Occupant, falling
	f.Occupant, f.Falling = OccupantNone, false
	f.Water, t.Water = t.Water, 0
}

// stepWater is the water phase, after gravity on the same ticks. Water
// first falls, then spreads sideways; it only enters cells that hold water
// (see Cell.HoldsWater), so walls, earth, closed gates, valves, the exit,
// objects and the cave edge all contain it. Every transfer moves units
// between two cells, so the total never changes.
//
//  1. Fall: rows bottom to top; each cell pours as much as fits into the
//     cell below. Water moves at most one cell down per step.
//  2. Spread: for every pair of side-by-side cells, if one holds at least
//     two units more than the other, one unit flows to the lower. All pairs
//     are judged on the levels after the fall and applied together, so the
//     scan order cannot favour a direction.
func (s *Sim) stepWater() {
	w, h := s.initial.Width, s.initial.Height
	cells := s.st.cells
	for y := h - 2; y >= 0; y-- {
		for x := range w {
			c, b := &cells[y*w+x], &cells[(y+1)*w+x]
			if c.Water == 0 || !b.HoldsWater() {
				continue
			}
			n := min(c.Water, WaterCapacity-b.Water)
			c.Water -= n
			b.Water += n
		}
	}
	for i := range cells {
		s.level[i] = cells[i].Water
	}
	for y := range h {
		for x := range w - 1 {
			i := y*w + x
			if !cells[i].HoldsWater() || !cells[i+1].HoldsWater() {
				continue
			}
			switch a, b := s.level[i], s.level[i+1]; {
			case a >= b+2:
				cells[i].Water--
				cells[i+1].Water++
			case b >= a+2:
				cells[i+1].Water--
				cells[i].Water++
			}
		}
	}
}

// adjacentMechanism returns the index of the mechanism whose valve is next
// to the player, checking up, down, left, right in that order.
func (s *Sim) adjacentMechanism() (int, bool) {
	for _, d := range [...]Dir{DirUp, DirDown, DirLeft, DirRight} {
		p := s.st.player.Add(d)
		if !s.initial.InBounds(p) || !s.st.cells[s.initial.Index(p)].Terrain.Valve() {
			continue
		}
		for i, m := range s.initial.Mechanisms {
			if m.Valve == p {
				return i, true
			}
		}
	}
	return 0, false
}

// turnValve opens a shut valve's gates, or closes an open valve's gates if
// every one of them is clear: no occupant and no water. A jammed valve
// changes nothing and, like a blocked move, does not start the cooldown.
func (s *Sim) turnValve(m int, events []Event) []Event {
	st := &s.st
	mech := s.initial.Mechanisms[m]
	valve := &st.cells[s.initial.Index(mech.Valve)]
	opening := valve.Terrain == TerrainValveShut
	if !opening {
		for _, g := range mech.Gates {
			if c := st.cells[s.initial.Index(g)]; c.Occupant != OccupantNone || c.Water > 0 {
				return append(events, Event{Tick: st.tick, Kind: EventGateJammed, Pos: g})
			}
		}
	}
	gate, valveTo, kind := TerrainGateClosed, TerrainValveShut, EventGateClosed
	if opening {
		gate, valveTo, kind = TerrainGateOpen, TerrainValveOpen, EventGateOpened
	}
	valve.Terrain = valveTo
	events = append(events, Event{Tick: st.tick, Kind: EventValveTurned, Pos: mech.Valve})
	for _, g := range mech.Gates {
		st.cells[s.initial.Index(g)].Terrain = gate
		events = append(events, Event{Tick: st.tick, Kind: kind, Pos: g})
	}
	st.moveCooldown = MoveIntervalTicks - 1
	return events
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

// Crawlers returns the number of crawlers in the cave.
func (s *Sim) Crawlers() int {
	n := 0
	for _, c := range s.st.cells {
		if c.Occupant == OccupantCrawler {
			n++
		}
	}
	return n
}

// Glimmers returns the number of glimmers in the cave.
func (s *Sim) Glimmers() int {
	n := 0
	for _, c := range s.st.cells {
		if c.Occupant == OccupantGlimmer {
			n++
		}
	}
	return n
}

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

// Water returns the total water in the cave, in units. The rules conserve
// it: it never changes after New.
func (s *Sim) Water() int {
	n := 0
	for _, c := range s.st.cells {
		n += int(c.Water)
	}
	return n
}

// AdjacentValve returns the valve an Interact input would turn this tick,
// if any: the first valve next to the player, checking up, down, left,
// right. Whether it opens or closes follows from its terrain.
func (s *Sim) AdjacentValve() (Pos, bool) {
	m, ok := s.adjacentMechanism()
	if !ok {
		return Pos{}, false
	}
	return s.initial.Mechanisms[m].Valve, true
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
