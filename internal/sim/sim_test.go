package sim

import (
	"reflect"
	"strings"
	"testing"
)

func TestMoveIntoEmptyCell(t *testing.T) {
	tests := []struct {
		dir  Dir
		want Pos
	}{
		{DirUp, Pos{2, 0}},
		{DirDown, Pos{2, 2}},
		{DirLeft, Pos{1, 1}},
		{DirRight, Pos{3, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.dir.String(), func(t *testing.T) {
			s := newSim(t, 0,
				"     ",
				"  @  ",
				"     ",
			)
			ev := s.Step(Input{Move: tt.dir})
			wantKinds(t, ev, EventMoved)
			if got := s.Player(); got != tt.want {
				t.Fatalf("player at %s, want %s", got, tt.want)
			}
			if ev[0].Pos != tt.want || ev[0].Tick != 0 {
				t.Fatalf("event = %v, want moved at %s on tick 0", ev[0], tt.want)
			}
			if got := s.Cell(Pos{2, 1}).Occupant; got != OccupantNone {
				t.Fatalf("start cell still holds %s", got)
			}
			if n := countOccupant(s, OccupantPlayer); n != 1 {
				t.Fatalf("%d player occupants, want 1", n)
			}
		})
	}
}

// Walls, the cave edge and a sealed exit all block. A blocked move changes
// nothing and does not consume the movement window.
func TestBlockedMoves(t *testing.T) {
	tests := []struct {
		name string
		rows []string
		dir  Dir
		want []EventKind
	}{
		{"wall up", []string{" # ", " @ ", "   "}, DirUp, nil},
		{"wall down", []string{"   ", " @ ", " # "}, DirDown, nil},
		{"wall left", []string{"   ", "#@ ", "   "}, DirLeft, nil},
		{"wall right", []string{"   ", " @#", "   "}, DirRight, nil},
		{"top edge", []string{" @ ", "   "}, DirUp, nil},
		{"bottom edge", []string{"   ", " @ "}, DirDown, nil},
		{"left edge", []string{"@  ", "   "}, DirLeft, nil},
		{"right edge", []string{"  @", "   "}, DirRight, nil},
		{"top-left corner up", []string{"@ ", "  "}, DirUp, nil},
		{"top-left corner left", []string{"@ ", "  "}, DirLeft, nil},
		{"bottom-right corner down", []string{"  ", " @"}, DirDown, nil},
		{"bottom-right corner right", []string{"  ", " @"}, DirRight, nil},
		{"sealed exit", []string{"*@E"}, DirRight, []EventKind{EventExitSealed}},
		{"invalid direction value", []string{" @ "}, Dir(99), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			required := strings.Count(strings.Join(tt.rows, ""), "*")
			s := newSim(t, required, tt.rows...)
			before := s.ASCII()
			start := s.Player()
			wantKinds(t, s.Step(Input{Move: tt.dir}), tt.want...)
			if s.Player() != start {
				t.Fatalf("player moved to %s, want to stay at %s", s.Player(), start)
			}
			if got := s.ASCII(); got != before {
				t.Fatalf("grid changed:\n%s\nwant:\n%s", got, before)
			}
			if !s.MoveReady() {
				t.Fatal("blocked move consumed the movement window")
			}
			if !s.Cave().InBounds(s.Player()) {
				t.Fatalf("player at invalid coordinate %s", s.Player())
			}
		})
	}
}

func TestBlockedMoveLeavesWindowOpenForAnotherDirection(t *testing.T) {
	s := newSim(t, 0,
		"@#",
		"  ",
	)
	wantKinds(t, s.Step(Input{Move: DirRight})) // blocked on tick 0
	wantKinds(t, s.Step(Input{Move: DirDown}), EventMoved)
	if got, want := s.Player(), (Pos{0, 1}); got != want {
		t.Fatalf("player at %s, want %s", got, want)
	}
}

