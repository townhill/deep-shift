package game

import (
	"strings"
	"testing"

	"github.com/townhill/deep-shift/internal/editor"
)

func TestSoundKeys(t *testing.T) {
	type step struct {
		key                string // "M" or "B"
		notice             string
		muted, music, held bool // after the key: Muted(), MusicPlaying(false), MusicPlaying(true)
	}
	tests := []struct {
		name             string
		available, music bool
		steps            []step
	}{
		{"music off and on", true, true, []step{
			{"B", "Music off (B: on)", false, false, false},
			{"B", "Music on", false, true, false},
		}},
		{"M silences the music too", true, true, []step{
			{"M", "Sound off (M: on)", true, false, false},
			{"M", "Sound on", false, true, false},
		}},
		{"music turned on while muted waits for M", true, true, []step{
			{"M", "Sound off (M: on)", true, false, false},
			{"B", "Music off (B: on)", true, false, false},
			{"B", "Music on (sound is off: M)", true, false, false},
			{"M", "Sound on", false, true, false},
		}},
		{"-no-music starts with it off", true, false, []step{
			{"M", "Sound off (M: on)", true, false, false},
			{"M", "Sound on", false, false, false},
			{"B", "Music on", false, true, false},
		}},
		{"-no-audio", false, true, []step{
			{"B", "No sound: started with -no-audio", true, false, false},
			{"M", "No sound: started with -no-audio", true, false, false},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSound(tt.available, tt.music)
			if got, want := s.MusicPlaying(false), tt.available && tt.music; got != want {
				t.Fatalf("music at start: %v, want %v", got, want)
			}
			for i, st := range tt.steps {
				var notice string
				if st.key == "M" {
					notice = s.ToggleMute()
				} else {
					notice = s.ToggleMusic()
				}
				if notice != st.notice || s.Muted() != st.muted || s.MusicPlaying(false) != st.music || s.MusicPlaying(true) != st.held {
					t.Fatalf("step %d (%s): notice %q, muted %v, music %v, music while paused %v; want %q, %v, %v, %v",
						i+1, st.key, notice, s.Muted(), s.MusicPlaying(false), s.MusicPlaying(true), st.notice, st.muted, st.music, st.held)
				}
			}
		})
	}
}

func TestPausedCaveHoldsTheMusic(t *testing.T) {
	sh := newShell(t)
	s := NewSound(true, true)
	if sh.Paused() || !s.MusicPlaying(sh.Paused()) {
		t.Fatal("no music on the title menu")
	}
	press(sh, Controls{Interact: true})
	press(sh, Controls{Pause: true})
	if !sh.Paused() || s.MusicPlaying(sh.Paused()) {
		t.Fatal("music plays while the cave is paused")
	}
	// The cave list is a menu, so the music comes back even though the
	// cave behind it is still paused.
	press(sh, Controls{Menu: true})
	if sh.Screen() != ScreenCaves || sh.Paused() || !s.MusicPlaying(sh.Paused()) {
		t.Fatalf("screen %d: no music in the cave list", sh.Screen())
	}
}

func TestPausedPlaytestHoldsTheMusic(t *testing.T) {
	sh, _ := newEditorShell(t)
	if err := sh.OpenEditor(editor.CopyOf(embeddedCave(t, "cave_01")), ""); err != nil {
		t.Fatal(err)
	}
	s := NewSound(true, true)
	sh.UpdateEditor(editor.Input{})
	if sh.Paused() || !s.MusicPlaying(sh.Paused()) {
		t.Fatal("no music in the editor")
	}
	editorTap(sh, editor.Input{Playtest: true})
	press(sh, Controls{}) // release, as after any screen change
	press(sh, Controls{Pause: true})
	if sh.Screen() != ScreenPlaytest || !sh.Paused() || s.MusicPlaying(sh.Paused()) {
		t.Fatalf("screen %d: music plays while the playtest is paused", sh.Screen())
	}
	press(sh, Controls{Pause: true})
	if sh.Paused() || !s.MusicPlaying(sh.Paused()) {
		t.Fatal("the music did not resume with the playtest")
	}
}

// The HUD and menus draw text in the 2x debug font, 12 px a glyph, on a
// 960 px wide screen, the HUD from x = 8 (internal/render), so a line holds
// 79 characters.
func TestKeyHintsFitTheirLines(t *testing.T) {
	sh := newShell(t)
	title := sh.Menu().Footer
	press(sh, Controls{Down: true})
	press(sh, Controls{Interact: true})
	hints := []string{ControlsHint, title, sh.Menu().Footer}
	ed, _ := newEditorShell(t)
	openEditList(t, ed)
	hints = append(hints, ed.Menu().Footer)
	for _, hint := range hints {
		if len(hint) > 79 {
			t.Errorf("%q is %d characters, more than the 79 that fit", hint, len(hint))
		}
		if !strings.Contains(hint, "M: sound") || !strings.Contains(hint, "B: music") {
			t.Errorf("%q does not mention the M and B keys", hint)
		}
	}
}
