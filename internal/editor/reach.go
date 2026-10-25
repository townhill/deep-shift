package editor

import "github.com/townhill/deep-shift/internal/sim"

// reach is where the player can get from the start by walking and digging
// alone, with nothing else moving: a first look at a cave's layout, not a
// solver (D-053).
type reach struct {
	cells    []bool // reachable cells, row-major; nil when there is no player start
	crystals int    // crystals in reachable cells
	exit     bool   // the exit is reachable
}

// reachable searches from every player start in cells (a grid w wide).
//
//   - Entered: open floor and open gates, unless the water there is deep;
//     earth (dug); crystals (collected); crawlers (they move on).
//   - Blocked: walls, closed gates, valves, deep water, boulders and cells
//     with unknown symbols.
//   - The exit is reached but not passed: entering it ends the cave.
//
// Boulders falling or being pushed, valves being turned and water flowing
// are not considered, so a cave can be solvable when this says it is not,
// and the reverse.
func reachable(cells []sim.Cell, unknown []bool, w int) reach {
	var r reach
	var queue []int
	for i, c := range cells {
		if c.Occupant == sim.OccupantPlayer {
			if r.cells == nil {
				r.cells = make([]bool, len(cells))
			}
			r.cells[i] = true
			queue = append(queue, i)
		}
	}
	h := len(cells) / max(w, 1)
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		if cells[i].Occupant == sim.OccupantCrystal {
			r.crystals++
		}
		if cells[i].Terrain == sim.TerrainExit {
			r.exit = true
			continue
		}
		p := sim.Pos{X: i % w, Y: i / w}
		for _, d := range [...]sim.Dir{sim.DirUp, sim.DirDown, sim.DirLeft, sim.DirRight} {
			q := p.Add(d)
			if q.X < 0 || q.Y < 0 || q.X >= w || q.Y >= h {
				continue
			}
			j := q.Y*w + q.X
			if r.cells[j] || unknown != nil && unknown[j] || !enterable(cells[j]) {
				continue
			}
			r.cells[j] = true
			queue = append(queue, j)
		}
	}
	return r
}

// enterable reports whether the walking, digging player can enter c.
func enterable(c sim.Cell) bool {
	switch {
	case c.Terrain == sim.TerrainExit, c.Terrain == sim.TerrainEarth:
		return true
	case !c.Terrain.Floor(), c.Deep(), c.Occupant == sim.OccupantBoulder:
		return false
	}
	return true
}
