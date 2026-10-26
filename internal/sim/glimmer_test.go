package sim

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func glimmerAt(s *Sim, p Pos) bool { return s.Cell(p).Occupant == OccupantGlimmer }

// A glimmer keeps the wall on its right: the crawler's corridor test,
// mirrored, circled anticlockwise.
func TestGlimmerFollowsTheWallOnItsRight(t *testing.T) {
	s := newSim(t, 0,
		"#########",
		"#@#    ▲#",
		"### ### #",
		"###     #",
		"#########",
	)
	path := []Pos{
		{6, 1}, {5, 1}, {4, 1}, {3, 1}, {3, 2}, {3, 3},
		{4, 3}, {5, 3}, {6, 3}, {7, 3}, {7, 2}, {7, 1}, {6, 1},
	}
	for i, want := range path {
		ev := s.Step(Input{}) // a creature tick
		wantKinds(t, ev, EventGlimmerMoved)
		if ev[0].Object != OccupantGlimmer || !glimmerAt(s, want) || s.Glimmers() != 1 {
			t.Fatalf("step %d: event %v, glimmer not at %s:\n%s", i, ev[0], want, s.ASCII())
		}
		if ev := idle(s, CrawlerIntervalTicks-1); len(ev) != 0 {
			t.Fatalf("step %d: events between creature steps: %v", i, ev)
		}
	}
}

// In open space the right-first rule turns the glimmer in a tight square,
// the mirror image of a crawler's.
func TestGlimmerInOpenSpaceCirclesASquare(t *testing.T) {
	s := newSim(t, 0,
		"#######",
		"#     #",
		"#     #",
		"#  ▲  #",
		"#     #",
		"#@    #",
		"#######",
	)
	for i, want := range []Pos{{4, 3}, {4, 4}, {3, 4}, {3, 3}, {4, 3}} {
		crawlerTicks(s, 1)
		if !glimmerAt(s, want) {
			t.Fatalf("step %d: glimmer not at %s:\n%s", i, want, s.ASCII())
		}
	}
}

// mirrorCell is c seen in a mirror: a crawler becomes a glimmer and the
// other way round, and facing left and right swap.
func mirrorCell(c Cell) Cell {
	switch c.Occupant {
	case OccupantCrawler:
		c.Occupant = OccupantGlimmer
	case OccupantGlimmer:
		c.Occupant = OccupantCrawler
	}
	switch c.Facing {
	case DirLeft:
		c.Facing = DirRight
	case DirRight:
		c.Facing = DirLeft
	}
	return c
}

// randomCreatureRows builds a closed box of walls, earth, empty floor and a
// little water, with the player at (1,1) and no objects: drier than
// randomWaterRows, so a creature has room to roam.
func randomCreatureRows(rng *SplitMix64, w, h int) []string {
	rows := make([]string, h)
	for y := range h {
		b := make([]byte, w)
		for x := range w {
			switch r := rng.Intn(40); {
			case x == 0 || y == 0 || x == w-1 || y == h-1, r < 10:
				b[x] = '#'
			case r < 14:
				b[x] = '.'
			case r < 15:
				b[x] = byte('1' + rng.Intn(WaterCapacity-1))
			case r < 16:
				b[x] = '~'
			default:
				b[x] = ' '
			}
		}
		rows[y] = string(b)
	}
	rows[1] = "#@" + rows[1][2:]
	return rows
}

