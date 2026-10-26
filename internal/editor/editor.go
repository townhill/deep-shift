package editor

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"unicode"

	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/sim"
)

// Request is what the editor asks of the shell after an update.
type Request int

// Requests.
const (
	RequestNone     Request = iota
	RequestLeave            // close the editor; unsaved changes, if any, were confirmed lost
	RequestPlaytest         // play the cave as it stands (Level returns it)
)

// Input is the platform-neutral editor input for one update. Mouse
// positions are logical screen pixels (render's layout, not the window's).
type Input struct {
	MouseX, MouseY        int
	Paint, Erase          bool    // left and right mouse buttons, held
	Shift, Alt, Cmd       bool    // held: Shift-drag fills a box, Alt-drag selects (Alt-click picks), Cmd-click flood-fills
	WheelX, WheelY        float64 // scrolled this update, in wheel steps
	Up, Down, Left, Right bool    // arrow keys or WASD held (without Cmd)

	// Pressed this update (edge-triggered).
	Brush        int     // 1..len(Palette) chooses that brush; 0 for none
	Undo, Redo   bool    // Cmd+Z, Cmd+Shift+Z
	Save         bool    // Cmd+S
	Playtest     bool    // P
	Grid         bool    // G
	Toggle       bool    // O: open or close the mechanism under the mouse
	Reach        bool    // R: the reachability overlay
	Copy, Cut    bool    // Cmd+C, Cmd+X: copy or cut the selection
	Paste        bool    // Cmd+V: start pasting the copy
	SelectAll    bool    // Cmd+A
	Settings     bool    // Tab: the cave settings panel
	Back         bool    // Esc
	Enter        bool    // confirms a settings field
	Backspace    bool    // deletes in a settings field, or clears the selection
	Grow, Shrink sim.Dir // Cmd+arrow, Cmd+Shift+arrow: add or remove a column or row on that side
	Chars        []rune  // text typed this update, for settings fields
}

// busy reports whether any button or key is held or pressed.
func (in Input) busy() bool {
	return in.Paint || in.Erase || in.Up || in.Down || in.Left || in.Right || in.pressed(Input{})
}

// pressed reports whether a key or button was pressed this update, given the
// previous update's input.
func (in Input) pressed(prev Input) bool {
	return in.Paint && !prev.Paint || in.Erase && !prev.Erase || in.Brush != 0 || in.Undo || in.Redo ||
		in.Save || in.Playtest || in.Grid || in.Toggle || in.Reach || in.Settings || in.Back || in.Enter || in.Backspace ||
		in.Copy || in.Cut || in.Paste || in.SelectAll ||
		in.Grow != sim.DirNone || in.Shrink != sim.DirNone || len(in.Chars) > 0
}

const (
	maxUndo       = 200 // undo steps kept
	noticeUpdates = 180 // how long a notice shows: 3 s at 60 updates per second
	panTiles      = 0.3 // view scroll per update while an arrow key is held (18 tiles/s)
)

// Editor edits one cave. Create it with New, feed it Input every update,
// and draw its View.
type Editor struct {
	layout  Layout
	storage Storage
	doc     doc
	path    string // where saves go; empty until a new cave or a copy is first saved
	saved   []byte // encoding when last opened or saved, for Dirty

	// Derived from doc by refresh after every change.
	stale    bool
	encoded  []byte
	level    *level.Level // the cave, when it is valid
	problems []level.Problem
	cells    []sim.Cell
	unknown  []bool
	reach    reach

	undo, redo []doc

	brush   int
	variant []int // the current variant of each brush

	camX, camY  float64 // top-left of the view, in tiles
	hover       sim.Pos
	hoverOK     bool // hover is a cave cell under the mouse
	stroke      *stroke
	grid        bool
	reachOn     bool    // show the reachability overlay
	linking     sim.Pos // the selected valve: gates painted now join its mechanism
	linkingOK   bool
	selection   Box // the selected cells, when selected
	selected    bool
	clip        Clip // the last copy, kept across caves by the shell
	clipOK      bool
	pasting     bool // clicks stamp the copy
	settings    settings
	confirm     confirm
	notice      string
	noticeLeft  int
	prev        Input
	waitRelease bool
}

