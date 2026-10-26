package replay

import (
	"slices"
	"testing"

	"github.com/townhill/deep-shift/internal/sim"
)

const hollowComplete = "glimmer_hollow_complete.json"

// Places in glimmer_hollow.
var (
	hollowVault     = sim.Pos{X: 9, Y: 6} // the boxed glimmer, under the vault gate
	hollowVaultGate = sim.Pos{X: 9, Y: 5}
	hollowVaultRock = sim.Pos{X: 9, Y: 4} // the boulder resting on the gate
	hollowVaultLock = sim.Pos{X: 6, Y: 4}
	hollowSluice    = sim.Pos{X: 15, Y: 9}
	// hollowShards are the cells the vault glimmer's crystals form in, in
	// row-major order: beside the gate, its own cell, below the vault walls.
	hollowShards = []sim.Pos{{X: 8, Y: 5}, {X: 10, Y: 5}, {X: 9, Y: 6}, {X: 8, Y: 7}, {X: 10, Y: 7}}
	// hollowBasin is where the two crystals formed below the vault walls come
	// to rest.
	hollowBasin    = []sim.Pos{{X: 8, Y: 9}, {X: 10, Y: 9}}
	hollowPockets  = []sim.Pos{{X: 18, Y: 5}, {X: 28, Y: 5}, {X: 23, Y: 8}} // beside the ring, on its glimmer's left
	hollowCorridor = []sim.Pos{{X: 14, Y: 13}, {X: 15, Y: 13}, {X: 16, Y: 13}, {X: 17, Y: 13}, {X: 18, Y: 13}}
)

// valveTurnedAt matches the turning of the valve at p.
func valveTurnedAt(p sim.Pos) func(sim.Event) bool {
	return func(e sim.Event) bool { return e.Kind == sim.EventValveTurned && e.Pos == p }
}

