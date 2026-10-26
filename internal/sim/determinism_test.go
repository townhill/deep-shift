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
	s = base(1)
	s.st.cells[s.Cave().Index(Pos{3, 1})] = Cell{Occupant: OccupantCrawler, Facing: DirUp}
	add("crawler facing up", s)
	s.st.cells[s.Cave().Index(Pos{3, 1})].Facing = DirLeft
	add("crawler facing left", s)
	s.st.cells[s.Cave().Index(Pos{3, 1})].Occupant = OccupantGlimmer
	add("glimmer facing left", s)
}

// Golden hashes pin the hash byte layout. If this test fails, the encoding or
// a rule changed: bump RuleVersion or the hash magic deliberately, document
// it, and review every replay fixture rather than regenerating blindly.
// Last re-pinned for rule version 5 (glimmers); the hash layout is still v4
// (facing byte per cell, D-041).
func TestStateHashGolden(t *testing.T) {
	s, err := New(caveFromRows(t, 1, "#####", "#@.*E", "#####"), 42)
	if err != nil {
		t.Fatal(err)
	}
	const wantStart = "7473ffe36e4f49133523844551c10c3e7fea14a2b8fa89690c10798b22997aac"
	if got := s.StateHash().String(); got != wantStart {
		t.Errorf("tick 0 hash = %s, want %s", got, wantStart)
	}
	for range 3 {
		move(t, s, DirRight)
	}
	if s.Status() != StatusComplete {
		t.Fatalf("status = %s, want complete", s.Status())
	}
	const wantEnd = "a8bd33d757a7b00017c0914131a48a7744212c09b806ff815651dc27af2b6f87"
	if got := s.StateHash().String(); got != wantEnd {
		t.Errorf("completed hash = %s, want %s", got, wantEnd)
	}
}

// randomCave builds a bordered cave from rng with exactly one player, one
// exit, water, crawlers and glimmers (none within three cells of the player
// start), and two valves (one beside the player start) each linked to a few
// gates. It is test-only and makes no solvability promise.
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
			case r == 18 && x+y > 5:
				// No extra draw, so the rest of the cave matches the
				// pre-crawler layout for the same seed.
				cell.Occupant, cell.Facing = OccupantCrawler, Dir(1+(x+y)%4)
			case r == 19 && x+y > 5:
				cell.Occupant, cell.Facing = OccupantGlimmer, Dir(1+(x+y)%4)
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