// stroke is a mouse gesture in progress: painting along the pointer's path,
// or (with Shift) dragging out a box that is filled on release.
type stroke struct {
	erase    bool // right button
	symbol   rune
	box      bool
	anchor   sim.Pos // the box's first corner, in the cave
	last     sim.Pos // the pointer's cell at the previous update
	before   doc     // the cave before the stroke, for undo and cancel
	changed  bool
	unlinked bool // a gate was painted with no valve to join
	selects  bool // an Alt-drag selecting cells, not painting
}

// confirm is a question awaiting a second press of the same key.
type confirm int

const (
	confirmNone    confirm = iota
	confirmLeave           // Esc again leaves without saving
	confirmReplace         // Cmd+S again replaces an existing file
)

// New opens d for editing. path is the file d was read from, where saves
// go; it is empty for a new cave or a copy, which are saved in storage's
// folder as <id>.json. Input is ignored until every key and button has been
// released, so the key that opened the editor does nothing in it.
func New(d level.Draft, path string, layout Layout, storage Storage) (*Editor, error) {
	dc, err := docFromDraft(d)
	if err != nil {
		return nil, err
	}
	e := &Editor{
		layout: layout, storage: storage, doc: dc, path: path,
		variant: make([]int, len(Palette)), grid: true, waitRelease: true,
	}
	e.refresh()
	e.saved = e.encoded
	e.centreOn(e.playerStart())
	return e, nil
}

// NewCave returns the starting point for a new cave: one screen of earth
// inside a wall, with the player start at the top left, the exit at the
// bottom right and no quota.
func NewCave() level.Draft {
	const w, h = 30, 16
	rows := make([]string, h)
	for y := range h {
		row := make([]rune, w)
		for x := range w {
			row[x] = sim.SymbolEarth
			if x == 0 || y == 0 || x == w-1 || y == h-1 {
				row[x] = sim.SymbolWall
			}
		}
		switch y {
		case 1:
			row[1] = sim.SymbolPlayer
		case h - 2:
			row[w-2] = sim.SymbolExit
		}
		rows[y] = string(row)
	}
	return level.Draft{
		FormatVersion: level.FormatVersion, ID: "new_cave", Title: "New cave",
		Width: w, Height: h, Seed: 1, Rows: rows,
	}
}

// CopyOf returns a copy of l to edit as a new cave: "_copy" is added to its
// id and " (copy)" to its title, and its metadata is replaced by a note of
// where it came from. Layout, seed, quota and format version are kept.
func CopyOf(l *level.Level) level.Draft {
	d := l.Draft()
	const idSuffix, titleSuffix = "_copy", " (copy)"
	id := l.ID
	if len(id)+len(idSuffix) > level.MaxIDLen {
		id = id[:level.MaxIDLen-len(idSuffix)]
	}
	d.ID = id + idSuffix
	title := []rune(l.Title)
	if n := level.MaxTitleLen - len([]rune(titleSuffix)); len(title) > n {
		title = title[:n]
	}
	d.Title = string(title) + titleSuffix
	d.Metadata = map[string]string{"based_on": l.ID}
	return d
}

// Update advances the editor by one platform update.
func (e *Editor) Update(in Input) Request {
	prev := e.prev
	e.prev = in
	if e.noticeLeft > 0 {
		e.noticeLeft--
	}
	if e.waitRelease {
		if in.busy() {
			return RequestNone
		}
		e.waitRelease = false
	}
	var req Request
	if e.settings.open {
		e.updateSettings(in, prev)
	} else {
		req = e.updateEditing(in, prev)
	}
	if e.stale {
		e.refresh()
	}
	return req
}

