package game

import (
	"fmt"
	"path/filepath"

	"github.com/townhill/deep-shift/internal/editor"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/sim"
)

// Screen is what the app is showing.
type Screen int

// Screens.
const (
	ScreenTitle    Screen = iota // title menu
	ScreenCaves                  // cave select
	ScreenPlaying                // a cave session (the playlist)
	ScreenEditList               // choose a cave to edit
	ScreenEditor                 // the cave editor (fed by UpdateEditor)
	ScreenPlaytest               // playing the cave in the editor
)

// Shell is the whole app above the simulation: the title menu, the cave
// select list, the playlist being played, and the cave editor with its
// playtests. Pure Go and display-free, like the rest of this package.
//
// Menu keys: Up/Down move the selection, Space/Enter (Interact) choose, Esc
// (Pause) goes back. While playing, C (Controls.Menu) opens the cave list
// from the pause, completion or death screen. In a playtest, P
// (Controls.Edit) returns to the editor at any time. After every screen
// change, input is ignored until every key has been released, so a key that
// chose a menu entry never leaks into the game as a move or a valve turn.
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

	editorOn  bool // EnableEditor was called: the title menu offers the editor
	layout    editor.Layout
	storage   editor.Storage
	editItems []editItem
	editor    *editor.Editor
	playtest  *Session
}

// editItem is one cave the edit list offers.
type editItem struct {
	group int // editNew, editYours or editBuiltIn: the heading it sits under
	label string
	open  func() (level.Draft, string, error) // the draft and the path saves go to
}

// Edit list groups, in order.
const (
	editNew = iota
	editYours
	editBuiltIn
)

var editHeadings = [...]string{editYours: "Your caves", editBuiltIn: "Copy a built-in cave"}

// Title menu choices, in order. The editor entry is offered once
// EnableEditor has been called.
const (
	titlePlay = iota
	titleChoose
	titleEdit
)

var titleEntries = [...]string{titlePlay: "Play the campaign", titleChoose: "Choose a cave", titleEdit: "Cave editor"}

// menuLines is how many entry lines (headings included) a menu screen
// shows; longer lists scroll with the selection.
const menuLines = 8

// playtestKeys are the overlay's navigation hints in a playtest.
const playtestKeys = "P: back to the editor"

// NewShell opens on the title menu. levels[:campaign] are the campaign caves
// in play order; any after them are extras.
func NewShell(levels []*level.Level, campaign, viewW, viewH int) (*Shell, error) {
	if len(levels) == 0 || campaign < 0 || campaign > len(levels) {
		return nil, fmt.Errorf("shell: %d campaign caves out of range for %d caves", campaign, len(levels))
	}
	return &Shell{levels: levels, campaign: campaign, viewW: viewW, viewH: viewH, completed: map[string]bool{}}, nil
}

// EnableEditor adds the cave editor to the title menu. layout is the
// renderer's editor layout; storage keeps the player's own caves.
func (s *Shell) EnableEditor(layout editor.Layout, storage editor.Storage) {
	s.editorOn, s.layout, s.storage = true, layout, storage
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

// OpenEditor opens the editor on d directly, skipping the menus (the -edit
// flag). path is where saves go: the file d was read from, or empty for a
// new cave or a copy. Leaving the editor shows the edit list.
func (s *Shell) OpenEditor(d level.Draft, path string) error {
	e, err := editor.New(d, path, s.layout, s.storage)
	if err != nil {
		return err
	}
	s.editor = e
	s.change(ScreenEditor)
	return nil
}

// Update advances the shell by one platform update. On the editor screen
// it does nothing: the platform layer calls UpdateEditor instead.
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
	case ScreenEditList:
		s.updateEditList(c, prev)
	case ScreenPlaytest:
		s.updatePlaytest(c)
	}
}

// UpdateEditor advances the editor by one platform update; it does nothing
// on other screens.
func (s *Shell) UpdateEditor(in editor.Input) {
	if s.screen != ScreenEditor {
		return
	}
	switch s.editor.Update(in) {
	case editor.RequestLeave:
		s.openEditList()
	case editor.RequestPlaytest:
		s.startPlaytest()
	}
}

func (s *Shell) titleCount() int {
	if s.editorOn {
		return len(titleEntries)
	}
	return titleEdit
}

