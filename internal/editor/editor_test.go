package editor

import (
	"bytes"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/townhill/deep-shift/assets"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/sim"
)

// testLayout is the app's layout: a 960x640 logical screen of 32-pixel
// tiles, with 64-pixel bars above and below a 30x16-tile view.
var testLayout = Layout{Tile: 32, Width: 960, Height: 640, Top: 64, Bottom: 64}

// memStorage keeps cave files in memory.
type memStorage struct {
	dir   string
	files map[string][]byte
	fail  error
}

func newMemStorage() *memStorage { return &memStorage{dir: "/caves", files: map[string][]byte{}} }

func (m *memStorage) Dir() string { return m.dir }

func (m *memStorage) Write(path string, data []byte, replace bool) error {
	if m.fail != nil {
		return m.fail
	}
	if _, ok := m.files[path]; ok && !replace {
		return ErrExists
	}
	m.files[path] = slices.Clone(data)
	return nil
}

func (m *memStorage) List() ([]File, error) {
	var files []File
	for _, path := range slices.Sorted(maps.Keys(m.files)) {
		d, err := level.DecodeDraft(path, m.files[path])
		files = append(files, File{Path: path, Draft: d, Err: err})
	}
	return files, nil
}

func open(t *testing.T, d level.Draft, path string) (*Editor, *memStorage) {
	t.Helper()
	st := newMemStorage()
	e, err := New(d, path, testLayout, st)
	if err != nil {
		t.Fatal(err)
	}
	e.Update(Input{}) // nothing held: the editor starts listening
	return e, st
}

func embedded(t *testing.T, id string) *level.Level {
	t.Helper()
	levels, err := level.LoadDir(assets.Levels, assets.LevelDir)
	if err != nil {
		t.Fatal(err)
	}
	l, ok := level.ByID(levels, id)
	if !ok {
		t.Fatalf("no embedded cave %q", id)
	}
	return l
}

func pos(x, y int) sim.Pos { return sim.Pos{X: x, Y: y} }

// over returns in with the mouse over the centre of cell p.
func over(e *Editor, p sim.Pos, in Input) Input {
	in.MouseX = int((float64(p.X) - e.camX + 0.5) * float64(e.layout.Tile))
	in.MouseY = e.layout.Top + int((float64(p.Y)-e.camY+0.5)*float64(e.layout.Tile))
	return in
}

// drag presses the left (or right) button over the first cell, moves over
// the rest one update each, and releases over the last. mods carries Shift
// or Alt.
func drag(e *Editor, erase bool, mods Input, cells ...sim.Pos) {
	held := mods
	if erase {
		held.Erase = true
	} else {
		held.Paint = true
	}
	for _, p := range cells {
		e.Update(over(e, p, held))
	}
	e.Update(over(e, cells[len(cells)-1], mods))
}

func click(e *Editor, p sim.Pos) { drag(e, false, Input{}, p) }

// tap presses keys for one update and releases them for one.
func tap(e *Editor, in Input) Request {
	r := e.Update(in)
	e.Update(Input{})
	return r
}

func occurrences(e *Editor, r rune) int {
	n := 0
	for _, row := range e.doc.grid {
		for _, s := range row {
			if s == r {
				n++
			}
		}
	}
	return n
}

func hasProblem(e *Editor, text string) bool {
	return slices.ContainsFunc(e.Problems(), func(p level.Problem) bool { return strings.Contains(p.Msg, text) })
}

func TestNewCaveIsValidAndClean(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	if len(e.Problems()) != 0 || e.Level() == nil || e.Dirty() {
		t.Fatalf("problems %v, level %v, dirty %v", e.Problems(), e.Level(), e.Dirty())
	}
	if e.Level().Width != 30 || e.Level().Height != 16 || e.Level().FormatVersion != level.FormatVersion {
		t.Fatalf("new cave %dx%d v%d", e.Level().Width, e.Level().Height, e.Level().FormatVersion)
	}
	if got := tap(e, Input{Playtest: true}); got != RequestPlaytest {
		t.Fatalf("P on a valid cave = %v, want RequestPlaytest", got)
	}
}

func TestPaintingValidatesAsTheGameDoes(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	tap(e, Input{Brush: 4}) // crystal
	click(e, pos(5, 5))
	if e.doc.at(pos(5, 5)) != sim.SymbolCrystal || !e.Dirty() || e.Level() == nil || e.Level().Crystals() != 1 {
		t.Fatalf("crystal not painted: %q, dirty %v", e.doc.at(pos(5, 5)), e.Dirty())
	}
	tap(e, Input{Brush: 1}) // wall over the exit
	click(e, pos(28, 14))
	if e.Level() != nil || !hasProblem(e, "no exit") {
		t.Fatalf("a cave without an exit validated: %v", e.Problems())
	}
	if got := tap(e, Input{Playtest: true}); got != RequestNone || !strings.Contains(e.View().Hint, "Fix the 1 problem") {
		t.Fatalf("P with problems = %v, hint %q", got, e.View().Hint)
	}
}

