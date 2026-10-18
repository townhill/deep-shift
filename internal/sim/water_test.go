package sim

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// newSimMech is newSim for caves with valves and gates.
func newSimMech(t testing.TB, mechs []Mechanism, rows ...string) *Sim {
	t.Helper()
	c := caveFromRows(t, 0, rows...)
	c.Mechanisms = mechs
	s, err := New(c, 1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func waterAt(s *Sim, p Pos) uint8 { return s.Cell(p).Water }

func wantASCII(t testing.TB, s *Sim, rows ...string) {
	t.Helper()
	if got, want := s.ASCII(), rowsASCII(rows...); got != want {
		t.Fatalf("after tick %d:\n%s\nwant:\n%s", s.Tick(), got, want)
	}
}

// settle steps with no input until a whole gravity interval leaves every
// cell unchanged, and fails if that takes more than limit ticks.
func settle(t testing.TB, s *Sim, limit int) {
	t.Helper()
	for n := 0; n < limit; n += GravityIntervalTicks {
		before := slices.Clone(s.st.cells)
		idle(s, GravityIntervalTicks)
		if slices.Equal(before, s.st.cells) {
			return
		}
	}
	t.Fatalf("water did not settle within %d ticks:\n%s", limit, s.ASCII())
}

func TestWaterFallsOneCellPerStep(t *testing.T) {
	s := newSim(t, 0,
		"@#~#",
		" # #",
		" # #",
		" ###",
	)
	wantY := []int{1, 1, 2, 2, 2, 2} // after ticks 0..5
	for tick, y := range wantY {
		s.Step(Input{})
		if waterAt(s, Pos{2, y}) != WaterCapacity || s.Water() != WaterCapacity {
			t.Fatalf("after tick %d: water not all at (2,%d):\n%s", tick, y, s.ASCII())
		}
	}
}

// One deep cell on a flat floor spreads one unit to each side, then stops:
// neighbours that differ by one unit do not flow.
func TestWaterSpreadsEvenlyToBothSides(t *testing.T) {
	s := newSim(t, 0,
		"@#   ~   #",
		"##########",
	)
	s.Step(Input{})
	wantASCII(t, s, "@#  121  #", "##########")
	idle(s, 20)
	wantASCII(t, s, "@#  121  #", "##########")
}

// Water never leaves through a barrier: walls, earth, the exit, a closed
// gate, a valve, a boulder or a crystal, nor through the cave edge on the
// left or the cave floor.
func TestWaterIsContainedByEveryBarrier(t *testing.T) {
	tests := []struct {
		name  string
		row0  string
		mechs []Mechanism
		cell  rune
	}{
		{"wall", "@    ", nil, '#'},
		{"earth", "@    ", nil, '.'},
		{"exit", "@    ", nil, 'E'},
		{"closed gate", "@   V", []Mechanism{{Valve: Pos{4, 0}, Gates: []Pos{{2, 1}}}}, 'G'},
		{"valve", "@   G", []Mechanism{{Valve: Pos{2, 1}, Gates: []Pos{{4, 0}}}}, 'V'},
		{"boulder", "@    ", nil, 'O'},
		{"crystal", "@    ", nil, '*'},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := []string{tt.row0, "~~" + string(tt.cell) + "  ", "#####"}
			s := newSimMech(t, tt.mechs, rows...)
			for range 200 {
				s.Step(Input{})
				if s.Water() != 2*WaterCapacity {
					t.Fatalf("tick %d: total water %d, want %d", s.Tick(), s.Water(), 2*WaterCapacity)
				}
			}
			for _, p := range []Pos{{3, 1}, {4, 1}, {1, 0}, {2, 0}, {3, 0}} {
				if waterAt(s, p) != 0 {
					t.Fatalf("water leaked to %s:\n%s", p, s.ASCII())
				}
			}
		})
	}
}

func TestWaterRestsOnTheCaveFloor(t *testing.T) {
	s := newSim(t, 0, "@#", "~#")
	idle(s, 10)
	wantASCII(t, s, "@#", "~#")
}

// mirrorRows reverses every row, swapping left and right.
func mirrorRows(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		b := []byte(r)
		for l, h := 0, len(b)-1; l < h; l, h = l+1, h-1 {
			b[l], b[h] = b[h], b[l]
		}
		out[i] = string(b)
	}
	return out
}

