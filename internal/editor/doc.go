// Package editor is the cave editor: the cave being edited, brushes and
// tools, undo history, validation, saving and the view the renderer draws.
//
// Like internal/game it is pure Go with no Ebitengine import, so all of it
// is tested without a display. cmd/deepshift turns mouse and keyboard state
// into Input, internal/game.Shell runs the Editor and its playtests, and
// internal/render draws its View. Caves are checked by the same
// level.Parse the game uses, so the editor and the game cannot disagree
// about what a valid cave is.
package editor

import (
	"fmt"
	"maps"
	"slices"

	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/sim"
)

// doc is the cave being edited: a cave file's content with the rows as a
// grid of symbols. It may be invalid at any time; level.Parse of its
// encoding reports the problems.
type doc struct {
	formatVersion int
	id, title     string
	seed          uint64
	quota         int
	grid          [][]rune // rows of equal length; 1..sim.MaxDimension each way
	mechanisms    []sim.Mechanism
	metadata      map[string]string
}

// docFromDraft opens a draft for editing. It needs rows of equal length
// within the size limits; everything else may be wrong and is reported by
// validation instead. A missing format version becomes the current one.
func docFromDraft(d level.Draft) (doc, error) {
	switch {
	case d.FormatVersion > level.FormatVersion:
		return doc{}, fmt.Errorf("format_version %d is newer than this build reads (%d)", d.FormatVersion, level.FormatVersion)
	case len(d.Rows) == 0 || len(d.Rows) > sim.MaxDimension:
		return doc{}, fmt.Errorf("the editor needs 1 to %d rows, not %d", sim.MaxDimension, len(d.Rows))
	}
	grid := make([][]rune, len(d.Rows))
	for y, row := range d.Rows {
		grid[y] = []rune(row)
		if len(grid[y]) != len(grid[0]) {
			return doc{}, fmt.Errorf("rows[%d] has %d symbols but rows[0] has %d; the editor needs rows of equal length",
				y, len(grid[y]), len(grid[0]))
		}
	}
	if w := len(grid[0]); w == 0 || w > sim.MaxDimension {
		return doc{}, fmt.Errorf("the editor needs rows of 1 to %d symbols, not %d", sim.MaxDimension, w)
	}
	version := d.FormatVersion
	if version < level.MinFormatVersion {
		version = level.FormatVersion
	}
	c := d.Clone()
	return doc{
		formatVersion: version,
		id:            c.ID,
		title:         c.Title,
		seed:          c.Seed,
		quota:         c.RequiredCrystals,
		grid:          grid,
		mechanisms:    c.Mechanisms,
		metadata:      c.Metadata,
	}, nil
}

// draft returns the doc as a cave file's content. Width and height are the
// grid's.
func (d *doc) draft() level.Draft {
	rows := make([]string, len(d.grid))
	for y, row := range d.grid {
		rows[y] = string(row)
	}
	return level.Draft{
		FormatVersion:    d.formatVersion,
		ID:               d.id,
		Title:            d.title,
		Width:            d.width(),
		Height:           d.height(),
		Seed:             d.seed,
		RequiredCrystals: d.quota,
		Rows:             rows,
		Mechanisms:       d.mechanisms,
		Metadata:         d.metadata,
	}
}

// clone returns a deep copy, for the undo history.
func (d *doc) clone() doc {
	c := *d
	c.grid = make([][]rune, len(d.grid))
	for y, row := range d.grid {
		c.grid[y] = slices.Clone(row)
	}
	c.mechanisms = make([]sim.Mechanism, len(d.mechanisms))
	for i, m := range d.mechanisms {
		c.mechanisms[i] = sim.Mechanism{Valve: m.Valve, Gates: slices.Clone(m.Gates)}
	}
	c.metadata = maps.Clone(d.metadata)
	return c
}

func (d *doc) width() int  { return len(d.grid[0]) }
func (d *doc) height() int { return len(d.grid) }

func (d *doc) inBounds(p sim.Pos) bool {
	return p.X >= 0 && p.Y >= 0 && p.X < d.width() && p.Y < d.height()
}

func (d *doc) at(p sim.Pos) rune { return d.grid[p.Y][p.X] }