func (e *Editor) updateEditing(in, prev Input) Request {
	e.scroll(in)
	inView := e.layout.InView(in.MouseX, in.MouseY)
	cell := e.layout.CellAt(in.MouseX, in.MouseY, e.camX, e.camY)
	e.hover, e.hoverOK = cell, inView && e.doc.inBounds(cell)

	if e.stroke != nil {
		e.continueStroke(in, inView, cell)
		return RequestNone
	}
	if in.pressed(prev) && (e.confirm == confirmLeave && !in.Back || e.confirm == confirmReplace && !in.Save) {
		e.confirm, e.noticeLeft = confirmNone, 0
	}
	if e.pasting {
		switch {
		case in.Back, in.Erase && !prev.Erase:
			e.pasting = false
			return RequestNone
		case in.Paint && !prev.Paint:
			if e.hoverOK { // where the preview shows
				e.stamp(cell)
			}
			return RequestNone
		case in.Brush != 0, in.Settings, in.Playtest, in.Copy, in.Cut, in.SelectAll:
			e.pasting = false
		}
	}
	switch {
	case in.Back && e.selected:
		e.selected = false
	case in.Back:
		return e.back()
	case in.Copy, in.Cut:
		e.copySelection(in.Cut)
	case in.Paste:
		e.startPaste()
	case in.SelectAll:
		e.selection, e.selected = Box{0, 0, e.doc.width() - 1, e.doc.height() - 1}, true
	case in.Backspace && e.selected:
		e.clearSelection()
	case in.Save:
		e.save()
	case in.Playtest:
		if e.level != nil {
			return RequestPlaytest
		}
		e.setNotice(fmt.Sprintf("Fix the %s below before playtesting", count(len(e.problems), "problem")))
	case in.Undo:
		e.step(&e.undo, &e.redo, "Nothing to undo")
	case in.Redo:
		e.step(&e.redo, &e.undo, "Nothing to redo")
	case in.Grid:
		e.grid = !e.grid
	case in.Reach:
		e.reachOn = !e.reachOn
	case in.Toggle:
		e.toggle()
	case in.Settings:
		e.settings = settings{open: true}
	case in.Brush > 0 && in.Brush <= len(Palette):
		e.choose(in.Brush - 1)
	case in.Grow != sim.DirNone:
		e.resize(in.Grow, true)
	case in.Shrink != sim.DirNone:
		e.resize(in.Shrink, false)
	}
	switch {
	case in.Paint && !prev.Paint:
		e.press(in, inView, cell, false)
	case in.Erase && !prev.Erase:
		e.press(in, inView, cell, true)
	}
	return RequestNone
}

// scroll moves the view with held arrow keys and the mouse wheel. The wheel
// moves the cave the way the fingers or wheel go, like any scrolled page.
func (e *Editor) scroll(in Input) {
	dx, dy := -in.WheelX, -in.WheelY
	if in.Left {
		dx -= panTiles
	}
	if in.Right {
		dx += panTiles
	}
	if in.Up {
		dy -= panTiles
	}
	if in.Down {
		dy += panTiles
	}
	e.camX += dx
	e.camY += dy
	e.clampCamera()
}

// clampCamera keeps the view on the cave. A cave narrower or shorter than
// the view is centred in it.
func (e *Editor) clampCamera() {
	vw, vh := e.layout.ViewTiles()
	e.camX = clampAxis(e.camX, vw, float64(e.doc.width()))
	e.camY = clampAxis(e.camY, vh, float64(e.doc.height()))
}

func clampAxis(pos, view, size float64) float64 {
	if size <= view {
		return (size - view) / 2
	}
	return min(max(pos, 0), size-view)
}

// centreOn puts cell p in the middle of the view, as far as the cave allows.
func (e *Editor) centreOn(p sim.Pos) {
	vw, vh := e.layout.ViewTiles()
	e.camX = math.Round(float64(p.X) + 0.5 - vw/2)
	e.camY = math.Round(float64(p.Y) + 0.5 - vh/2)
	e.clampCamera()
}