func TestPlayerStartAndExitMove(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	tap(e, Input{Brush: 6})
	click(e, pos(10, 8))
	tap(e, Input{Brush: 7})
	click(e, pos(20, 8))
	if occurrences(e, '@') != 1 || occurrences(e, 'E') != 1 || e.doc.at(pos(10, 8)) != '@' || e.doc.at(pos(20, 8)) != 'E' {
		t.Fatalf("player and exit should move:\n%s", e.doc.draft().Encode())
	}
	if e.doc.at(pos(1, 1)) != ' ' || e.doc.at(pos(28, 14)) != ' ' {
		t.Fatal("the old player start and exit should become floor")
	}
	if len(e.Problems()) != 0 {
		t.Fatalf("problems after moving: %v", e.Problems())
	}
	// A box of player starts places one, where the drag ended.
	tap(e, Input{Brush: 6})
	drag(e, false, Input{Shift: true}, pos(3, 3), pos(6, 6))
	if occurrences(e, '@') != 1 || e.doc.at(pos(6, 6)) != '@' {
		t.Fatalf("box of player starts: %d starts, (6,6) is %q", occurrences(e, '@'), e.doc.at(pos(6, 6)))
	}
}

func TestFastDragPaintsAContinuousLine(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	drag(e, false, Input{}, pos(2, 3), pos(12, 7)) // one update from start to end
	for _, p := range line(pos(2, 3), pos(12, 7)) {
		if e.doc.at(p) != '#' {
			t.Fatalf("gap at %s:\n%s", p, e.doc.draft().Encode())
		}
	}
	if len(e.undo) != 1 {
		t.Fatalf("one stroke should be one undo step, got %d", len(e.undo))
	}
}

func TestBoxFillAndErase(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	tap(e, Input{Brush: 5}) // boulder
	drag(e, false, Input{Shift: true}, pos(8, 4), pos(4, 6), pos(5, 7))
	for y := 4; y <= 7; y++ {
		for x := 5; x <= 8; x++ {
			if e.doc.at(pos(x, y)) != 'O' {
				t.Fatalf("(%d,%d) = %q, want a boulder", x, y, e.doc.at(pos(x, y)))
			}
		}
	}
	if e.doc.at(pos(4, 6)) == 'O' {
		t.Fatal("the box follows the pointer: (4,6) was only passed over")
	}
	drag(e, true, Input{}, pos(5, 4), pos(8, 4))
	for x := 5; x <= 8; x++ {
		if e.doc.at(pos(x, 4)) != ' ' {
			t.Fatalf("right-drag should erase to floor at (%d,4)", x)
		}
	}
	if len(e.undo) != 2 {
		t.Fatalf("undo steps = %d, want 2", len(e.undo))
	}
}

func TestEscCancelsAStroke(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	before := e.doc.draft().Encode()
	e.Update(over(e, pos(3, 3), Input{Paint: true}))
	e.Update(over(e, pos(9, 3), Input{Paint: true}))
	e.Update(over(e, pos(9, 3), Input{Paint: true, Back: true}))
	e.Update(Input{})
	if !bytes.Equal(e.doc.draft().Encode(), before) || len(e.undo) != 0 {
		t.Fatal("Esc during a stroke should put the cave back")
	}
}

func TestUndoRedo(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	original := e.encoded
	var states [][]byte
	for i, b := range []int{1, 4, 5, 3} {
		tap(e, Input{Brush: b})
		drag(e, false, Input{}, pos(2+i, 2), pos(2+i, 12))
		states = append(states, e.encoded)
	}
	for i := len(states) - 2; i >= -1; i-- {
		tap(e, Input{Undo: true})
		want := original
		if i >= 0 {
			want = states[i]
		}
		if !bytes.Equal(e.encoded, want) {
			t.Fatalf("after undo to step %d:\n%s", i, e.encoded)
		}
	}
	if e.Dirty() {
		t.Fatal("undoing everything should leave the cave clean")
	}
	tap(e, Input{Undo: true})
	if !strings.Contains(e.View().Hint, "Nothing to undo") {
		t.Fatalf("hint %q", e.View().Hint)
	}
	tap(e, Input{Redo: true})
	tap(e, Input{Redo: true})
	if !bytes.Equal(e.encoded, states[1]) {
		t.Fatal("redo should replay the changes")
	}
	click(e, pos(20, 10)) // a new change drops the redo history
	if tap(e, Input{Redo: true}); len(e.redo) != 0 || !strings.Contains(e.View().Hint, "Nothing to redo") {
		t.Fatal("a new change should clear redo")
	}
}