// randomWaterRows builds a closed box of walls, earth, empty floor and water
// with the player at (1,1) and no objects.
func randomWaterRows(rng *SplitMix64, w, h int) []string {
	rows := make([]string, h)
	for y := range h {
		b := make([]byte, w)
		for x := range w {
			switch r := rng.Intn(20); {
			case x == 0 || y == 0 || x == w-1 || y == h-1, r < 5:
				b[x] = '#'
			case r < 7:
				b[x] = '.'
			case r < 9:
				b[x] = '~'
			case r < 12:
				b[x] = byte('1' + rng.Intn(WaterCapacity-1))
			default:
				b[x] = ' '
			}
		}
		rows[y] = string(b)
	}
	rows[1] = "#@" + rows[1][2:]
	return rows
}

// The water rules have no left/right bias: a mirrored cave evolves into the
// mirror image, tick for tick. They also conserve water and settle. The
// player stands still, mirrored with the cave.
func TestWaterIsMirrorSymmetricConservedAndSettles(t *testing.T) {
	const w, h = 20, 12
	for seed := uint64(1); seed <= 20; seed++ {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			rng := NewSplitMix64(seed)
			rows := randomWaterRows(&rng, w, h)
			a, b := newSim(t, 0, rows...), newSim(t, 0, mirrorRows(rows)...)
			total := a.Water()
			for range 400 {
				a.Step(Input{})
				b.Step(Input{})
				for i, c := range a.st.cells {
					if m := b.st.cells[i/w*w+w-1-i%w]; m != c {
						t.Fatalf("seed %d tick %d: mirrored run diverged at %s\n%s\nmirror of\n%s",
							seed, a.Tick(), Pos{i % w, i / w}, b.ASCII(), a.ASCII())
					}
				}
				if a.Water() != total {
					t.Fatalf("seed %d tick %d: total water %d, want %d", seed, a.Tick(), a.Water(), total)
				}
			}
			settle(t, a, 2000)
		})
	}
}

func TestDeepWaterBlocksThePlayerAndShallowWaterDoesNot(t *testing.T) {
	s := newSim(t, 0, "@~", "##")
	ev := move(t, s, DirRight)
	wantKinds(t, ev, EventTooDeep)
	if s.Player() != (Pos{0, 0}) || !s.MoveReady() {
		t.Fatalf("player at %s, move ready %v; a blocked move must not start the cooldown", s.Player(), s.MoveReady())
	}
	// The same tick, one unit spread into the player's cell, so the water
	// beside them is now shallow enough to wade into.
	wantASCII(t, s, "@3", "##")
	wantKinds(t, move(t, s, DirRight), EventMoved)
	if s.Player() != (Pos{1, 0}) || waterAt(s, Pos{1, 0}) != 3 || waterAt(s, Pos{0, 0}) != 1 {
		t.Fatalf("after wading in:\n%s(water %d and %d)", s.ASCII(), waterAt(s, Pos{0, 0}), waterAt(s, Pos{1, 0}))
	}
}

func TestWaterFlowsIntoThePlayersCell(t *testing.T) {
	s := newSim(t, 0, "~@ ", "###")
	s.Step(Input{})
	if waterAt(s, Pos{0, 0}) != 3 || waterAt(s, Pos{1, 0}) != 1 || s.Player() != (Pos{1, 0}) {
		t.Fatalf("water %d, %d; player %s", waterAt(s, Pos{0, 0}), waterAt(s, Pos{1, 0}), s.Player())
	}
}

// Falling objects treat water as empty space and swap places with it, so an
// object sinks through a pool one cell per gravity step and the water rises
// above it (D-028).
func TestObjectsSinkThroughWaterDisplacingIt(t *testing.T) {
	for _, obj := range []string{"O", "*"} {
		t.Run(obj, func(t *testing.T) {
			s := newSim(t, 0,
				"@#"+obj+"#",
				" #~#",
				" #~#",
				" ###",
			)
			s.Step(Input{})
			wantASCII(t, s, "@#~#", " #"+obj+"#", " #~#", " ###")
			idle(s, 2)
			wantASCII(t, s, "@#~#", " #~#", " #"+obj+"#", " ###")
			idle(s, 10)
			wantASCII(t, s, "@#~#", " #~#", " #"+obj+"#", " ###")
			if s.Water() != 2*WaterCapacity {
				t.Fatalf("total water %d, want %d", s.Water(), 2*WaterCapacity)
			}
		})
	}
}

