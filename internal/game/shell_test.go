package game

import (
	"strings"
	"testing"

	"github.com/townhill/deep-shift/assets"
	"github.com/townhill/deep-shift/internal/level"
)

func newShell(t *testing.T) *Shell {
	t.Helper()
	all, err := level.LoadDir(assets.Levels, assets.LevelDir)
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := OrderLevels(all, assets.PlayOrder)
	if err != nil {
		t.Fatal(err)
	}
	sh, err := NewShell(ordered, len(assets.Campaign), 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	return sh
}

// press holds c for one update, then releases everything for one update, as
// a quick key tap does.
func press(sh *Shell, c Controls) {
	sh.Update(c)
	sh.Update(Controls{})
}

func selected(v MenuView) string {
	for _, e := range v.Entries {
		if e.Selected {
			return e.Text
		}
	}
	return ""
}

func TestShellOpensOnTheTitleAndStartsTheCampaign(t *testing.T) {
	sh := newShell(t)
	if sh.Screen() != ScreenTitle || sh.Playlist() != nil {
		t.Fatalf("screen %d, playlist %v", sh.Screen(), sh.Playlist())
	}
	v := sh.Menu()
	if v.Title != "DEEP SHIFT" || selected(v) != "Play the campaign" {
		t.Fatalf("title menu %+v", v)
	}
	press(sh, Controls{Interact: true})
	if sh.Screen() != ScreenPlaying || sh.Playlist().Session().Level().ID != "cave_01" {
		t.Fatalf("screen %d after choosing Play", sh.Screen())
	}
}

func TestCaveListChoosesAnyCaveAndWraps(t *testing.T) {
	sh := newShell(t)
	press(sh, Controls{Down: true})
	if selected(sh.Menu()) != "Choose a cave" {
		t.Fatalf("selected %q", selected(sh.Menu()))
	}
	press(sh, Controls{Interact: true})
	if sh.Screen() != ScreenCaves {
		t.Fatalf("screen %d, want the cave list", sh.Screen())
	}
	v := sh.Menu()
	text := []string{}
	for _, e := range v.Entries {
		text = append(text, e.Text)
	}
	joined := strings.Join(text, "|")
	for _, want := range []string{"1  First Descent", "5  The Sluice", "Extras", "Proving Ground"} {
		if !strings.Contains(joined, want) {
			t.Errorf("cave list %q lacks %q", joined, want)
		}
	}
	press(sh, Controls{Up: true}) // wraps from the first cave to the last (the extra)
	if got := selected(sh.Menu()); !strings.Contains(got, "Proving Ground") {
		t.Fatalf("Up from the top selected %q", got)
	}
	for range 4 {
		press(sh, Controls{Down: true}) // wraps to cave 1, then on to cave 4
	}
	press(sh, Controls{Interact: true})
	if sh.Screen() != ScreenPlaying || sh.Playlist().Session().Level().ID != "cave_04" {
		t.Fatalf("screen %d, cave %v", sh.Screen(), sh.Playlist())
	}
}

// A key still held from a menu choice must not reach the game: holding
// Enter would otherwise turn a valve beside the start, and a held arrow
// would walk.
func TestHeldMenuKeyDoesNotLeakIntoTheGame(t *testing.T) {
	sh := newShell(t)
	sh.Update(Controls{Interact: true, Right: true}) // chooses Play, keys still held
	for range 30 * UpdatesPerTick {
		sh.Update(Controls{Interact: true, Right: true})
	}
	s := sh.Playlist().Session()
	if len(s.Commands()) != 0 || s.Snapshot().Tick != 0 {
		t.Fatalf("held keys reached the game: %d commands, tick %d", len(s.Commands()), s.Snapshot().Tick)
	}
	run2 := func(c Controls, n int) {
		for range n {
			sh.Update(c)
		}
	}
	run2(Controls{}, 1)
	run2(Controls{Right: true}, UpdatesPerTick+1)
	if len(s.Commands()) != 1 {
		t.Fatalf("after release, a fresh press gave %d commands", len(s.Commands()))
	}
}

func TestEscFromTheCaveListReturnsToTheTitle(t *testing.T) {
	sh := newShell(t)
	press(sh, Controls{Down: true})
	press(sh, Controls{Interact: true})
	press(sh, Controls{Pause: true})
	if sh.Screen() != ScreenTitle || selected(sh.Menu()) != "Choose a cave" {
		t.Fatalf("screen %d, selected %q", sh.Screen(), selected(sh.Menu()))
	}
}

func TestCOpensTheCaveListOnlyFromPauseOrEnd(t *testing.T) {
	sh := newShell(t)
	if err := sh.Play(2); err != nil {
		t.Fatal(err)
	}
	press(sh, Controls{})
	press(sh, Controls{Menu: true})
	if sh.Screen() != ScreenPlaying {
		t.Fatal("C left the cave during play")
	}
	press(sh, Controls{Pause: true})
	press(sh, Controls{Menu: true})
	if sh.Screen() != ScreenCaves || !strings.Contains(selected(sh.Menu()), "The Roll") {
		t.Fatalf("screen %d, selected %q", sh.Screen(), selected(sh.Menu()))
	}
}

func TestCompletedCavesAreMarkedInTheList(t *testing.T) {
	sh := newShell(t)
	if err := sh.Play(0); err != nil {
		t.Fatal(err)
	}
	press(sh, Controls{}) // release before play, as after any screen change
	driveShellFixture(t, sh, loadFixture(t, "cave_01_complete.json"))
	if !sh.Completed("cave_01") || sh.Completed("cave_02") {
		t.Fatalf("completed: cave_01 %v, cave_02 %v", sh.Completed("cave_01"), sh.Completed("cave_02"))
	}
	press(sh, Controls{Menu: true})
	v := sh.Menu()
	if !strings.Contains(selected(v), "First Descent   (done)") || !strings.Contains(v.Subtitle, "1 of 5") {
		t.Fatalf("cave list after completing cave_01: selected %q, subtitle %q", selected(v), v.Subtitle)
	}
}
