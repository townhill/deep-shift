package game

import (
	"slices"
	"testing"

	"github.com/townhill/deep-shift/internal/replay"
	"github.com/townhill/deep-shift/internal/sfx"
	"github.com/townhill/deep-shift/internal/sim"
)

// valveTick is when the cave_04 fixture turns the valve.
func valveTick(t *testing.T, r *replay.Replay) uint64 {
	t.Helper()
	for _, c := range r.Commands {
		if c.Action == replay.ActionInteract {
			return c.Tick
		}
	}
	t.Fatal("fixture has no interact command")
	return 0
}

// Space presses go through the same latch as moves, and the attempt the
// session records replays to the golden hashes.
func TestKeyboardPlayOfCave04MatchesTheGoldenReplay(t *testing.T) {
	fixture := loadFixture(t, "cave_04_complete.json")
	s, err := NewSession(embeddedCave(t, "cave_04"), 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	driveFixture(t, s, fixture)
	got := s.Replay("keyboard run")
	if !slices.Equal(got.Commands, fixture.Commands) {
		t.Fatalf("recorded %d commands, fixture has %d; first difference matters", len(got.Commands), len(fixture.Commands))
	}
	if got.Expect.FinalHash != fixture.Expect.FinalHash || got.Expect.TraceHash != fixture.Expect.TraceHash {
		t.Fatalf("session hashes %s / %s, fixture %s / %s",
			got.Expect.FinalHash, got.Expect.TraceHash, fixture.Expect.FinalHash, fixture.Expect.TraceHash)
	}
}

// besideTheValve returns a cave_04 session with the player beside the valve
// and the action window open.
func besideTheValve(t *testing.T) *Session {
	t.Helper()
	fixture := loadFixture(t, "cave_04_complete.json")
	s, err := NewSession(embeddedCave(t, "cave_04"), 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	driveFixtureUntil(t, s, fixture, valveTick(t, fixture))
	if v, ok := s.sim.AdjacentValve(); !ok || v != (sim.Pos{X: 12, Y: 6}) || !s.sim.MoveReady() {
		t.Fatalf("not beside the valve: player %s", s.Snapshot().Player)
	}
	return s
}

func TestHoldingSpaceTurnsTheValveOnce(t *testing.T) {
	s := besideTheValve(t)
	if got := s.HUD().Prompt; got != "Space: open the sluice gate" {
		t.Fatalf("prompt beside the valve = %q", got)
	}
	before := len(s.Commands())
	run(s, Controls{Interact: true}, 30*UpdatesPerTick)
	cmds := s.Commands()[before:]
	if len(cmds) != 1 || cmds[0].Action != replay.ActionInteract {
		t.Fatalf("holding Space recorded %v, want one interact", cmds)
	}
	if s.Snapshot().Cell(sim.Pos{X: 12, Y: 6}).Terrain != sim.TerrainValveOpen {
		t.Fatal("the valve did not open")
	}
	if got := s.HUD().Prompt; got != "Sluice gate open" {
		t.Fatalf("prompt after opening = %q", got)
	}
}

func TestSpacePressDuringCooldownIsKeptAndAwayFromAValveIsDropped(t *testing.T) {
	s := besideTheValve(t)
	beside := s.Snapshot().Player
	moveAndArrive := func(c Controls, want sim.Pos) {
		t.Helper()
		run(s, c, 1)
		for i := 0; s.Snapshot().Player != want; i++ {
			if i > 2*MoveIntervalUpdates() {
				t.Fatalf("player at %s, want %s", s.Snapshot().Player, want)
			}
			s.Update(none)
		}
	}
	moveAndArrive(Controls{Left: true}, beside.Add(sim.DirLeft))
	before := len(s.Commands())
	run(s, Controls{Interact: true}, 1) // not beside the valve: dropped
	run(s, none, MoveIntervalUpdates())
	if cmds := s.Commands()[before:]; len(cmds) != 0 {
		t.Fatalf("an Interact press away from the valve was recorded: %v", cmds)
	}
	moveAndArrive(Controls{Right: true}, beside)
	if s.sim.MoveReady() {
		t.Fatal("expected to be inside the move's cooldown")
	}
	run(s, Controls{Interact: true}, 1) // during the cooldown: latched
	run(s, none, MoveIntervalUpdates())
	cmds := s.Commands()[before:]
	if len(cmds) != 2 || cmds[0].Action != "right" || cmds[1].Action != replay.ActionInteract {
		t.Fatalf("commands %v, want the move then the latched interact", cmds)
	}
}

// MoveIntervalUpdates is one movement interval in platform updates.
func MoveIntervalUpdates() int { return sim.MoveIntervalTicks * UpdatesPerTick }

func TestTooDeepPromptAndWaterEasing(t *testing.T) {
	fixture := loadFixture(t, "cave_04_complete.json")
	s, err := NewSession(embeddedCave(t, "cave_04"), 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	driveFixtureUntil(t, s, fixture, 65) // the fixture bumps the deep shaft at tick 64
	if got := s.HUD().Prompt; got != "Too deep to wade: find a way to lower the water" {
		t.Fatalf("prompt after bumping deep water = %q", got)
	}
	driveFixtureUntil(t, s, fixture, valveTick(t, fixture)+1)
	// The gates opened on this tick; the shaft's foot drains over the next
	// world steps. Displayed levels move smoothly and always end exact.
	foot := sim.Pos{X: 13, Y: 10}
	i := foot.Y*s.Snapshot().Width + foot.X
	sawFraction := false
	for range 40 * UpdatesPerTick {
		s.Update(none)
		w := s.Frame().Water[i]
		if w != float64(int(w)) {
			sawFraction = true
		}
		if w < 0 || w > sim.WaterCapacity {
			t.Fatalf("displayed level %v out of range", w)
		}
	}
	if !sawFraction {
		t.Fatal("water levels jumped instead of easing")
	}
	run(s, none, ObjectSlideUpdates)
	f := s.Frame()
	for j, c := range f.Snapshot.Cells {
		if f.Water[j] != float64(c.Water) {
			t.Fatalf("cell %d displays %v after easing, snapshot has %d", j, f.Water[j], c.Water)
		}
	}
}

// Every kind of feedback the cave 04 run produces is voiced: digging,
// collecting, the deep-water refusal, the valve and gates, the exit opening
// and the completion.
func TestSessionVoicesTheEventsOfAWholeRun(t *testing.T) {
	fixture := loadFixture(t, "cave_04_complete.json")
	s, err := NewSession(embeddedCave(t, "cave_04"), 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	heard := map[sfx.Cue]int{}
	drive(t, func(c Controls) {
		s.Update(c)
		for _, cue := range s.TakeCues() {
			heard[cue]++
		}
	}, func() *Session { return s }, fixture, fixture.Ticks)
	for _, want := range []sfx.Cue{sfx.CueDig, sfx.CueCollect, sfx.CueBlocked, sfx.CueValve, sfx.CueGate, sfx.CueExitOpen, sfx.CueComplete, sfx.CueLand} {
		if heard[want] == 0 {
			t.Errorf("never heard %s; heard %v", want, heard)
		}
	}
	if heard[sfx.CueComplete] != 1 || heard[sfx.CueValve] != 1 {
		t.Errorf("complete %d times, valve %d times; want once each", heard[sfx.CueComplete], heard[sfx.CueValve])
	}
	if heard[sfx.CueCrush] != 0 {
		t.Error("heard a crush in a run without one")
	}
	if len(s.TakeCues()) != 0 {
		t.Error("TakeCues did not forget the cues it returned")
	}
}

func TestCrushIsVoicedAndPauseIsSilent(t *testing.T) {
	fixture := loadFixture(t, "cave_02_crushed.json")
	s, err := NewSession(embeddedCave(t, "cave_02"), 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	s.Update(Controls{Pause: true})
	run(s, none, 60)
	if cues := s.TakeCues(); len(cues) != 0 {
		t.Fatalf("a paused session made sounds: %v", cues)
	}
	s.Update(Controls{Pause: true})
	var crush bool
	drive(t, func(c Controls) {
		s.Update(c)
		crush = crush || slices.Contains(s.TakeCues(), sfx.CueCrush)
	}, func() *Session { return s }, fixture, fixture.Ticks)
	if !crush {
		t.Fatal("the crush was not voiced")
	}
}
