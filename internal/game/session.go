// Package game orchestrates one cave session between the platform layer and
// the simulation: fixed-ratio tick scheduling, keyboard intent, pause and
// restart, replay recording, camera and HUD text.
//
// It is pure Go with no Ebitengine import, so all of it is unit-tested
// without a display. cmd/deepshift translates real keyboard state into
// Controls and hands Frames to internal/render.
package game

import (
	"fmt"
	"slices"

	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/replay"
	"github.com/townhill/deep-shift/internal/sim"
)

// UpdatesPerSecond is the platform update rate (ebiten.SetTPS). Ebitengine
// calls Update at this fixed rate independent of the display refresh rate,
// catching up after slow frames, so 60 Hz and 120 Hz displays simulate at
// the same speed.
const UpdatesPerSecond = 60

// UpdatesPerTick is how many updates make one simulation tick (60/20 = 3).
const UpdatesPerTick = UpdatesPerSecond / sim.TicksPerSecond

// SlideUpdates is how long the player sprite glides between cells (100 ms).
// Purely visual and shorter than the move interval, so the sprite always
// arrives before the next move can start. Provisional.
const SlideUpdates = 6

// promptUpdates is how long transient HUD prompts stay visible.
const promptUpdates = 2 * UpdatesPerSecond

// Controls is the platform-neutral input for one update.
type Controls struct {
	Up, Down, Left, Right bool // held during this update
	Pause                 bool // pressed this update (edge-triggered)
	Restart               bool // pressed this update (edge-triggered)
}

func (c Controls) held(d sim.Dir) bool {
	switch d {
	case sim.DirUp:
		return c.Up
	case sim.DirDown:
		return c.Down
	case sim.DirLeft:
		return c.Left
	case sim.DirRight:
		return c.Right
	}
	return false
}

var directions = [...]sim.Dir{sim.DirUp, sim.DirDown, sim.DirLeft, sim.DirRight}

// Session is one play session of one cave.
type Session struct {
	level *level.Level
	sim   *sim.Sim
	snap  sim.Snapshot

	paused  bool
	phase   int       // updates since the last tick, 0..UpdatesPerTick-1
	held    [5]bool   // indexed by sim.Dir
	order   []sim.Dir // held directions, most recently pressed last
	pending sim.Dir   // a press not yet consumed by a movement window

	commands []replay.Command // every move input sent this attempt

	slideFrom, slideTo sim.Pos
	slide              int
	facing             sim.Dir
	prompt             string
	promptLeft         int
	camera             Camera
}

// NewSession starts a session on l with a view of viewW x viewH tiles.
func NewSession(l *level.Level, viewW, viewH int) (*Session, error) {
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		return nil, err
	}
	gs := &Session{level: l, sim: s, camera: Camera{ViewW: float64(viewW), ViewH: float64(viewH)}}
	gs.resetPresentation()
	gs.setPrompt("Collect crystals to unseal the exit")
	return gs, nil
}

// Update advances the session by one platform update.
func (s *Session) Update(c Controls) {
	s.trackDirections(c)
	complete := s.sim.Status() == sim.StatusComplete
	switch {
	case c.Restart && (s.paused || complete):
		s.Restart()
		return
	case c.Pause && !complete:
		s.paused = !s.paused
		s.pending = sim.DirNone // input is not consumed across a pause
	}
	if s.paused {
		return
	}
	if !complete {
		s.phase++
		if s.phase >= UpdatesPerTick {
			s.phase = 0
			s.tick()
		}
	}
	s.advancePresentation()
}

// trackDirections records press order and latches presses so that a tap
// shorter than a tick still produces a move. While paused, presses are
// tracked but not latched.
func (s *Session) trackDirections(c Controls) {
	for _, d := range directions {
		now := c.held(d)
		switch {
		case now && !s.held[d]:
			s.order = append(slices.DeleteFunc(s.order, func(o sim.Dir) bool { return o == d }), d)
			if !s.paused {
				s.pending = d
			}
		case !now && s.held[d]:
			s.order = slices.DeleteFunc(s.order, func(o sim.Dir) bool { return o == d })
		}
		s.held[d] = now
	}
}

// tick runs one simulation step. The move input is chosen only when the
// movement window is open, so recorded commands are exactly the effective
// ones: the latched press first, otherwise the most recently pressed key
// still held.
func (s *Session) tick() {
	var in sim.Input
	if s.sim.MoveReady() {
		in.Move = s.pending
		if in.Move == sim.DirNone && len(s.order) > 0 {
			in.Move = s.order[len(s.order)-1]
		}
		s.pending = sim.DirNone
	}
	if in.Move != sim.DirNone {
		s.commands = append(s.commands, replay.Command{Tick: s.sim.Tick(), Action: in.Move.String()})
	}
	from := s.sim.Player()
	events := s.sim.Step(in)
	s.snap = s.sim.Snapshot()
	for _, e := range events {
		switch e.Kind {
		case sim.EventMoved:
			s.slideFrom, s.slideTo, s.slide = from, e.Pos, 0
			s.facing = in.Move
		case sim.EventExitSealed:
			need := s.sim.Required() - s.sim.Collected()
			s.setPrompt(fmt.Sprintf("Exit sealed: collect %d more %s", need, plural(need, "crystal")))
		case sim.EventExitOpened:
			s.setPrompt("Quota met: the exit is open")
		}
	}
}

