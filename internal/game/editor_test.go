package game

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/townhill/deep-shift/internal/editor"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/sim"
)

// editorLayout is the app's editor layout (render's constants).
var editorLayout = editor.Layout{Tile: 32, Width: 960, Height: 640, Top: 64, Bottom: 64}

func newEditorShell(t *testing.T) (*Shell, string) {
	t.Helper()
	sh := newShell(t)
	dir := filepath.Join(t.TempDir(), "caves")
	sh.EnableEditor(editorLayout, editor.DirStorage{Path: dir})
	return sh, dir
}

// openEditList chooses "Cave editor" on the title menu.
func openEditList(t *testing.T, sh *Shell) {
	t.Helper()
	press(sh, Controls{Up: true}) // wraps to the last entry
	if selected(sh.Menu()) != "Cave editor" {
		t.Fatalf("selected %q", selected(sh.Menu()))
	}
	press(sh, Controls{Interact: true})
	if sh.Screen() != ScreenEditList {
		t.Fatalf("screen %d, want the edit list", sh.Screen())
	}
}

// editorTap sends in for one update and an idle input for one more.
func editorTap(sh *Shell, in editor.Input) {
	sh.UpdateEditor(in)
	sh.UpdateEditor(editor.Input{})
}

func menuText(v MenuView) string {
	var text []string
	for _, e := range v.Entries {
		text = append(text, e.Text)
	}
	return strings.Join(text, "|")
}

func TestTitleOffersTheEditorOnlyWhenEnabled(t *testing.T) {
	sh := newShell(t)
	if strings.Contains(menuText(sh.Menu()), "Cave editor") {
		t.Fatal("the editor entry needs EnableEditor")
	}
	sh, _ = newEditorShell(t)
	openEditList(t, sh)
	v := sh.Menu()
	if v.Title != "CAVE EDITOR" || selected(v) != "New cave" || !strings.Contains(menuText(v), "Copy a built-in cave|First Descent") {
		t.Fatalf("edit list %q", menuText(v))
	}
	press(sh, Controls{Pause: true})
	if sh.Screen() != ScreenTitle || selected(sh.Menu()) != "Cave editor" {
		t.Fatalf("Esc: screen %d, selected %q", sh.Screen(), selected(sh.Menu()))
	}
}

func TestEditPlaytestAndReturn(t *testing.T) {
	sh, _ := newEditorShell(t)
	openEditList(t, sh)
	press(sh, Controls{Interact: true}) // New cave
	if sh.Screen() != ScreenEditor || sh.Editor() == nil {
		t.Fatalf("screen %d", sh.Screen())
	}
	sh.UpdateEditor(editor.Input{}) // the key that chose the cave is released
	editorTap(sh, editor.Input{Brush: 4})
	x, y := 5*32+16, 64+3*32+16 // cell (5,3)
	editorTap(sh, editor.Input{MouseX: x, MouseY: y, Paint: true})
	if sh.Editor().Level().Crystals() != 1 {
		t.Fatal("the crystal was not painted")
	}

	editorTap(sh, editor.Input{Playtest: true})
	if sh.Screen() != ScreenPlaytest || sh.Session() == nil || sh.Session().Level().Crystals() != 1 {
		t.Fatalf("screen %d after P", sh.Screen())
	}
	if !strings.Contains(sh.Session().HUD().Prompt, "P returns to the editor") {
		t.Fatalf("prompt %q", sh.Session().HUD().Prompt)
	}
	press(sh, Controls{}) // release, as after any screen change
	press(sh, Controls{Pause: true})
	if o := strings.Join(sh.Session().HUD().Overlay, " | "); !strings.Contains(o, "P: back to the editor") || strings.Contains(o, "N: next cave") {
		t.Fatalf("playtest overlay %q", o)
	}
	press(sh, Controls{Next: true, Menu: true}) // playlist keys do nothing in a playtest
	if sh.Screen() != ScreenPlaytest {
		t.Fatalf("N/C left the playtest: screen %d", sh.Screen())
	}
	press(sh, Controls{Edit: true})
	if sh.Screen() != ScreenEditor || sh.Session() != nil {
		t.Fatalf("P should return to the editor: screen %d", sh.Screen())
	}
	if sh.Editor().Level().Crystals() != 1 || !sh.Editor().Dirty() {
		t.Fatal("the edit should survive a playtest")
	}
	// The P press that ended the playtest must not start another.
	sh.UpdateEditor(editor.Input{Playtest: true})
	if sh.Screen() != ScreenEditor {
		t.Fatal("a held P restarted the playtest")
	}
}