// A glimmer is a crawler's mirror image: in a cave with one creature and no
// loose objects, the mirrored cave with the other kind of creature evolves
// into the mirror image, tick for tick. Water is mirror symmetric (see
// TestWaterIsMirrorSymmetricConservedAndSettles), so any difference comes
// from the turn tables. The player stands still.
func TestGlimmerIsTheMirrorImageOfACrawler(t *testing.T) {
	const w, h, ticks = 20, 12, 600
	moves := 0
	for seed := uint64(1); seed <= 30; seed++ {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			rng := NewSplitMix64(seed)
			c := caveFromRows(t, 0, randomCreatureRows(&rng, w, h)...)
			var open []int
			for i, cell := range c.Cells {
				if cell.Terrain == TerrainEmpty && cell.Occupant == OccupantNone && cell.Water == 0 {
					open = append(open, i)
				}
			}
			kind := OccupantCrawler
			if seed%2 == 1 {
				kind = OccupantGlimmer
			}
			c.Cells[open[rng.Intn(len(open))]] = Cell{Occupant: kind, Facing: Dir(1 + rng.Intn(4))}
			m := c.Clone()
			for i, cell := range c.Cells {
				m.Cells[i/w*w+w-1-i%w] = mirrorCell(cell)
			}
			a, err := New(c, 1)
			if err != nil {
				t.Fatal(err)
			}
			b, err := New(m, 1)
			if err != nil {
				t.Fatal(err)
			}
			for range ticks {
				ev := a.Step(Input{})
				b.Step(Input{})
				for _, e := range ev {
					if e.Kind == EventCrawlerMoved || e.Kind == EventGlimmerMoved {
						moves++
					}
				}
				for i, cell := range a.st.cells {
					if got := b.st.cells[i/w*w+w-1-i%w]; got != mirrorCell(cell) {
						t.Fatalf("seed %d tick %d: mirrored run diverged at %s\n%s\nmirror of\n%s",
							seed, a.Tick(), Pos{i % w, i / w}, b.ASCII(), a.ASCII())
					}
				}
			}
		})
	}
	if moves < 1000 {
		t.Errorf("only %d creature moves in all runs; the comparison says little", moves)
	}
}

// Glimmers catch the player like crawlers: by stepping into them, or when
// the player walks into one. The event names the creature.
func TestCreaturesCatchThePlayerEitherWay(t *testing.T) {
	for _, tt := range []struct {
		right, down string
		kind        Occupant
	}{{"→", "↓", OccupantCrawler}, {"▶", "▼", OccupantGlimmer}} {
		t.Run(tt.kind.String(), func(t *testing.T) {
			s := newSim(t, 0, tt.right+" @")
			crawlerTicks(s, 1)
			if s.Status() != StatusPlaying {
				t.Fatalf("being next to a %s killed the player", tt.kind)
			}
			idle(s, CrawlerIntervalTicks-1)
			ev := s.Step(Input{})
			wantKinds(t, ev, EventPlayerCaught)
			if ev[0].Pos != (Pos{2, 0}) || ev[0].From != (Pos{1, 0}) || ev[0].Object != tt.kind || s.Status() != StatusDead {
				t.Fatalf("event %v, status %s", ev[0], s.Status())
			}

			s = newSim(t, 0, "@"+tt.down, "##")
			ev = s.Step(Input{Move: DirRight})
			wantKinds(t, ev, EventPlayerCaught)
			if ev[0].Pos != (Pos{0, 0}) || ev[0].From != (Pos{1, 0}) || ev[0].Object != tt.kind || s.Player() != (Pos{0, 0}) {
				t.Fatalf("event %v, player %s", ev[0], s.Player())
			}
		})
	}
}

func TestGlimmersAreBlockedByTerrainObjectsAndCreatures(t *testing.T) {
	for _, b := range []string{"#", ".", "E", "O", "*", "1", "↑", "▼"} {
		t.Run(b, func(t *testing.T) {
			s := newSim(t, 0, "▶"+b+"#@")
			if ev := idle(s, 5*CrawlerIntervalTicks); len(ev) != 0 || !glimmerAt(s, Pos{0, 0}) {
				t.Fatalf("glimmer passed %q: %v", b, ev)
			}
			if s.Cell(Pos{0, 0}).Facing != DirRight {
				t.Fatalf("facing changed to %s", s.Cell(Pos{0, 0}).Facing)
			}
		})
	}
}

// Crawlers and glimmers share one scan: of two creatures wanting the same
// cell, the first in row-major order takes it.
func TestCrawlersAndGlimmersShareOneScan(t *testing.T) {
	s := newSim(t, 0, "→ ◀#@")
	ev := s.Step(Input{})
	wantKinds(t, ev, EventCrawlerMoved)
	if !crawlerAt(s, Pos{1, 0}) || !glimmerAt(s, Pos{2, 0}) {
		t.Fatalf("after one creature step:\n%s", s.ASCII())
	}
	idle(s, CrawlerIntervalTicks-1)
	ev = s.Step(Input{}) // the crawler turns back; the glimmer follows it
	wantKinds(t, ev, EventCrawlerMoved, EventGlimmerMoved)
	if !crawlerAt(s, Pos{0, 0}) || !glimmerAt(s, Pos{1, 0}) {
		t.Fatalf("after two creature steps:\n%s", s.ASCII())
	}
}

