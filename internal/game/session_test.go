package game

import (
	"strings"
	"testing"

	"github.com/townhill/deep-shift/assets"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/replay"
	"github.com/townhill/deep-shift/internal/sim"
)

func testCave(t *testing.T) *level.Level {
	t.Helper()
	l, err := level.Load(assets.Levels, assets.TestCave)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func newSession(t *testing.T) *Session {
	t.Helper()
	s, err := NewSession(testCave(t), 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func run(s *Session, c Controls, updates int) {
	for range updates {
		s.Update(c)
	}
}

var (
	none  = Controls{}
	right = Controls{Right: true}
	down  = Controls{Down: true}
)

func TestTicksAdvanceAtFixedRatio(t *testing.T) {
	s := newSession(t)
	run(s, none, 300)
	if got, want := s.Snapshot().Tick, uint64(300/UpdatesPerTick); got != want {
		t.Fatalf("tick = %d after 300 updates, want %d", got, want)
	}
}

func TestPauseStopsTicksAndInput(t *testing.T) {
	s := newSession(t)
	start := s.Snapshot().Player
	run(s, none, 6)
	tick := s.Snapshot().Tick

	s.Update(Controls{Pause: true})
	if !s.Paused() {
		t.Fatal("Esc did not pause")
	}
	run(s, right, 30) // pressed and held while paused...
	run(s, none, 30)  // ...and released before resuming
	if s.Snapshot().Tick != tick {
		t.Fatal("simulation advanced while paused")
	}
	if got := s.HUD().Overlay; len(got) == 0 || got[0] != "PAUSED" {
		t.Fatalf("pause overlay = %q", got)
	}

	s.Update(Controls{Pause: true})
	run(s, none, 60)
	if s.Paused() || s.Snapshot().Tick == tick {
		t.Fatal("session did not resume")
	}
	if s.Snapshot().Player != start {
		t.Fatal("a key pressed during pause moved the player after resuming")
	}
}

func TestTapShorterThanATickStillMovesOnce(t *testing.T) {
	s := newSession(t)
	s.Update(right) // held for a single update (1/3 of a tick)
	run(s, none, 60)
	if got, want := s.Snapshot().Player, (sim.Pos{X: 2, Y: 1}); got != want {
		t.Fatalf("player at %s, want %s", got, want)
	}
}

func TestHeldKeyRepeatsAtSimulationCadence(t *testing.T) {
	s := newSession(t)
	// 3 moves need ticks 0..8, i.e. 9 ticks = 27 updates; stop before the 4th.
	run(s, right, 9*UpdatesPerTick)
	run(s, none, 30)
	if got, want := s.Snapshot().Player, (sim.Pos{X: 4, Y: 1}); got != want {
		t.Fatalf("player at %s, want %s (one move per %d ticks)", got, want, sim.MoveIntervalTicks)
	}
}

func TestMostRecentlyPressedDirectionWins(t *testing.T) {
	s := newSession(t)
	run(s, right, 3)                              // first tick: move right to (2,1)
	run(s, Controls{Right: true, Down: true}, 15) // down pressed later, both held
	run(s, none, 30)
	if got, want := s.Snapshot().Player, (sim.Pos{X: 2, Y: 2}); got != want {
		t.Fatalf("player at %s, want %s", got, want)
	}
}

func TestRestartOnlyFromPauseOrCompletion(t *testing.T) {
	s := newSession(t)
	run(s, down, 30)
	moved := s.Snapshot()

	s.Update(Controls{Restart: true})
	if s.Snapshot().Player != moved.Player || s.Snapshot().Tick < moved.Tick {
		t.Fatal("R restarted the cave during play")
	}

	fresh := newSession(t)
	s.Update(Controls{Pause: true})
	s.Update(Controls{Restart: true})
	if s.Paused() {
		t.Fatal("restart left the session paused")
	}
	if s.StateHash() != fresh.StateHash() || len(s.Commands()) != 0 {
		t.Fatal("restart did not restore the initial state and clear recorded commands")
	}
}

// Drives the session with keyboard-style controls through a complete run of
// the test cave, then checks the recorded commands replay to the same hash.
func TestPlayedSessionReplaysToTheSameHash(t *testing.T) {
	fixture := func() *replay.Replay {
		r, err := replay.Parse("fixture", mustRead(t, "../../testdata/replays/test_cave_complete.json"))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}()
	s := newSession(t)
	// Press each fixture direction exactly when the fixture does, by holding
	// the key across that tick's updates only.
	next := 0
	for s.Snapshot().Status != sim.StatusComplete && s.Snapshot().Tick < fixture.Ticks {
		var c Controls
		if next < len(fixture.Commands) && fixture.Commands[next].Tick == s.Snapshot().Tick {
			switch fixture.Commands[next].Action {
			case "up":
				c.Up = true
			case "down":
				c.Down = true
			case "left":
				c.Left = true
			case "right":
				c.Right = true
			}
		}
		before := s.Snapshot().Tick
		for s.Snapshot().Tick == before {
			s.Update(c)
		}
		if next < len(fixture.Commands) && fixture.Commands[next].Tick == before {
			next++
		}
		// Release between ticks so every press is a fresh edge.
		s.Update(none)
		if s.Snapshot().Tick != before+1 {
			t.Fatal("release update unexpectedly crossed a tick")
		}
	}
	if s.Snapshot().Status != sim.StatusComplete {
		t.Fatalf("session did not complete the cave:\n%s", s.Snapshot().ASCII())
	}
	if got := s.HUD().Overlay; len(got) == 0 || got[0] != "CAVE COMPLETE" || !strings.Contains(strings.Join(got, " "), "R: play again") {
		t.Fatalf("completion overlay = %q", got)
	}

	r := s.Replay("recorded by TestPlayedSessionReplaysToTheSameHash")
	res, err := replay.Run(r, testCave(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.FinalHash != s.StateHash() {
		t.Fatal("replaying the session's recorded commands gave a different hash")
	}
	if got, want := res.FinalHash.String(), fixture.Expect.FinalHash; got != want {
		t.Fatalf("session final hash %s differs from the fixture's %s", got, want)
	}
}

func TestSealedExitPrompt(t *testing.T) {
	s := newSession(t)
	run(s, right, 4*sim.MoveIntervalTicks*UpdatesPerTick)
	if got, want := s.HUD().Prompt, "Exit sealed: collect 8 more crystals"; got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
	if !strings.Contains(s.HUD().Status, "Crystals 0/8   Exit SEALED") {
		t.Fatalf("status = %q", s.HUD().Status)
	}
}

func TestPlayerSlideIsInterpolatedAndBounded(t *testing.T) {
	s := newSession(t)
	run(s, right, UpdatesPerTick) // the move happens on this update
	s.Update(none)
	var last float64
	for i := range SlideUpdates + 2 {
		x, y := s.PlayerPosition()
		if x < last || x > 2 || y != 1 {
			t.Fatalf("update %d: sprite at (%v,%v), want monotonic within [1,2] on y=1", i, x, y)
		}
		last = x
		s.Update(none)
	}
	if x, _ := s.PlayerPosition(); x != 2 {
		t.Fatalf("slide ended at x=%v, want 2", x)
	}
}

func TestCameraTarget(t *testing.T) {
	tests := []struct {
		name         string
		focusX       float64
		focusY       float64
		facing       sim.Dir
		mapW, mapH   int
		wantX, wantY float64
	}{
		{"clamped at top-left", 1.5, 1.5, sim.DirNone, 40, 24, 0, 0},
		{"clamped at bottom-right", 39.5, 23.5, sim.DirNone, 40, 24, 10, 6},
		{"centred in the middle", 20, 12, sim.DirNone, 40, 24, 5, 3},
		{"lookahead right", 20, 12, sim.DirRight, 40, 24, 5 + LookaheadTiles, 3},
		{"lookahead up", 20, 12, sim.DirUp, 40, 24, 5, 3 - LookaheadTiles},
		{"lookahead still clamps", 38, 12, sim.DirRight, 40, 24, 10, 3},
		{"small cave is centred", 2, 2, sim.DirRight, 10, 6, -10, -6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Camera{ViewW: 30, ViewH: 18}
			x, y := c.Target(tt.focusX, tt.focusY, tt.facing, tt.mapW, tt.mapH)
			if x != tt.wantX || y != tt.wantY {
				t.Fatalf("target = (%v,%v), want (%v,%v)", x, y, tt.wantX, tt.wantY)
			}
		})
	}
}

func TestCameraEasesWithoutOvershoot(t *testing.T) {
	c := Camera{ViewW: 30, ViewH: 18}
	c.Follow(1, 1, sim.DirNone, 40, 24) // first call snaps to (0,0)
	for range 200 {
		c.Follow(39, 23, sim.DirNone, 40, 24)
		if c.X > 10 || c.Y > 6 {
			t.Fatalf("camera overshot to (%v,%v)", c.X, c.Y)
		}
	}
	if 10-c.X > 1e-6 || 6-c.Y > 1e-6 {
		t.Fatalf("camera settled at (%v,%v), want (10,6)", c.X, c.Y)
	}
}
