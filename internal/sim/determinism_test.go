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
}

// Golden hashes pin the hash byte layout. If this test fails, the encoding or
// a rule changed: bump RuleVersion or the hash magic deliberately, document
// it, and review every replay fixture rather than regenerating blindly.
func TestStateHashGolden(t *testing.T) {
	s, err := New(caveFromRows(t, 1, "#####", "#@.*E", "#####"), 42)
	if err != nil {
		t.Fatal(err)
	}
	const wantStart = "c4a49d84401c0e3f614a02487b2791a7a6fbdf88659e02ed2be6d8233c47c9a8"
	if got := s.StateHash().String(); got != wantStart {
		t.Errorf("tick 0 hash = %s, want %s", got, wantStart)
	}
	for range 3 {
		move(t, s, DirRight)
	}
	if s.Status() != StatusComplete {
		t.Fatalf("status = %s, want complete", s.Status())
	}
	const wantEnd = "363a510e5507771d7bbbbd8e60c1e0b5ea27fb288a7a560b58156519973023d2"
	if got := s.StateHash().String(); got != wantEnd {
		t.Errorf("completed hash = %s, want %s", got, wantEnd)
	}
}

// randomCave builds a bordered cave from rng with exactly one player and one
// exit. It is test-only and makes no solvability promise.
func randomCave(rng *SplitMix64, w, h int) Cave {
	c := Cave{Width: w, Height: h, Cells: make([]Cell, w*h)}
	for y := range h {
		for x := range w {
			cell := &c.Cells[y*w+x]
			if x == 0 || y == 0 || x == w-1 || y == h-1 {
				cell.Terrain = TerrainWall
				continue
			}
			switch r := rng.Intn(10); {
			case r < 4:
				cell.Terrain = TerrainEarth
			case r < 6:
				cell.Terrain = TerrainWall
			case r < 7:
				cell.Occupant = OccupantCrystal
			}
		}
	}
	c.Cells[1*w+1] = Cell{Occupant: OccupantPlayer}
	c.Cells[(h-2)*w+(w-2)] = Cell{Terrain: TerrainExit}
	c.RequiredCrystals = c.CountOccupant(OccupantCrystal) / 2
	return c
}

func checkInvariants(s *Sim, initialCrystals, lastCollected int) error {
	players := 0
	for i, c := range s.st.cells {
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
	if s.ExitOpen() != (s.Collected() >= s.Required()) {
		return fmt.Errorf("exit open = %v with %d/%d crystals", s.ExitOpen(), s.Collected(), s.Required())
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
			inputRNG := NewSplitMix64(seed ^ 0xdeadbeef)
			lastCollected := 0
			for tick := range ticks {
				in := Input{Move: Dir(inputRNG.Intn(5))}
				a.Step(in)
				b.Step(in)
				if err := checkInvariants(a, initialCrystals, lastCollected); err != nil {
					t.Fatalf("seed %d tick %d: %v\n%s", seed, tick, err, a.ASCII())
				}
				lastCollected = a.Collected()
				if a.StateHash() != b.StateHash() {
					t.Fatalf("seed %d tick %d: identical runs diverged", seed, tick)
				}
			}
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
