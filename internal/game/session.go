// Package game orchestrates one cave session between the platform layer and
// the simulation: fixed-ratio tick scheduling, keyboard intent, pause and
// restart, replay recording, camera and HUD text.
//
// It is pure Go with no Ebitengine import, so all of it is unit-tested
// without a display. cmd/deepshift translates real keyboard state into
// Controls and hands Frames to internal/render.
package game

import (
	"cmp"
	"crypto/sha256"
	"fmt"
	"hash"
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

// ObjectSlideUpdates is how long a boulder or crystal sprite glides between
// cells: one gravity interval, so a falling object moves continuously.
const ObjectSlideUpdates = sim.GravityIntervalTicks * UpdatesPerTick

// promptUpdates is how long transient HUD prompts stay visible.
const promptUpdates = 2 * UpdatesPerSecond

// Controls is the platform-neutral input for one update.
type Controls struct {
	Up, Down, Left, Right bool // held during this update
	Pause                 bool // pressed this update (edge-triggered)
	Restart               bool // pressed this update (edge-triggered)
	Next                  bool // pressed this update (edge-triggered); handled by Playlist
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
	trace    hash.Hash        // SHA-256 over every state hash this attempt (see replay.Result)
	finished bool             // the attempt ended and FinishedReplay has not been taken

	slideFrom, slideTo sim.Pos
	slide              int
	objects            map[int]objectSlide // gliding boulders and crystals by destination cell index
	crushedBy          sim.Occupant
	facing             sim.Dir
	prompt             string
	promptLeft         int
	camera             Camera
}

type objectSlide struct {
	fromX, fromY float64 // visual start, tile units
	to           sim.Pos
	object       sim.Occupant
	age          int // updates since the move
}

// NewSession starts a session on l with a view of viewW x viewH tiles.
func NewSession(l *level.Level, viewW, viewH int) (*Session, error) {
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		return nil, err
	}
	gs := &Session{level: l, sim: s, camera: Camera{ViewW: float64(viewW), ViewH: float64(viewH)}}
	gs.resetAttempt()
	gs.resetPresentation()
	gs.setPrompt("Collect crystals to unseal the exit")
	return gs, nil
}

