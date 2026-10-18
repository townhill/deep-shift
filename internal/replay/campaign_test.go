package replay

import (
	"testing"

	"github.com/townhill/deep-shift/assets"
	"github.com/townhill/deep-shift/internal/sim"
)

// SPEC §8: cave 01 must be completable without death. Nothing in it can
// hurt the player: no boulders, no water, and every crystal rests on wall,
// so no digging can make one fall.
func TestCave01CannotHurtThePlayer(t *testing.T) {
	l := freshCave(t, "cave_01")
	c := l.Cave()
	for i, cell := range c.Cells {
		p := sim.Pos{X: i % c.Width, Y: i / c.Width}
		switch {
		case cell.Occupant == sim.OccupantBoulder:
			t.Errorf("boulder at %s", p)
		case cell.Water > 0:
			t.Errorf("water at %s", p)
		case cell.Occupant == sim.OccupantCrystal:
			if below := p.Add(sim.DirDown); !c.InBounds(below) || c.Cells[c.Index(below)].Terrain != sim.TerrainWall {
				t.Errorf("crystal at %s does not rest on wall", p)
			}
		}
	}
}

// SPEC §8 lists the five starter caves; the campaign plays them in order.
func TestCampaignMatchesTheSpecTable(t *testing.T) {
	want := []struct{ id, title string }{
		{"cave_01", "First Descent"},
		{"cave_02", "Unstable Ground"},
		{"cave_03", "The Roll"},
		{"cave_04", "Reservoir"},
		{"cave_05", "The Sluice"},
	}
	if len(assets.Campaign) != len(want) {
		t.Fatalf("campaign has %d caves, want %d", len(assets.Campaign), len(want))
	}
	for i, w := range want {
		l := freshCave(t, assets.Campaign[i])
		if l.ID != w.id || l.Title != w.title {
			t.Errorf("campaign[%d] = %s %q, want %s %q", i, l.ID, l.Title, w.id, w.title)
		}
	}
}

// cave05Channel lists the flooded channel's cells in cave_05.
func cave05Channel() []sim.Pos {
	var ch []sim.Pos
	for x := 7; x <= 13; x++ {
		ch = append(ch, sim.Pos{X: x, Y: 9})
	}
	return ch
}

func deepCells(s *sim.Sim, cells []sim.Pos) int {
	n := 0
	for _, p := range cells {
		if s.Cell(p).Deep() {
			n++
		}
	}
	return n
}

// The exit side of cave_05 (x >= 12) is reachable only across the flooded
// channel: without the valve or the boulder, a flood fill that may dig any
// earth stops at deep water.
func TestCave05TheChannelIsTheOnlyWayAcross(t *testing.T) {
	l := freshCave(t, "cave_05")
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		t.Fatal(err)
	}
	start := s.ASCII()
	for range 400 {
		s.Step(sim.Input{})
	}
	if s.ASCII() != start {
		t.Fatalf("cave_05 is not at rest before the player acts:\n%s", s.ASCII())
	}
	seen := map[sim.Pos]bool{s.Player(): true}
	queue := []sim.Pos{s.Player()}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if p.X >= 12 {
			t.Fatalf("reached %s on the exit side without crossing the channel", p)
		}
		for _, d := range []sim.Dir{sim.DirUp, sim.DirDown, sim.DirLeft, sim.DirRight} {
			n := p.Add(d)
			c := s.Cell(n)
			if seen[n] || c.Deep() || c.Occupant == sim.OccupantBoulder || c.Terrain == sim.TerrainWall ||
				c.Terrain == sim.TerrainExit || c.Terrain == sim.TerrainGateClosed || c.Terrain.Valve() {
				continue
			}
			seen[n] = true
			queue = append(queue, n)
		}
	}
}

// Water only flows to a neighbour two units lower, so a drain dug beside the
// channel empties only its near end: even with the whole left region dug
// out, the far end of the channel stays too deep to wade. Only the sluice in
// its floor drains it.
func TestCave05ChannelCannotBeDrainedFromTheSide(t *testing.T) {
	c := freshCave(t, "cave_05").Cave()
	for y := 1; y <= 14; y++ {
		for x := 1; x <= 6; x++ {
			if cell := &c.Cells[c.Index(sim.Pos{X: x, Y: y})]; cell.Terrain == sim.TerrainEarth {
				cell.Terrain = sim.TerrainEmpty
			}
		}
	}
	s, err := sim.New(c, 1)
	if err != nil {
		t.Fatal(err)
	}
	for range 1000 {
		s.Step(sim.Input{})
	}
	far := cave05Channel()[2:]
	if got := deepCells(s, far); got != len(far) {
		t.Fatalf("only %d of the channel's %d far cells are still deep after a side drain:\n%s", got, len(far), s.ASCII())
	}
}

// SPEC §8 and §11 Phase 4: cave 05 has at least two distinct solution routes,
// demonstrated by replays. Both cross the same flooded channel, differently:
// the sluice route drains it first and pushes nothing; the boulder route
// never turns the valve and crosses while the channel is still flooded,
// pushing the boulder through the water.
func TestCave05RoutesCrossTheChannelDifferently(t *testing.T) {
	exitCell := sim.Pos{X: 12, Y: 8}
	tests := []struct {
		fixture         string
		wantValve       bool
		wantPushes      int
		deepWhenCrossed func(int) bool
	}{
		{"cave_05_sluice_route.json", true, 0, func(n int) bool { return n == 0 }},
		{"cave_05_boulder_route.json", false, 6, func(n int) bool { return n >= 4 }},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			r := loadFixture(t, tt.fixture)
			valve, pushes, crossedDeep := false, 0, -1
			res, err := RunObserved(r, freshCave(t, r.CaveID), func(_ uint64, events []sim.Event, s *sim.Sim) {
				for _, e := range events {
					switch e.Kind {
					case sim.EventValveTurned:
						valve = true
					case sim.EventBoulderPushed:
						if e.Pos.Y == 9 && e.Pos.X >= 7 && e.Pos.X <= 13 {
							pushes++
						}
					}
				}
				if crossedDeep < 0 && s.Player() == exitCell {
					crossedDeep = deepCells(s, cave05Channel())
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Check(res); err != nil {
				t.Fatal(err)
			}
			if res.Status != sim.StatusComplete {
				t.Fatalf("status %s, want complete", res.Status)
			}
			if valve != tt.wantValve || pushes != tt.wantPushes {
				t.Errorf("valve turned %v (want %v), channel pushes %d (want %d)", valve, tt.wantValve, pushes, tt.wantPushes)
			}
			if crossedDeep < 0 || !tt.deepWhenCrossed(crossedDeep) {
				t.Errorf("%d deep channel cells when the player climbed out at %s", crossedDeep, exitCell)
			}
		})
	}
}
