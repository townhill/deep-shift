package game

import (
	"fmt"

	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/sim"
)

// Screen is what the app is showing.
type Screen int

// Screens.
const (
	ScreenTitle   Screen = iota // title menu
	ScreenCaves                 // cave select
	ScreenPlaying               // a cave session (the playlist)
)

// Shell is the whole app above the simulation: the title menu, the cave
// select list and the playlist being played. Pure Go and display-free, like
// the rest of this package.
//
// Menu keys: Up/Down move the selection, Space/Enter (Interact) choose, Esc
// (Pause) goes back. While playing, C (Controls.Menu) opens the cave list
// from the pause, completion or death screen. After every screen change,
// input is ignored until every key has been released, so a key that chose a
// menu entry never leaks into the game as a move or a valve turn.
type Shell struct {
	levels       []*level.Level
	campaign     int // levels[:campaign] are the campaign, the rest extras
	viewW, viewH int

	screen      Screen
	cursor      int
	playlist    *Playlist
	completed   map[string]bool
	prev        Controls
	waitRelease bool
	notice      string
}

// titleEntries are the title menu's choices, in order.
var titleEntries = [...]string{"Play the campaign", "Choose a cave"}

// NewShell opens on the title menu. levels[:campaign] are the campaign caves
// in play order; any after them are extras.
func NewShell(levels []*level.Level, campaign, viewW, viewH int) (*Shell, error) {
	if len(levels) == 0 || campaign < 0 || campaign > len(levels) {
		return nil, fmt.Errorf("shell: %d campaign caves out of range for %d caves", campaign, len(levels))
	}
	return &Shell{levels: levels, campaign: campaign, viewW: viewW, viewH: viewH, completed: map[string]bool{}}, nil
}

// Play starts levels[i] directly, skipping the menus (the -cave flag).
func (s *Shell) Play(i int) error {
	p, err := NewPlaylist(s.levels, i, s.viewW, s.viewH)
	if err != nil {
		return err
	}
	s.playlist = p
	s.change(ScreenPlaying)
	return nil
}

// Update advances the shell by one platform update.
func (s *Shell) Update(c Controls) {
	prev := s.prev
	s.prev = c
	if s.waitRelease {
		if c != (Controls{}) {
			return
		}
		s.waitRelease = false
	}
	switch s.screen {
	case ScreenTitle:
		s.updateTitle(c, prev)
	case ScreenCaves:
		s.updateCaves(c, prev)
	case ScreenPlaying:
		s.updatePlaying(c)
	}
}

func (s *Shell) updateTitle(c, prev Controls) {
	switch {
	case c.Up && !prev.Up:
		s.cursor = (s.cursor + len(titleEntries) - 1) % len(titleEntries)
	case c.Down && !prev.Down:
		s.cursor = (s.cursor + 1) % len(titleEntries)
	case c.Interact && !prev.Interact:
		if s.cursor == 0 {
			s.start(0)
			return
		}
		s.cursor = 0
		s.change(ScreenCaves)
	}
}

func (s *Shell) updateCaves(c, prev Controls) {
	switch {
	case c.Up && !prev.Up:
		s.cursor = (s.cursor + len(s.levels) - 1) % len(s.levels)
	case c.Down && !prev.Down:
		s.cursor = (s.cursor + 1) % len(s.levels)
	case c.Interact && !prev.Interact:
		s.start(s.cursor)
	case c.Pause:
		s.cursor = 1
		s.change(ScreenTitle)
	}
}

func (s *Shell) updatePlaying(c Controls) {
	session := s.playlist.Session()
	if c.Menu && (session.Paused() || session.Ended()) {
		s.cursor = s.playlist.Index()
		s.change(ScreenCaves)
		return
	}
	s.playlist.Update(c)
	if session = s.playlist.Session(); session.Snapshot().Status == sim.StatusComplete {
		s.completed[session.Level().ID] = true
	}
}

func (s *Shell) start(i int) {
	if err := s.Play(i); err != nil {
		s.notice = fmt.Sprintf("Cannot open %s: %v", s.levels[i].ID, err)
	}
}

func (s *Shell) change(to Screen) {
	s.screen = to
	s.waitRelease = true
	s.notice = ""
}

// Screen returns the screen being shown.
func (s *Shell) Screen() Screen { return s.screen }

// Paused reports whether a cave is on screen and paused. A paused cave left
// for the cave list is not on screen.
func (s *Shell) Paused() bool {
	return s.screen == ScreenPlaying && s.playlist.Session().Paused()
}

// Playlist returns the playlist; nil until a cave has been started.
func (s *Shell) Playlist() *Playlist { return s.playlist }

// Completed reports whether the cave with this id was completed during
// this run of the app. Progress is not saved between runs.
func (s *Shell) Completed(id string) bool { return s.completed[id] }

// MenuView is the text of a menu screen, for the renderer.
type MenuView struct {
	Title    string
	Subtitle string
	Entries  []MenuEntry
	Footer   string
}

// MenuEntry is one line of a menu.
type MenuEntry struct {
	Text     string
	Selected bool
	Heading  bool // a section label, not selectable
}

// Menu returns the current menu screen's text. It is only meaningful on the
// title and cave-select screens.
func (s *Shell) Menu() MenuView {
	if s.screen == ScreenTitle {
		v := MenuView{
			Title:    "DEEP SHIFT",
			Subtitle: "Dig, collect, and let the cave do the rest",
			Footer:   "Up/Down: choose   Space/Enter: select   M: sound   B: music   Cmd+Q: quit",
		}
		for i, e := range titleEntries {
			v.Entries = append(v.Entries, MenuEntry{Text: e, Selected: i == s.cursor})
		}
		if s.notice != "" {
			v.Footer = s.notice
		}
		return v
	}
	v := MenuView{
		Title:  "CHOOSE A CAVE",
		Footer: "Up/Down: choose   Space/Enter: play   Esc: back   M: sound   B: music",
	}
	done := 0
	for i, l := range s.levels {
		if i == s.campaign {
			v.Entries = append(v.Entries, MenuEntry{Text: "Extras", Heading: true})
		}
		label := fmt.Sprintf("%d  %s", i+1, l.Title)
		if i >= s.campaign {
			label = "   " + l.Title
		}
		if s.completed[l.ID] {
			label += "   (done)"
			if i < s.campaign {
				done++
			}
		}
		v.Entries = append(v.Entries, MenuEntry{Text: label, Selected: i == s.cursor})
	}
	v.Subtitle = fmt.Sprintf("Campaign: %d of %d caves completed this session", done, s.campaign)
	if s.notice != "" {
		v.Footer = s.notice
	}
	return v
}
