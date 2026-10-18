package replay

import (
	"slices"
	"testing"

	"github.com/townhill/deep-shift/internal/sim"
)

// Places in crawler_warren.
var (
	warrenPockets  = []sim.Pos{{X: 1, Y: 5}, {X: 12, Y: 5}, {X: 6, Y: 8}} // beside the ring, on its crawler's right
	warrenShaftTop = sim.Pos{X: 16, Y: 3}
	warrenShaftEnd = sim.Pos{X: 16, Y: 10}
	warrenCorridor = []sim.Pos{{X: 20, Y: 13}, {X: 21, Y: 13}, {X: 22, Y: 13}, {X: 23, Y: 13}, {X: 24, Y: 13}}
)

// warrenAt replays the completion fixture's commands that come before tick
// before, idles up to tick until, and returns the simulation there.
func warrenAt(t *testing.T, before, until uint64) *sim.Sim {
	t.Helper()
	r := loadFixture(t, "crawler_warren_complete.json")
	l := freshCave(t, r.CaveID)
	s, err := sim.New(l.Cave(), r.Seed)
	if err != nil {
		t.Fatal(err)
	}
	next := 0
	for s.Tick() < until {
		var in sim.Input
		if next < len(r.Commands) && r.Commands[next].Tick == s.Tick() && s.Tick() < before {
			in, _ = r.Commands[next].Input()
			next++
		}
		s.Step(in)
	}
	if s.Status() != sim.StatusPlaying {
		t.Fatalf("status %s at tick %d", s.Status(), until)
	}
	return s
}

// eventTick returns the tick of the completion fixture's first event of
// kind k.
func eventTick(t *testing.T, k sim.EventKind) uint64 {
	t.Helper()
	r := loadFixture(t, "crawler_warren_complete.json")
	tick, found := uint64(0), false
	if _, err := RunObserved(r, freshCave(t, r.CaveID), func(n uint64, events []sim.Event, _ *sim.Sim) {
		if !found && countKind(events, k) > 0 {
			tick, found = n, true
		}
	}); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("no %s event", k)
	}
	return tick
}

func countKind(events []sim.Event, k sim.EventKind) int {
	n := 0
	for _, e := range events {
		if e.Kind == k {
			n++
		}
	}
	return n
}