func TestUndoHistoryIsBounded(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	for i := range maxUndo + 10 {
		tap(e, Input{Brush: 1 + i%4})
		click(e, pos(5, 5))
	}
	if len(e.undo) != maxUndo {
		t.Fatalf("undo steps = %d, want %d", len(e.undo), maxUndo)
	}
}

func TestBrushKeysPaletteAndPick(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	tap(e, Input{Brush: 8})
	if e.symbol() != '~' {
		t.Fatalf("water brush starts deep, got %q", e.symbol())
	}
	tap(e, Input{Brush: 8})
	tap(e, Input{Brush: 8})
	if e.symbol() != '2' {
		t.Fatalf("pressing 8 again makes the water shallower, got %q", e.symbol())
	}
	tap(e, Input{Brush: 9})
	tap(e, Input{Brush: 9})
	if e.symbol() != '→' {
		t.Fatalf("pressing 9 again turns the crawler clockwise, got %q", e.symbol())
	}
	tap(e, Input{Brush: 8}) // each brush remembers its variant
	if e.symbol() != '2' {
		t.Fatalf("water brush forgot its depth: %q", e.symbol())
	}
	click(e, pos(4, 4))

	// A click on a palette slot chooses it and paints nothing.
	before := e.encoded
	x, y := testLayout.Slot(3)
	tap(e, Input{MouseX: x + 4, MouseY: y + 4, Paint: true})
	if e.symbol() != '*' || !bytes.Equal(e.encoded, before) {
		t.Fatalf("palette click: brush %q", e.symbol())
	}
	// Alt-click picks the brush of a cell.
	drag(e, false, Input{Alt: true}, pos(4, 4))
	if e.symbol() != '2' || !bytes.Equal(e.encoded, before) {
		t.Fatalf("Alt-click picked %q", e.symbol())
	}
}

func TestPaintingOverMechanismsUnlinksThem(t *testing.T) {
	e, _ := open(t, embedded(t, "cave_04").Draft(), "")
	// cave_04: valve (12,6), gates (13,11) and (14,11).
	tap(e, Input{Brush: 1})
	click(e, pos(13, 11))
	if len(e.doc.mechanisms) != 1 || !slices.Equal(e.doc.mechanisms[0].Gates, []sim.Pos{pos(14, 11)}) {
		t.Fatalf("mechanisms after removing a gate: %+v", e.doc.mechanisms)
	}
	if len(e.Problems()) != 0 {
		t.Fatalf("a valve with one gate left is still valid: %v", e.Problems())
	}
	click(e, pos(12, 6))
	if len(e.doc.mechanisms) != 0 || !hasProblem(e, "not listed") {
		t.Fatalf("removing the valve should drop its mechanism and orphan its gate: %+v %v", e.doc.mechanisms, e.Problems())
	}
	if v := e.View(); !slices.Contains(v.Problems, pos(14, 11)) {
		t.Fatalf("the orphaned gate should be outlined: %v", v.Problems)
	}
}

func TestResize(t *testing.T) {
	e, _ := open(t, embedded(t, "cave_04").Draft(), "")
	tap(e, Input{Grow: sim.DirLeft})
	tap(e, Input{Grow: sim.DirUp})
	if e.doc.width() != 31 || e.doc.height() != 17 || e.doc.at(pos(0, 9)) != '#' || e.doc.at(pos(9, 0)) != '#' {
		t.Fatalf("grown to %dx%d", e.doc.width(), e.doc.height())
	}
	m := e.doc.mechanisms[0]
	if m.Valve != pos(13, 7) || !slices.Equal(m.Gates, []sim.Pos{pos(14, 12), pos(15, 12)}) || len(e.Problems()) != 0 {
		t.Fatalf("mechanism did not move with its cells: %+v, problems %v", m, e.Problems())
	}
	tap(e, Input{Shrink: sim.DirLeft})
	tap(e, Input{Shrink: sim.DirUp})
	tap(e, Input{Grow: sim.DirRight})
	tap(e, Input{Shrink: sim.DirRight})
	tap(e, Input{Grow: sim.DirDown})
	tap(e, Input{Shrink: sim.DirDown})
	if e.Dirty() {
		t.Fatalf("growing and shrinking back should restore the cave:\n%s", e.encoded)
	}
	for range 5 {
		tap(e, Input{Shrink: sim.DirDown})
	}
	if !hasProblem(e, "not listed") && !hasProblem(e, "no exit") {
		t.Fatalf("shrinking away the bottom should report the lost exit or gates: %v", e.Problems())
	}
	for _, m := range e.doc.mechanisms {
		for _, g := range m.Gates {
			if !e.doc.inBounds(g) {
				t.Fatalf("gate %s left the cave but is still linked", g)
			}
		}
	}
	for range 7 {
		tap(e, Input{Undo: true})
	}
	if e.Dirty() {
		t.Fatal("undo should restore the original size")
	}
}