// checkInvariants checks s after a tick. crystals is the number of crystals
// the cave has held: those placed in it plus those formed by shattered
// glimmers so far.
func checkInvariants(s *Sim, crystals, initialBoulders, lastCollected, initialWater int) error {
	players := 0
	for i, c := range s.st.cells {
		if c.Occupant.Creature() != (c.Facing != DirNone) || c.Facing > DirRight {
			return fmt.Errorf("%s facing %s at index %d", c.Occupant, c.Facing, i)
		}
		if c.Occupant.Creature() && (!c.Terrain.Floor() || c.Water > 0) {
			return fmt.Errorf("%s on %s terrain with %d water at index %d", c.Occupant, c.Terrain, c.Water, i)
		}
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
	if got := s.Collected() + countOccupant(s, OccupantCrystal); got != crystals {
		return fmt.Errorf("crystals not conserved: %d collected + remaining, want %d placed + formed", got, crystals)
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

// checkMoveEvents verifies one tick's object and creature movement: falls go
// exactly one cell down, rolls and pushes one cell sideways, creatures one
// cell in any direction, and nothing moves twice (no move starts where an
// earlier move in the same tick ended, and no two moves share a start or an
// end). A crystal formed by a shattered glimmer counts as having moved, so it
// may not move again in the same tick.
func checkMoveEvents(events []Event) error {
	starts, ends := map[Pos]bool{}, map[Pos]bool{}
	for _, e := range events {
		var dx, dy int
		switch e.Kind {
		case EventCrystalFormed:
			if ends[e.Pos] {
				return fmt.Errorf("%v: a crystal formed where an object moved this tick", e)
			}
			ends[e.Pos] = true
			continue
		case EventFell:
			dx, dy = 0, 1
		case EventRolled, EventBoulderPushed:
			dx, dy = e.Pos.X-e.From.X, 0
			if dx != 1 && dx != -1 {
				return fmt.Errorf("%v is not one cell sideways", e)
			}
		case EventCrawlerMoved, EventGlimmerMoved:
			dx, dy = e.Pos.X-e.From.X, e.Pos.Y-e.From.Y
			if dx*dx+dy*dy != 1 {
				return fmt.Errorf("%v is not one cell", e)
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

// checkShatters verifies one tick's shattered glimmers: each glimmer-shattered
// is followed by the crystals it formed, all inside the 3×3 block around the
// glimmer and one in the glimmer's own cell, and then by the shattering
// object landing where it was.
func checkShatters(events []Event) error {
	for i, e := range events {
		if e.Kind != EventGlimmerShattered {
			continue
		}
		j, own := i+1, false
		for ; j < len(events) && events[j].Kind == EventCrystalFormed; j++ {
			p := events[j].Pos
			if p.X < e.Pos.X-1 || p.X > e.Pos.X+1 || p.Y < e.Pos.Y-1 || p.Y > e.Pos.Y+1 {
				return fmt.Errorf("%v formed a crystal outside its block at %s", e, p)
			}
			own = own || p == e.Pos
		}
		if !own {
			return fmt.Errorf("%v left no crystal in the glimmer's cell", e)
		}
		if j == len(events) || events[j].Kind != EventLanded || events[j].Pos != e.From || events[j].Object != e.Object {
			return fmt.Errorf("%v: the shattering %s did not land at %s", e, e.Object, e.From)
		}
	}
	return nil
}

// Seeded randomised runs: invariants hold after every tick, and two runs of
// the same inputs agree on every tick's hash. The seed is in every failure.
func TestRandomisedRunsKeepInvariantsAndDeterminism(t *testing.T) {
	const ticks = 3000
	shatters := 0
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
			crawlers, glimmers := a.Crawlers(), a.Glimmers()
			crystals := initialCrystals
			seen := map[EventKind]int{}
			for tick := range ticks {
				// One input in six is Interact, so valves near the player's
				// path get turned both ways.
				r := inputRNG.Intn(6)
				in := Input{Move: Dir(r % 5), Interact: r == 5}
				ev := a.Step(in)
				b.Step(in)
				for _, e := range ev {
					seen[e.Kind]++
					switch e.Kind {
					case EventValveTurned:
						turns++
					case EventCrawlerCrushed, EventCrawlerDrowned:
						crawlers--
					case EventGlimmerShattered, EventGlimmerDrowned:
						glimmers--
					case EventCrystalFormed:
						crystals++
					case EventPlayerCaught:
						if a.Status() != StatusDead {
							t.Fatalf("seed %d tick %d: caught but %s", seed, tick, a.Status())
						}
					}
				}
				if a.Crawlers() != crawlers || a.Glimmers() != glimmers {
					t.Fatalf("seed %d tick %d: %d crawlers and %d glimmers, want %d and %d after removals\n%s",
						seed, tick, a.Crawlers(), a.Glimmers(), crawlers, glimmers, a.ASCII())
				}
				if err := checkMoveEvents(ev); err != nil {
					t.Fatalf("seed %d tick %d: %v\n%s", seed, tick, err, a.ASCII())
				}
				if err := checkShatters(ev); err != nil {
					t.Fatalf("seed %d tick %d: %v\n%s", seed, tick, err, a.ASCII())
				}
				if err := checkInvariants(a, crystals, initialBoulders, lastCollected, initialWater); err != nil {
					t.Fatalf("seed %d tick %d: %v\n%s", seed, tick, err, a.ASCII())
				}
				lastCollected = a.Collected()
				if a.StateHash() != b.StateHash() {
					t.Fatalf("seed %d tick %d: identical runs diverged", seed, tick)
				}
			}
			shatters += seen[EventGlimmerShattered]
			t.Logf("seed %d: %d valve turns, %d water units, %d crawler moves, %d crushed, %d drowned; "+
				"%d glimmer moves, %d shattered into %d crystals, %d drowned; final status %s at tick %d",
				seed, turns, initialWater, seen[EventCrawlerMoved], seen[EventCrawlerCrushed], seen[EventCrawlerDrowned],
				seen[EventGlimmerMoved], seen[EventGlimmerShattered], seen[EventCrystalFormed], seen[EventGlimmerDrowned],
				a.Status(), a.Tick())
		})
	}
	if shatters == 0 {
		t.Error("no glimmer was shattered in any run, so the shatter invariants were never exercised")
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
