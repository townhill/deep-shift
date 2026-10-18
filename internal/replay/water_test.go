package replay

import (
	"testing"

	"github.com/townhill/deep-shift/internal/sim"
)

// cave04Shaft lists the flooded shaft's cells in cave_04.
func cave04Shaft() []sim.Pos {
	var shaft []sim.Pos
	for y := 6; y <= 10; y++ {
		shaft = append(shaft, sim.Pos{X: 13, Y: y}, sim.Pos{X: 14, Y: y})
	}
	return shaft
}

// SPEC §11 Phase 3: cave 04 has an understandable use of the new mechanic.
// The lower chamber holds the exit, and the only way down is a shaft that
// stays too deep to enter until the valve opens the sluice gate below it.
func TestCave04TheFloodedShaftIsTheOnlyWayDown(t *testing.T) {
	l := freshCave(t, "cave_04")
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		t.Fatal(err)
	}
	start := s.ASCII()
	for range 400 {
		s.Step(sim.Input{})
	}
	if s.ASCII() != start {
		t.Fatalf("the reservoir is not at rest before the valve is turned:\n%s", s.ASCII())
	}
	for _, p := range cave04Shaft() {
		if !s.Cell(p).Deep() {
			t.Fatalf("shaft cell %s is not deep", p)
		}
	}
	// Flood-fill everything the player could dig or walk to without the
	// valve: deep water, walls, closed gates and valves stop it. Earth cannot
	// hold the shaft's water back from above, so digging changes nothing.
	seen := map[sim.Pos]bool{s.Player(): true}
	queue := []sim.Pos{s.Player()}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if p.Y >= 11 {
			t.Fatalf("reached %s below the wall band without the valve", p)
		}
		for _, d := range []sim.Dir{sim.DirUp, sim.DirDown, sim.DirLeft, sim.DirRight} {
			n := p.Add(d)
			c := s.Cell(n)
			if seen[n] || c.Deep() || c.Terrain == sim.TerrainWall || c.Terrain == sim.TerrainExit ||
				c.Terrain == sim.TerrainGateClosed || c.Terrain.Valve() {
				continue
			}
			seen[n] = true
			queue = append(queue, n)
		}
	}
	if s.Cell(sim.Pos{X: 26, Y: 14}).Terrain != sim.TerrainExit {
		t.Fatal("the exit is no longer in the lower chamber")
	}
}

// The completion fixture goes below only after turning the valve, and the
// water is conserved on every tick of the drain and the digging after it.
func TestCave04FixtureDrainsTheShaftBeforeGoingDown(t *testing.T) {
	r := loadFixture(t, "cave_04_complete.json")
	l := freshCave(t, r.CaveID)
	initial, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		t.Fatal(err)
	}
	total := initial.Water()
	turned := uint64(0)
	var tooDeep, below int
	res, err := RunObserved(r, l, func(tick uint64, events []sim.Event, s *sim.Sim) {
		for _, e := range events {
			switch e.Kind {
			case sim.EventValveTurned:
				if turned == 0 {
					turned = tick
					for _, p := range cave04Shaft() {
						if !s.Cell(p).Deep() {
							t.Errorf("tick %d: shaft cell %s was not deep when the valve turned", tick, p)
						}
					}
				}
			case sim.EventTooDeep:
				tooDeep++
			}
		}
		if s.Player().Y >= 12 {
			if turned == 0 {
				t.Fatalf("tick %d: the player is below at %s before the valve turned", tick, s.Player())
			}
			below++
		}
		if s.Water() != total {
			t.Fatalf("tick %d: total water %d, want %d", tick, s.Water(), total)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Check(res); err != nil {
		t.Fatal(err)
	}
	if turned == 0 || tooDeep == 0 || below == 0 || res.Status != sim.StatusComplete {
		t.Fatalf("valve turned at %d, too-deep bumps %d, ticks below %d, status %s", turned, tooDeep, below, res.Status)
	}
	if total != 44 {
		t.Fatalf("cave_04 holds %d units of water, want 44 (update the fixture description if this changes)", total)
	}
}