// shatterRows is a glimmer at (2,2) under a boulder at (2,1), with the player
// out of reach. The 3×3 block around the glimmer is rows 1 to 3, columns 1
// to 3.
var shatterRows = []string{
	"#######",
	"# O #@#",
	"# ▲ ###",
	"#   ###",
	"#######",
}

// shatterSim builds shatterRows with the given cells replaced and the object
// at (2,1) (a boulder or a crystal) already falling, so it lands on the
// glimmer on the first gravity step.
func shatterSim(t *testing.T, obj rune, set map[Pos]rune, mechs []Mechanism) *Sim {
	t.Helper()
	grid := make([][]rune, len(shatterRows))
	for y, r := range shatterRows {
		grid[y] = []rune(r)
	}
	grid[1][2] = obj
	for p, r := range set {
		grid[p.Y][p.X] = r
	}
	rows := make([]string, len(grid))
	for y, g := range grid {
		rows[y] = string(g)
	}
	c := caveFromRows(t, 0, rows...)
	c.Mechanisms = mechs
	c.Cells[c.Index(Pos{2, 1})].Falling = true
	s, err := New(c, 1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// A falling boulder or crystal shatters a glimmer: the glimmer's cell and
// every empty, dry floor cell around it hold a resting crystal, and nothing
// else in the block changes. A boulder turns to crystal with them; a crystal
// comes to rest where it was. Nothing lands.
func TestFallingObjectShattersAGlimmer(t *testing.T) {
	object := Pos{2, 1}
	block := []Pos{{1, 1}, object, {3, 1}, {1, 2}, {2, 2}, {3, 2}, {1, 3}, {2, 3}, {3, 3}}
	gate := []Mechanism{{Valve: Pos{5, 2}, Gates: []Pos{{1, 1}}}}
	tests := []struct {
		name  string
		at    Pos  // the neighbour under test
		r     rune // what it holds
		forms bool // whether a crystal forms there
		mechs []Mechanism
		valve rune // the valve's symbol, for caves with a gate
		check func(t *testing.T, s *Sim)
	}{
		{name: "empty floor", at: Pos{1, 1}, r: ' ', forms: true},
		{name: "earth", at: Pos{1, 1}, r: '.'},
		{name: "wall", at: Pos{3, 2}, r: '#'},
		{name: "exit", at: Pos{1, 2}, r: 'E'},
		{name: "shallow water", at: Pos{1, 3}, r: '2', check: func(t *testing.T, s *Sim) {
			if s.Water() != 2 {
				t.Errorf("water %d, want 2", s.Water())
			}
		}},
		{name: "deep water", at: Pos{3, 3}, r: '~'},
		{name: "resting boulder", at: Pos{1, 3}, r: 'O'},
		{name: "resting crystal", at: Pos{3, 3}, r: '*'},
		{name: "crawler", at: Pos{1, 3}, r: '↑', check: func(t *testing.T, s *Sim) {
			if s.Crawlers() != 1 {
				t.Errorf("the shattering removed a crawler")
			}
		}},
		{name: "another glimmer", at: Pos{3, 3}, r: '▲', check: func(t *testing.T, s *Sim) {
			if s.Glimmers() != 1 {
				t.Errorf("%d glimmers left, want 1", s.Glimmers())
			}
		}},
		{name: "open gate", at: Pos{1, 1}, r: 'g', forms: true, mechs: gate, valve: 'v'},
		{name: "closed gate", at: Pos{1, 1}, r: 'G', mechs: gate, valve: 'V'},
	}
	for _, obj := range []rune{SymbolBoulder, SymbolCrystal} {
		for _, tt := range tests {
			t.Run(string(obj)+" "+tt.name, func(t *testing.T) {
				set := map[Pos]rune{tt.at: tt.r}
				if tt.valve != 0 {
					set[Pos{5, 2}] = tt.valve
				}
				s := shatterSim(t, obj, set, tt.mechs)
				before := s.Cell(tt.at)
				falling, _ := CellForSymbol(obj)
				want := []Event{{Kind: EventGlimmerShattered, Pos: Pos{2, 2}, From: object, Object: falling.Occupant}}
				for _, p := range block {
					switch {
					case p == object && falling.Occupant == OccupantBoulder, p != object && (p != tt.at || tt.forms):
						want = append(want, Event{Kind: EventCrystalFormed, Pos: p})
					}
				}
				if ev := s.Step(Input{}); !slices.Equal(ev, want) {
					t.Fatalf("events\n%v\nwant\n%v\n%s", ev, want, s.ASCII())
				}
				for _, p := range block {
					c := s.Cell(p)
					if p == tt.at && !tt.forms {
						if c.Terrain != before.Terrain || c.Occupant != before.Occupant {
							t.Errorf("%s changed from %+v to %+v", p, before, c)
						}
						continue
					}
					if c.Occupant != OccupantCrystal || c.Falling {
						t.Errorf("%s holds %+v, want a resting crystal", p, c)
					}
				}
				if c := s.Cell(object); c.Occupant != OccupantCrystal || c.Falling {
					t.Errorf("the falling %s ended as %+v, want a resting crystal at %s", falling.Occupant, c, object)
				}
				if s.Glimmers() > 1 || s.Glimmers() == 1 && tt.r != '▲' {
					t.Errorf("the glimmer survived:\n%s", s.ASCII())
				}
				if tt.check != nil {
					tt.check(t, s)
				}
			})
		}
	}
}

// A shattering never harms the player, even next to the glimmer, and a
// crystal formed over the player's head rests there without crushing them.
func TestShatteringSparesThePlayer(t *testing.T) {
	s := shatterSim(t, SymbolBoulder, map[Pos]rune{{3, 3}: '@', {5, 1}: '#'}, nil)
	ev := s.Step(Input{})
	if !slices.ContainsFunc(ev, func(e Event) bool { return e.Kind == EventCrystalFormed && e.Pos == (Pos{3, 2}) }) {
		t.Fatalf("no crystal formed over the player: %v", ev)
	}
	if ev := idle(s, 40); len(ev) != 0 || s.Status() != StatusPlaying || s.Cell(Pos{3, 3}).Occupant != OccupantPlayer {
		t.Fatalf("events %v, status %s\n%s", ev, s.Status(), s.ASCII())
	}
	// The player can collect the crystal above them.
	ev = move(t, s, DirUp)
	wantKinds(t, ev, EventMoved, EventCollected)
}

// Crystals formed by a shattering sit out the rest of that gravity step,
// like a pushed boulder, and fall from the next. Mutation-checked: without
// the updated marks, the crystal at (3,1) falls into the wet cell below it in
// the same tick.
func TestNewCrystalsWaitForTheNextGravityStep(t *testing.T) {
	s := shatterSim(t, SymbolBoulder, map[Pos]rune{{3, 2}: '1'}, nil)
	ev := s.Step(Input{})
	if slices.ContainsFunc(ev, func(e Event) bool { return e.Kind == EventFell || e.Kind == EventRolled }) {
		t.Fatalf("a crystal moved in the tick it formed: %v\n%s", ev, s.ASCII())
	}
	if c := s.Cell(Pos{3, 1}); c.Occupant != OccupantCrystal || waterAt(s, Pos{3, 2}) != 1 {
		t.Fatalf("after the shattering:\n%s", s.ASCII())
	}
	idle(s, 1)
	ev = s.Step(Input{}) // tick 2: it falls into the water, displacing it
	wantKinds(t, ev, EventFell)
	if ev[0].From != (Pos{3, 1}) || ev[0].Pos != (Pos{3, 2}) || ev[0].Object != OccupantCrystal || waterAt(s, Pos{3, 1}) != 1 {
		t.Fatalf("event %v\n%s", ev[0], s.ASCII())
	}
}

// At the cave edge the block is cut short; only cells inside the cave count.
// The empty cells at the right edge are where a block that wrapped around
// the rows would wrongly reach.
func TestShatteringAtTheCaveEdge(t *testing.T) {
	c := caveFromRows(t, 0, "O#@ ", "▲ # ")
	c.Cells[0].Falling = true
	s, err := New(c, 1)
	if err != nil {
		t.Fatal(err)
	}
	ev := s.Step(Input{})
	wantKinds(t, ev, EventGlimmerShattered, EventCrystalFormed, EventCrystalFormed, EventCrystalFormed)
	if ev[1].Pos != (Pos{0, 0}) || ev[2].Pos != (Pos{0, 1}) || ev[3].Pos != (Pos{1, 1}) {
		t.Fatalf("events %v", ev)
	}
}

// Only a falling object shatters a glimmer; a resting one is held up.
func TestRestingObjectDoesNotShatterAGlimmer(t *testing.T) {
	s := newSim(t, 0,
		"##@",
		"O##",
		"▲##",
		"###",
	)
	if ev := idle(s, 30); len(ev) != 0 || s.Glimmers() != 1 {
		t.Fatalf("a resting boulder shattered the glimmer: %v", ev)
	}
}

// Glimmers never enter water, and a drowned glimmer leaves no crystals.
func TestDrownedGlimmerLeavesNoCrystals(t *testing.T) {
	s := newSim(t, 0, "▶1#@")
	if ev := idle(s, 5*CrawlerIntervalTicks); len(ev) != 0 || !glimmerAt(s, Pos{0, 0}) {
		t.Fatalf("the glimmer entered water or drowned: %v", ev)
	}

	s = newSim(t, 0, "~▲#@", "####")
	ev := s.Step(Input{})
	wantKinds(t, ev, EventGlimmerDrowned)
	if ev[0].Pos != (Pos{1, 0}) || s.Glimmers() != 0 || countOccupant(s, OccupantCrystal) != 0 || s.Water() != WaterCapacity {
		t.Fatalf("event %v, glimmers %d, water %d\n%s", ev[0], s.Glimmers(), s.Water(), s.ASCII())
	}
}

func TestResetRestoresGlimmers(t *testing.T) {
	s := shatterSim(t, SymbolBoulder, nil, nil)
	start, layout := s.StateHash(), s.ASCII()
	idle(s, 10)
	if s.Glimmers() != 0 || countOccupant(s, OccupantCrystal) == 0 {
		t.Fatalf("the glimmer did not shatter:\n%s", s.ASCII())
	}
	s.Reset()
	if s.ASCII() != layout || s.StateHash() != start || s.Glimmers() != 1 {
		t.Fatalf("reset did not restore the glimmer:\n%s", s.ASCII())
	}
}

func TestGlimmerValidation(t *testing.T) {
	tests := []struct {
		name     string
		required int
		mutate   func(*Cave)
		wantErr  string
	}{
		{"no facing", 0, func(c *Cave) { c.Cells[1].Facing = DirNone }, "a glimmer must face up, down, left or right"},
		{"in water", 0, func(c *Cave) { c.Cells[1].Water = 1 }, "a glimmer cannot start in water"},
		{"in earth", 0, func(c *Cave) { c.Cells[1].Terrain = TerrainEarth }, "glimmer cannot be embedded in earth"},
		{"in a wall", 0, func(c *Cave) { c.Cells[1].Terrain = TerrainWall }, "glimmer cannot occupy a wall"},
		{"quota above what it can yield", 1 + ShatterCrystals + 1, func(*Cave) {},
			"required crystals 11 exceeds the 10 the cave can yield (1 placed, up to 9 from each of 1 glimmer(s))"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := caveFromRows(t, tt.required, "@▶ *")
			tt.mutate(&c)
			_, err := New(c, 0)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %v, want %q", err, tt.wantErr)
			}
		})
	}
	// A quota may count on every crystal a glimmer could leave.
	if _, err := New(caveFromRows(t, 1+ShatterCrystals, "@▶ *"), 0); err != nil {
		t.Fatalf("quota of placed plus shatterable crystals rejected: %v", err)
	}
}

func TestGlimmerSymbolsRoundTrip(t *testing.T) {
	rows := []string{"▲▶▼◀@"}
	s := newSim(t, 0, rows...)
	for i, d := range []Dir{DirUp, DirRight, DirDown, DirLeft} {
		if c := s.Cell(Pos{i, 0}); c.Occupant != OccupantGlimmer || c.Facing != d {
			t.Fatalf("cell %d: %+v, want a glimmer facing %s", i, c, d)
		}
	}
	if s.ASCII() != rowsASCII(rows...) {
		t.Fatalf("ASCII %q", s.ASCII())
	}
}