func TestDiggingRemovesExactlyOneEarthTile(t *testing.T) {
	tests := []struct {
		dir  Dir
		want Pos
	}{
		{DirUp, Pos{1, 0}},
		{DirDown, Pos{1, 2}},
		{DirLeft, Pos{0, 1}},
		{DirRight, Pos{2, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.dir.String(), func(t *testing.T) {
			s := newSim(t, 0,
				"...",
				".@.",
				"...",
			)
			earthBefore := countTerrain(s, TerrainEarth)
			ev := s.Step(Input{Move: tt.dir})
			wantKinds(t, ev, EventMoved, EventDug)
			if ev[1].Pos != tt.want {
				t.Fatalf("dug %s, want %s", ev[1].Pos, tt.want)
			}
			if got := countTerrain(s, TerrainEarth); got != earthBefore-1 {
				t.Fatalf("earth tiles %d -> %d, want exactly one removed", earthBefore, got)
			}
			if got := s.Cell(tt.want).Terrain; got != TerrainEmpty {
				t.Fatalf("dug cell terrain = %s, want empty", got)
			}
		})
	}
}

func TestDugTunnelIsWalkableWithoutDiggingAgain(t *testing.T) {
	s := newSim(t, 0, "@..")
	wantKinds(t, move(t, s, DirRight), EventMoved, EventDug)
	wantKinds(t, move(t, s, DirLeft), EventMoved)
	wantKinds(t, move(t, s, DirRight), EventMoved)
}

func TestCollectingCrystalIncrementsOnce(t *testing.T) {
	s := newSim(t, 1, "@* E")
	ev := move(t, s, DirRight)
	wantKinds(t, ev, EventMoved, EventCollected, EventExitOpened)
	if s.Collected() != 1 {
		t.Fatalf("collected = %d, want 1", s.Collected())
	}
	if n := countOccupant(s, OccupantCrystal); n != 0 {
		t.Fatalf("%d crystals remain, want 0", n)
	}
	// Walking back over the now-empty cell never counts again.
	for _, d := range []Dir{DirRight, DirLeft, DirLeft, DirRight} {
		for _, e := range move(t, s, d) {
			if e.Kind == EventCollected {
				t.Fatalf("crystal collected again: %v", e)
			}
		}
	}
	if s.Collected() != 1 {
		t.Fatalf("collected = %d after revisiting, want 1", s.Collected())
	}
}

func TestCrystalOnEarthDigsAndCollects(t *testing.T) {
	c := caveFromRows(t, 1, "@.")
	c.Cells[1].Occupant = OccupantCrystal // not expressible in level files
	s, err := New(c, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantKinds(t, s.Step(Input{Move: DirRight}), EventMoved, EventDug, EventCollected)
}

func TestMovementCadence(t *testing.T) {
	s := newSim(t, 0, "@           ")
	var moveTicks []uint64
	for range 20 {
		for _, e := range s.Step(Input{Move: DirRight}) {
			if e.Kind == EventMoved {
				moveTicks = append(moveTicks, e.Tick)
			}
		}
	}
	want := []uint64{0, 4, 8, 12, 16}
	if !reflect.DeepEqual(moveTicks, want) {
		t.Fatalf("held move fired on ticks %v, want %v (every %d ticks)", moveTicks, want, MoveIntervalTicks)
	}
	if got, want := s.Player(), (Pos{5, 0}); got != want {
		t.Fatalf("player at %s, want %s", got, want)
	}
}

func TestMoveInputDuringCooldownIsIgnored(t *testing.T) {
	s := newSim(t, 0, "    ", " @  ", "    ")
	wantKinds(t, s.Step(Input{Move: DirRight}), EventMoved)
	for i := 1; i < MoveIntervalTicks; i++ {
		if s.MoveReady() {
			t.Fatalf("window open on tick %d, want closed until tick %d", s.Tick(), MoveIntervalTicks)
		}
		wantKinds(t, s.Step(Input{Move: DirDown}))
	}
	if !s.MoveReady() {
		t.Fatal("window still closed after the cooldown")
	}
	if got, want := s.Player(), (Pos{2, 1}); got != want {
		t.Fatalf("player at %s, want %s (cooldown inputs must be dropped, not queued)", got, want)
	}
}

func TestExitCannotCompleteBeforeQuota(t *testing.T) {
	s := newSim(t, 2,
		"*@E",
		"*  ",
	)
	// Sealed exit: bumping it is reported but nothing else happens.
	wantKinds(t, move(t, s, DirRight), EventExitSealed)
	if s.Status() != StatusPlaying || s.Player() != (Pos{1, 0}) || s.ExitOpen() {
		t.Fatalf("sealed exit let the player through: status=%s player=%s open=%v", s.Status(), s.Player(), s.ExitOpen())
	}

	wantKinds(t, move(t, s, DirLeft), EventMoved, EventCollected)
	if s.ExitOpen() {
		t.Fatal("exit opened with 1 of 2 crystals")
	}
	ev := move(t, s, DirDown)
	wantKinds(t, ev, EventMoved, EventCollected, EventExitOpened)
	if ev[2].Pos != (Pos{2, 0}) || ev[2].Tick != ev[0].Tick {
		t.Fatalf("exit-opened event = %v, want at (2,0) on the collecting tick", ev[2])
	}
	if !s.ExitOpen() {
		t.Fatal("exit still sealed with quota met")
	}

	wantKinds(t, move(t, s, DirRight), EventMoved)
	wantKinds(t, move(t, s, DirUp), EventMoved)
	if s.Status() != StatusPlaying {
		t.Fatal("cave completed before entering the exit")
	}
	ev = move(t, s, DirRight)
	wantKinds(t, ev, EventMoved, EventCaveComplete)
	if s.Status() != StatusComplete {
		t.Fatalf("status = %s, want complete", s.Status())
	}
	// docs/PHYSICS.md walks through this scenario tick by tick.
	if ev[1].Tick != 17 {
		t.Fatalf("completed on tick %d, want 17 as documented in docs/PHYSICS.md", ev[1].Tick)
	}
}

func TestQuotaBelowTotalOpensEarly(t *testing.T) {
	s := newSim(t, 1, "@**E")
	wantKinds(t, move(t, s, DirRight), EventMoved, EventCollected, EventExitOpened)
	wantKinds(t, move(t, s, DirRight), EventMoved, EventCollected)
	wantKinds(t, move(t, s, DirRight), EventMoved, EventCaveComplete)
}

func TestZeroQuotaExitStartsOpen(t *testing.T) {
	s := newSim(t, 0, "@E")
	if !s.ExitOpen() {
		t.Fatal("zero-quota exit is sealed at tick 0")
	}
	if got, want := s.ASCII(), "@X\n"; got != want {
		t.Fatalf("ASCII = %q, want %q", got, want)
	}
	wantKinds(t, s.Step(Input{Move: DirRight}), EventMoved, EventCaveComplete)
}

func TestCompletedCaveIsFrozen(t *testing.T) {
	s := newSim(t, 0, "@E ")
	s.Step(Input{Move: DirRight})
	tick, hash := s.Tick(), s.StateHash()
	for range 10 {
		if ev := s.Step(Input{Move: DirRight}); ev != nil {
			t.Fatalf("completed cave produced events %v", ev)
		}
	}
	if s.Tick() != tick || s.StateHash() != hash {
		t.Fatal("state changed after completion")
	}
	if s.MoveReady() {
		t.Fatal("MoveReady true after completion")
	}
}

func TestResetRestoresAllState(t *testing.T) {
	rows := []string{
		"#######",
		"#@.*.E#",
		"#.*.. #",
		"#######",
	}
	fresh := newSim(t, 2, rows...)
	s := newSim(t, 2, rows...)
	inputs := []Dir{DirDown, DirRight, DirUp, DirRight, DirRight, DirRight}

	run := func() Hash {
		for _, d := range inputs {
			move(t, s, d)
		}
		return s.StateHash()
	}
	first := run()
	if s.Status() != StatusComplete || s.Collected() != 2 {
		t.Fatalf("scripted run did not complete: %s, %d collected\n%s", s.Status(), s.Collected(), s.ASCII())
	}
	s.st.rng.Next() // advance the RNG so Reset must restore it too

	s.Reset()
	if !reflect.DeepEqual(s.st, fresh.st) {
		t.Fatalf("state after Reset differs from a fresh simulation:\n got %+v\nwant %+v", s.st, fresh.st)
	}
	if s.StateHash() != fresh.StateHash() {
		t.Fatal("hash after Reset differs from a fresh simulation")
	}
	if s.Tick() != 0 || s.Collected() != 0 || s.ExitOpen() || s.Status() != StatusPlaying || s.st.rng.State != s.Seed() {
		t.Fatal("Reset left counters, status or seed behind")
	}
	if got := s.ASCII(); got != rowsASCII(rows...) {
		t.Fatalf("grid after Reset:\n%s", got)
	}
	if second := run(); second != first {
		t.Fatal("the same inputs after Reset produced a different hash")
	}
}

func TestNewCopiesTheCave(t *testing.T) {
	c := caveFromRows(t, 0, "@.")
	s, err := New(c, 0)
	if err != nil {
		t.Fatal(err)
	}
	c.Cells[1] = Cell{Terrain: TerrainWall}
	if s.Cell(Pos{1, 0}).Terrain != TerrainEarth {
		t.Fatal("mutating the input cave changed the simulation")
	}
	snap := s.Snapshot()
	snap.Cells[1] = Cell{Terrain: TerrainWall}
	if s.Cell(Pos{1, 0}).Terrain != TerrainEarth {
		t.Fatal("mutating a snapshot changed the simulation")
	}
}

func TestNewRejectsInvalidCaves(t *testing.T) {
	valid := func() Cave { return caveFromRows(t, 1, "@*E") }
	tests := []struct {
		name    string
		mutate  func(*Cave)
		wantErr string
	}{
		{"zero width", func(c *Cave) { c.Width = 0 }, "out of range"},
		{"too tall", func(c *Cave) { c.Height = MaxDimension + 1 }, "out of range"},
		{"cell count mismatch", func(c *Cave) { c.Cells = c.Cells[:2] }, "want width*height = 3"},
		{"no player", func(c *Cave) { c.Cells[0] = Cell{} }, "no player start"},
		{"two players", func(c *Cave) { c.Cells[1] = Cell{Occupant: OccupantPlayer}; c.RequiredCrystals = 0 }, "2 player starts"},
		{"quota too high", func(c *Cave) { c.RequiredCrystals = 2 }, "exceeds the 1 crystals"},
		{"negative quota", func(c *Cave) { c.RequiredCrystals = -1 }, "negative"},
		{"occupant in wall", func(c *Cave) { c.Cells[1].Terrain = TerrainWall }, "cannot occupy a wall"},
		{"crystal on exit", func(c *Cave) { c.Cells[2].Occupant = OccupantCrystal }, "cannot start on the exit"},
		{"unknown terrain", func(c *Cave) { c.Cells[1].Terrain = 200 }, "unknown terrain"},
		{"unknown occupant", func(c *Cave) { c.Cells[1].Occupant = 200 }, "unknown occupant"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid()
			tt.mutate(&c)
			_, err := New(c, 0)
			if err == nil {
				t.Fatal("New accepted an invalid cave")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestASCIIRoundTrip(t *testing.T) {
	rows := []string{
		"##########",
		"#@ ..*  E#",
		"#.##..*. #",
		"##########",
	}
	s := newSim(t, 2, rows...)
	if got, want := s.ASCII(), rowsASCII(rows...); got != want {
		t.Fatalf("ASCII:\n%s\nwant:\n%s", got, want)
	}
	snap := s.Snapshot()
	if snap.ASCII() != s.ASCII() {
		t.Fatal("snapshot ASCII differs from sim ASCII")
	}
}

// Cave exposes the initial layout to tests in this package only.
func (s *Sim) Cave() *Cave { return &s.initial }
