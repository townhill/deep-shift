package replay

import (
	"testing"

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