func TestResizeLimits(t *testing.T) {
	e, _ := open(t, level.Draft{FormatVersion: 1, ID: "x", Title: "X", Rows: []string{"@"}}, "")
	tap(e, Input{Shrink: sim.DirLeft})
	if e.doc.width() != 1 || !strings.Contains(e.View().Hint, "at least one column") {
		t.Fatalf("width %d, hint %q", e.doc.width(), e.View().Hint)
	}
	rows := make([]string, sim.MaxDimension)
	for i := range rows {
		rows[i] = "#"
	}
	e, _ = open(t, level.Draft{FormatVersion: 1, ID: "x", Title: "X", Rows: rows}, "")
	tap(e, Input{Grow: sim.DirDown})
	if e.doc.height() != sim.MaxDimension || !strings.Contains(e.View().Hint, "at most 256") {
		t.Fatalf("height %d, hint %q", e.doc.height(), e.View().Hint)
	}
}

// A cave keeps its format version, since the version is part of the
// checksum golden replays pin. Painting a newer symbol raises it, with a
// warning.
func TestFormatVersionRisesOnlyWhenNeeded(t *testing.T) {
	e, _ := open(t, embedded(t, "test_cave").Draft(), "")
	if e.doc.formatVersion != 1 {
		t.Fatalf("test cave is version %d", e.doc.formatVersion)
	}
	click(e, pos(3, 1)) // a wall on the floor: still version 1
	if e.doc.at(pos(3, 1)) != '#' || e.doc.formatVersion != 1 {
		t.Fatalf("a wall raised the version to %d", e.doc.formatVersion)
	}
	p := pos(8, 10)
	tap(e, Input{Brush: 8})
	click(e, p)
	if e.doc.formatVersion != 3 || !strings.Contains(e.View().Hint, "Format version raised to 3") {
		t.Fatalf("version %d, hint %q", e.doc.formatVersion, e.View().Hint)
	}
	tap(e, Input{Undo: true})
	tap(e, Input{Undo: true})
	if e.doc.formatVersion != 1 || e.Dirty() {
		t.Fatalf("undo should restore version 1, got %d", e.doc.formatVersion)
	}
}

// Opening and saving an embedded cave unchanged writes its file byte for
// byte, so its checksum and golden replays survive the editor.
func TestEmbeddedCavesSaveUnchanged(t *testing.T) {
	names, err := fs.Glob(assets.Levels, assets.LevelDir+"/*.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		data, _ := fs.ReadFile(assets.Levels, name)
		d, err := level.DecodeDraft(name, data)
		if err != nil {
			t.Fatal(err)
		}
		path := "/repo/" + name
		e, st := open(t, d, path)
		if e.Dirty() || len(e.Problems()) != 0 {
			t.Fatalf("%s: dirty %v, problems %v", name, e.Dirty(), e.Problems())
		}
		tap(e, Input{Save: true})
		if !bytes.Equal(st.files[path], data) {
			t.Fatalf("%s: saving unchanged rewrote it as:\n%s", name, st.files[path])
		}
	}
}

func TestSavingNewCavesAndCopies(t *testing.T) {
	e, st := open(t, NewCave(), "")
	tap(e, Input{Save: true})
	if _, ok := st.files["/caves/new_cave.json"]; !ok || e.Path() != "/caves/new_cave.json" || e.Dirty() {
		t.Fatalf("files %v, path %q", st.files, e.Path())
	}
	click(e, pos(5, 5))
	tap(e, Input{Save: true}) // its own file: replaced without asking
	if !bytes.Equal(st.files[e.Path()], e.encoded) || e.Dirty() {
		t.Fatal("second save did not replace the file")
	}

	// A second new cave with the same id must not replace the first silently.
	e2, _ := New(NewCave(), "", testLayout, st)
	e2.Update(Input{})
	click(e2, pos(9, 9))
	tap(e2, Input{Save: true})
	if !strings.Contains(e2.View().Hint, "new_cave.json already exists: Cmd+S again") || e2.Path() != "" {
		t.Fatalf("hint %q", e2.View().Hint)
	}
	tap(e2, Input{Save: true})
	if e2.Path() != "/caves/new_cave.json" || !bytes.Equal(st.files[e2.Path()], e2.encoded) {
		t.Fatal("second Cmd+S should replace the file")
	}

	// A copy of a built-in cave gets its own id.
	c, st := open(t, CopyOf(embedded(t, "cave_04")), "")
	tap(c, Input{Save: true})
	saved, ok := st.files["/caves/cave_04_copy.json"]
	if !ok {
		t.Fatalf("copy saved as %v", st.files)
	}
	l, err := level.Parse("copy", saved)
	if err != nil || l.Title != "Reservoir (copy)" || l.Metadata["based_on"] != "cave_04" || l.Checksum() != embedded(t, "cave_04").Checksum() {
		t.Fatalf("copy: %v %+v", err, l)
	}
}

