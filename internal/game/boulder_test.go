package game

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/townhill/deep-shift/assets"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/replay"
	"github.com/townhill/deep-shift/internal/sim"
)

// SPEC §11 Phase 2: a crush ends the cave, the end screen offers a restart,
// and the attempt is available as a replay that reproduces the death.
func TestCrushEndsAttemptAndLogsReproducibleReplay(t *testing.T) {
	fixture := loadFixture(t, "cave_02_crushed.json")
	cave := embeddedCave(t, "cave_02")
	s, err := NewSession(cave, 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	driveFixture(t, s, fixture)
	if s.Snapshot().Status != sim.StatusDead || !s.Ended() {
		t.Fatalf("status %s, want dead", s.Snapshot().Status)
	}
	overlay := strings.Join(s.HUD().Overlay, " | ")
	if !strings.Contains(overlay, "CRUSHED") || !strings.Contains(overlay, "R: try again") {
		t.Fatalf("death overlay = %q", overlay)
	}

	// The world stays frozen on the death screen.
	frozen := s.StateHash()
	for range 60 {
		s.Update(Controls{Right: true})
	}
	if s.StateHash() != frozen {
		t.Fatal("simulation changed after death")
	}
	s.Update(Controls{Pause: true})
	if s.Paused() {
		t.Fatal("Esc paused a finished attempt")
	}

	r, ok := s.FinishedReplay()
	if !ok {
		t.Fatal("no finished replay after death")
	}
	if _, again := s.FinishedReplay(); again {
		t.Fatal("finished replay handed out twice")
	}
	res, err := replay.Run(r, cave)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Check(res); err != nil {
		t.Fatalf("logged replay does not reproduce the death: %v", err)
	}
	if res.FinalHash.String() != fixture.Expect.FinalHash || res.TraceHash.String() != fixture.Expect.TraceHash {
		t.Fatal("logged replay disagrees with the golden death fixture")
	}

	path, err := replay.Save(t.TempDir(), r, time.Date(2026, 10, 18, 9, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(path); got != "cave_02-dead-20261018-093000.json" {
		t.Fatalf("saved as %s", got)
	}

	s.Update(Controls{Restart: true})
	if s.Ended() || s.Snapshot().Tick != 0 || s.Snapshot().Collected != 0 {
		t.Fatal("R did not restart after death")
	}
	if _, ok := s.FinishedReplay(); ok {
		t.Fatal("restart left a finished replay behind")
	}
}

func TestBoulderGlidesBetweenCells(t *testing.T) {
	s, err := NewSession(embeddedCave(t, "cave_02"), 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	// The demonstration boulder at (25,1) falls on tick 0.
	run(s, none, UpdatesPerTick)
	f := s.Frame()
	if len(f.Gliding) != 1 || f.Gliding[0].Cell != (sim.Pos{X: 25, Y: 2}) {
		t.Fatalf("gliding = %+v", f.Gliding)
	}
	lastY := f.Gliding[0].Y
	if lastY <= 1 || lastY >= 2 {
		t.Fatalf("glide started at y=%v, want between the cells", lastY)
	}
	for range BoulderSlideUpdates - 2 {
		s.Update(none)
		g := s.Frame().Gliding
		if len(g) == 0 {
			break
		}
		if g[0].X != 25 || g[0].Y < lastY {
			t.Fatalf("glide went backwards or sideways: %+v", g[0])
		}
		lastY = g[0].Y
	}
}

func TestPlaylistSwitchesCaveOnlyFromPauseOrEnd(t *testing.T) {
	all, err := level.LoadDir(assets.Levels, assets.LevelDir)
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := OrderLevels(all, assets.PlayOrder)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewPlaylist(ordered, 0, 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	p.Update(Controls{Next: true})
	if p.Index() != 0 {
		t.Fatal("N switched cave during play")
	}
	p.Update(Controls{Pause: true})
	p.Update(Controls{Next: true})
	if p.Index() != 1 || p.Session().Level().ID != "cave_02" || p.Session().Paused() {
		t.Fatalf("N from pause: index %d cave %s", p.Index(), p.Session().Level().ID)
	}
	for range 2 {
		p.Update(Controls{Pause: true})
		p.Update(Controls{Next: true})
	}
	if p.Index() != 0 {
		t.Fatalf("playlist did not wrap: index %d", p.Index())
	}
}

func TestPlayOrderListsEveryEmbeddedCaveOnce(t *testing.T) {
	all, err := level.LoadDir(assets.Levels, assets.LevelDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OrderLevels(all, assets.PlayOrder); err != nil {
		t.Fatalf("assets.PlayOrder is out of date: %v", err)
	}
	if _, err := OrderLevels(all, []string{"test_cave", "cave_02", "nope"}); err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("unknown id accepted: %v", err)
	}
	if _, err := OrderLevels(all, []string{"test_cave"}); err == nil {
		t.Fatal("a play order missing caves was accepted")
	}
	if i, ok := IndexOf(all, "cave_03"); !ok || all[i].ID != "cave_03" {
		t.Fatal("IndexOf failed")
	}
}
