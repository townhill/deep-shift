package game

import (
	"slices"
	"strings"
	"testing"

	"github.com/townhill/deep-shift/internal/replay"
	"github.com/townhill/deep-shift/internal/sfx"
	"github.com/townhill/deep-shift/internal/sim"
)

// A glimmer's catch ends the cave with an overlay naming it and the wall it
// keeps, the renderer learns who caught the player, and the logged replay
// reproduces the death.
func TestGlimmerCatchEndsTheAttemptWithItsOwnOverlay(t *testing.T) {
	fixture := loadFixture(t, "glimmer_hollow_caught.json")
	cave := embeddedCave(t, "glimmer_hollow")
	s, err := NewSession(cave, 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	if p := s.HUD().Prompt; p != "Glimmers keep the wall on their right: drop a rock on one for crystals" {
		t.Errorf("opening prompt in a glimmer cave = %q", p)
	}
	driveFixture(t, s, fixture)
	overlay := strings.Join(s.HUD().Overlay, " | ")
	if f := s.Frame(); f.Death != DeathCaught || f.Killer != sim.OccupantGlimmer ||
		!strings.HasPrefix(overlay, "CAUGHT | A glimmer caught you: they keep the wall on their right | R: try again") {
		t.Fatalf("death %d, killer %s, overlay %q", f.Death, f.Killer, overlay)
	}
	r, ok := s.FinishedReplay()
	if !ok || !strings.Contains(r.Description, "was caught by a glimmer") {
		t.Fatalf("finished replay %v: %+v", ok, r)
	}
	res, err := replay.Run(r, cave)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Check(res); err != nil || res.FinalHash.String() != fixture.Expect.FinalHash {
		t.Fatalf("logged replay: %v, final hash %s, fixture %s", err, res.FinalHash, fixture.Expect.FinalHash)
	}
}

// Glimmers glide facing their way; a shattered glimmer is announced with the
// crystals it left and a chime, and a drowned one with the news that it left
// none.
func TestGlimmerPromptsAndSounds(t *testing.T) {
	fixture := loadFixture(t, "glimmer_hollow_complete.json")
	s, err := NewSession(embeddedCave(t, "glimmer_hollow"), 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	driveFixtureUntil(t, s, fixture, 1) // tick 0 is a creature step
	if !slices.ContainsFunc(s.Frame().Gliding, func(g GlidingObject) bool {
		return g.Cell == (sim.Pos{X: 21, Y: 3}) && g.Object == sim.OccupantGlimmer && g.Facing == sim.DirRight
	}) {
		t.Fatalf("the ring glimmer is not gliding right into (21,3): %+v", s.Frame().Gliding)
	}
	for _, tt := range []struct {
		after  uint64 // the tick of the event
		prompt string
		cue    sfx.Cue
	}{
		{550, "A falling boulder shattered a glimmer into 6 crystals", sfx.CueShatter},
		{682, "The water drowned a glimmer: it left no crystals", sfx.CueCreatureGone},
	} {
		s.TakeCues()
		driveFixtureUntil(t, s, fixture, tt.after+1)
		if p := s.HUD().Prompt; p != tt.prompt {
			t.Errorf("after tick %d the prompt is %q, want %q", tt.after, p, tt.prompt)
		}
		if cues := s.TakeCues(); !slices.Contains(cues, tt.cue) {
			t.Errorf("after tick %d the cues are %v, want %s", tt.after, cues, tt.cue)
		}
	}
}

// A cave with both kinds of creature names the wall each keeps.
func TestOpeningPromptForBothCreatures(t *testing.T) {
	s, err := NewSession(tinyCave(t, "#######", "#@ ↑▲E#", "#######"), 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	if p := s.HUD().Prompt; p != "Crawlers keep the wall on their left, glimmers on their right" {
		t.Fatalf("opening prompt %q", p)
	}
}

// A glimmer shattered while still gliding into its cell stops being drawn
// at once, like a crushed crawler: the boulder falls into (1,1) and the
// glimmer steps into (1,2) below it on tick 0, and the boulder lands on it on
// tick 2, a third of the way through its glide. The boulder turns to crystal
// with the glimmer's cell and the cell it left.
func TestShatteredGlimmerStopsGliding(t *testing.T) {
	s, err := NewSession(tinyCave(t,
		"#O###@",
		"# ##E#",
		"# ◀###",
		"######",
	), 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	for s.Snapshot().Tick < 3 {
		s.Update(Controls{})
	}
	if s.Snapshot().Cell(sim.Pos{X: 1, Y: 2}).Occupant != sim.OccupantCrystal {
		t.Fatalf("the glimmer did not shatter:\n%s", s.Snapshot().ASCII())
	}
	for _, g := range s.Frame().Gliding {
		if g.Object == sim.OccupantGlimmer {
			t.Fatalf("the shattered glimmer is still drawn gliding: %+v", g)
		}
	}
	if p := s.HUD().Prompt; p != "A falling boulder shattered a glimmer into 3 crystals" {
		t.Fatalf("prompt %q", p)
	}
}
