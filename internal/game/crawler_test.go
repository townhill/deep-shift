package game

import (
	"fmt"
	"strings"
	"testing"

	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/replay"
	"github.com/townhill/deep-shift/internal/sim"
)

// A crawler's catch ends the cave with its own overlay and replay wording,
// and the logged replay reproduces it.
func TestCaughtEndsTheAttemptWithItsOwnOverlay(t *testing.T) {
	fixture := loadFixture(t, "crawler_warren_caught.json")
	cave := embeddedCave(t, "crawler_warren")
	s, err := NewSession(cave, 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.HUD().Prompt, "keep the wall on their left") {
		t.Errorf("opening prompt in a crawler cave = %q", s.HUD().Prompt)
	}
	driveFixture(t, s, fixture)
	overlay := strings.Join(s.HUD().Overlay, " | ")
	if s.Snapshot().Status != sim.StatusDead || s.Frame().Death != DeathCaught ||
		!strings.HasPrefix(overlay, "CAUGHT | A crawler caught you: they keep the wall on their left | R: try again") {
		t.Fatalf("status %s, death %d, overlay %q", s.Snapshot().Status, s.Frame().Death, overlay)
	}
	r, ok := s.FinishedReplay()
	if !ok || !strings.Contains(r.Description, "was caught by a crawler") {
		t.Fatalf("finished replay %v: %+v", ok, r)
	}
	res, err := replay.Run(r, cave)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Check(res); err != nil || res.FinalHash.String() != fixture.Expect.FinalHash {
		t.Fatalf("logged replay: %v, final hash %s, fixture %s", err, res.FinalHash, fixture.Expect.FinalHash)
	}
	s.Restart()
	if s.Frame().Death != DeathNone || len(s.HUD().Overlay) != 0 {
		t.Fatalf("after restart: death %d, overlay %q", s.Frame().Death, s.HUD().Overlay)
	}
}

func tinyCave(t *testing.T, rows ...string) *level.Level {
	t.Helper()
	quoted := make([]string, len(rows))
	for i, r := range rows {
		quoted[i] = fmt.Sprintf("%q", r)
	}
	doc := fmt.Sprintf(`{"format_version": %d, "id": "tiny", "title": "Tiny", "width": %d, "height": %d,
		"seed": 1, "required_crystals": 0, "rows": [%s]}`, level.FormatVersion, len([]rune(rows[0])), len(rows), strings.Join(quoted, ","))
	l, err := level.Parse("tiny.json", []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// The overlay tells walking into a creature apart from a creature stepping
// into the player, even one the player has just walked up to, and names the
// creature and the wall it keeps.
func TestCaughtOverlaySaysWhoMoved(t *testing.T) {
	tests := []struct {
		row, want string
	}{
		{"#@↓E#", "You walked into a crawler"},
		{"#@ ←E#", "A crawler caught you: they keep the wall on their left"},
		{"#@▼E#", "You walked into a glimmer"},
		{"#@ ◀E#", "A glimmer caught you: they keep the wall on their right"},
	}
	for _, tt := range tests {
		t.Run(tt.row, func(t *testing.T) {
			w := len([]rune(tt.row))
			s, err := NewSession(tinyCave(t, strings.Repeat("#", w), tt.row, strings.Repeat("#", w)), 30, 18)
			if err != nil {
				t.Fatal(err)
			}
			for range UpdatesPerTick {
				s.Update(Controls{Right: true})
			}
			if h := s.HUD(); s.Frame().Death != DeathCaught || len(h.Overlay) < 2 || !strings.HasPrefix(h.Overlay[1], tt.want) {
				t.Fatalf("tick %d, death %d, overlay %q", s.Snapshot().Tick, s.Frame().Death, h.Overlay)
			}
		})
	}
}

// Crawlers glide over a whole crawler interval, facing their way.
func TestCrawlersGlideFacingTheirWay(t *testing.T) {
	s, err := NewSession(embeddedCave(t, "crawler_warren"), 30, 18)
	if err != nil {
		t.Fatal(err)
	}
	for range UpdatesPerTick {
		s.Update(Controls{}) // tick 0 is a crawler step
	}
	find := func() (GlidingObject, bool) {
		for _, g := range s.Frame().Gliding {
			if g.Cell == (sim.Pos{X: 5, Y: 3}) {
				return g, true
			}
		}
		return GlidingObject{}, false
	}
	g, ok := find()
	if !ok || g.Object != sim.OccupantCrawler || g.Facing != sim.DirLeft || g.Y != 3 || g.X <= 5 || g.X >= 6 {
		t.Fatalf("ring crawler glide %+v (found %v), gliding %+v", g, ok, s.Frame().Gliding)
	}
	for range CrawlerSlideUpdates - 2 {
		s.Update(Controls{})
	}
	if g, ok := find(); !ok || g.X <= 5 {
		t.Fatalf("glide ended before the crawler interval: %+v %v", g, ok)
	}
	s.Update(Controls{})
	if g, ok := find(); ok {
		t.Fatalf("glide still running after the crawler interval: %+v", g)
	}
}

// Whatever happens in the cave (falls, pushes, collections, crushed and
// drowned crawlers, shattered and drowned glimmers), every gliding sprite is
// headed for a cell that holds it, so nothing is drawn that is no longer
// there.
func TestGlidingSpritesAlwaysMatchTheCave(t *testing.T) {
	for _, tt := range []struct{ fixture, cave string }{
		{"crawler_warren_complete.json", "crawler_warren"},
		{"glimmer_hollow_complete.json", "glimmer_hollow"},
		{"cave_03_complete.json", "cave_03"},
		{"cave_05_boulder_route.json", "cave_05"},
	} {
		t.Run(tt.fixture, func(t *testing.T) {
			fixture := loadFixture(t, tt.fixture)
			s, err := NewSession(embeddedCave(t, tt.cave), 30, 18)
			if err != nil {
				t.Fatal(err)
			}
			for stop := uint64(1); !s.Ended() && stop <= fixture.Ticks; stop++ {
				driveFixtureUntil(t, s, fixture, stop)
				for _, g := range s.Frame().Gliding {
					if c := s.Snapshot().Cell(g.Cell); c.Occupant != g.Object {
						t.Fatalf("tick %d: a %s glides to %s, which holds %s", s.Snapshot().Tick, g.Object, g.Cell, c.Occupant)
					}
				}
			}
			if s.Snapshot().Status != sim.StatusComplete {
				t.Fatalf("status %s", s.Snapshot().Status)
			}
		})
	}
}