func TestSavingInvalidCavesWarnsAndKeepsWork(t *testing.T) {
	e, st := open(t, NewCave(), "")
	tap(e, Input{Brush: 1})
	click(e, pos(1, 1)) // wall over the player start
	tap(e, Input{Save: true})
	if _, ok := st.files["/caves/new_cave.json"]; !ok || !strings.Contains(e.View().Hint, "with 1 problem: the game cannot play it yet") {
		t.Fatalf("hint %q", e.View().Hint)
	}
	d, err := level.DecodeDraft("x", st.files["/caves/new_cave.json"])
	if err != nil {
		t.Fatal(err)
	}
	if e2, err := New(d, "/caves/new_cave.json", testLayout, st); err != nil || e2.Dirty() || len(e2.Problems()) != 1 {
		t.Fatalf("reopening work in progress: %v", err)
	}

	st.fail = errors.New("disk full")
	click(e, pos(2, 2))
	tap(e, Input{Save: true})
	if !strings.Contains(e.View().Hint, "Could not save: disk full") || !e.Dirty() {
		t.Fatalf("hint %q", e.View().Hint)
	}
}

func TestSavingNeedsAValidIDAndAFolder(t *testing.T) {
	d := NewCave()
	d.ID = "Not Valid"
	e, st := open(t, d, "")
	tap(e, Input{Save: true})
	if len(st.files) != 0 || !strings.Contains(e.View().Hint, "valid id") {
		t.Fatalf("hint %q", e.View().Hint)
	}
	e, st = open(t, NewCave(), "")
	st.dir = ""
	tap(e, Input{Save: true})
	if len(st.files) != 0 || !strings.Contains(e.View().Hint, "no folder") {
		t.Fatalf("hint %q", e.View().Hint)
	}
}

func TestLeavingAsksAboutUnsavedChanges(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	if got := tap(e, Input{Back: true}); got != RequestLeave {
		t.Fatalf("Esc on a clean cave = %v", got)
	}
	click(e, pos(5, 5))
	if got := tap(e, Input{Back: true}); got != RequestNone || !strings.Contains(e.View().Hint, "Unsaved changes") {
		t.Fatalf("first Esc = %v, hint %q", got, e.View().Hint)
	}
	tap(e, Input{Grid: true}) // any other key cancels the question
	if got := tap(e, Input{Back: true}); got != RequestNone {
		t.Fatalf("Esc after another key = %v", got)
	}
	if got := tap(e, Input{Back: true}); got != RequestLeave {
		t.Fatalf("second Esc = %v", got)
	}
}

func typeText(e *Editor, s string) {
	e.Update(Input{Chars: []rune(s)})
	e.Update(Input{})
}

func TestSettingsPanel(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	tap(e, Input{Settings: true})
	if !e.settings.open || e.View().Settings == nil {
		t.Fatal("Tab should open the settings")
	}
	// Title: clear it, type a new one.
	tap(e, Input{Enter: true})
	for range len("New cave") {
		tap(e, Input{Backspace: true})
	}
	if !e.TextEntry() {
		t.Fatal("editing a field is text entry")
	}
	typeText(e, "Deep <End>")
	tap(e, Input{Enter: true})
	// Id: letters are lowercased, spaces and dashes become '_', the rest dropped.
	tap(e, Input{Down: true})
	tap(e, Input{Enter: true})
	for range len("new_cave") {
		tap(e, Input{Backspace: true})
	}
	typeText(e, "My Cave-2!")
	tap(e, Input{Enter: true})
	// Seed: too large is refused and the field stays open.
	tap(e, Input{Down: true})
	tap(e, Input{Enter: true})
	tap(e, Input{Backspace: true})
	typeText(e, "9999999999999999")
	tap(e, Input{Enter: true})
	if !e.TextEntry() || !strings.Contains(e.View().Hint, "seed must be") {
		t.Fatalf("an out-of-range seed should be refused: hint %q", e.View().Hint)
	}
	tap(e, Input{Back: true}) // cancel
	// Quota.
	tap(e, Input{Down: true})
	tap(e, Input{Enter: true})
	tap(e, Input{Backspace: true})
	typeText(e, "x3")
	tap(e, Input{Enter: true})
	tap(e, Input{Settings: true})
	if e.settings.open {
		t.Fatal("Tab should close the settings")
	}
	if e.doc.title != "Deep <End>" || e.doc.id != "my_cave_2" || e.doc.seed != 1 || e.doc.quota != 3 {
		t.Fatalf("settings: title %q id %q seed %d quota %d", e.doc.title, e.doc.id, e.doc.seed, e.doc.quota)
	}
	if !hasProblem(e, "exceeds the 0 crystal") {
		t.Fatalf("a quota above the crystal count is a problem: %v", e.Problems())
	}
	tap(e, Input{Undo: true})
	if e.doc.quota != 0 || e.doc.id != "my_cave_2" {
		t.Fatal("each field change is one undo step")
	}
}

