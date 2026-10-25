package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/townhill/deep-shift/internal/editor"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/replay"
	"github.com/townhill/deep-shift/internal/sim"
)

// saveCave writes d into the player's caves folder as <name>.json.
func saveCave(t *testing.T, dir, name string, d level.Draft) string {
	t.Helper()
	path := filepath.Join(dir, name+".json")
	if err := (editor.DirStorage{Path: dir}).Write(path, d.Encode(), true); err != nil {
		t.Fatal(err)
	}
	return path
}

// myCopyOfCave01 is cave 01's layout under another id: cave 01's golden
// replay completes it.
func myCopyOfCave01(t *testing.T) level.Draft {
	d := embeddedCave(t, "cave_01").Draft()
	d.ID, d.Title = "mine", "Mine"
	return d
}

// openCaveList chooses "Choose a cave" on the title menu.
func openCaveListFromTitle(t *testing.T, sh *Shell) {
	t.Helper()
	press(sh, Controls{Down: true})
	press(sh, Controls{Interact: true})
	if sh.Screen() != ScreenCaves {
		t.Fatalf("screen %d, want the cave list", sh.Screen())
	}
}

// selectEntry moves the cave list selection to the entry containing text.
func selectEntry(t *testing.T, sh *Shell, text string) {
	t.Helper()
	for range len(sh.caveItems) {
		if strings.Contains(selected(sh.Menu()), text) {
			return
		}
		press(sh, Controls{Down: true})
	}
	t.Fatalf("no entry %q in %q", text, menuText(sh.Menu()))
}