func (e *Editor) playerStart() sim.Pos {
	for y, row := range e.doc.grid {
		if x := slices.Index(row, sim.SymbolPlayer); x >= 0 {
			return sim.Pos{X: x, Y: y}
		}
	}
	return sim.Pos{}
}

// press starts a mouse gesture: choosing a palette brush, picking the brush
// of a cell (Alt), or a paint, erase or box stroke.
func (e *Editor) press(in Input, inView bool, cell sim.Pos, erase bool) {
	if !erase {
		if i, ok := e.layout.SlotAt(in.MouseX, in.MouseY); ok {
			e.choose(i)
			return
		}
	}
	if !inView {
		return
	}
	if in.Alt && !erase {
		e.stroke = &stroke{selects: true, anchor: e.clamp(cell), last: cell}
		return
	}
	s := &stroke{erase: erase, symbol: e.symbol(), box: in.Shift, anchor: e.clamp(cell), last: cell, before: e.doc.clone()}
	if erase {
		s.symbol = eraser
	}
	if in.Cmd {
		e.fill(s, cell)
		return
	}
	e.stroke = s
	if !s.box {
		e.paintLine(s, cell, cell)
	}
}

// fill flood-fills the region of cells around p that hold the same symbol as
// p with the stroke's symbol, as one undo step.
func (e *Editor) fill(s *stroke, p sim.Pos) {
	if !e.doc.inBounds(p) {
		return
	}
	if oneOnly(s.symbol) {
		e.paintCell(s, p)
	} else {
		for _, q := range e.doc.region(p) {
			e.paintCell(s, q)
		}
	}
	e.finish(s)
}

func (e *Editor) continueStroke(in Input, inView bool, cell sim.Pos) {
	s := e.stroke
	switch {
	case s.selects && in.Back:
		e.stroke = nil
		return
	case s.selects && !in.Paint:
		// A click picks the cell's brush; a drag selects the cells.
		e.stroke = nil
		if end := e.clamp(s.last); end == s.anchor {
			e.pick(s.anchor)
		} else {
			e.selection, e.selected = boxOf(s.anchor, end), true
		}
		return
	case in.Back: // cancel: put the cave back as it was
		e.doc, e.stroke, e.stale = s.before, nil, true
		return
	case s.erase && !in.Erase, !s.erase && !in.Paint:
		e.stroke = nil
		if s.box {
			e.fillBox(s, s.anchor, e.clamp(s.last))
		}
		e.finish(s)
		return
	case !inView:
		return
	}
	if !s.box && !s.selects {
		e.paintLine(s, s.last, cell)
	}
	s.last = cell
}

// paintLine paints every cell on the line from a to b, so a fast drag
// leaves no gaps. Cells outside the cave are skipped.
func (e *Editor) paintLine(s *stroke, a, b sim.Pos) {
	for _, p := range line(a, b) {
		e.paintCell(s, p)
	}
}

// paintCell paints the stroke's symbol at p, noting any change on s. A
// valve gets a mechanism of its own and becomes the selected valve; a gate
// joins the selected valve's mechanism.
func (e *Editor) paintCell(s *stroke, p sim.Pos) {
	var changed bool
	switch s.symbol {
	case sim.SymbolValveShut:
		changed = e.doc.placeValve(p)
		if e.doc.mechanismOf(p) >= 0 {
			e.linking, e.linkingOK = p, true
		}
	case sim.SymbolGateClosed:
		v := e.selectedValve()
		changed = e.doc.placeGate(p, v)
		if changed && e.doc.mechanismOf(v) < 0 {
			s.unlinked = true
		}
	default:
		changed = e.doc.paint(p, s.symbol)
	}
	if changed {
		s.changed, e.stale = true, true
	}
}