// A playtest is a real session: the cave plays exactly as in the game. A
// copy of cave 01 keeps its layout, so cave 01's golden replay completes it,
// but a playtest is not campaign progress.
func TestPlaytestPlaysLikeTheGame(t *testing.T) {
	sh, _ := newEditorShell(t)
	if err := sh.OpenEditor(editor.CopyOf(embeddedCave(t, "cave_01")), ""); err != nil {
		t.Fatal(err)
	}
	sh.UpdateEditor(editor.Input{})
	editorTap(sh, editor.Input{Playtest: true})
	if sh.Screen() != ScreenPlaytest {
		t.Fatalf("screen %d", sh.Screen())
	}
	press(sh, Controls{})
	fixture := loadFixture(t, "cave_01_complete.json")
	drive(t, sh.Update, sh.Session, fixture, fixture.Ticks)
	s := sh.Session()
	if s.Snapshot().Status != sim.StatusComplete || s.StateHash().String() != fixture.Expect.FinalHash {
		t.Fatalf("status %v, hash %s", s.Snapshot().Status, s.StateHash())
	}
	if sh.Completed("cave_01") || sh.Completed("cave_01_copy") {
		t.Fatal("a playtest is not campaign progress")
	}
	if o := strings.Join(s.HUD().Overlay, " | "); !strings.Contains(o, "R: play again   P: back to the editor") {
		t.Fatalf("completion overlay %q", o)
	}
}

func TestSavedCavesAppearInTheEditList(t *testing.T) {
	sh, dir := newEditorShell(t)
	openEditList(t, sh)
	press(sh, Controls{Interact: true}) // New cave
	sh.UpdateEditor(editor.Input{})
	editorTap(sh, editor.Input{Save: true})
	path := filepath.Join(dir, "new_cave.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("not saved: %v", err)
	}
	editorTap(sh, editor.Input{Back: true})
	if sh.Screen() != ScreenEditList {
		t.Fatalf("Esc on a saved cave: screen %d", sh.Screen())
	}
	v := sh.Menu()
	if selected(v) != "New cave   (new_cave.json)" || !strings.Contains(menuText(v), "Your caves|New cave   (new_cave.json)|Copy a built-in cave") {
		t.Fatalf("edit list %q, selected %q", menuText(v), selected(v))
	}
	press(sh, Controls{}) // release, as after any screen change
	press(sh, Controls{Interact: true})
	if sh.Screen() != ScreenEditor || sh.Editor().Path() != path || sh.Editor().Dirty() {
		t.Fatalf("reopening: screen %d", sh.Screen())
	}
}