func TestWaitsForReleaseOnOpenAndResume(t *testing.T) {
	st := newMemStorage()
	e, err := New(NewCave(), "", testLayout, st)
	if err != nil {
		t.Fatal(err)
	}
	if e.Update(over(e, pos(3, 3), Input{Paint: true, Playtest: true})) != RequestNone || e.doc.at(pos(3, 3)) != '.' {
		t.Fatal("input held from before the editor opened must be ignored")
	}
	e.Update(Input{})
	click(e, pos(3, 3))
	if e.doc.at(pos(3, 3)) != '#' {
		t.Fatal("the editor should listen once everything is released")
	}
	e.Resume()
	if e.Update(Input{Playtest: true}) != RequestNone {
		t.Fatal("after Resume, input waits for a release")
	}
}

func TestOpeningDrafts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		draft level.Draft
		want  string
	}{
		{"ragged rows", level.Draft{Rows: []string{"###", "#"}}, "rows of equal length"},
		{"no rows", level.Draft{}, "1 to 256 rows"},
		{"empty rows", level.Draft{Rows: []string{""}}, "rows of 1 to 256 symbols"},
		{"newer format", level.Draft{FormatVersion: level.FormatVersion + 1, Rows: []string{"@"}}, "newer than this build"},
	} {
		if _, err := New(tc.draft, "", testLayout, newMemStorage()); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
	// Anything else opens, and validation reports it.
	e, _ := open(t, level.Draft{Rows: []string{"@@x", "###"}}, "")
	if e.doc.formatVersion != level.FormatVersion || !hasProblem(e, "unknown tile symbol") || !hasProblem(e, "found 2 player starts") {
		t.Fatalf("problems %v", e.Problems())
	}
	if v := e.View(); v.Unknown == nil || !v.Unknown[2] || !slices.Contains(v.Problems, pos(2, 0)) {
		t.Fatalf("unknown symbol not marked: %+v", v.Unknown)
	}
}

func TestViewAndScrolling(t *testing.T) {
	e, _ := open(t, embedded(t, "test_cave").Draft(), "") // 40x24: larger than the view
	v := e.View()
	if v.Width != 40 || v.Height != 24 || len(v.Cells) != 40*24 || len(v.Palette) != len(Palette) || !v.Palette[0].Selected {
		t.Fatalf("view %dx%d, %d cells", v.Width, v.Height, len(v.Cells))
	}
	if v.CameraX != 0 || v.CameraY != 0 {
		t.Fatalf("the player start (1,1) is at the top left; camera %v,%v", v.CameraX, v.CameraY)
	}
	for range 100 {
		e.Update(Input{Right: true, Down: true})
	}
	e.Update(Input{})
	if v := e.View(); v.CameraX != 10 || v.CameraY != 8 {
		t.Fatalf("scrolling should stop at the cave edge: %v,%v", v.CameraX, v.CameraY)
	}
	tap(e, Input{Grow: sim.DirRight})
	if v := e.View(); v.CameraX != 11 {
		t.Fatalf("growing the visible right edge should keep it in view: camera x %v", v.CameraX)
	}
	tap(e, Input{Shrink: sim.DirRight})
	e.Update(Input{WheelY: 3})
	if v := e.View(); v.CameraX != 10 || v.CameraY != 5 {
		t.Fatalf("wheel up should scroll up three tiles: %v,%v", v.CameraX, v.CameraY)
	}
	e.Update(over(e, pos(20, 12), Input{}))
	v = e.View()
	if !v.HoverOK || v.Hover != pos(20, 12) || !v.GhostOK || v.Ghost.Terrain != sim.TerrainWall {
		t.Fatalf("hover %v %v ghost %v", v.Hover, v.HoverOK, v.GhostOK)
	}
	if !strings.HasPrefix(v.Info[0], "(20,12) ") {
		t.Fatalf("info %q", v.Info[0])
	}
	e.Update(over(e, pos(20, 12), Input{Paint: true, Shift: true}))
	e.Update(over(e, pos(22, 14), Input{Paint: true, Shift: true}))
	if v := e.View(); !v.BoxOK || v.Box != (Box{20, 12, 22, 14}) || v.GhostOK {
		t.Fatalf("box preview %+v %v", v.Box, v.BoxOK)
	}

	// A cave smaller than the view is centred in it.
	e, _ = open(t, level.Draft{Rows: []string{"@E"}}, "")
	if v := e.View(); v.CameraX != -14 || v.CameraY != -7.5 {
		t.Fatalf("small cave camera %v,%v", v.CameraX, v.CameraY)
	}
	click(e, pos(1, 0))
	if e.doc.at(pos(1, 0)) != '#' {
		t.Fatal("painting a centred cave")
	}
}