// selectedValve returns the valve gates are linked to, or a position no
// mechanism has when none is selected or it has since been removed.
func (e *Editor) selectedValve() sim.Pos {
	if e.linkingOK && e.doc.mechanismOf(e.linking) >= 0 {
		return e.linking
	}
	return sim.Pos{X: -1, Y: -1}
}

// finish records a finished stroke or fill for undo.
func (e *Editor) finish(s *stroke) {
	if s.changed {
		e.commit(s.before)
	}
	if s.unlinked {
		e.setNotice("A gate needs a valve: paint a valve first, or Alt-click one, then paint its gates")
	}
}

// fillBox fills the box from a to b. A box of player starts, exits or
// valves places one, at b, the corner where the drag ended (oneOnly).
func (e *Editor) fillBox(s *stroke, a, b sim.Pos) {
	if oneOnly(s.symbol) {
		e.paintCell(s, b)
		return
	}
	box := boxOf(a, b)
	for y := box.Y0; y <= box.Y1; y++ {
		for x := box.X0; x <= box.X1; x++ {
			e.paintCell(s, sim.Pos{X: x, Y: y})
		}
	}
}

// clamp returns the cave cell nearest to p.
func (e *Editor) clamp(p sim.Pos) sim.Pos {
	return sim.Pos{X: min(max(p.X, 0), e.doc.width()-1), Y: min(max(p.Y, 0), e.doc.height()-1)}
}

// line returns the cells of a Bresenham line from a to b, both included.
func line(a, b sim.Pos) []sim.Pos {
	dx, dy := abs(b.X-a.X), -abs(b.Y-a.Y)
	sx, sy := sign(b.X-a.X), sign(b.Y-a.Y)
	err := dx + dy
	var ps []sim.Pos
	for p := a; ; {
		ps = append(ps, p)
		if p == b {
			return ps
		}
		if e2 := 2 * err; e2 >= dy {
			err += dy
			p.X += sx
		} else {
			err += dx
			p.Y += sy
		}
	}
}

func abs(v int) int { return max(v, -v) }

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// commit records a finished change for undo.
func (e *Editor) commit(before doc) {
	e.undo = append(e.undo, before)
	if len(e.undo) > maxUndo {
		e.undo = slices.Delete(e.undo, 0, 1)
	}
	e.redo = nil
	e.stale = true
	if e.doc.formatVersion > before.formatVersion {
		e.setNotice(fmt.Sprintf("Format version raised to %d for the new symbols: replays of the old version will not match",
			e.doc.formatVersion))
	}
}

