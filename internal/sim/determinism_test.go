package sim

import (
	"fmt"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// SplitMix64 reference outputs for seed 0, as published with the algorithm.
func TestSplitMix64ReferenceVector(t *testing.T) {
	r := NewSplitMix64(0)
	want := []uint64{0xe220a8397b1dcdaf, 0x6e789e6aa1b965f4, 0x06c45d188009454f}
	for i, w := range want {
		if got := r.Next(); got != w {
			t.Fatalf("output %d = %#016x, want %#016x", i, got, w)
		}
	}
}

func TestStateHashIsSensitiveToEveryStateComponent(t *testing.T) {
	rows := []string{"@.*E", " .  "}
	base := func(seed uint64) *Sim {
		s, err := New(caveFromRows(t, 1, rows...), seed)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	variants := map[string]Hash{}
	add := func(name string, s *Sim) {
		h := s.StateHash()
		for other, oh := range variants {
			if oh == h {
				t.Fatalf("%q and %q hash identically", name, other)
			}
		}
		variants[name] = h
	}

	add("fresh", base(1))
	add("other seed", base(2))
	s := base(1)
	s.Step(Input{})
	add("one idle tick", s)
	s = base(1)
	s.Step(Input{Move: DirRight})
	add("dug + cooldown", s)
	s.Step(Input{})
	add("dug + shorter cooldown", s)
	s = base(1)
	s.st.rng.Next()
	add("rng advanced", s)
	s = base(1)
	s.st.cells[s.Cave().Index(Pos{1, 1})].Terrain = TerrainEmpty
	add("one cell differs", s)
	s = base(1)
	s.st.cells[s.Cave().Index(Pos{1, 1})] = Cell{Occupant: OccupantBoulder}
	add("resting boulder", s)
	s.st.cells[s.Cave().Index(Pos{1, 1})].Falling = true
	add("falling boulder", s)
	s = base(1)
	s.st.status = StatusDead
	add("dead", s)
	s = base(1)
	s.st.cells[s.Cave().Index(Pos{3, 1})].Water = 1
	add("one unit of water", s)
	s.st.cells[s.Cave().Index(Pos{3, 1})].Water = 2
	add("two units of water", s)
	s = base(1)
	s.st.cells[s.Cave().Index(Pos{3, 1})].Terrain = TerrainGateClosed
	add("closed gate", s)
	s.st.cells[s.Cave().Index(Pos{3, 1})].Terrain = TerrainGateOpen
	add("open gate", s)
	s.initial.Mechanisms = []Mechanism{{Valve: Pos{2, 1}, Gates: []Pos{{3, 1}}}}
	add("open gate linked to a valve", s)
}

// Golden hashes pin the hash byte layout. If this test fails, the encoding or
// a rule changed: bump RuleVersion or the hash magic deliberately, document
// it, and review every replay fixture rather than regenerating blindly.
// Last re-pinned for rule version 3 / hash layout v3 (water byte per cell,
// mechanism links); see docs/DECISIONS.md D-030.
func TestStateHashGolden(t *testing.T) {
	s, err := New(caveFromRows(t, 1, "#####", "#@.*E", "#####"), 42)
	if err != nil {
		t.Fatal(err)
	}
	const wantStart = "e5357ece5bc5563c4322d50fcd36d23bbfd8714a94d80e991d50e3134ccbd146"
	if got := s.StateHash().String(); got != wantStart {
		t.Errorf("tick 0 hash = %s, want %s", got, wantStart)
	}
	for range 3 {
		move(t, s, DirRight)
	}
	if s.Status() != StatusComplete {
		t.Fatalf("status = %s, want complete", s.Status())
	}
	const wantEnd = "01fcc7c99ab91f3edb65cea2d512cbbe82852f6320b425e832614ad113342722"
	if got := s.StateHash().String(); got != wantEnd {
		t.Errorf("completed hash = %s, want %s", got, wantEnd)
	}
}

// randomCave builds a bordered cave from rng with exactly one player, one
// exit, water, and two valves (one beside the player start) each linked to
// a few gates. It is test-only and makes no solvability promise.
func randomCave(rng *SplitMix64, w, h int) Cave {
	c := Cave{Width: w, Height: h, Cells: make([]Cell, w*h)}
	for y := range h {
		for x := range w {
			cell := &c.Cells[y*w+x]
			if x == 0 || y == 0 || x == w-1 || y == h-1 {
				cell.Terrain = TerrainWall
				continue
			}
			switch r := rng.Intn(20); {
			case r < 7:
				cell.Terrain = TerrainEarth
			case r < 11:
				cell.Terrain = TerrainWall
			case r < 13:
				cell.Occupant = OccupantCrystal
			case r < 15:
				cell.Occupant = OccupantBoulder
			case r < 18:
				cell.Water = uint8(1 + rng.Intn(WaterCapacity))
			}
		}
	}
	c.Cells[1*w+1] = Cell{Occupant: OccupantPlayer}
	c.Cells[(h-2)*w+(w-2)] = Cell{Terrain: TerrainExit}
	valves := []Pos{{2, 1}, {1 + rng.Intn(w-2), 2 + rng.Intn(h-4)}}
	for _, v := range valves {
		c.Cells[c.Index(v)] = Cell{Terrain: TerrainValveShut}
	}
	for _, v := range valves {
		gate, valve := TerrainGateClosed, TerrainValveShut
		if rng.Intn(2) == 0 {
			gate, valve = TerrainGateOpen, TerrainValveOpen
		}
		c.Cells[c.Index(v)].Terrain = valve
		m := Mechanism{Valve: v}
		for want, tries := 1+rng.Intn(3), 0; len(m.Gates) < want && tries < 100; tries++ {
			g := Pos{1 + rng.Intn(w-2), 1 + rng.Intn(h-2)}
			cell := &c.Cells[c.Index(g)]
			if cell.Occupant == OccupantPlayer || cell.Terrain == TerrainExit || cell.Terrain.Gate() || cell.Terrain.Valve() {
				continue
			}
			*cell = Cell{Terrain: gate}
			m.Gates = append(m.Gates, g)
		}
		c.Mechanisms = append(c.Mechanisms, m)
	}
	c.RequiredCrystals = c.CountOccupant(OccupantCrystal) / 2
	return c
}

func checkInvariants(s *Sim, initialCrystals, initialBoulders, lastCollected, initialWater int) error {
	players := 0
	for i, c := range s.st.cells {
		if c.Falling && !c.Occupant.Falls() {
			return fmt.Errorf("falling flag on %s at index %d", c.Occupant, i)
		}
		if c.Occupant == OccupantBoulder && !c.Terrain.Floor() {
			return fmt.Errorf("boulder on %s terrain at index %d", c.Terrain, i)
		}
		if c.Water > WaterCapacity || c.Water > 0 && !c.HoldsWater() {
			return fmt.Errorf("%d water with %s on %s terrain at index %d", c.Water, c.Occupant, c.Terrain, i)
		}
		if c.Occupant != OccupantNone && (c.Terrain == TerrainGateClosed || c.Terrain.Valve()) {
			return fmt.Errorf("%s inside a %s at index %d", c.Occupant, c.Terrain, i)
		}
		if c.Occupant == OccupantPlayer {
			players++
			if p := (Pos{i % s.Width(), i / s.Width()}); p != s.Player() {
				return fmt.Errorf("player occupant at %s but Player() = %s", p, s.Player())
			}
		}
		if c.Occupant != OccupantNone && c.Terrain == TerrainWall {
			return fmt.Errorf("occupant inside wall at index %d", i)
		}
	}
	if players != 1 {
		return fmt.Errorf("%d player occupants", players)
	}
	if !s.Cave().InBounds(s.Player()) {
		return fmt.Errorf("player out of bounds at %s", s.Player())
	}
	if got := s.Collected() + countOccupant(s, OccupantCrystal); got != initialCrystals {
		return fmt.Errorf("crystals not conserved: %d collected + remaining, want %d", got, initialCrystals)
	}
	if s.Collected() < lastCollected {
		return fmt.Errorf("collected count decreased from %d to %d", lastCollected, s.Collected())
	}
	if got := s.Boulders(); got != initialBoulders {
		return fmt.Errorf("boulders not conserved: %d, want %d", got, initialBoulders)
	}
	if got := s.Water(); got != initialWater {
		return fmt.Errorf("water not conserved: %d units, want %d", got, initialWater)
	}
	for i, m := range s.initial.Mechanisms {
		open := s.Cell(m.Valve).Terrain == TerrainValveOpen
		for _, g := range m.Gates {
			if (s.Cell(g).Terrain == TerrainGateOpen) != open {
				return fmt.Errorf("mechanism %d: gate %s is %s but its valve is %s", i, g, s.Cell(g).Terrain, s.Cell(m.Valve).Terrain)
			}
		}
	}
	// The quota check is skipped on the tick the player dies.
	if s.ExitOpen() && s.Collected() < s.Required() ||
		s.Status() != StatusDead && s.ExitOpen() != (s.Collected() >= s.Required()) {
		return fmt.Errorf("exit open = %v with %d/%d crystals (status %s)", s.ExitOpen(), s.Collected(), s.Required(), s.Status())
	}
	return nil
}

// checkMoveEvents verifies one tick's object movement: falls go exactly one
// cell down, rolls and pushes one cell sideways, and no object moves twice
// (no move starts where an earlier move in the same tick ended, and no two
// moves share a start or an end).
func checkMoveEvents(events []Event) error {
	starts, ends := map[Pos]bool{}, map[Pos]bool{}
	for _, e := range events {
		var dx, dy int
		switch e.Kind {
		case EventFell:
			dx, dy = 0, 1
		case EventRolled, EventBoulderPushed:
			dx, dy = e.Pos.X-e.From.X, 0
			if dx != 1 && dx != -1 {
				return fmt.Errorf("%v is not one cell sideways", e)
			}
		default:
			continue
		}
		if e.Pos != (Pos{e.From.X + dx, e.From.Y + dy}) {
			return fmt.Errorf("%v moved more than one cell", e)
		}
		if ends[e.From] || starts[e.From] || ends[e.Pos] {
			return fmt.Errorf("%v: an object moved twice in one tick", e)
		}
		starts[e.From], ends[e.Pos] = true, true
	}
	return nil
}

// Seeded randomised runs: invariants hold after every tick, and two runs of
// the same inputs agree on every tick's hash. The seed is in every failure.
func TestRandomisedRunsKeepInvariantsAndDeterminism(t *testing.T) {
	const ticks = 3000
	for seed := uint64(1); seed <= 20; seed++ {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			caveRNG := NewSplitMix64(seed)
			c := randomCave(&caveRNG, 24, 16)
			a, err := New(c, seed)
			if err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
			b, _ := New(c, seed)
			initialCrystals := c.CountOccupant(OccupantCrystal)
			initialBoulders := c.CountOccupant(OccupantBoulder)
			initialWater := a.Water()
			inputRNG := NewSplitMix64(seed ^ 0xdeadbeef)
			lastCollected, turns := 0, 0
			for tick := range ticks {
				// One input in six is Interact, so valves near the player's
				// path get turned both ways.
				r := inputRNG.Intn(6)
				in := Input{Move: Dir(r % 5), Interact: r == 5}
				ev := a.Step(in)
				b.Step(in)
				for _, e := range ev {
					if e.Kind == EventValveTurned {
						turns++
					}
				}
				if err := checkMoveEvents(ev); err != nil {
					t.Fatalf("seed %d tick %d: %v\n%s", seed, tick, err, a.ASCII())
				}
				if err := checkInvariants(a, initialCrystals, initialBoulders, lastCollected, initialWater); err != nil {
					t.Fatalf("seed %d tick %d: %v\n%s", seed, tick, err, a.ASCII())
				}
				lastCollected = a.Collected()
				if a.StateHash() != b.StateHash() {
					t.Fatalf("seed %d tick %d: identical runs diverged", seed, tick)
				}
			}
			t.Logf("seed %d: %d valve turns, %d water units, final status %s", seed, turns, initialWater, a.Status())
		})
	}
}

// internal/sim must stay renderer-independent (SPEC §5, CLAUDE.md). This
// allow-list makes adding any non-pure dependency a deliberate, reviewed act.
func TestImportsArePure(t *testing.T) {
	allowed := map[string]bool{
		"crypto/sha256":   true,
		"encoding/binary": true,
		"encoding/hex":    true,
		"errors":          true,
		"fmt":             true,
		"strings":         true,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if !allowed[path] {
				t.Errorf("%s imports %q; internal/sim may only use the pure standard-library allow-list in this test", name, path)
			}
		}
	}
}

// BenchmarkStep measures one tick on the 60x40 demonstration size (SPEC §12).
func BenchmarkStep(b *testing.B) {
	rng := NewSplitMix64(7)
	s, err := New(randomCave(&rng, 60, 40), 7)
	if err != nil {
		b.Fatal(err)
	}
	dirs := []Dir{DirRight, DirDown, DirLeft, DirUp}
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		s.Step(Input{Move: dirs[(i/8)%4]})
	}
}

func BenchmarkStateHash(b *testing.B) {
	rng := NewSplitMix64(7)
	s, err := New(randomCave(&rng, 60, 40), 7)
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		s.StateHash()
	}
}
