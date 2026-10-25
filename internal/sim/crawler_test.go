package sim

import (
	"strings"
	"testing"
)

func crawlerAt(s *Sim, p Pos) bool { return s.Cell(p).Occupant == OccupantCrawler }

// crawlerTicks steps until n crawler steps have run (ticks that are
// multiples of CrawlerIntervalTicks) and returns every event on the way.
func crawlerTicks(s *Sim, n int) []Event {
	var all []Event
	for done := 0; done < n; {
		if s.Tick()%CrawlerIntervalTicks == 0 {
			done++
		}
		all = append(all, s.Step(Input{})...)
	}
	return all
}

// A crawler keeps the wall on its left: around a block it circles the
// corridor clockwise, one cell per crawler step, and nothing moves between
// steps.
func TestCrawlerFollowsTheWallOnItsLeft(t *testing.T) {
	s := newSim(t, 0,
		"#########",
		"#↑    #@#",
		"# ### ###",
		"#     ###",
		"#########",
	)
	path := []Pos{
		{2, 1}, {3, 1}, {4, 1}, {5, 1}, {5, 2}, {5, 3},
		{4, 3}, {3, 3}, {2, 3}, {1, 3}, {1, 2}, {1, 1}, {2, 1},
	}
	for i, want := range path {
		ev := s.Step(Input{}) // a crawler tick
		wantKinds(t, ev, EventCrawlerMoved)
		if !crawlerAt(s, want) || s.Crawlers() != 1 {
			t.Fatalf("step %d: crawler not at %s:\n%s", i, want, s.ASCII())
		}
		if ev := idle(s, CrawlerIntervalTicks-1); len(ev) != 0 {
			t.Fatalf("step %d: events between crawler steps: %v", i, ev)
		}
	}
}

// In open space the left-first rule turns the crawler in a tight square.
func TestCrawlerInOpenSpaceCirclesASquare(t *testing.T) {
	s := newSim(t, 0,
		"#######",
		"#     #",
		"#     #",
		"#  ↑  #",
		"#     #",
		"#    @#",
		"#######",
	)
	for i, want := range []Pos{{2, 3}, {2, 4}, {3, 4}, {3, 3}, {2, 3}} {
		crawlerTicks(s, 1)
		if !crawlerAt(s, want) {
			t.Fatalf("step %d: crawler not at %s:\n%s", i, want, s.ASCII())
		}
	}
}

func TestBoxedInCrawlerStays(t *testing.T) {
	s := newSim(t, 0, "#→#@", "####")
	if ev := idle(s, 5*CrawlerIntervalTicks); len(ev) != 0 || !crawlerAt(s, Pos{1, 0}) {
		t.Fatalf("a boxed-in crawler moved: %v", ev)
	}
	if s.Cell(Pos{1, 0}).Facing != DirRight {
		t.Fatalf("facing changed to %s", s.Cell(Pos{1, 0}).Facing)
	}
}

// The crawler catches the player by stepping into them; the player is not
// hurt by standing next to one.
func TestCrawlerCatchesThePlayerByMovingIntoThem(t *testing.T) {
	s := newSim(t, 0, "→ @")
	wantKinds(t, s.Step(Input{}), EventCrawlerMoved)
	if s.Status() != StatusPlaying {
		t.Fatal("being next to a crawler killed the player")
	}
	idle(s, CrawlerIntervalTicks-1)
	ev := s.Step(Input{})
	wantKinds(t, ev, EventPlayerCaught)
	if ev[0].Pos != (Pos{2, 0}) || ev[0].From != (Pos{1, 0}) || s.Status() != StatusDead {
		t.Fatalf("event %v, status %s", ev[0], s.Status())
	}
	if !crawlerAt(s, Pos{1, 0}) || s.Player() != (Pos{2, 0}) {
		t.Fatalf("after the catch:\n%s", s.ASCII())
	}
	if ev := idle(s, 20); len(ev) != 0 || s.Tick() != CrawlerIntervalTicks+1 {
		t.Fatalf("the cave did not freeze after the catch: %v at tick %d", ev, s.Tick())
	}
}

// A catch ends the crawler step: crawlers later in the scan stay put.
func TestCatchStopsTheCrawlerStep(t *testing.T) {
	s := newSim(t, 0, "→@##", "## →")
	wantKinds(t, s.Step(Input{}), EventPlayerCaught)
	if !crawlerAt(s, Pos{3, 1}) {
		t.Fatalf("a crawler moved after the catch:\n%s", s.ASCII())
	}
}

func TestWalkingIntoACrawlerIsFatal(t *testing.T) {
	s := newSim(t, 0, "@↓", "##")
	ev := s.Step(Input{Move: DirRight})
	wantKinds(t, ev, EventPlayerCaught)
	if ev[0].Pos != (Pos{0, 0}) || ev[0].From != (Pos{1, 0}) || s.Status() != StatusDead || s.Player() != (Pos{0, 0}) {
		t.Fatalf("event %v, status %s, player %s", ev[0], s.Status(), s.Player())
	}
}

// Only a falling object crushes a crawler, like the player; it then falls on
// into the crawler's cell.
func TestFallingObjectCrushesACrawler(t *testing.T) {
	for _, obj := range []string{"O", "*"} {
		t.Run(obj, func(t *testing.T) {
			s := newSim(t, 0,
				obj+"#@",
				" ##",
				"↑##",
				"###",
			)
			ev := s.Step(Input{}) // tick 0: the object falls; the crawler is blocked by it
			wantKinds(t, ev, EventFell)
			idle(s, 1)
			ev = s.Step(Input{}) // tick 2
			wantKinds(t, ev, EventCrawlerCrushed, EventFell)
			if ev[0].Pos != (Pos{0, 2}) || ev[0].From != (Pos{0, 1}) || s.Crawlers() != 0 {
				t.Fatalf("events %v, crawlers %d", ev, s.Crawlers())
			}
			if s.Cell(Pos{0, 2}).Occupant.String() != map[string]string{"O": "boulder", "*": "crystal"}[obj] {
				t.Fatalf("after the crush:\n%s", s.ASCII())
			}
		})
	}
}