func TestEditListScrollsAndReportsBrokenFiles(t *testing.T) {
	sh, dir := newEditorShell(t)
	st := editor.DirStorage{Path: dir}
	for i := range 6 {
		d := editor.NewCave()
		d.ID, d.Title = fmt.Sprintf("mine_%d", i), fmt.Sprintf("Mine %d", i)
		if err := st.Write(filepath.Join(dir, d.ID+".json"), d.Encode(), false); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Write(filepath.Join(dir, "a_broken.json"), []byte(`{"rows": [1]}`), false); err != nil {
		t.Fatal(err)
	}
	openEditList(t, sh)
	v := sh.Menu()
	if len(v.Entries) != menuLines || v.Entries[menuLines-1].Text != "(more below)" {
		t.Fatalf("a long list should scroll: %q", menuText(v))
	}
	press(sh, Controls{Down: true}) // the broken file sorts first
	if got := selected(sh.Menu()); got != "a_broken.json   (cannot open)" {
		t.Fatalf("selected %q", got)
	}
	press(sh, Controls{Interact: true})
	if sh.Screen() != ScreenEditList || !strings.Contains(sh.Menu().Footer, "Cannot edit it: a_broken.json") {
		t.Fatalf("screen %d, footer %q", sh.Screen(), sh.Menu().Footer)
	}
	press(sh, Controls{Up: true})
	press(sh, Controls{Up: true}) // wraps to the last built-in cave
	v = sh.Menu()
	if v.Entries[0].Text != "(more above)" || !strings.Contains(selected(v), "Proving Ground") {
		t.Fatalf("scrolled to the end: %q", menuText(v))
	}
}

func TestOpenEditorDirectlyAndLeave(t *testing.T) {
	sh, _ := newEditorShell(t)
	l := embeddedCave(t, "cave_04")
	path := filepath.Join(t.TempDir(), "cave_04_reservoir.json")
	if err := os.WriteFile(path, l.Draft().Encode(), 0o644); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	d, err := level.DecodeDraft(path, data)
	if err != nil {
		t.Fatal(err)
	}
	if err := sh.OpenEditor(d, path); err != nil {
		t.Fatal(err)
	}
	sh.UpdateEditor(editor.Input{})
	if sh.Screen() != ScreenEditor || sh.Editor().Dirty() || sh.Editor().Level().Checksum() != l.Checksum() {
		t.Fatal("-edit should open the file unchanged")
	}
	if sh.TextEntry() {
		t.Fatal("painting is not text entry")
	}
	editorTap(sh, editor.Input{Settings: true})
	editorTap(sh, editor.Input{Enter: true})
	if !sh.TextEntry() {
		t.Fatal("typing in a settings field is text entry")
	}
	editorTap(sh, editor.Input{Back: true}) // stop typing
	editorTap(sh, editor.Input{Back: true}) // close the settings
	editorTap(sh, editor.Input{Back: true}) // leave
	if sh.Screen() != ScreenEditList {
		t.Fatalf("leaving -edit: screen %d", sh.Screen())
	}
	if err := sh.OpenEditor(level.Draft{Rows: []string{"##", "#"}}, ""); err == nil {
		t.Fatal("ragged rows should not open")
	}
}

// A sluice built in the editor works in a playtest: Space at the valve
// opens its gate.
func TestPlaytestEditorBuiltSluice(t *testing.T) {
	sh, _ := newEditorShell(t)
	if err := sh.OpenEditor(editor.NewCave(), ""); err != nil {
		t.Fatal(err)
	}
	sh.UpdateEditor(editor.Input{})
	cell := func(x, y int) (int, int) { return x*32 + 16, 64 + y*32 + 16 }
	editorTap(sh, editor.Input{Brush: 10}) // valve, beside the player start at (1,1)
	x, y := cell(2, 1)
	editorTap(sh, editor.Input{MouseX: x, MouseY: y, Paint: true})
	editorTap(sh, editor.Input{Brush: 11}) // gate
	x, y = cell(6, 5)
	editorTap(sh, editor.Input{MouseX: x, MouseY: y, Paint: true})
	if p := sh.Editor().Problems(); len(p) != 0 {
		t.Fatalf("problems: %v", p)
	}
	editorTap(sh, editor.Input{Playtest: true})
	if sh.Screen() != ScreenPlaytest {
		t.Fatalf("screen %d", sh.Screen())
	}
	press(sh, Controls{})
	if got := sh.Session().HUD().Prompt; got != "Playtest: P returns to the editor" {
		t.Fatalf("prompt %q", got)
	}
	for range 2 * UpdatesPerTick {
		sh.Update(Controls{Interact: true})
	}
	if c := sh.Session().Snapshot().Cell(sim.Pos{X: 6, Y: 5}); c.Terrain != sim.TerrainGateOpen {
		t.Fatalf("the gate is %v after turning the valve", c.Terrain)
	}
}