func (s *Session) advancePresentation() {
	if s.slide < SlideUpdates {
		s.slide++
	}
	if s.promptLeft > 0 {
		s.promptLeft--
	}
	px, py := s.PlayerPosition()
	s.camera.Follow(px+0.5, py+0.5, s.facing, s.snap.Width, s.snap.Height)
}

// Restart restores the cave's initial state, counters and seed and clears
// the recorded commands.
func (s *Session) Restart() {
	s.sim.Reset()
	s.paused = false
	s.phase = 0
	s.pending = sim.DirNone
	s.commands = nil
	s.resetPresentation()
	s.setPrompt("Cave restarted")
}

func (s *Session) resetPresentation() {
	s.snap = s.sim.Snapshot()
	s.slideFrom, s.slideTo, s.slide = s.snap.Player, s.snap.Player, SlideUpdates
	s.facing = sim.DirNone
	s.camera.Snap()
	px, py := s.PlayerPosition()
	s.camera.Follow(px+0.5, py+0.5, s.facing, s.snap.Width, s.snap.Height)
}

func (s *Session) setPrompt(p string) { s.prompt, s.promptLeft = p, promptUpdates }

// PlayerPosition is the player's interpolated position in tile units.
func (s *Session) PlayerPosition() (x, y float64) {
	t := float64(s.slide) / SlideUpdates
	lerp := func(a, b int) float64 { return float64(a) + (float64(b)-float64(a))*t }
	return lerp(s.slideFrom.X, s.slideTo.X), lerp(s.slideFrom.Y, s.slideTo.Y)
}

// Paused reports whether the session is paused.
func (s *Session) Paused() bool { return s.paused }

// Snapshot returns the latest simulation snapshot.
func (s *Session) Snapshot() sim.Snapshot { return s.snap }

// StateHash returns the simulation's current state hash.
func (s *Session) StateHash() sim.Hash { return s.sim.StateHash() }

// Commands returns a copy of the move inputs sent during this attempt.
func (s *Session) Commands() []replay.Command { return slices.Clone(s.commands) }

// Replay packages this attempt as a replay covering every tick so far.
func (s *Session) Replay(description string) *replay.Replay {
	return &replay.Replay{
		FormatVersion: replay.FormatVersion,
		Description:   description,
		CaveID:        s.level.ID,
		CaveChecksum:  s.level.Checksum(),
		Seed:          s.sim.Seed(),
		RuleVersion:   sim.RuleVersion,
		Ticks:         max(s.sim.Tick(), 1),
		Commands:      s.Commands(),
	}
}

// HUD is the text the renderer shows. Building it here keeps the wording
// testable without a display.
type HUD struct {
	Status  string   // top line: cave, quota, exit state, time
	Prompt  string   // second line: transient prompt or the controls hint
	Overlay []string // centred lines over the cave when paused or complete
}

// ControlsHint is shown when no prompt is active.
const ControlsHint = "Arrows/WASD: move   Esc: pause   F: fullscreen"

// HUD returns the current HUD text.
func (s *Session) HUD() HUD {
	exit := "SEALED"
	if s.snap.ExitOpen {
		exit = "OPEN"
	}
	h := HUD{
		Status: fmt.Sprintf("%s   Crystals %d/%d   Exit %s   Time %s",
			s.level.Title, s.snap.Collected, s.snap.Required, exit, clock(s.snap.Tick)),
		Prompt: ControlsHint,
	}
	if s.promptLeft > 0 {
		h.Prompt = s.prompt
	}
	switch {
	case s.snap.Status == sim.StatusComplete:
		h.Overlay = []string{
			"CAVE COMPLETE",
			fmt.Sprintf("%d %s in %s", s.snap.Collected, plural(s.snap.Collected, "crystal"), clock(s.snap.Tick)),
			"R: play again",
		}
	case s.paused:
		h.Overlay = []string{"PAUSED", "Esc: resume   R: restart cave"}
	}
	return h
}

// Frame is everything the renderer needs for one frame. It holds copies, so
// the renderer cannot influence the simulation.
type Frame struct {
	Snapshot         sim.Snapshot
	PlayerX, PlayerY float64 // interpolated, tile units
	CameraX, CameraY float64 // top-left of the view, tile units
	Facing           sim.Dir
	HUD              HUD
}

// Frame returns the current view model.
func (s *Session) Frame() Frame {
	px, py := s.PlayerPosition()
	return Frame{
		Snapshot: s.snap,
		PlayerX:  px, PlayerY: py,
		CameraX: s.camera.X, CameraY: s.camera.Y,
		Facing: s.facing,
		HUD:    s.HUD(),
	}
}

func clock(tick uint64) string {
	secs := tick / sim.TicksPerSecond
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
