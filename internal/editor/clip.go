package editor

import (
	"slices"

	"github.com/townhill/deep-shift/internal/sim"
)

// Clip is a copied rectangle of a cave: its symbols, and the mechanisms
// whose valves it holds, with every position relative to its top-left
// corner. Gates whose valve was not copied are in Rows but in no mechanism.
type Clip struct {
	W, H       int
	Rows       [][]rune
	Mechanisms []sim.Mechanism
}

// copyBox copies the cells of b, which must lie inside the cave.
func (d *doc) copyBox(b Box) Clip {
	c := Clip{W: b.X1 - b.X0 + 1, H: b.Y1 - b.Y0 + 1}
	for y := b.Y0; y <= b.Y1; y++ {
		c.Rows = append(c.Rows, slices.Clone(d.grid[y][b.X0:b.X1+1]))
	}
	in := func(p sim.Pos) bool { return p.X >= b.X0 && p.X <= b.X1 && p.Y >= b.Y0 && p.Y <= b.Y1 }
	rel := func(p sim.Pos) sim.Pos { return sim.Pos{X: p.X - b.X0, Y: p.Y - b.Y0} }
	for _, m := range d.mechanisms {
		if !in(m.Valve) {
			continue
		}
		cm := sim.Mechanism{Valve: rel(m.Valve)}
		for _, g := range m.Gates {
			if in(g) {
				cm.Gates = append(cm.Gates, rel(g))
			}
		}
		c.Mechanisms = append(c.Mechanisms, cm)
	}
	return c
}

// clear paints every cell of b with floor and reports whether anything
// changed.
func (d *doc) clear(b Box) bool {
	changed := false
	for y := b.Y0; y <= b.Y1; y++ {
		for x := b.X0; x <= b.X1; x++ {
			changed = d.paint(sim.Pos{X: x, Y: y}, sim.SymbolEmpty) || changed
		}
	}
	return changed
}

// stamp pastes c with its top-left corner at at; cells outside the cave are
// left out. It reports whether anything changed.
//
//   - Plain symbols are painted as a brush would paint them, so a copied
//     player start or exit moves the cave's own.
//   - Each copied mechanism becomes a new one among the pasted cells: the
//     valve gets a mechanism of its own, its copied gates join it, and it
//     keeps the open or shut state it was copied with.
//   - A copied gate whose valve was not copied, or was left outside the
//     cave, joins the selected valve sel, as a painted gate does. A copied
//     valve that had no mechanism gets one, as a painted valve does.
func (d *doc) stamp(c Clip, at, sel sim.Pos) bool {
	abs := func(p sim.Pos) sim.Pos { return sim.Pos{X: at.X + p.X, Y: at.Y + p.Y} }
	changed := false
	for y, row := range c.Rows {
		for x, r := range row {
			if !mechanical(r) {
				changed = d.paint(abs(sim.Pos{X: x, Y: y}), r) || changed
			}
		}
	}
	linked := map[sim.Pos]bool{} // clip positions of valves, and of gates placed with their valve
	for _, m := range c.Mechanisms {
		v := abs(m.Valve)
		linked[m.Valve] = true
		if !d.inBounds(v) {
			continue
		}
		changed = d.placeValve(v) || changed
		for _, g := range m.Gates {
			linked[g] = true
			changed = d.placeGate(abs(g), v) || changed
		}
		if open := c.Rows[m.Valve.Y][m.Valve.X] == sim.SymbolValveOpen; open != (d.at(v) == sim.SymbolValveOpen) {
			changed = d.toggle(v) || changed
		}
	}
	for y, row := range c.Rows {
		for x, r := range row {
			p := sim.Pos{X: x, Y: y}
			cell, _ := sim.CellForSymbol(r)
			switch {
			case linked[p]:
			case cell.Terrain.Gate():
				changed = d.placeGate(abs(p), sel) || changed
			case cell.Terrain.Valve(): // copied from a cave where it had no mechanism
				changed = d.placeValve(abs(p)) || changed
			}
		}
	}
	return changed
}