func TestLayoutHitTesting(t *testing.T) {
	l := testLayout
	if w, h := l.ViewTiles(); w != 30 || h != 16 {
		t.Fatalf("view %vx%v tiles", w, h)
	}
	if !l.InView(0, 64) || l.InView(0, 63) || l.InView(959, 576) || !l.InView(959, 575) {
		t.Fatal("InView edges")
	}
	if got := l.CellAt(33, 64+31, 2, 3); got != pos(3, 3) {
		t.Fatalf("CellAt = %v", got)
	}
	if got := l.CellAt(0, 64, -0.5, -0.5); got != pos(-1, -1) {
		t.Fatalf("CellAt left of the cave = %v", got)
	}
	for i := range Palette {
		x, y := l.Slot(i)
		if got, ok := l.SlotAt(x+31, y); !ok || got != i {
			t.Fatalf("slot %d at %d,%d -> %d %v", i, x, y, got, ok)
		}
		if _, ok := l.SlotAt(x+32, y); ok {
			t.Fatalf("gap after slot %d is not a slot", i)
		}
	}
	if l.TextX() <= slotPad+len(Palette)*slotPitch {
		t.Fatal("text overlaps the palette")
	}
}

func TestLine(t *testing.T) {
	for _, tc := range [][2]sim.Pos{{pos(0, 0), pos(5, 2)}, {pos(5, 5), pos(1, 9)}, {pos(3, 3), pos(3, 3)}, {pos(0, 4), pos(0, -2)}} {
		ps := line(tc[0], tc[1])
		if ps[0] != tc[0] || ps[len(ps)-1] != tc[1] {
			t.Fatalf("line %v: %v", tc, ps)
		}
		for i := 1; i < len(ps); i++ {
			if abs(ps[i].X-ps[i-1].X)+abs(ps[i].Y-ps[i-1].Y) != 1 {
				t.Fatalf("line %v is not 4-connected at %d: %v", tc, i, ps)
			}
		}
	}
}

// Random input never breaks the editor's invariants, and undoing every
// change gives back the original file.
func TestRandomEditingKeepsInvariants(t *testing.T) {
	for _, id := range []string{"cave_04", "crawler_warren", "test_cave"} {
		t.Run(id, func(t *testing.T) {
			rng := rand.New(rand.NewPCG(2026, uint64(len(id))))
			e, _ := open(t, embedded(t, id).Draft(), "")
			for step := range 3000 {
				e.Update(randomInput(rng))
				if err := checkInvariants(e); err != nil {
					t.Fatalf("step %d: %v\n%s", step, err, e.encoded)
				}
			}

			// A shorter run stays within the undo history, so undoing
			// every change must give back the original exactly.
			e, _ = open(t, embedded(t, id).Draft(), "")
			original := e.encoded
			for range 300 {
				e.Update(randomInput(rng))
			}
			e.settings.open = false
			e.Update(Input{})
			e.Update(Input{})
			final := e.encoded
			undos := len(e.undo)
			if undos == 0 || undos >= maxUndo {
				t.Fatalf("%d undo steps; the run should make some changes and stay within the history", undos)
			}
			for range undos {
				tap(e, Input{Undo: true})
			}
			if !bytes.Equal(e.encoded, original) {
				t.Fatalf("undoing all %d steps did not restore the cave:\n%s", undos, e.encoded)
			}
			for range undos {
				tap(e, Input{Redo: true})
			}
			if !bytes.Equal(e.encoded, final) {
				t.Fatal("redoing every step did not restore the final cave")
			}
		})
	}
}

func randomInput(rng *rand.Rand) Input {
	in := Input{
		MouseX: rng.IntN(testLayout.Width+64) - 32, MouseY: rng.IntN(testLayout.Height+64) - 32,
		Paint: rng.IntN(3) == 0, Erase: rng.IntN(8) == 0, Shift: rng.IntN(4) == 0, Alt: rng.IntN(10) == 0,
		Cmd:  rng.IntN(12) == 0,
		Left: rng.IntN(10) == 0, Right: rng.IntN(10) == 0, Up: rng.IntN(10) == 0, Down: rng.IntN(10) == 0,
	}
	switch rng.IntN(44) {
	case 0:
		in.Brush = 1 + rng.IntN(len(Palette))
	case 1:
		in.Undo = true
	case 2:
		in.Redo = true
	case 3:
		in.Grow = sim.Dir(1 + rng.IntN(4))
	case 4:
		in.Shrink = sim.Dir(1 + rng.IntN(4))
	case 5:
		in.Settings = true
	case 6:
		in.Enter = true
	case 7:
		in.Chars = []rune(strconv.Itoa(rng.IntN(100)))
	case 8:
		in.Back = true
	case 9:
		in.WheelX, in.WheelY = rng.Float64()*4-2, rng.Float64()*4-2
	case 10, 11:
		in.Toggle = true
	case 12:
		in.Reach = true
	case 13: // the valve and gate brushes, more often than chance
		in.Brush = len(Palette) - rng.IntN(2)
	}
	return in
}