// step undoes (from undo to redo) or redoes (from redo to undo) one change.
func (e *Editor) step(from, to *[]doc, empty string) {
	if len(*from) == 0 {
		e.setNotice(empty)
		return
	}
	*to = append(*to, e.doc)
	e.doc = (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	e.stale = true
	e.clampCamera()
}

// choose selects palette brush i, or moves on to its next variant if it is
// already selected.
func (e *Editor) choose(i int) {
	if e.brush == i {
		e.variant[i] = (e.variant[i] + 1) % len(Palette[i].Variants)
	}
	e.brush = i
}

// pick selects the brush that paints the cell at p. On a valve or gate it
// selects that mechanism's valve, and the gate brush, so the next gates
// painted join it.
func (e *Editor) pick(p sim.Pos) {
	if !e.doc.inBounds(p) {
		return
	}
	r := e.doc.at(p)
	if mechanical(r) {
		m := e.doc.owner(p)
		if m < 0 {
			e.setNotice(fmt.Sprintf("This %s belongs to no valve: paint over it, or select a valve and paint it again", symbolName(r)))
			return
		}
		v := e.doc.mechanisms[m].Valve
		e.linking, e.linkingOK, e.brush = v, true, gateBrush
		e.setNotice(fmt.Sprintf("Valve %s selected: gates you paint now open and close with it", v))
		return
	}
	b, v, ok := brushFor(r)
	if !ok {
		e.setNotice(fmt.Sprintf("No brush paints %s yet", symbolName(r)))
		return
	}
	e.brush, e.variant[b] = b, v
}

// symbol is the current brush's symbol.
func (e *Editor) symbol() rune { return Palette[e.brush].Variants[e.variant[e.brush]] }

// copySelection copies the selected cells, and with cut clears them to
// floor.
func (e *Editor) copySelection(cut bool) {
	if !e.selected {
		e.setNotice("Select cells to copy first: Option-drag over them, or Cmd+A for the whole cave")
		return
	}
	e.clip, e.clipOK = e.doc.copyBox(e.selection), true
	size := fmt.Sprintf("%d x %d", e.clip.W, e.clip.H)
	if !cut {
		e.setNotice("Copied " + size + ": Cmd+V to paste")
		return
	}
	before := e.doc.clone()
	if e.doc.clear(e.selection) {
		e.commit(before)
	}
	e.setNotice("Cut " + size + ": Cmd+V to paste")
}

// clearSelection clears the selected cells to floor.
func (e *Editor) clearSelection() {
	before := e.doc.clone()
	if e.doc.clear(e.selection) {
		e.commit(before)
	}
}

// startPaste makes clicks stamp the copy, until Esc or a right-click.
func (e *Editor) startPaste() {
	if !e.clipOK {
		e.setNotice("Nothing copied yet: Option-drag to select, then Cmd+C")
		return
	}
	e.pasting = true // the hint says how to place it while pasting lasts
	e.noticeLeft = 0
}

// stamp pastes the copy with its top-left corner at p, as one undo step.
func (e *Editor) stamp(p sim.Pos) {
	before := e.doc.clone()
	if e.doc.stamp(e.clip, p, e.selectedValve()) {
		e.commit(before)
	}
}

// Clip returns the last copy, if any, so the shell can carry it to the next
// cave opened.
func (e *Editor) Clip() (Clip, bool) { return e.clip, e.clipOK }

// SetClip gives the editor a copy made in another cave, ready to paste.
func (e *Editor) SetClip(c Clip) { e.clip, e.clipOK = c, true }

// toggle opens or closes the mechanism of the valve or gate under the mouse,
// setting how the cave starts.
func (e *Editor) toggle() {
	if !e.hoverOK {
		return
	}
	before := e.doc.clone()
	if !e.doc.toggle(e.hover) {
		e.setNotice("O opens or closes a valve and its gates: point at one of them")
		return
	}
	e.commit(before)
	m := e.doc.mechanisms[e.doc.owner(e.hover)]
	state := "shut: its gates start closed"
	if e.doc.at(m.Valve) == sim.SymbolValveOpen {
		state = "open: its gates start open"
	}
	e.setNotice(fmt.Sprintf("Valve %s now starts %s", m.Valve, state))
}

func (e *Editor) resize(side sim.Dir, grow bool) {
	before := e.doc.clone()
	if !e.doc.resize(side, grow) {
		if grow {
			e.setNotice(fmt.Sprintf("A cave can be at most %d cells across and down", sim.MaxDimension))
		} else {
			e.setNotice("A cave needs at least one column and one row")
		}
		return
	}
	// Keep the view on the same cells when the cave grows or shrinks at the
	// left or top, and keep a right or bottom edge in view if it was.
	step := 1.0
	if !grow {
		step = -1
	}
	vw, vh := e.layout.ViewTiles()
	switch side {
	case sim.DirLeft:
		e.camX += step
	case sim.DirUp:
		e.camY += step
	case sim.DirRight:
		if e.camX+vw >= float64(before.width()) {
			e.camX += step
		}
	case sim.DirDown:
		if e.camY+vh >= float64(before.height()) {
			e.camY += step
		}
	}
	e.commit(before)
	e.clampCamera()
	e.setNotice(fmt.Sprintf("The cave is now %d x %d", e.doc.width(), e.doc.height()))
}

func (e *Editor) back() Request {
	if !e.Dirty() || e.confirm == confirmLeave {
		return RequestLeave
	}
	e.confirm = confirmLeave
	e.setNotice("Unsaved changes: Esc again to leave without saving, Cmd+S to save")
	return RequestNone
}

// save writes the cave to its file, or for a new cave or a copy to
// <storage folder>/<id>.json. It never replaces a file the editor did not
// open or save without a second Cmd+S. A cave with problems is saved too, so
// work in progress is never lost; the game will not play it until they are
// fixed.
func (e *Editor) save() {
	path := e.path
	if path == "" {
		switch {
		case !level.ValidID(e.doc.id):
			e.setNotice("Give the cave a valid id before saving (Tab: cave settings)")
			return
		case e.storage == nil || e.storage.Dir() == "":
			e.setNotice("There is no folder to save new caves in")
			return
		}
		path = filepath.Join(e.storage.Dir(), e.doc.id+".json")
	}
	replace := path == e.path || e.confirm == confirmReplace
	e.confirm = confirmNone
	err := e.storage.Write(path, e.encoded, replace)
	switch {
	case errors.Is(err, ErrExists):
		e.confirm = confirmReplace
		e.setNotice(fmt.Sprintf("%s already exists: Cmd+S again to replace it", filepath.Base(path)))
		return
	case err != nil:
		e.setNotice("Could not save: " + err.Error())
		return
	}
	e.path, e.saved = path, e.encoded
	msg := "Saved " + filepath.Base(path)
	if n := len(e.problems); n > 0 {
		msg += fmt.Sprintf(" with %s: the game cannot play it yet", count(n, "problem"))
	}
	e.setNotice(msg)
}

// refresh re-derives everything that depends on the cave: its encoding,
// validation through level.Parse (the game's own check), the cells the
// renderer draws, and where the player can reach.
func (e *Editor) refresh() {
	e.stale = false
	e.encoded = e.doc.draft().Encode()
	l, err := level.Parse(e.sourceName(), e.encoded)
	e.level, e.problems = l, nil
	var ve *level.ValidationError
	switch {
	case errors.As(err, &ve):
		e.problems = ve.Problems
	case err != nil:
		e.problems = []level.Problem{{Field: "file", Msg: err.Error()}}
	}
	w := e.doc.width()
	e.cells, e.unknown = e.cells[:0], nil
	for y, row := range e.doc.grid {
		for x, r := range row {
			c, ok := sim.CellForSymbol(r)
			if !ok {
				if e.unknown == nil {
					e.unknown = make([]bool, w*e.doc.height())
				}
				e.unknown[y*w+x] = true
			}
			e.cells = append(e.cells, c)
		}
	}
	e.reach = reachable(e.cells, e.unknown, w)
	e.fitSelection()
}

// fitSelection keeps the selection inside the cave after it shrinks, or
// drops it if nothing of it is left.
func (e *Editor) fitSelection() {
	b := &e.selection
	w, h := e.doc.width(), e.doc.height()
	if b.X0 >= w || b.Y0 >= h {
		e.selected = false
	}
	b.X1, b.Y1 = min(b.X1, w-1), min(b.Y1, h-1)
}

func (e *Editor) sourceName() string {
	if e.path != "" {
		return filepath.Base(e.path)
	}
	return e.doc.id + ".json"
}

func (e *Editor) setNotice(msg string) { e.notice, e.noticeLeft = msg, noticeUpdates }

// Notify shows a transient message, for messages from the shell.
func (e *Editor) Notify(msg string) { e.setNotice(msg) }

// Level returns the cave as the game would load it, or nil while it has
// problems.
func (e *Editor) Level() *level.Level { return e.level }

// Problems returns the cave's validation problems.
func (e *Editor) Problems() []level.Problem { return e.problems }

// Draft returns the cave being edited.
func (e *Editor) Draft() level.Draft { return e.doc.draft().Clone() }

// Path returns the file the cave is saved to; empty until a new cave or a
// copy is first saved.
func (e *Editor) Path() string { return e.path }

// Dirty reports whether the cave differs from the file it was opened from
// or last saved to.
func (e *Editor) Dirty() bool { return string(e.encoded) != string(e.saved) }

// TextEntry reports whether a settings field is being typed into, so the
// platform layer can keep letter shortcuts (F, M, B) out of the way.
func (e *Editor) TextEntry() bool { return e.settings.open && e.settings.editing }

// Resume is called when the shell returns to the editor (after a playtest):
// input is ignored until every key and button is released.
func (e *Editor) Resume() {
	e.waitRelease = true
	e.stroke = nil
}

func count(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// settings is the cave settings panel (Tab): the title, id, seed and quota,
// each edited as text.
type settings struct {
	open    bool
	cursor  field
	editing bool
	text    []rune
}

type field int

const (
	fieldTitle field = iota
	fieldID
	fieldSeed
	fieldQuota
	fieldCount
)

func (e *Editor) updateSettings(in, prev Input) {
	s := &e.settings
	if s.editing {
		for _, r := range in.Chars {
			e.typeRune(r)
		}
		switch {
		case in.Back:
			s.editing = false
		case in.Enter:
			e.commitField()
		case in.Backspace && len(s.text) > 0:
			s.text = s.text[:len(s.text)-1]
		}
		return
	}
	switch {
	case in.Back || in.Settings:
		s.open = false
	case in.Up && !prev.Up:
		s.cursor = (s.cursor + fieldCount - 1) % fieldCount
	case in.Down && !prev.Down:
		s.cursor = (s.cursor + 1) % fieldCount
	case in.Enter:
		s.editing, s.text = true, []rune(e.fieldValue(s.cursor))
	}
}

func (e *Editor) fieldValue(f field) string {
	switch f {
	case fieldTitle:
		return e.doc.title
	case fieldID:
		return e.doc.id
	case fieldSeed:
		return strconv.FormatUint(e.doc.seed, 10)
	case fieldQuota:
		return strconv.Itoa(e.doc.quota)
	}
	return ""
}

// typeRune adds a typed character to the field being edited, if the field
// accepts it. Ids take lowercase letters, digits and '_' (a space or '-'
// becomes '_'); the seed and quota take digits.
func (e *Editor) typeRune(r rune) {
	s := &e.settings
	switch s.cursor {
	case fieldTitle:
		if !unicode.IsPrint(r) || len(s.text) >= level.MaxTitleLen {
			return
		}
	case fieldID:
		r = unicode.ToLower(r)
		if r == ' ' || r == '-' {
			r = '_'
		}
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') || len(s.text) >= level.MaxIDLen {
			return
		}
	case fieldSeed, fieldQuota:
		limit := len(strconv.Itoa(level.MaxSeed))
		if s.cursor == fieldQuota {
			limit = 5
		}
		if r < '0' || r > '9' || len(s.text) >= limit {
			return
		}
	}
	s.text = append(s.text, r)
}

func (e *Editor) commitField() {
	s := &e.settings
	text := string(s.text)
	before := e.doc.clone()
	switch s.cursor {
	case fieldTitle:
		e.doc.title = text
	case fieldID:
		e.doc.id = text
	case fieldSeed:
		v, err := strconv.ParseUint(text, 10, 64)
		if err != nil || v > level.MaxSeed {
			e.setNotice(fmt.Sprintf("The seed must be a whole number from 0 to %d", uint64(level.MaxSeed)))
			return
		}
		e.doc.seed = v
	case fieldQuota:
		v, err := strconv.Atoi(text)
		if err != nil {
			e.setNotice("The quota must be a whole number of crystals")
			return
		}
		e.doc.quota = v
	}
	s.editing = false
	if e.doc.title != before.title || e.doc.id != before.id || e.doc.seed != before.seed || e.doc.quota != before.quota {
		e.commit(before)
	}
}