func TestMyCavesAppearInTheCaveList(t *testing.T) {
	sh, dir := newEditorShell(t)
	mine := saveCave(t, dir, "mine", myCopyOfCave01(t))
	broken := myCopyOfCave01(t)
	broken.ID, broken.Title, broken.RequiredCrystals = "wip", "Work in progress", 99
	saveCave(t, dir, "wip", broken)
	taken := myCopyOfCave01(t)
	taken.ID, taken.Title = "cave_02", "Taken id"
	saveCave(t, dir, "taken", taken)
	twin := myCopyOfCave01(t)
	twin.Title = "Twin"
	saveCave(t, dir, "twin", twin) // same id as mine.json, which sorts first
	if err := os.WriteFile(filepath.Join(dir, "garbled.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	openCaveListFromTitle(t, sh)
	all := strings.Join(func() []string {
		var text []string
		for i := range sh.caveItems {
			sh.cursor = i
			text = append(text, selected(sh.Menu()))
		}
		sh.cursor = 0
		return text
	}(), "|")
	for _, want := range []string{
		"   garbled.json   (cannot play)", "   Mine", "   Taken id   (cannot play)",
		"   Twin   (cannot play)", "   Work in progress   (cannot play)",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("cave list %q lacks %q", all, want)
		}
	}
	sh.cursor = len(sh.levels)
	if v := sh.Menu(); !strings.Contains(menuText(v), "My caves|") {
		t.Fatalf("no My caves heading: %q", menuText(v))
	}

	for entry, why := range map[string]string{
		"garbled.json":     "garbled.json: the file cannot be read",
		"Taken id":         `taken.json: its id "cave_02" is taken by a built-in cave`,
		"Twin":             `twin.json: its id "mine" is taken by mine.json`,
		"Work in progress": "wip.json: 1 problem; fix it in the Cave editor",
	} {
		selectEntry(t, sh, entry)
		press(sh, Controls{Interact: true})
		if sh.Screen() != ScreenCaves || sh.Menu().Footer != "Cannot play "+why {
			t.Errorf("%s: screen %d, footer %q", entry, sh.Screen(), sh.Menu().Footer)
		}
	}

	selectEntry(t, sh, "Mine")
	press(sh, Controls{Interact: true}) // its release is the update the shell waits for
	if sh.Screen() != ScreenPlaying || sh.Session().Level().ID != "mine" {
		t.Fatalf("screen %d", sh.Screen())
	}
	driveShellFixture(t, sh, loadFixture(t, "cave_01_complete.json"))
	if !sh.Completed("mine") || sh.Completed("cave_01") {
		t.Fatal("completing my cave marks it, and only it, done")
	}
	r, ended := sh.FinishedAttempt()
	if !ended || r == nil || !strings.Contains(r.Description, "Cave file: "+mine) {
		t.Fatalf("replay of my cave: %v %+v", ended, r)
	}
	if _, again := sh.FinishedAttempt(); again {
		t.Fatal("an attempt is reported once")
	}
	l, err := level.Parse(mine, mustRead(t, mine))
	if err != nil {
		t.Fatal(err)
	}
	if res, err := replay.Run(r, l); err != nil || r.Check(res) != nil {
		t.Fatalf("the replay should re-run against the cave file: %v %v", err, r.Check(res))
	}

	press(sh, Controls{Menu: true})
	if got := selected(sh.Menu()); got != "   Mine   (done)" {
		t.Fatalf("back in the list, selected %q", got)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// N plays on from the last built-in cave into the player's own.
func TestNMovesOnIntoMyCaves(t *testing.T) {
	sh, dir := newEditorShell(t)
	saveCave(t, dir, "mine", myCopyOfCave01(t))
	openCaveListFromTitle(t, sh)
	selectEntry(t, sh, "Proving Ground")
	press(sh, Controls{Interact: true})
	press(sh, Controls{})
	press(sh, Controls{Pause: true})
	press(sh, Controls{Next: true})
	if got := sh.Session().Level().ID; got != "mine" {
		t.Fatalf("N after the last built-in cave played %q", got)
	}
	press(sh, Controls{Pause: true})
	press(sh, Controls{Next: true})
	if got := sh.Session().Level().ID; got != "cave_01" {
		t.Fatalf("N after my last cave played %q, want a wrap to cave_01", got)
	}
}

func TestBuiltInReplaysNameNoFile(t *testing.T) {
	sh := newShell(t)
	if err := sh.Play(0); err != nil {
		t.Fatal(err)
	}
	press(sh, Controls{})
	if _, ended := sh.FinishedAttempt(); ended {
		t.Fatal("nothing has ended yet")
	}
	driveShellFixture(t, sh, loadFixture(t, "cave_01_complete.json"))
	r, ended := sh.FinishedAttempt()
	if !ended || r == nil || r.CaveID != "cave_01" || strings.Contains(r.Description, "Cave file") {
		t.Fatalf("built-in replay: %v %+v", ended, r)
	}
}

// oneStepCave is completed by a single step right into its open exit.
func oneStepCave() level.Draft {
	return level.Draft{
		FormatVersion: level.FormatVersion, ID: "one_step", Title: "One step",
		Width: 5, Height: 3, Seed: 1, Rows: []string{"#####", "#@E #", "#####"},
	}
}

// playtestToTheEnd playtests the editor's cave and steps right into the
// exit, returning what FinishedAttempt reports.
func playtestToTheEnd(t *testing.T, sh *Shell) (*replay.Replay, bool) {
	t.Helper()
	editorTap(sh, editor.Input{Playtest: true})
	if sh.Screen() != ScreenPlaytest {
		t.Fatalf("screen %d, want a playtest: %v", sh.Screen(), sh.Editor().Problems())
	}
	press(sh, Controls{})
	for range 2 * UpdatesPerTick {
		sh.Update(Controls{Right: true})
	}
	if sh.Session().Snapshot().Status != sim.StatusComplete {
		t.Fatalf("status %v", sh.Session().Snapshot().Status)
	}
	return sh.FinishedAttempt()
}

func TestPlaytestsOfSavedUnchangedCavesAreRecorded(t *testing.T) {
	sh, dir := newEditorShell(t)

	// Unsaved: a new cave has no file.
	if err := sh.OpenEditor(oneStepCave(), ""); err != nil {
		t.Fatal(err)
	}
	sh.UpdateEditor(editor.Input{})
	if r, ended := playtestToTheEnd(t, sh); !ended || r != nil {
		t.Fatalf("an unsaved cave's playtest: ended %v, replay %+v", ended, r)
	}

	// Saved and unchanged: recorded, naming the file, and it re-runs.
	press(sh, Controls{Edit: true})
	sh.UpdateEditor(editor.Input{}) // the editor listens once P is released
	editorTap(sh, editor.Input{Save: true})
	path := filepath.Join(dir, "one_step.json")
	r, ended := playtestToTheEnd(t, sh)
	if !ended || r == nil || !strings.HasPrefix(r.Description, "Recorded in an editor playtest: the player completed") ||
		!strings.HasSuffix(r.Description, " Cave file: "+path+" (cavetool replay needs it as -cave).") {
		t.Fatalf("a saved cave's playtest: ended %v, replay %+v", ended, r)
	}
	l, err := level.Parse(path, mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	if res, err := replay.Run(r, l); err != nil || r.Check(res) != nil || res.Sim.Status() != sim.StatusComplete {
		t.Fatalf("the playtest replay should re-run against the saved file: %v", err)
	}

	// Changed since the save: not recorded. The 5x3 cave is centred in the
	// 30x16 view, so its cell (3,1) is the view's cell (15.5,7.5).
	press(sh, Controls{Edit: true})
	sh.UpdateEditor(editor.Input{})
	editorTap(sh, editor.Input{Brush: 2})
	editorTap(sh, editor.Input{MouseX: 16 * 32, MouseY: 64 + 8*32, Paint: true})
	if !sh.Editor().Dirty() {
		t.Fatal("the edit did not change the cave")
	}
	if r, ended := playtestToTheEnd(t, sh); !ended || r != nil {
		t.Fatalf("a changed cave's playtest: ended %v, replay %+v", ended, r)
	}
}