func TestRestingObjectDoesNotCrushACrawler(t *testing.T) {
	s := newSim(t, 0,
		"##@",
		"O##",
		"↑##",
		"###",
	)
	if ev := idle(s, 30); len(ev) != 0 || s.Crawlers() != 1 {
		t.Fatalf("a resting boulder crushed the crawler: %v", ev)
	}
}

func TestCrawlersNeverEnterWaterAndDrownWhenItReachesThem(t *testing.T) {
	s := newSim(t, 0, "→1#@")
	if ev := idle(s, 5*CrawlerIntervalTicks); len(ev) != 0 || !crawlerAt(s, Pos{0, 0}) {
		t.Fatalf("the crawler entered water or drowned: %v", ev)
	}

	s = newSim(t, 0, "~↑#@", "####")
	ev := s.Step(Input{})
	wantKinds(t, ev, EventCrawlerDrowned)
	if ev[0].Pos != (Pos{1, 0}) || s.Crawlers() != 0 || s.Water() != WaterCapacity {
		t.Fatalf("event %v, crawlers %d, water %d", ev[0], s.Crawlers(), s.Water())
	}
}

func TestCrawlersAreBlockedByTerrainAndObjects(t *testing.T) {
	for _, b := range []string{"#", ".", "E", "O", "*"} {
		t.Run(b, func(t *testing.T) {
			s := newSim(t, 0, "→"+b+"#@")
			if ev := idle(s, 5*CrawlerIntervalTicks); len(ev) != 0 || !crawlerAt(s, Pos{0, 0}) {
				t.Fatalf("crawler passed %q: %v", b, ev)
			}
		})
	}
	t.Run("closed gate", func(t *testing.T) {
		s := newSimMech(t, []Mechanism{{Valve: Pos{4, 0}, Gates: []Pos{{1, 0}}}}, "→G#@V")
		if ev := idle(s, 5*CrawlerIntervalTicks); len(ev) != 0 || !crawlerAt(s, Pos{0, 0}) {
			t.Fatalf("crawler passed a closed gate: %v", ev)
		}
	})
	t.Run("open gate", func(t *testing.T) {
		s := newSimMech(t, []Mechanism{{Valve: Pos{5, 0}, Gates: []Pos{{1, 0}}}}, "→g #@v")
		crawlerTicks(s, 2)
		if !crawlerAt(s, Pos{2, 0}) {
			t.Fatalf("crawler did not pass an open gate:\n%s", s.ASCII())
		}
	})
}

// The row-major scan skips a cell a crawler has just moved into, so a
// crawler moving down or right is not moved again in the same step.
func TestCrawlerMovesAtMostOncePerStep(t *testing.T) {
	s := newSim(t, 0,
		"#↓#",
		"# #",
		"###",
		"→  ",
		"###",
		"@##",
	)
	ev := s.Step(Input{})
	wantKinds(t, ev, EventCrawlerMoved, EventCrawlerMoved)
	if !crawlerAt(s, Pos{1, 1}) || !crawlerAt(s, Pos{1, 3}) {
		t.Fatalf("after one crawler step:\n%s", s.ASCII())
	}
}

func TestResetRestoresCrawlers(t *testing.T) {
	rows := []string{"#######", "#↑   #@", "#######"}
	s := newSim(t, 0, rows...)
	start := s.StateHash()
	crawlerTicks(s, 3)
	if s.ASCII() == rowsASCII(rows...) {
		t.Fatal("nothing moved before the reset")
	}
	s.Reset()
	if s.ASCII() != rowsASCII(rows...) || s.StateHash() != start {
		t.Fatalf("reset did not restore the crawler:\n%s", s.ASCII())
	}
}

func TestCrawlerValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Cave)
		wantErr string
	}{
		{"no facing", func(c *Cave) { c.Cells[1].Facing = DirNone }, "must face up, down, left or right"},
		{"facing on a boulder", func(c *Cave) { c.Cells[2] = Cell{Occupant: OccupantBoulder, Facing: DirUp} }, "only crawlers and glimmers have a facing"},
		{"in water", func(c *Cave) { c.Cells[1].Water = 1 }, "cannot start in water"},
		{"in earth", func(c *Cave) { c.Cells[1].Terrain = TerrainEarth }, "embedded in earth"},
		{"in a wall", func(c *Cave) { c.Cells[1].Terrain = TerrainWall }, "crawler cannot occupy a wall"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := caveFromRows(t, 0, "@→ ")
			tt.mutate(&c)
			_, err := New(c, 0)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestCrawlerSymbolsRoundTrip(t *testing.T) {
	rows := []string{"↑→↓←@"}
	s := newSim(t, 0, rows...)
	for i, d := range []Dir{DirUp, DirRight, DirDown, DirLeft} {
		if c := s.Cell(Pos{i, 0}); c.Occupant != OccupantCrawler || c.Facing != d {
			t.Fatalf("cell %d: %+v, want a crawler facing %s", i, c, d)
		}
	}
	if s.ASCII() != rowsASCII(rows...) {
		t.Fatalf("ASCII %q", s.ASCII())
	}
}