// paint puts symbol r at p and reports whether anything changed. A cave has
// exactly one player start and one exit, so painting either moves it: the
// one already placed becomes empty floor. A symbol the file's format version
// cannot hold raises the version to the first that can.
func (d *doc) paint(p sim.Pos, r rune) bool {
	if !d.inBounds(p) || d.at(p) == r {
		return false
	}
	if single(r) {
		for y, row := range d.grid {
			for x, s := range row {
				if s == r {
					d.put(sim.Pos{X: x, Y: y}, sim.SymbolEmpty)
				}
			}
		}
	}
	d.put(p, r)
	d.formatVersion = max(d.formatVersion, level.SymbolVersion(r))
	return true
}

// single reports whether a cave holds exactly one of symbol r.
func single(r rune) bool { return r == sim.SymbolPlayer || r == sim.SymbolExit }

// put sets one cell. A valve or gate it replaces leaves its mechanism: a
// replaced valve takes its mechanism with it, and its gates are then
// reported as unlisted until they are linked or removed.
func (d *doc) put(p sim.Pos, r rune) {
	if mechanical(d.at(p)) {
		d.unlink(p)
	}
	d.grid[p.Y][p.X] = r
}

// mechanical reports whether r is a valve or a gate.
func mechanical(r rune) bool {
	c, ok := sim.CellForSymbol(r)
	return ok && (c.Terrain.Valve() || c.Terrain.Gate())
}

func (d *doc) unlink(p sim.Pos) {
	d.mechanisms = slices.DeleteFunc(d.mechanisms, func(m sim.Mechanism) bool { return m.Valve == p })
	for i := range d.mechanisms {
		d.mechanisms[i].Gates = slices.DeleteFunc(d.mechanisms[i].Gates, func(g sim.Pos) bool { return g == p })
	}
}

// resize grows or shrinks the cave by one column or row on the given side
// and reports whether it changed. New cells are wall, as solid rock is the
// neutral fill at a cave's edge. Mechanism positions move with the cells;
// valves and gates on a removed edge leave their mechanisms.
func (d *doc) resize(side sim.Dir, grow bool) bool {
	horizontal := side == sim.DirLeft || side == sim.DirRight
	size := d.height()
	if horizontal {
		size = d.width()
	}
	if grow && size >= sim.MaxDimension || !grow && size <= 1 {
		return false
	}
	if !grow {
		for _, p := range d.edge(side) {
			if mechanical(d.at(p)) {
				d.unlink(p)
			}
		}
	}
	switch {
	case side == sim.DirLeft && grow:
		for y := range d.grid {
			d.grid[y] = slices.Insert(d.grid[y], 0, sim.SymbolWall)
		}
		d.shift(1, 0)
	case side == sim.DirLeft:
		for y := range d.grid {
			d.grid[y] = slices.Delete(d.grid[y], 0, 1)
		}
		d.shift(-1, 0)
	case side == sim.DirRight && grow:
		for y := range d.grid {
			d.grid[y] = append(d.grid[y], sim.SymbolWall)
		}
	case side == sim.DirRight:
		for y := range d.grid {
			d.grid[y] = d.grid[y][:len(d.grid[y])-1]
		}
	case side == sim.DirUp && grow:
		d.grid = slices.Insert(d.grid, 0, wallRow(d.width()))
		d.shift(0, 1)
	case side == sim.DirUp:
		d.grid = slices.Delete(d.grid, 0, 1)
		d.shift(0, -1)
	case side == sim.DirDown && grow:
		d.grid = append(d.grid, wallRow(d.width()))
	case side == sim.DirDown:
		d.grid = d.grid[:len(d.grid)-1]
	default:
		return false
	}
	return true
}

// edge lists the cells along one side of the cave.
func (d *doc) edge(side sim.Dir) []sim.Pos {
	var ps []sim.Pos
	switch side {
	case sim.DirLeft, sim.DirRight:
		x := 0
		if side == sim.DirRight {
			x = d.width() - 1
		}
		for y := range d.height() {
			ps = append(ps, sim.Pos{X: x, Y: y})
		}
	case sim.DirUp, sim.DirDown:
		y := 0
		if side == sim.DirDown {
			y = d.height() - 1
		}
		for x := range d.width() {
			ps = append(ps, sim.Pos{X: x, Y: y})
		}
	}
	return ps
}

func wallRow(w int) []rune {
	row := make([]rune, w)
	for x := range row {
		row[x] = sim.SymbolWall
	}
	return row
}

// shift moves every mechanism position by (dx, dy), after a column or row
// was added or removed at the left or top.
func (d *doc) shift(dx, dy int) {
	move := func(p sim.Pos) sim.Pos { return sim.Pos{X: p.X + dx, Y: p.Y + dy} }
	for i, m := range d.mechanisms {
		d.mechanisms[i].Valve = move(m.Valve)
		for j, g := range m.Gates {
			d.mechanisms[i].Gates[j] = move(g)
		}
	}
}

