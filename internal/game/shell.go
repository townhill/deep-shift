package game

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/townhill/deep-shift/internal/editor"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/replay"
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
// select list (the built-in caves and the player's own), the playlist being
// played, and the cave editor with its playtests. Pure Go and display-free,
// like the rest of this package.
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
	caveItems   []caveItem // the cave list, rebuilt each time it opens
	playlist    *Playlist
	playItems   []caveItem // the caves in the playlist, in its order
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

// caveItem is one entry of the cave list: a built-in cave or one of the
// player's own. One of the player's caves that cannot be played is still
// listed, saying why, so a cave saved with problems is not just missing.
type caveItem struct {
	level *level.Level // nil when the cave cannot be played
	path  string       // the file of one of the player's caves; empty for a built-in cave
	label string
	why   string // why the cave cannot be played
}

// same reports whether a and b are the same cave, across rebuilds of the
// list: the same file, or the same built-in level.
func (a caveItem) same(b caveItem) bool {
	if a.path != "" || b.path != "" {
		return a.path == b.path
	}
	return a.level == b.level
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

// Play starts levels[i] directly, skipping the menus (the -cave flag). N
// then moves through the built-in caves.
func (s *Shell) Play(i int) error {
	items := make([]caveItem, len(s.levels))
	for j, l := range s.levels {
		items[j] = caveItem{level: l, label: l.Title}
	}
	return s.playFrom(items, i)
}

// playFrom starts a playlist of the playable caves in items at items[i].
func (s *Shell) playFrom(items []caveItem, i int) error {
	var levels []*level.Level
	var play []caveItem
	start := 0
	for j, it := range items {
		if it.level == nil {
			continue
		}
		if j == i {
			start = len(levels)
		}
		levels = append(levels, it.level)
		play = append(play, it)
	}
	p, err := NewPlaylist(levels, start, s.viewW, s.viewH)
	if err != nil {
		return err
	}
	s.playlist, s.playItems = p, play
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
			s.openCaveList()
			s.cursor = 0
		case titleEdit:
			s.openEditList()
		}
	}
}

func (s *Shell) updateCaves(c, prev Controls) {
	n := len(s.caveItems)
	switch {
	case c.Up && !prev.Up:
		s.cursor = (s.cursor + n - 1) % n
	case c.Down && !prev.Down:
		s.cursor = (s.cursor + 1) % n
	case c.Interact && !prev.Interact:
		it := s.caveItems[s.cursor]
		if it.level == nil {
			s.notice = "Cannot play " + it.why
			return
		}
		if err := s.playFrom(s.caveItems, s.cursor); err != nil {
			s.notice = fmt.Sprintf("Cannot open %s: %v", it.level.ID, err)
		}
	case c.Pause:
		s.cursor = titleChoose
		s.change(ScreenTitle)
	}
}

// openCaveList lists the built-in caves, then the player's own caves from
// the editor's storage, read afresh so caves saved since appear.
func (s *Shell) openCaveList() {
	s.caveItems = s.caveItems[:0]
	used := map[string]string{} // cave id → what already uses it
	for _, l := range s.levels {
		s.caveItems = append(s.caveItems, caveItem{level: l, label: l.Title})
		used[l.ID] = "a built-in cave"
	}
	var notice string
	var files []editor.File
	if s.storage != nil {
		var err error
		if files, err = s.storage.List(); err != nil {
			notice = "Cannot list your caves: " + err.Error()
		}
	}
	for _, f := range files {
		name := filepath.Base(f.Path)
		it := caveItem{path: f.Path, label: f.Draft.Title}
		if f.Err != nil {
			it.label, it.why = name, name+": the file cannot be read"
		} else {
			l, err := level.Parse(name, f.Draft.Encode())
			var ve *level.ValidationError
			switch {
			case errors.As(err, &ve):
				it.why = fmt.Sprintf("%s: %s; fix it in the Cave editor", name, problems(len(ve.Problems)))
			case err != nil:
				it.why = name + ": " + err.Error()
			case used[l.ID] != "":
				it.why = fmt.Sprintf("%s: its id %q is taken by %s", name, l.ID, used[l.ID])
			default:
				it.level = l
				used[l.ID] = name
			}
		}
		s.caveItems = append(s.caveItems, it)
	}
	s.change(ScreenCaves)
	s.notice = notice
}

func problems(n int) string {
	if n == 1 {
		return "1 problem"
	}
	return fmt.Sprintf("%d problems", n)
}

func (s *Shell) updatePlaying(c Controls) {
	session := s.playlist.Session()
	if c.Menu && (session.Paused() || session.Ended()) {
		current := s.playItems[s.playlist.Index()]
		s.openCaveList()
		s.cursor = 0
		for i, it := range s.caveItems {
			if it.same(current) {
				s.cursor = i
			}
		}
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

// FinishedAttempt reports an attempt that has just ended (completed or
// died), once per attempt, with its replay for the platform layer to save.
//
// The replay is nil for a playtest of a cave with unsaved changes: no cave
// file matches it, so nothing could re-run it. A replay of one of the
// player's own caves, played from the list or playtested, names the cave
// file, since cavetool finds only the built-in caves by id.
func (s *Shell) FinishedAttempt() (*replay.Replay, bool) {
	var session *Session
	var path string
	switch s.screen {
	case ScreenPlaying:
		session, path = s.playlist.Session(), s.playItems[s.playlist.Index()].path
	case ScreenPlaytest:
		session, path = s.playtest, s.editor.Path()
	default:
		return nil, false
	}
	r, ok := session.FinishedReplay()
	if !ok {
		return nil, false
	}
	if s.screen == ScreenPlaytest {
		if path == "" || s.editor.Dirty() {
			return nil, true
		}
		r.Description = strings.Replace(r.Description, "Recorded in the app", "Recorded in an editor playtest", 1)
	}
	if path != "" {
		r.Description += fmt.Sprintf(" Cave file: %s (cavetool replay needs it as -cave).", path)
	}
	return r, true
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
// count, but the player's own caves played from the list do.
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
	for i, it := range s.caveItems {
		switch {
		case i == s.campaign && i < len(s.levels):
			v.Entries = append(v.Entries, MenuEntry{Text: "Extras", Heading: true})
		case i == len(s.levels):
			v.Entries = append(v.Entries, MenuEntry{Text: "My caves", Heading: true})
		}
		label := "   " + it.label
		if i < s.campaign {
			label = fmt.Sprintf("%d  %s", i+1, it.label)
		}
		switch {
		case it.level == nil:
			label += "   (cannot play)"
		case s.completed[it.level.ID]:
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