func (s *Shell) updateTitle(c, prev Controls) {
	n := s.titleCount()
	switch {
	case c.Up && !prev.Up:
		s.cursor = (s.cursor + n - 1) % n
	case c.Down && !prev.Down:
		s.cursor = (s.cursor + 1) % n
	case c.Interact && !prev.Interact:
		switch s.cursor {
		case titlePlay:
			s.start(0)
		case titleChoose:
			s.cursor = 0
			s.change(ScreenCaves)
		case titleEdit:
			s.openEditList()
		}
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
		s.cursor = titleChoose
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

// openEditList lists the caves to edit: a new cave, the player's own caves
// and copies of the built-in ones. Coming back from the editor, the cave
// just edited is selected.
func (s *Shell) openEditList() {
	s.editItems = s.editItems[:0]
	s.editItems = append(s.editItems, editItem{group: editNew, label: "New cave", open: func() (level.Draft, string, error) {
		return editor.NewCave(), "", nil
	}})
	var notice string
	var files []editor.File
	if s.storage != nil {
		var err error
		if files, err = s.storage.List(); err != nil {
			notice = "Cannot list your caves: " + err.Error()
		}
	}
	selected := 0
	for _, f := range files {
		label := fmt.Sprintf("%s   (%s)", f.Draft.Title, filepath.Base(f.Path))
		if f.Err != nil {
			label = filepath.Base(f.Path) + "   (cannot open)"
		}
		if s.editor != nil && f.Path == s.editor.Path() {
			selected = len(s.editItems)
		}
		s.editItems = append(s.editItems, editItem{group: editYours, label: label, open: func() (level.Draft, string, error) {
			return f.Draft, f.Path, f.Err
		}})
	}
	for _, l := range s.levels {
		s.editItems = append(s.editItems, editItem{group: editBuiltIn, label: l.Title, open: func() (level.Draft, string, error) {
			return editor.CopyOf(l), "", nil
		}})
	}
	s.editor, s.playtest = nil, nil
	s.cursor = selected
	s.change(ScreenEditList)
	s.notice = notice
}

func (s *Shell) updateEditList(c, prev Controls) {
	n := len(s.editItems)
	switch {
	case c.Up && !prev.Up:
		s.cursor = (s.cursor + n - 1) % n
	case c.Down && !prev.Down:
		s.cursor = (s.cursor + 1) % n
	case c.Interact && !prev.Interact:
		d, path, err := s.editItems[s.cursor].open()
		if err == nil {
			err = s.OpenEditor(d, path)
		}
		if err != nil {
			s.notice = "Cannot edit it: " + err.Error()
		}
	case c.Pause:
		s.cursor = titleEdit
		s.change(ScreenTitle)
	}
}

// startPlaytest plays the editor's cave, which it has already validated
// with the game's own loader. Playtest attempts are not saved as replays:
// the cave may not be saved yet, and a replay is only useful beside its cave
// file.
func (s *Shell) startPlaytest() {
	p, err := NewSession(s.editor.Level(), s.viewW, s.viewH)
	if err != nil {
		s.editor.Notify("Cannot playtest: " + err.Error())
		return
	}
	p.SetNavKeys(playtestKeys)
	p.Notify("Playtest: P returns to the editor")
	s.playtest = p
	s.change(ScreenPlaytest)
}

func (s *Shell) updatePlaytest(c Controls) {
	if c.Edit {
		s.playtest = nil
		s.editor.Resume()
		s.editor.Notify("Back in the editor")
		s.change(ScreenEditor)
		return
	}
	s.playtest.Update(c)
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

// Playlist returns the playlist; nil until a cave has been started.
func (s *Shell) Playlist() *Playlist { return s.playlist }

// Session returns the session being played: the playtest on the playtest
// screen, otherwise the playlist's current cave (nil before any).
func (s *Shell) Session() *Session {
	if s.screen == ScreenPlaytest {
		return s.playtest
	}
	if s.playlist == nil {
		return nil
	}
	return s.playlist.Session()
}

// Editor returns the cave editor; nil unless a cave is open in it.
func (s *Shell) Editor() *editor.Editor { return s.editor }

// TextEntry reports whether typed letters belong to a text field, so the
// platform layer should not treat them as shortcuts.
func (s *Shell) TextEntry() bool {
	return s.screen == ScreenEditor && s.editor.TextEntry()
}

// Completed reports whether the cave with this id was completed during
// this run of the app. Progress is not saved between runs; playtests do not
// count.
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
// title, cave-select and edit-list screens.
func (s *Shell) Menu() MenuView {
	var v MenuView
	switch s.screen {
	case ScreenTitle:
		v = MenuView{
			Title:    "DEEP SHIFT",
			Subtitle: "Dig, collect, and let the cave do the rest",
			Footer:   "Up/Down: choose   Space/Enter: select   Cmd+Q: quit",
		}
		for i, e := range titleEntries[:s.titleCount()] {
			v.Entries = append(v.Entries, MenuEntry{Text: e, Selected: i == s.cursor})
		}
	case ScreenEditList:
		v = MenuView{
			Title:    "CAVE EDITOR",
			Subtitle: "Start a new cave, open one of yours, or copy a built-in one",
			Footer:   "Up/Down: choose   Space/Enter: open   Esc: back",
		}
		group := editNew
		for i, item := range s.editItems {
			if item.group != group {
				group = item.group
				v.Entries = append(v.Entries, MenuEntry{Text: editHeadings[group], Heading: true})
			}
			v.Entries = append(v.Entries, MenuEntry{Text: item.label, Selected: i == s.cursor})
		}
		v.Entries = scrollEntries(v.Entries)
	default:
		v = s.caveMenu()
	}
	if s.notice != "" {
		v.Footer = s.notice
	}
	return v
}

func (s *Shell) caveMenu() MenuView {
	v := MenuView{
		Title:  "CHOOSE A CAVE",
		Footer: "Up/Down: choose   Space/Enter: play   Esc: back",
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
	v.Entries = scrollEntries(v.Entries)
	return v
}

// scrollEntries cuts a long menu down to menuLines lines around the
// selection, marking the cut ends.
func scrollEntries(entries []MenuEntry) []MenuEntry {
	if len(entries) <= menuLines {
		return entries
	}
	sel := 0
	for i, e := range entries {
		if e.Selected {
			sel = i
		}
	}
	// Keep the selection away from the marker lines where possible.
	start := min(max(sel-menuLines/2, 0), len(entries)-menuLines)
	out := append([]MenuEntry(nil), entries[start:start+menuLines]...)
	if start > 0 {
		out[0] = MenuEntry{Text: "(more above)", Heading: true}
	}
	if start+menuLines < len(entries) {
		out[menuLines-1] = MenuEntry{Text: "(more below)", Heading: true}
	}
	return out
}
