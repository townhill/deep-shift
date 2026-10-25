package editor

import (
	"fmt"
	"strconv"

	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/sim"
)

// View is everything the renderer needs to draw the editor. Its slices
// belong to the editor: the renderer must not change them, and they are
// valid until the next Update.
type View struct {
	Layout           Layout
	Width, Height    int        // cave size in cells
	Cells            []sim.Cell // the cave's initial cells, row-major
	Unknown          []bool     // cells whose symbol is not a valid tile; nil when there are none
	CameraX, CameraY float64    // top-left of the cave view, in tiles
	Grid             bool       // draw cell boundaries
	Hover            sim.Pos    // the cell under the mouse, when HoverOK
	HoverOK          bool
	Ghost            sim.Cell // the brush, drawn faintly at Hover, when GhostOK
	GhostOK          bool
	Box              Box // the box a Shift-drag will fill, when BoxOK
	BoxOK            bool
	Problems         []sim.Pos // cells named by validation problems
	Mechanisms       []sim.Mechanism
	Palette          []Slot
	Status, Hint     string   // the top bar's two lines
	Info             []string // the bottom bar's lines, right of the palette
	Settings         []Line   // the cave settings panel; nil when it is closed
}

// Box is an inclusive rectangle of cells.
type Box struct{ X0, Y0, X1, Y1 int }

func boxOf(a, b sim.Pos) Box {
	return Box{min(a.X, b.X), min(a.Y, b.Y), max(a.X, b.X), max(a.Y, b.Y)}
}

// Slot is one palette entry as drawn: its current variant and its key.
type Slot struct {
	Cell     sim.Cell
	Key      string
	Selected bool
}

// Line is one line of the settings panel.
type Line struct {
	Text     string
	Selected bool
}

// statusTitleLen is how much of the title the status line shows.
const statusTitleLen = 24

// View returns the current view model.
func (e *Editor) View() View {
	v := View{
		Layout: e.layout, Width: e.doc.width(), Height: e.doc.height(),
		Cells: e.cells, Unknown: e.unknown, CameraX: e.camX, CameraY: e.camY, Grid: e.grid,
		Hover: e.hover, HoverOK: e.hoverOK && !e.settings.open,
		Mechanisms: e.doc.mechanisms,
	}
	if s := e.stroke; s != nil && s.box {
		v.Box, v.BoxOK = boxOf(s.anchor, e.clamp(s.last)), true
	}
	if v.HoverOK && !v.BoxOK {
		sym := e.symbol()
		if s := e.stroke; s != nil {
			sym = s.symbol
		}
		v.Ghost, v.GhostOK = sim.CellForSymbol(sym)
	}
	for _, p := range e.problems {
		v.Problems = append(v.Problems, p.At...)
	}
	for i, b := range Palette {
		c, _ := sim.CellForSymbol(b.Variants[e.variant[i]])
		v.Palette = append(v.Palette, Slot{Cell: c, Key: strconv.Itoa(i + 1), Selected: i == e.brush})
	}
	v.Status, v.Hint, v.Info = e.status(), e.hint(), e.info()
	if e.settings.open {
		v.Settings = e.settingsLines()
	}
	return v
}

func (e *Editor) status() string {
	title := []rune(e.doc.title)
	if len(title) > statusTitleLen {
		title = append(title[:statusTitleLen-3], []rune("...")...)
	}
	state := "OK"
	if n := len(e.problems); n > 0 {
		state = count(n, "problem")
	}
	if e.Dirty() {
		state += ", unsaved"
	}
	return fmt.Sprintf("EDIT %s  %dx%d  Quota %d of %d  %s",
		string(title), e.doc.width(), e.doc.height(), e.doc.quota, e.crystals(), state)
}

func (e *Editor) crystals() int {
	n := 0
	for _, c := range e.cells {
		if c.Occupant == sim.OccupantCrystal {
			n++
		}
	}
	return n
}

func (e *Editor) hint() string {
	if e.noticeLeft > 0 {
		return e.notice
	}
	if e.settings.open {
		return "Cave settings"
	}
	return fmt.Sprintf("Brush %d: %s   P: playtest   Tab: settings   Cmd+S: save", e.brush+1, symbolName(e.symbol()))
}

// info is the bottom bar's text: the cell under the mouse or the file, then
// the first problems, or help when there are none.
func (e *Editor) info() []string {
	first := "File: " + e.path
	if e.path == "" {
		first = "File: not saved yet (Cmd+S saves it as " + e.sourceName() + ")"
	}
	if e.hoverOK {
		first = fmt.Sprintf("%s %s", e.hover, symbolName(e.doc.at(e.hover)))
	}
	lines := []string{first}
	switch n := len(e.problems); {
	case n == 0:
		lines = append(lines,
			"No problems: P plays the cave. Left: paint, Right: erase, Shift-drag: box, Alt-click: pick",
			"Arrows/wheel: scroll  Cmd+Z: undo  Cmd+Shift+Z: redo  G: grid  Cmd(+Shift)+arrows: resize")
	default:
		describe := func(p level.Problem) string { return p.Field + ": " + p.Msg }
		lines = append(lines, describe(e.problems[0]))
		if n == 2 {
			lines = append(lines, describe(e.problems[1]))
		} else if n > 2 {
			lines = append(lines, fmt.Sprintf("...and %d more problems; the cells concerned are outlined in red", n-1))
		}
	}
	return lines
}

func (e *Editor) settingsLines() []Line {
	s := e.settings
	value := func(f field) string {
		if s.editing && s.cursor == f {
			return string(s.text) + "_"
		}
		return e.fieldValue(f)
	}
	lines := []Line{
		{Text: "CAVE SETTINGS"},
		{Text: "Title  " + value(fieldTitle), Selected: s.cursor == fieldTitle},
		{Text: "Id     " + value(fieldID), Selected: s.cursor == fieldID},
		{Text: "Seed   " + value(fieldSeed), Selected: s.cursor == fieldSeed},
		{Text: fmt.Sprintf("Quota  %s of %d crystals", value(fieldQuota), e.crystals()), Selected: s.cursor == fieldQuota},
		{Text: fmt.Sprintf("Size   %d x %d", e.doc.width(), e.doc.height())},
		{Text: fmt.Sprintf("Format version %d", e.doc.formatVersion)},
		{},
	}
	if s.editing {
		return append(lines, Line{Text: "Type, then Enter: keep   Esc: cancel"})
	}
	return append(lines,
		Line{Text: "Up/Down: choose   Enter: change   Tab/Esc: close"},
		Line{Text: "The id is also the file name of a new cave"})
}