// placeValve paints a shut valve at p with a mechanism of its own. It has no
// gates yet, which validation reports until one is linked. An existing valve
// is kept, but given a mechanism if it had none. It reports whether anything
// changed.
func (d *doc) placeValve(p sim.Pos) bool {
	if !d.inBounds(p) {
		return false
	}
	if c, _ := sim.CellForSymbol(d.at(p)); c.Terrain.Valve() {
		if d.mechanismOf(p) >= 0 {
			return false
		}
		d.mechanisms = append(d.mechanisms, sim.Mechanism{Valve: p})
		return true
	}
	d.put(p, sim.SymbolValveShut)
	d.mechanisms = append(d.mechanisms, sim.Mechanism{Valve: p})
	d.formatVersion = max(d.formatVersion, level.SymbolVersion(sim.SymbolValveShut))
	return true
}

// placeGate paints a gate at p linked into valve v's mechanism, open or
// closed to match the valve. A gate linked elsewhere moves to v. With no
// mechanism for v (no valve selected, or it was removed) a new gate is left
// unlinked, which validation reports, and an existing gate is kept as it is.
// It reports whether anything changed.
func (d *doc) placeGate(p, v sim.Pos) bool {
	if !d.inBounds(p) {
		return false
	}
	m := d.mechanismOf(v)
	want := sim.SymbolGateClosed
	if m >= 0 && d.at(v) == sim.SymbolValveOpen {
		want = sim.SymbolGateOpen
	}
	cell, _ := sim.CellForSymbol(d.at(p))
	switch {
	case m < 0 && cell.Terrain.Gate():
		return false
	case m >= 0 && d.at(p) == want && slices.Contains(d.mechanisms[m].Gates, p):
		return false
	}
	d.put(p, want) // unlinks p from wherever it was
	if m = d.mechanismOf(v); m >= 0 {
		d.mechanisms[m].Gates = append(d.mechanisms[m].Gates, p)
	}
	d.formatVersion = max(d.formatVersion, level.SymbolVersion(want))
	return true
}

// mechanismOf returns the index of the mechanism whose valve is at v, or -1.
func (d *doc) mechanismOf(v sim.Pos) int {
	return slices.IndexFunc(d.mechanisms, func(m sim.Mechanism) bool { return m.Valve == v })
}

// owner returns the index of the mechanism p is the valve or a gate of, or
// -1.
func (d *doc) owner(p sim.Pos) int {
	return slices.IndexFunc(d.mechanisms, func(m sim.Mechanism) bool { return m.Valve == p || slices.Contains(m.Gates, p) })
}

// toggle opens or closes the mechanism that p's valve or gate belongs to.
// The valve and all its gates change together, so they stay consistent: it
// sets how the cave starts, as a valve turned before the first tick would.
// It reports whether anything changed.
func (d *doc) toggle(p sim.Pos) bool {
	m := d.owner(p)
	if m < 0 {
		return false
	}
	mech := d.mechanisms[m]
	open := d.at(mech.Valve) == sim.SymbolValveShut
	flip := func(q sim.Pos, shut, opened rune) {
		if d.inBounds(q) && (d.at(q) == shut || d.at(q) == opened) {
			d.grid[q.Y][q.X] = shut
			if open {
				d.grid[q.Y][q.X] = opened
			}
		}
	}
	flip(mech.Valve, sim.SymbolValveShut, sim.SymbolValveOpen)
	for _, g := range mech.Gates {
		flip(g, sim.SymbolGateClosed, sim.SymbolGateOpen)
	}
	return true
}

// region returns the cells 4-connected to p that hold the same symbol as p,
// p included, for flood fill.
func (d *doc) region(p sim.Pos) []sim.Pos {
	if !d.inBounds(p) {
		return nil
	}
	want := d.at(p)
	seen := map[sim.Pos]bool{p: true}
	out := []sim.Pos{p}
	for i := 0; i < len(out); i++ {
		for _, dir := range [...]sim.Dir{sim.DirUp, sim.DirDown, sim.DirLeft, sim.DirRight} {
			q := out[i].Add(dir)
			if d.inBounds(q) && !seen[q] && d.at(q) == want {
				seen[q] = true
				out = append(out, q)
			}
		}
	}
	return out
}