// The completion route meets all three glimmers: it slips past the ring's,
// shatters the vault's with its boulder and collects the crystals it leaves,
// and drowns the corridor's, which leaves none.
func TestGlimmerHollowRouteShattersOneGlimmerAndDrownsAnother(t *testing.T) {
	r := loadFixture(t, hollowComplete)
	var shattered, drowned []sim.Event
	var formed, collected []sim.Pos
	res, err := RunObserved(r, freshCave(t, r.CaveID), func(_ uint64, events []sim.Event, _ *sim.Sim) {
		for _, e := range events {
			switch e.Kind {
			case sim.EventGlimmerShattered:
				shattered = append(shattered, e)
			case sim.EventGlimmerDrowned:
				drowned = append(drowned, e)
			case sim.EventCrystalFormed:
				formed = append(formed, e.Pos)
			case sim.EventCollected:
				collected = append(collected, e.Pos)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Check(res); err != nil {
		t.Fatal(err)
	}
	if len(shattered) != 1 || shattered[0].Pos != hollowVault || shattered[0].Object != sim.OccupantBoulder {
		t.Errorf("shattered %v, want the vault glimmer shattered by its boulder", shattered)
	}
	if !slices.Equal(formed, hollowShards) {
		t.Errorf("crystals formed at %v, want %v (and none from the drowning)", formed, hollowShards)
	}
	if len(drowned) != 1 || !slices.Contains(hollowCorridor, drowned[0].Pos) {
		t.Errorf("drowned %v, want one glimmer drowned in the corridor", drowned)
	}
	for _, p := range append([]sim.Pos{hollowShards[0], hollowShards[1]}, hollowBasin...) {
		if !slices.Contains(collected, p) {
			t.Errorf("the route never collected the vault crystal at %s (collected %v)", p, collected)
		}
	}
	// The fifth crystal stays sealed in the vault under the boulder.
	if res.Sim.Cell(hollowVault).Occupant != sim.OccupantCrystal || res.Sim.Cell(hollowVaultGate).Occupant != sim.OccupantBoulder {
		t.Errorf("the vault does not hold its sealed crystal under the boulder:\n%s", res.Sim.ASCII())
	}
	if res.Sim.Glimmers() != 1 {
		t.Errorf("%d glimmers left, want the ring's one:\n%s", res.Sim.Glimmers(), res.Sim.ASCII())
	}
}

// The ring glimmer keeps its pillar on its right: it goes round clockwise,
// one corridor cell per creature step, and never turns into the pockets on
// its left.
func TestGlimmerHollowRingGlimmerGoesRoundClockwise(t *testing.T) {
	l := freshCave(t, "glimmer_hollow")
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		t.Fatal(err)
	}
	var want []sim.Pos
	for x := 21; x <= 27; x++ {
		want = append(want, sim.Pos{X: x, Y: 3})
	}
	for y := 4; y <= 7; y++ {
		want = append(want, sim.Pos{X: 27, Y: y})
	}
	for x := 26; x >= 19; x-- {
		want = append(want, sim.Pos{X: x, Y: 7})
	}
	for y := 6; y >= 3; y-- {
		want = append(want, sim.Pos{X: 19, Y: y})
	}
	want = append(want, sim.Pos{X: 20, Y: 3})
	var got []sim.Pos
	for len(got) < len(want) {
		for _, e := range s.Step(sim.Input{}) {
			if e.Kind == sim.EventGlimmerMoved && e.From.X >= 19 && e.From.Y <= 7 {
				got = append(got, e.Pos)
			}
		}
		if s.Tick() > 1000 {
			t.Fatalf("the ring glimmer moved only %d times", len(got))
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ring glimmer went\n%v\nwant\n%v", got, want)
	}
	for _, p := range hollowPockets {
		if s.Cell(p).Occupant != sim.OccupantCrystal {
			t.Errorf("the pocket at %s lost its crystal", p)
		}
	}
}

// The vault glimmer cannot move: every cell beside it is closed to it. The
// cells diagonal to it are empty, dry floor, so a shattering fills them, and
// the boulder above the gate sits between walls, so it cannot be pushed off.
func TestGlimmerHollowVaultGlimmerIsBoxedIn(t *testing.T) {
	l := freshCave(t, "glimmer_hollow")
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		t.Fatal(err)
	}
	if s.Cell(hollowVault).Occupant != sim.OccupantGlimmer {
		t.Fatalf("no glimmer in the vault:\n%s", s.ASCII())
	}
	for _, d := range []sim.Dir{sim.DirUp, sim.DirDown, sim.DirLeft, sim.DirRight} {
		if c := s.Cell(hollowVault.Add(d)); c.OpenToCreature() {
			t.Errorf("the vault glimmer can move %s into %+v", d, c)
		}
	}
	for _, p := range []sim.Pos{{X: 8, Y: 5}, {X: 10, Y: 5}, {X: 8, Y: 7}, {X: 10, Y: 7}} {
		if c := s.Cell(p); !c.Empty() || c.Water > 0 {
			t.Errorf("diagonal cell %s holds %+v, want empty dry floor", p, c)
		}
	}
	if s.Cell(hollowVaultGate).Terrain != sim.TerrainGateClosed || s.Cell(hollowVaultRock).Occupant != sim.OccupantBoulder {
		t.Errorf("the boulder does not rest on the closed vault gate:\n%s", s.ASCII())
	}
	for _, d := range []sim.Dir{sim.DirLeft, sim.DirRight} {
		if c := s.Cell(hollowVaultRock.Add(d)); c.Terrain != sim.TerrainWall {
			t.Errorf("the boulder can be pushed %s: %+v", d, c)
		}
	}
}

// Opened at any moment, the vault drops its boulder onto the glimmer, which
// shatters into the same five crystals every time: gravity runs before the
// creatures in every tick, so the boulder fills the open gate before the
// glimmer could leave through it. Two crystals rest beside the gate, two
// fall into the basin and one stays sealed under the boulder.
func TestGlimmerHollowVaultShattersWhateverTheTiming(t *testing.T) {
	turn := fixtureEventTick(t, hollowComplete, valveTurnedAt(hollowVaultLock))
	for k := range uint64(2 * sim.CrawlerIntervalTicks) {
		s := fixtureAt(t, hollowComplete, turn, turn+k)
		ev := s.Step(sim.Input{Interact: true})
		if !slices.ContainsFunc(ev, valveTurnedAt(hollowVaultLock)) {
			t.Fatalf("k=%d: vault valve not turned: %v", k, ev)
		}
		for range 20 {
			ev = append(ev, s.Step(sim.Input{})...)
		}
		var formed []sim.Pos
		for _, e := range ev {
			if e.Kind == sim.EventCrystalFormed {
				formed = append(formed, e.Pos)
			}
		}
		if countKind(ev, sim.EventGlimmerShattered) != 1 || !slices.Equal(formed, hollowShards) || s.Status() != sim.StatusPlaying {
			t.Fatalf("k=%d: %d shattered, crystals formed at %v, status %s:\n%s",
				k, countKind(ev, sim.EventGlimmerShattered), formed, s.Status(), s.ASCII())
		}
		for _, p := range append([]sim.Pos{hollowShards[0], hollowShards[1], hollowVault}, hollowBasin...) {
			if c := s.Cell(p); c.Occupant != sim.OccupantCrystal || c.Falling {
				t.Fatalf("k=%d: %s holds %+v, want a resting crystal:\n%s", k, p, c, s.ASCII())
			}
		}
		if s.Cell(hollowVaultGate).Occupant != sim.OccupantBoulder {
			t.Fatalf("k=%d: the boulder is not in the vault gate:\n%s", k, s.ASCII())
		}
	}
}

// Opened at any moment of the corridor glimmer's patrol, the sluice drowns
// it, leaving no crystals, and the nine units of water always settle
// 1,2,3,2,1: never deep, so the player can wade through to the exit.
func TestGlimmerHollowFloodDrownsTheGlimmerWhateverTheTiming(t *testing.T) {
	turn := fixtureEventTick(t, hollowComplete, valveTurnedAt(hollowSluice))
	const period = 2 * 4 * sim.CrawlerIntervalTicks // five corridor cells: there and back
	for k := range uint64(period) {
		s := fixtureAt(t, hollowComplete, turn, turn+k)
		ev := s.Step(sim.Input{Interact: true})
		if !slices.ContainsFunc(ev, valveTurnedAt(hollowSluice)) {
			t.Fatalf("k=%d: sluice valve not turned: %v", k, ev)
		}
		for range 200 {
			ev = append(ev, s.Step(sim.Input{})...)
		}
		if countKind(ev, sim.EventGlimmerDrowned) != 1 || countKind(ev, sim.EventCrystalFormed) != 0 {
			t.Fatalf("k=%d: %d drownings, %d crystals formed:\n%s",
				k, countKind(ev, sim.EventGlimmerDrowned), countKind(ev, sim.EventCrystalFormed), s.ASCII())
		}
		var levels []uint8
		for _, p := range hollowCorridor {
			levels = append(levels, s.Cell(p).Water)
		}
		if !slices.Equal(levels, []uint8{1, 2, 3, 2, 1}) {
			t.Fatalf("k=%d: corridor water %v, want [1 2 3 2 1]", k, levels)
		}
	}
}

// The quota needs both lessons: the crystals placed in the cave fall short,
// and so do the ones outside the ring's pockets plus the four vault crystals
// the player can reach (the fifth stays sealed; see the route test).
func TestGlimmerHollowQuotaNeedsTheVaultAndTheRing(t *testing.T) {
	l := freshCave(t, "glimmer_hollow")
	c := l.Cave()
	placed := c.CountOccupant(sim.OccupantCrystal)
	if placed >= l.RequiredCrystals {
		t.Fatalf("%d placed crystals meet the quota of %d without the vault", placed, l.RequiredCrystals)
	}
	elsewhere := placed
	for _, p := range hollowPockets {
		if c.Cells[c.Index(p)].Occupant != sim.OccupantCrystal {
			t.Fatalf("no crystal in the pocket at %s", p)
		}
		elsewhere--
	}
	if reachable := len(hollowShards) - 1; elsewhere+reachable >= l.RequiredCrystals {
		t.Fatalf("%d crystals outside the pockets and %d from the vault meet the quota of %d without the ring",
			elsewhere, reachable, l.RequiredCrystals)
	}
}

// The corridor is the only way to the exit, so its glimmer must be dealt
// with: a flood fill that may dig any earth and pass any object, but stays
// out of the corridor, never reaches the exit.
func TestGlimmerHollowTheCorridorIsTheOnlyWayToTheExit(t *testing.T) {
	l := freshCave(t, "glimmer_hollow")
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[sim.Pos]bool{s.Player(): true}
	queue := []sim.Pos{s.Player()}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, d := range []sim.Dir{sim.DirUp, sim.DirDown, sim.DirLeft, sim.DirRight} {
			n := p.Add(d)
			c := s.Cell(n)
			if c.Terrain == sim.TerrainExit {
				t.Fatalf("reached the exit from %s without entering the corridor", p)
			}
			if seen[n] || slices.Contains(hollowCorridor, n) || c.Terrain == sim.TerrainWall {
				continue
			}
			seen[n] = true
			queue = append(queue, n)
		}
	}
	if !seen[sim.Pos{X: 14, Y: 12}] {
		t.Fatal("the flood fill never reached the way down into the corridor")
	}
}

// A glimmer catches the player and ends the cave, reproducibly.
func TestGlimmerCaughtFixtureEndsInDeath(t *testing.T) {
	r := loadFixture(t, "glimmer_hollow_caught.json")
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
	if res.Status != sim.StatusDead || len(caught) != 1 || caught[0].Object != sim.OccupantGlimmer || res.FinalTick != caught[0].Tick+1 {
		t.Fatalf("status %s, caught %v, final tick %d", res.Status, caught, res.FinalTick)
	}
	if res.Sim.Player() != caught[0].Pos || res.Sim.Cell(caught[0].From).Occupant != sim.OccupantGlimmer {
		t.Fatalf("player at %s, caught at %s by the glimmer at %s:\n%s", res.Sim.Player(), caught[0].Pos, caught[0].From, res.Sim.ASCII())
	}
}