// Update advances the session by one platform update.
func (s *Session) Update(c Controls) {
	s.trackDirections(c)
	ended := s.Ended()
	switch {
	case c.Restart && (s.paused || ended):
		s.Restart()
		return
	case c.Pause && !ended:
		s.paused = !s.paused
		s.pending = sim.DirNone // input is not consumed across a pause
	}
	if s.paused {
		return
	}
	if !ended {
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
	events := s.sim.Step(in)
	h := s.sim.StateHash()
	s.trace.Write(h[:])
	s.snap = s.sim.Snapshot()
	for _, e := range events {
		switch e.Kind {
		case sim.EventMoved:
			s.slideFrom, s.slideTo, s.slide = e.From, e.Pos, 0
			s.facing = in.Move
		case sim.EventFell, sim.EventRolled, sim.EventBoulderPushed:
			s.startObjectSlide(e.From, e.Pos, e.Object)
		case sim.EventPlayerCrushed:
			s.crushedBy = e.Object
		case sim.EventExitSealed:
			need := s.sim.Required() - s.sim.Collected()
			s.setPrompt(fmt.Sprintf("Exit sealed: collect %d more %s", need, plural(need, "crystal")))
		case sim.EventExitOpened:
			s.setPrompt("Quota met: the exit is open")
		}
	}
	if s.Ended() {
		s.finished = true
	}
}

// startObjectSlide begins a glide from the object's current visual position
// (it may still be gliding into from) to its new cell.
func (s *Session) startObjectSlide(from, to sim.Pos, object sim.Occupant) {
	fi, ti := s.index(from), s.index(to)
	fx, fy := float64(from.X), float64(from.Y)
	if prev, ok := s.objects[fi]; ok {
		fx, fy = prev.position()
		delete(s.objects, fi)
	}
	s.objects[ti] = objectSlide{fromX: fx, fromY: fy, to: to, object: object}
}

func (b objectSlide) position() (x, y float64) {
	t := float64(b.age) / ObjectSlideUpdates
	return b.fromX + (float64(b.to.X)-b.fromX)*t, b.fromY + (float64(b.to.Y)-b.fromY)*t
}

func (s *Session) index(p sim.Pos) int { return p.Y*s.snap.Width + p.X }

func (s *Session) advancePresentation() {
	if s.slide < SlideUpdates {
		s.slide++
	}
	for i, b := range s.objects {
		if b.age++; b.age >= ObjectSlideUpdates {
			delete(s.objects, i)
		} else {
			s.objects[i] = b
		}
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
	s.resetAttempt()
	s.resetPresentation()
	s.setPrompt("Cave restarted")
}

// resetAttempt clears the per-attempt record: commands and the trace hash,
// which starts with the tick-0 state hash just as a replay run does.
func (s *Session) resetAttempt() {
	s.commands = nil
	s.finished = false
	s.trace = sha256.New()
	h := s.sim.StateHash()
	s.trace.Write(h[:])
}

func (s *Session) resetPresentation() {
	s.objects = map[int]objectSlide{}
	s.crushedBy = sim.OccupantNone
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

// Ended reports whether the attempt is over (cave complete or player dead).
func (s *Session) Ended() bool { return s.sim.Status() != sim.StatusPlaying }

// Level returns the cave being played.
func (s *Session) Level() *level.Level { return s.level }

// Notify shows a transient HUD prompt, for messages from the platform layer.
func (s *Session) Notify(msg string) { s.setPrompt(msg) }

// Snapshot returns the latest simulation snapshot.
func (s *Session) Snapshot() sim.Snapshot { return s.snap }

// StateHash returns the simulation's current state hash.
func (s *Session) StateHash() sim.Hash { return s.sim.StateHash() }

// Commands returns a copy of the move inputs sent during this attempt.
func (s *Session) Commands() []replay.Command { return slices.Clone(s.commands) }

// Replay packages this attempt as a replay covering every tick so far, with
// an expect block (final and trace hashes) that re-running it must match.
// It returns nil before the first tick.
func (s *Session) Replay(description string) *replay.Replay {
	if s.sim.Tick() == 0 {
		return nil
	}
	r := &replay.Replay{
		FormatVersion: replay.FormatVersion,
		Description:   description,
		CaveID:        s.level.ID,
		CaveChecksum:  s.level.Checksum(),
		Seed:          s.sim.Seed(),
		RuleVersion:   sim.RuleVersion,
		Ticks:         s.sim.Tick(),
		Commands:      s.Commands(),
		Expect: &replay.Expect{
			FinalTick: s.sim.Tick(),
			Status:    s.sim.Status().String(),
			Collected: s.sim.Collected(),
			FinalHash: s.sim.StateHash().String(),
		},
	}
	var th sim.Hash
	copy(th[:], s.trace.Sum(nil))
	r.Expect.TraceHash = th.String()
	return r
}

// FinishedReplay returns the replay of an attempt that has just ended
// (completed or died), once per attempt, so the platform layer can log it.
func (s *Session) FinishedReplay() (*replay.Replay, bool) {
	if !s.finished {
		return nil, false
	}
	s.finished = false
	what := "completed the cave"
	if s.sim.Status() == sim.StatusDead {
		what = "was crushed"
	}
	return s.Replay(fmt.Sprintf("Recorded in the app: the player %s on %q at tick %d.", what, s.level.ID, s.sim.Tick())), true
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
			"R: play again   N: next cave",
		}
	case s.snap.Status == sim.StatusDead:
		h.Overlay = []string{
			"CRUSHED",
			fmt.Sprintf("A falling %s landed on you", s.crushedBy),
			"R: try again   N: next cave",
		}
	case s.paused:
		h.Overlay = []string{"PAUSED", "Esc: resume   R: restart   N: next cave"}
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
	// Gliding lists boulders and crystals still sliding into their cells, in
	// cell order; draw these at X, Y instead of at Cell.
	Gliding []GlidingObject
	HUD     HUD
}

// GlidingObject is a boulder or crystal drawn between cells.
type GlidingObject struct {
	Cell   sim.Pos
	X, Y   float64 // tile units
	Object sim.Occupant
}

// Frame returns the current view model.
func (s *Session) Frame() Frame {
	px, py := s.PlayerPosition()
	var gliding []GlidingObject
	for _, b := range s.objects {
		x, y := b.position()
		gliding = append(gliding, GlidingObject{Cell: b.to, X: x, Y: y, Object: b.object})
	}
	slices.SortFunc(gliding, func(a, b GlidingObject) int {
		return cmp.Or(cmp.Compare(a.Cell.Y, b.Cell.Y), cmp.Compare(a.Cell.X, b.Cell.X))
	})
	return Frame{
		Snapshot: s.snap,
		PlayerX:  px, PlayerY: py,
		CameraX: s.camera.X, CameraY: s.camera.Y,
		Facing:  s.facing,
		Gliding: gliding,
		HUD:     s.HUD(),
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