func TestPushingABoulderIntoWaterDisplacesItBehind(t *testing.T) {
	s := newSim(t, 0, "@O~ ", "####")
	wantKinds(t, move(t, s, DirRight), EventMoved, EventBoulderPushed)
	// The displaced water lands where the boulder was, which is where the
	// player now stands; one unit then spreads back into the cell they left.
	if !boulderAt(s, Pos{2, 0}) || s.Player() != (Pos{1, 0}) ||
		waterAt(s, Pos{0, 0}) != 1 || waterAt(s, Pos{1, 0}) != 3 || waterAt(s, Pos{3, 0}) != 0 {
		t.Fatalf("after the push:\n%s", s.ASCII())
	}
}

func TestBoulderRollsIntoWaterAndDisplacesIt(t *testing.T) {
	s := newSim(t, 0,
		" O  @",
		"~O   ",
		"#####",
	)
	ev := s.Step(Input{}) // tick 0: rolls left, beside the water
	wantKinds(t, ev, EventRolled)
	idle(s, 1)
	ev = s.Step(Input{}) // tick 2: falls into the water, which rises into (0,0)
	wantKinds(t, ev, EventFell)
	// Same tick: one unit spreads from (0,0) into the cell the roll vacated.
	wantASCII(t, s, "31  @", "OO   ", "#####")
	settle(t, s, 100)
	wantASCII(t, s, "21  @", "OO1  ", "#####")
	if s.Water() != WaterCapacity {
		t.Fatalf("total water %d, want %d", s.Water(), WaterCapacity)
	}
}

// reservoirRows: the player beside a valve, a reservoir held by a closed
// gate and a dry channel beyond it.
var reservoirMech = []Mechanism{{Valve: Pos{1, 0}, Gates: []Pos{{2, 1}}}}

var reservoirRows = []string{
	"@V    ",
	"~~G   ",
	"######",
}

func TestClosedGateHoldsWaterUntilTheValveOpensIt(t *testing.T) {
	s := newSimMech(t, reservoirMech, reservoirRows...)
	idle(s, 40)
	wantASCII(t, s, reservoirRows...)

	if v, ok := s.AdjacentValve(); !ok || v != (Pos{1, 0}) {
		t.Fatalf("AdjacentValve = %s, %v", v, ok)
	}
	for !s.MoveReady() {
		s.Step(Input{})
	}
	ev := s.Step(Input{Interact: true})
	wantKinds(t, ev, EventValveTurned, EventGateOpened)
	if ev[0].Pos != (Pos{1, 0}) || ev[1].Pos != (Pos{2, 1}) {
		t.Fatalf("events %v", ev)
	}
	if s.MoveReady() {
		t.Fatal("turning a valve must start the action cooldown")
	}
	if s.Cell(Pos{1, 0}).Terrain != TerrainValveOpen || s.Cell(Pos{2, 1}).Terrain != TerrainGateOpen {
		t.Fatalf("valve %s, gate %s", s.Cell(Pos{1, 0}).Terrain, s.Cell(Pos{2, 1}).Terrain)
	}
	settle(t, s, 200)
	if s.Water() != 2*WaterCapacity {
		t.Fatalf("total water %d, want %d", s.Water(), 2*WaterCapacity)
	}
	for x := range 5 {
		a, b := int(waterAt(s, Pos{x, 1})), int(waterAt(s, Pos{x + 1, 1}))
		if a-b > 1 || b-a > 1 {
			t.Fatalf("settled water still steps by more than one unit:\n%s", s.ASCII())
		}
	}
	if waterAt(s, Pos{3, 1}) == 0 {
		t.Fatalf("no water passed the open gate:\n%s", s.ASCII())
	}
}

func TestInteractWithoutAValveMovesInstead(t *testing.T) {
	s := newSim(t, 0, "@ ")
	if _, ok := s.AdjacentValve(); ok {
		t.Fatal("AdjacentValve found a valve in a cave without one")
	}
	wantKinds(t, s.Step(Input{Interact: true, Move: DirRight}), EventMoved)
}

func TestInteractDuringCooldownIsIgnored(t *testing.T) {
	s := newSimMech(t, []Mechanism{{Valve: Pos{1, 0}, Gates: []Pos{{0, 2}}}},
		" V",
		"@ ",
		"G#",
	)
	wantKinds(t, move(t, s, DirRight), EventMoved)
	// The valve is now directly above the player.
	if ev := s.Step(Input{Interact: true}); len(ev) != 0 || s.Cell(Pos{1, 0}).Terrain != TerrainValveShut {
		t.Fatalf("interact during the cooldown acted: %v", ev)
	}
	for !s.MoveReady() {
		s.Step(Input{})
	}
	wantKinds(t, s.Step(Input{Interact: true}), EventValveTurned, EventGateOpened)
}