// The completion route kills two crawlers through two older systems: a
// boulder crushes one, water drowns another. The ring crawler survives.
func TestCrawlerWarrenRouteCrushesOneCrawlerAndDrownsAnother(t *testing.T) {
	r := loadFixture(t, "crawler_warren_complete.json")
	var crushed, drowned []sim.Event
	res, err := RunObserved(r, freshCave(t, r.CaveID), func(_ uint64, events []sim.Event, _ *sim.Sim) {
		for _, e := range events {
			switch e.Kind {
			case sim.EventCrawlerCrushed:
				crushed = append(crushed, e)
			case sim.EventCrawlerDrowned:
				drowned = append(drowned, e)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Check(res); err != nil {
		t.Fatal(err)
	}
	if len(crushed) != 1 || crushed[0].Pos.X != warrenShaftTop.X || crushed[0].Object != sim.OccupantBoulder {
		t.Errorf("crushed %v, want one crawler crushed by the boulder in the shaft", crushed)
	}
	if len(drowned) != 1 || !slices.Contains(warrenCorridor, drowned[0].Pos) {
		t.Errorf("drowned %v, want one crawler drowned in the corridor", drowned)
	}
	if res.Sim.Crawlers() != 1 {
		t.Errorf("%d crawlers left, want the ring's one:\n%s", res.Sim.Crawlers(), res.Sim.ASCII())
	}
}

// The shaft is the only way down to the chamber, the corridor and the exit
// (everything at x >= 17, y >= 8): a flood fill that may dig any earth and
// pass any boulder, but stays out of the shaft, never gets there.
func TestCrawlerWarrenTheShaftIsTheOnlyWayDown(t *testing.T) {
	l := freshCave(t, "crawler_warren")
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[sim.Pos]bool{s.Player(): true}
	queue := []sim.Pos{s.Player()}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if p.X >= 17 && p.Y >= 8 {
			t.Fatalf("reached %s without going down the shaft", p)
		}
		for _, d := range []sim.Dir{sim.DirUp, sim.DirDown, sim.DirLeft, sim.DirRight} {
			n := p.Add(d)
			c := s.Cell(n)
			if seen[n] || n.X == warrenShaftTop.X && n.Y >= warrenShaftTop.Y && n.Y <= warrenShaftEnd.Y ||
				c.Terrain == sim.TerrainWall || c.Terrain == sim.TerrainExit {
				continue
			}
			seen[n] = true
			queue = append(queue, n)
		}
	}
	if !seen[sim.Pos{X: warrenShaftTop.X - 1, Y: warrenShaftTop.Y}] {
		t.Fatal("the flood fill never reached the boulder beside the shaft")
	}
}

// The quota needs crystals from the pockets beside the ring: the rest of the
// cave does not hold enough.
func TestCrawlerWarrenQuotaNeedsTheRing(t *testing.T) {
	l := freshCave(t, "crawler_warren")
	c := l.Cave()
	elsewhere := c.CountOccupant(sim.OccupantCrystal)
	for _, p := range warrenPockets {
		if c.Cells[c.Index(p)].Occupant != sim.OccupantCrystal {
			t.Fatalf("no crystal in the pocket at %s", p)
		}
		elsewhere--
	}
	if elsewhere >= l.RequiredCrystals {
		t.Fatalf("%d crystals outside the ring's pockets meet the quota of %d", elsewhere, l.RequiredCrystals)
	}
}

// Pushed into the shaft at any moment of the crawler's patrol, the boulder
// crushes it: it falls three times as fast as a crawler moves, and the
// crawler cannot leave the shaft. The only refusal is while the crawler sits
// in the shaft's top cell, blocking the push.
func TestCrawlerWarrenDropCrushesTheCrawlerWhateverTheTiming(t *testing.T) {
	push := eventTick(t, sim.EventBoulderPushed)
	const period = 2 * 7 * sim.CrawlerIntervalTicks // eight shaft cells: there and back
	pushed, refused := 0, 0
	for k := range uint64(period) {
		s := warrenAt(t, push, push+k)
		if !s.MoveReady() {
			t.Fatalf("k=%d: player not ready to push", k)
		}
		blocked := s.Cell(warrenShaftTop).Occupant == sim.OccupantCrawler
		ev := s.Step(sim.Input{Move: sim.DirRight})
		if countKind(ev, sim.EventBoulderPushed) == 0 {
			if !blocked {
				t.Fatalf("k=%d: push refused with the shaft top clear:\n%s", k, s.ASCII())
			}
			refused++
			continue
		}
		pushed++
		var all []sim.Event
		for range 40 {
			all = append(all, s.Step(sim.Input{})...)
		}
		if countKind(all, sim.EventCrawlerCrushed) != 1 || s.Cell(warrenShaftEnd).Occupant != sim.OccupantBoulder || s.Status() != sim.StatusPlaying {
			t.Fatalf("k=%d: crushes %d, status %s:\n%s", k, countKind(all, sim.EventCrawlerCrushed), s.Status(), s.ASCII())
		}
	}
	if pushed == 0 || refused == 0 {
		t.Fatalf("%d pushes and %d refusals over a full patrol; want both", pushed, refused)
	}
}

// Opened at any moment of the corridor crawler's patrol, the sluice drowns
// it, and the nine units of water always settle 1,2,3,2,1: never deep, so
// the player can wade through to the exit.
func TestCrawlerWarrenFloodDrownsTheCrawlerWhateverTheTiming(t *testing.T) {
	turn := eventTick(t, sim.EventValveTurned)
	const period = 2 * 4 * sim.CrawlerIntervalTicks // five corridor cells: there and back
	for k := range uint64(period) {
		s := warrenAt(t, turn, turn+k)
		ev := s.Step(sim.Input{Interact: true})
		if countKind(ev, sim.EventValveTurned) != 1 {
			t.Fatalf("k=%d: valve not turned: %v", k, ev)
		}
		for range 200 {
			ev = append(ev, s.Step(sim.Input{})...)
		}
		if countKind(ev, sim.EventCrawlerDrowned) != 1 {
			t.Fatalf("k=%d: %d drownings:\n%s", k, countKind(ev, sim.EventCrawlerDrowned), s.ASCII())
		}
		var levels []uint8
		for _, p := range warrenCorridor {
			levels = append(levels, s.Cell(p).Water)
		}
		if !slices.Equal(levels, []uint8{1, 2, 3, 2, 1}) {
			t.Fatalf("k=%d: corridor water %v, want [1 2 3 2 1]", k, levels)
		}
	}
}

// A crawler catches the player and ends the cave, reproducibly.
func TestCaughtFixtureEndsInDeath(t *testing.T) {
	r := loadFixture(t, "crawler_warren_caught.json")
	var caught []sim.Event
	res, err := RunObserved(r, freshCave(t, r.CaveID), func(_ uint64, events []sim.Event, _ *sim.Sim) {
		for _, e := range events {
			if e.Kind == sim.EventPlayerCaught {
				caught = append(caught, e)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Check(res); err != nil {
		t.Fatal(err)
	}
	if res.Status != sim.StatusDead || len(caught) != 1 || res.FinalTick != caught[0].Tick+1 {
		t.Fatalf("status %s, caught %v, final tick %d", res.Status, caught, res.FinalTick)
	}
	if res.Sim.Player() != caught[0].Pos || res.Sim.Cell(caught[0].From).Occupant != sim.OccupantCrawler {
		t.Fatalf("player at %s, caught at %s by the crawler at %s:\n%s", res.Sim.Player(), caught[0].Pos, caught[0].From, res.Sim.ASCII())
	}
}