// at is the symbol at p, or 0 outside the cave.
func at(d *doc, p sim.Pos) rune {
	if !d.inBounds(p) {
		return 0
	}
	return d.at(p)
}

func checkInvariants(e *Editor) error {
	d := &e.doc
	w, h := d.width(), d.height()
	if w < 1 || h < 1 || w > sim.MaxDimension || h > sim.MaxDimension {
		return fmt.Errorf("size %dx%d", w, h)
	}
	for y, row := range d.grid {
		if len(row) != w {
			return fmt.Errorf("row %d has %d symbols, want %d", y, len(row), w)
		}
	}
	if n := occurrences(e, '@'); n > 1 {
		return fmt.Errorf("%d player starts", n)
	}
	if n := occurrences(e, 'E'); n > 1 {
		return fmt.Errorf("%d exits", n)
	}
	linked := map[sim.Pos]bool{}
	for _, m := range d.mechanisms {
		valve, _ := sim.CellForSymbol(at(d, m.Valve))
		if !valve.Terrain.Valve() || linked[m.Valve] {
			return fmt.Errorf("mechanism valve %s is %q or already linked", m.Valve, at(d, m.Valve))
		}
		linked[m.Valve] = true
		for _, g := range m.Gates {
			gate, _ := sim.CellForSymbol(at(d, g))
			if !gate.Terrain.Gate() || linked[g] {
				return fmt.Errorf("mechanism gate %s is %q or already linked", g, at(d, g))
			}
			linked[g] = true
			if (gate.Terrain == sim.TerrainGateOpen) != (valve.Terrain == sim.TerrainValveOpen) {
				return fmt.Errorf("gate %s is %q but its valve %s is %q", g, at(d, g), m.Valve, at(d, m.Valve))
			}
		}
	}
	if !bytes.Equal(e.encoded, d.draft().Encode()) {
		return fmt.Errorf("encoding is stale")
	}
	if _, err := level.DecodeDraft("x", e.encoded); err != nil {
		return err
	}
	if len(e.cells) != w*h {
		return fmt.Errorf("%d cells for %dx%d", len(e.cells), w, h)
	}
	return nil
}

func TestDirStorage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "caves")
	s := DirStorage{Path: dir}
	if files, err := s.List(); err != nil || files != nil {
		t.Fatalf("a missing folder holds no caves: %v %v", files, err)
	}
	path := filepath.Join(dir, "new_cave.json")
	data := NewCave().Encode()
	if err := s.Write(path, data, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(path, data, false); !errors.Is(err, ErrExists) {
		t.Fatalf("second write without replace: %v", err)
	}
	if err := s.Write(path, append(slices.Clone(data), ' '), true); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o644 {
		t.Fatalf("saved cave mode %v, %v; want 0644 like any document", fi.Mode().Perm(), err)
	}
	if err := s.Write(filepath.Join(dir, "broken.json"), []byte("{"), false); err != nil {
		t.Fatal(err)
	}
	files, err := s.List()
	if err != nil || len(files) != 2 {
		t.Fatalf("list: %v %v", files, err)
	}
	if files[0].Err == nil || !strings.Contains(files[0].Err.Error(), "broken.json") {
		t.Fatalf("broken file: %+v", files[0])
	}
	if files[1].Err != nil || files[1].Path != path || files[1].Draft.ID != "new_cave" {
		t.Fatalf("cave file: %+v", files[1])
	}
	entries, _ := filepath.Glob(filepath.Join(dir, ".*"))
	if len(entries) != 0 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}

// internal/editor stays display-free like internal/game: it may use the
// standard library and the pure internal/level and internal/sim packages.
func TestImportsArePure(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			pure := !strings.Contains(path, ".") ||
				path == "github.com/townhill/deep-shift/internal/level" || path == "github.com/townhill/deep-shift/internal/sim"
			if !pure {
				t.Errorf("%s imports %q; internal/editor must stay display-free", name, path)
			}
		}
	}
}

// Status lines name every creature with its facing, glimmers included,
// though no brush paints a glimmer yet.
func TestSymbolNamesCreatures(t *testing.T) {
	for r, want := range map[rune]string{'↑': "crawler facing up", '◀': "glimmer facing left", '▼': "glimmer facing down"} {
		if got := symbolName(r); got != want {
			t.Errorf("symbolName(%q) = %q, want %q", r, got, want)
		}
	}
}