func TestInteractTurnsTheFirstAdjacentValveUpDownLeftRight(t *testing.T) {
	s := newSimMech(t, []Mechanism{
		{Valve: Pos{0, 1}, Gates: []Pos{{1, 2}}}, // left of the player
		{Valve: Pos{1, 0}, Gates: []Pos{{0, 2}}}, // above the player
	},
		" V ",
		"V@ ",
		"GG#",
	)
	if v, _ := s.AdjacentValve(); v != (Pos{1, 0}) {
		t.Fatalf("AdjacentValve = %s, want the valve above, (1,0)", v)
	}
	ev := s.Step(Input{Interact: true})
	wantKinds(t, ev, EventValveTurned, EventGateOpened)
	if ev[0].Pos != (Pos{1, 0}) || ev[1].Pos != (Pos{0, 2}) {
		t.Fatalf("turned the wrong valve: %v", ev)
	}
}

func TestPlayerCannotEnterAClosedGateButWalksThroughAnOpenOne(t *testing.T) {
	s := newSimMech(t, []Mechanism{{Valve: Pos{0, 1}, Gates: []Pos{{1, 0}}}},
		"@G ",
		"V##",
	)
	if ev := move(t, s, DirRight); len(ev) != 0 || s.Player() != (Pos{0, 0}) {
		t.Fatalf("entered a closed gate: %v", ev)
	}
	wantKinds(t, s.Step(Input{Interact: true}), EventValveTurned, EventGateOpened)
	wantKinds(t, move(t, s, DirRight), EventMoved)
	wantKinds(t, move(t, s, DirRight), EventMoved)
	if s.Player() != (Pos{2, 0}) {
		t.Fatalf("player at %s, want (2,0)", s.Player())
	}
}

// A closed gate under a boulder is a trapdoor: opening it drops the boulder
// on the same tick's gravity step.
func TestOpeningAGateUnderABoulderDropsIt(t *testing.T) {
	s := newSimMech(t, []Mechanism{{Valve: Pos{1, 0}, Gates: []Pos{{2, 1}}}},
		"@VO",
		"  G",
		"   ",
		"###",
	)
	if ev := idle(s, 10); len(ev) != 0 {
		t.Fatalf("a boulder on a closed gate moved: %v", ev)
	}
	for !s.MoveReady() || s.Tick()%GravityIntervalTicks != 0 {
		s.Step(Input{})
	}
	ev := s.Step(Input{Interact: true})
	wantKinds(t, ev, EventValveTurned, EventGateOpened, EventFell)
	if ev[2].From != (Pos{2, 0}) || ev[2].Pos != (Pos{2, 1}) {
		t.Fatalf("fall %v, want (2,0)->(2,1)", ev[2])
	}
	idle(s, 10)
	if !boulderAt(s, Pos{2, 2}) || s.Cell(Pos{2, 1}).Terrain != TerrainGateOpen {
		t.Fatalf("after the drop:\n%s", s.ASCII())
	}
	// The gate is clear again, so it closes.
	wantKinds(t, s.Step(Input{Interact: true}), EventValveTurned, EventGateClosed)
}

