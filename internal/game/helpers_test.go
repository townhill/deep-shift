package game

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/townhill/deep-shift/assets"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/replay"
)

func loadFixture(t *testing.T, name string) *replay.Replay {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "replays", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := replay.Parse(path, data)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func embeddedCave(t *testing.T, id string) *level.Level {
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

// driveFixture plays a fixture through the session with keyboard-style
// controls: each command's key is pressed for the updates of exactly that
// tick and released before the next, so every press is a fresh edge.
func driveFixture(t *testing.T, s *Session, fixture *replay.Replay) {
	t.Helper()
	next := 0
	for !s.Ended() && s.Snapshot().Tick < fixture.Ticks {
		var c Controls
		tick := s.Snapshot().Tick
		if next < len(fixture.Commands) && fixture.Commands[next].Tick == tick {
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
			next++
		}
		for s.Snapshot().Tick == tick {
			s.Update(c)
		}
		s.Update(Controls{})
		if !s.Ended() && s.Snapshot().Tick != tick+1 {
			t.Fatal("release update unexpectedly crossed a tick")
		}
	}
}