func TestClosingAGateNeedsItClear(t *testing.T) {
	mech := []Mechanism{{Valve: Pos{1, 0}, Gates: []Pos{{1, 1}}}}
	tests := []struct {
		name string
		fill func(*Cell)
	}{
		{"water in the gate", func(c *Cell) { c.Water = 1 }},
		{"boulder in the gate", func(c *Cell) { c.Occupant = OccupantBoulder }},
		{"crystal in the gate", func(c *Cell) { c.Occupant = OccupantCrystal }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := caveFromRows(t, 0, "@v", "#g", "##")
			c.Mechanisms = mech
			tt.fill(&c.Cells[c.Index(Pos{1, 1})])
			s, err := New(c, 1)
			if err != nil {
				t.Fatal(err)
			}
			ev := s.Step(Input{Interact: true})
			wantKinds(t, ev, EventGateJammed)
			if ev[0].Pos != (Pos{1, 1}) || !s.MoveReady() || s.Cell(Pos{1, 0}).Terrain != TerrainValveOpen {
				t.Fatalf("jammed close changed state or started the cooldown: %v", ev)
			}
		})
	}
	t.Run("player in the gate", func(t *testing.T) {
		c := caveFromRows(t, 0, "#v", " @", "##")
		c.Mechanisms = mech
		c.Cells[c.Index(Pos{1, 1})].Terrain = TerrainGateOpen
		s, err := New(c, 1)
		if err != nil {
			t.Fatal(err)
		}
		wantKinds(t, s.Step(Input{Interact: true}), EventGateJammed)
	})
	t.Run("clear gate closes", func(t *testing.T) {
		c := caveFromRows(t, 0, "@v", "#g", "##")
		c.Mechanisms = mech
		s, err := New(c, 1)
		if err != nil {
			t.Fatal(err)
		}
		wantKinds(t, s.Step(Input{Interact: true}), EventValveTurned, EventGateClosed)
		if s.Cell(Pos{1, 0}).Terrain != TerrainValveShut || s.Cell(Pos{1, 1}).Terrain != TerrainGateClosed {
			t.Fatalf("valve %s, gate %s", s.Cell(Pos{1, 0}).Terrain, s.Cell(Pos{1, 1}).Terrain)
		}
	})
}

func TestResetRestoresGatesValvesAndWater(t *testing.T) {
	s := newSimMech(t, reservoirMech, reservoirRows...)
	start := s.StateHash()
	s.Step(Input{Interact: true})
	idle(s, 30)
	if s.ASCII() == rowsASCII(reservoirRows...) {
		t.Fatal("nothing changed before the reset")
	}
	s.Reset()
	wantASCII(t, s, reservoirRows...)
	if s.StateHash() != start {
		t.Fatal("hash after Reset differs from the initial hash")
	}
}

func TestWaterAndMechanismValidation(t *testing.T) {
	base := func() Cave {
		c := caveFromRows(t, 0, "@V ", "G  ")
		c.Mechanisms = []Mechanism{{Valve: Pos{1, 0}, Gates: []Pos{{0, 1}}}}
		return c
	}
	if _, err := New(base(), 0); err != nil {
		t.Fatalf("base cave rejected: %v", err)
	}
	tests := []struct {
		name    string
		mutate  func(*Cave)
		wantErr string
	}{
		{"too much water", func(c *Cave) { c.Cells[2].Water = WaterCapacity + 1 }, "exceeds the capacity"},
		{"water with a boulder", func(c *Cave) { c.Cells[2] = Cell{Occupant: OccupantBoulder, Water: 1} }, "water cannot share a cell with boulder"},
		{"water in earth", func(c *Cave) { c.Cells[2] = Cell{Terrain: TerrainEarth, Water: 1} }, "water cannot share"},
		{"water in a closed gate", func(c *Cave) { c.Cells[3].Water = 1 }, "water cannot share"},
		{"occupant in a closed gate", func(c *Cave) { c.Cells[3].Occupant = OccupantCrystal }, "cannot occupy a closed gate"},
		{"no mechanism", func(c *Cave) { c.Mechanisms = nil }, "belongs to no mechanism"},
		{"valve is not a valve", func(c *Cave) { c.Mechanisms[0].Valve = Pos{2, 0} }, "is empty, not a valve"},
		{"gate is not a gate", func(c *Cave) { c.Mechanisms[0].Gates = []Pos{{2, 1}} }, "is empty, not a gate"},
		{"gate listed twice", func(c *Cave) { c.Mechanisms[0].Gates = []Pos{{0, 1}, {0, 1}} }, "already belongs to mechanism 0"},
		{"no gates", func(c *Cave) { c.Mechanisms[0].Gates = nil }, "operates no gates"},
		{"outside the cave", func(c *Cave) { c.Mechanisms[0].Gates = []Pos{{0, 1}, {5, 5}} }, "outside the cave"},
		{"state mismatch", func(c *Cave) { c.Cells[3].Terrain = TerrainGateOpen }, "must be closed like its valve"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base()
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

func TestWaterSymbolsRoundTrip(t *testing.T) {
	rows := []string{"@~321", "GgVv#"}
	s := newSimMech(t, []Mechanism{
		{Valve: Pos{2, 1}, Gates: []Pos{{0, 1}}},
		{Valve: Pos{3, 1}, Gates: []Pos{{1, 1}}},
	}, rows...)
	wantASCII(t, s, rows...)
	if s.Water() != 4+3+2+1 {
		t.Fatalf("total water %d, want 10", s.Water())
	}
}
