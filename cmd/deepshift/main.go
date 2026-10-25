// Command deepshift is the native desktop entrypoint: it opens one window on
// the title menu, plays the embedded caves and renders them with Ebitengine.
// No web view, no server. Each finished attempt (completed, crushed or
// caught) is saved as a replay under the user config directory, for bug
// reports. The cave editor saves the player's own caves beside it.
//
// Ebitengine is initialised only here and in internal/render, so `go test`
// never needs a display. GUI behaviour is compile-only in cloud CI and still
// needs a hands-on Mac playtest.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/townhill/deep-shift/assets"
	"github.com/townhill/deep-shift/internal/audio"
	"github.com/townhill/deep-shift/internal/editor"
	"github.com/townhill/deep-shift/internal/game"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/perf"
	"github.com/townhill/deep-shift/internal/render"
	"github.com/townhill/deep-shift/internal/replay"
	"github.com/townhill/deep-shift/internal/sim"
)

func main() {
	cave := flag.String("cave", "", "start at this embedded cave id (cave_01 to cave_05, crawler_warren or test_cave) or play this cave JSON file")
	edit := flag.String("edit", "", "open the cave editor on this cave: a cave JSON file (saved back to it), an embedded cave id (edited as a copy) or \"new\"")
	replayDir := flag.String("replay-dir", userDir("replays"), "directory for replays of finished attempts (empty: do not save)")
	cavesDir := flag.String("caves-dir", userDir("caves"), "directory for caves made in the editor (empty: new caves cannot be saved)")
	showVersion := flag.Bool("version", false, "print the build version and exit without opening a window")
	noAudio := flag.Bool("no-audio", false, "do not open an audio device (for machines without sound output)")
	noMusic := flag.Bool("no-music", false, "start with the music off (toggle with B)")
	showPerf := flag.Bool("perf", false, "start with the performance overlay on (toggle with ` or F3)")
	flag.Parse()

	build := buildVersion()
	if *showVersion {
		fmt.Println("deepshift", build)
		return
	}

	if *cave != "" && *edit != "" {
		log.Fatal("-cave and -edit cannot be used together")
	}
	levels, campaign, start, err := loadLevels(*cave)
	if err != nil {
		log.Fatal(err)
	}
	shell, err := game.NewShell(levels, campaign, render.ViewTilesW, render.ViewTilesH)
	if err != nil {
		log.Fatal(err)
	}
	shell.EnableEditor(render.EditorLayout(), editor.DirStorage{Path: *cavesDir})
	if start >= 0 {
		if err := shell.Play(start); err != nil {
			log.Fatal(err)
		}
	}
	if *edit != "" {
		d, path, err := editTarget(*edit, levels)
		if err == nil {
			err = shell.OpenEditor(d, path)
		}
		if err != nil {
			log.Fatalf("-edit %q: %v", *edit, err)
		}
	}

	ebiten.SetWindowTitle("Deep Shift")
	ebiten.SetWindowSize(render.ScreenWidth*5/4, render.ScreenHeight*5/4)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(game.UpdatesPerSecond)
	a := &app{
		shell: shell, renderer: render.New(build), settings: game.NewSound(!*noAudio, !*noMusic), replayDir: *replayDir,
		perfOn: *showPerf, updates: perf.NewWindow(perfWindow), draws: perf.NewWindow(perfWindow),
	}
	if !*noAudio {
		a.sound = audio.New()
	}
	if err := ebiten.RunGame(a); err != nil {
		log.Fatal(err)
	}
}

// loadLevels returns the caves to offer, how many of them are the campaign,
// and which to start playing at once (-1: open on the title menu). arg is
// empty (title menu), an embedded cave id (play it), or a file path (play
// that cave alone).
func loadLevels(arg string) (levels []*level.Level, campaign, start int, err error) {
	embedded, err := level.LoadDir(assets.Levels, assets.LevelDir)
	if err != nil {
		return nil, 0, 0, err
	}
	ordered, err := game.OrderLevels(embedded, assets.PlayOrder)
	if err != nil {
		return nil, 0, 0, err
	}
	if arg == "" {
		return ordered, len(assets.Campaign), -1, nil
	}
	if i, ok := game.IndexOf(ordered, arg); ok {
		return ordered, len(assets.Campaign), i, nil
	}
	data, err := os.ReadFile(arg)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("-cave %q is neither an embedded cave id nor a readable file: %w", arg, err)
	}
	l, err := level.Parse(arg, data)
	if err != nil {
		return nil, 0, 0, err
	}
	return []*level.Level{l}, 0, 0, nil
}

// editTarget resolves the -edit argument: "new" for a new cave, an embedded
// cave id for a copy of it, or a cave file, which saves go back to. A file
// opens even if it does not validate yet, so work in progress can be fixed.
func editTarget(arg string, levels []*level.Level) (level.Draft, string, error) {
	if arg == "new" {
		return editor.NewCave(), "", nil
	}
	if l, ok := level.ByID(levels, arg); ok {
		return editor.CopyOf(l), "", nil
	}
	data, err := os.ReadFile(arg)
	if err != nil {
		return level.Draft{}, "", fmt.Errorf("neither \"new\", an embedded cave id nor a readable file: %w", err)
	}
	d, err := level.DecodeDraft(arg, data)
	if err != nil {
		return level.Draft{}, "", err
	}
	path, err := filepath.Abs(arg)
	return d, path, err
}

// userDir is <user config dir>/Deep Shift/<name>, which is
// ~/Library/Application Support/Deep Shift/<name> on macOS.
func userDir(name string) string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Deep Shift", name)
}

// perfWindow is how many updates and draws the overlay averages over: four
// seconds at 60 per second.
const perfWindow = 4 * game.UpdatesPerSecond

// app adapts the shell to ebiten.Game.
type app struct {
	shell     *game.Shell
	renderer  *render.Renderer
	sound     *audio.Player // nil with -no-audio: silent
	settings  *game.Sound   // M and B
	replayDir string

	perfOn         bool
	updates, draws *perf.Window
}

func (a *app) Update() error {
	start := time.Now()
	defer func() { a.updates.Add(time.Since(start)) }()
	// Letters typed into an editor text field are text, not shortcuts.
	shortcuts := !a.shell.TextEntry()
	if inpututil.IsKeyJustPressed(ebiten.KeyF3) || shortcuts && inpututil.IsKeyJustPressed(ebiten.KeyBackquote) {
		a.perfOn = !a.perfOn
		if !a.perfOn {
			log.Print("perf: " + strings.Join(a.perfLines(), "; "))
		}
	}
	if shortcuts && inpututil.IsKeyJustPressed(ebiten.KeyF) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}
	var notice string
	if shortcuts && inpututil.IsKeyJustPressed(ebiten.KeyM) {
		notice = a.settings.ToggleMute()
		a.sound.SetMuted(a.settings.Muted())
	}
	if shortcuts && inpututil.IsKeyJustPressed(ebiten.KeyB) {
		notice = a.settings.ToggleMusic()
	}
	// Exactly one of the shell's inputs per update, chosen by the screen at
	// its start, so a key that switches screens is never seen twice.
	if a.shell.Screen() == game.ScreenEditor {
		a.shell.UpdateEditor(readEditorInput())
	} else {
		a.shell.Update(readControls())
	}
	a.sound.SetMusic(a.settings.MusicPlaying(a.shell.Paused()))
	playtest := a.shell.Screen() == game.ScreenPlaytest
	if a.shell.Screen() != game.ScreenPlaying && !playtest {
		return nil
	}
	s := a.shell.Session()
	if notice != "" {
		s.Notify(notice)
	}
	for _, cue := range s.TakeCues() {
		a.sound.Play(cue)
	}
	// Playtests are not saved: the cave may not be saved yet, and a replay
	// is only useful beside its cave file.
	if r, ok := s.FinishedReplay(); ok && a.replayDir != "" && !playtest {
		path, err := replay.Save(a.replayDir, r, time.Now())
		if err != nil {
			log.Printf("saving replay: %v", err)
			s.Notify("Could not save the replay (see terminal)")
		} else {
			log.Printf("replay saved: %s", path)
			s.Notify("Replay saved: " + filepath.Base(path))
		}
	}
	return nil
}

func (a *app) Draw(screen *ebiten.Image) {
	start := time.Now()
	switch a.shell.Screen() {
	case game.ScreenPlaying, game.ScreenPlaytest:
		a.renderer.Draw(screen, a.shell.Session().Frame())
	case game.ScreenEditor:
		a.renderer.DrawEditor(screen, a.shell.Editor().View())
	default:
		a.renderer.DrawMenu(screen, a.shell.Menu())
	}
	a.draws.Add(time.Since(start))
	if a.perfOn {
		a.renderer.DrawPerf(screen, a.perfLines())
	}
}

// perfLines is the performance overlay: frame and update rates as
// Ebitengine measures them, and CPU time per update (which includes every
// simulation tick) and per draw (submitting draw calls, not GPU time).
func (a *app) perfLines() []string {
	ms := func(d time.Duration) string { return fmt.Sprintf("%.3f ms", float64(d)/float64(time.Millisecond)) }
	return []string{
		fmt.Sprintf("FPS %.1f  TPS %.1f", ebiten.ActualFPS(), ebiten.ActualTPS()),
		fmt.Sprintf("update avg %s  p95 %s (incl. simulation)", ms(a.updates.Avg()), ms(a.updates.P95())),
		fmt.Sprintf("draw   avg %s  p95 %s (CPU)", ms(a.draws.Avg()), ms(a.draws.P95())),
	}
}

func (a *app) Layout(int, int) (int, int) {
	return render.ScreenWidth, render.ScreenHeight
}

// readControls samples the keyboard once per update. Movement uses the held
// state; the session's own cadence decides when moves happen, so operating-
// system key repeat never affects the simulation. Ebitengine key codes are
// physical positions, so WASD stays under the same fingers on non-US layouts.
func readControls() game.Controls {
	held := func(keys ...ebiten.Key) bool {
		for _, k := range keys {
			if ebiten.IsKeyPressed(k) {
				return true
			}
		}
		return false
	}
	return game.Controls{
		Up:       held(ebiten.KeyArrowUp, ebiten.KeyW),
		Down:     held(ebiten.KeyArrowDown, ebiten.KeyS),
		Left:     held(ebiten.KeyArrowLeft, ebiten.KeyA),
		Right:    held(ebiten.KeyArrowRight, ebiten.KeyD),
		Interact: held(ebiten.KeySpace, ebiten.KeyEnter, ebiten.KeyNumpadEnter),
		Pause:    inpututil.IsKeyJustPressed(ebiten.KeyEscape),
		Restart:  inpututil.IsKeyJustPressed(ebiten.KeyR),
		Next:     inpututil.IsKeyJustPressed(ebiten.KeyN),
		Menu:     inpututil.IsKeyJustPressed(ebiten.KeyC),
		Edit:     inpututil.IsKeyJustPressed(ebiten.KeyP),
	}
}

// brushKeys choose the editor's palette brushes, in order.
var brushKeys = [...]ebiten.Key{
	ebiten.KeyDigit1, ebiten.KeyDigit2, ebiten.KeyDigit3, ebiten.KeyDigit4, ebiten.KeyDigit5,
	ebiten.KeyDigit6, ebiten.KeyDigit7, ebiten.KeyDigit8, ebiten.KeyDigit9,
}

// readEditorInput samples the mouse and keyboard for the cave editor. Cmd or
// Control (for non-Mac keyboards) turns Z, S and the arrows into commands;
// without them the arrows and WASD scroll and letters are shortcuts. The
// mouse position is in logical screen pixels, the editor layout's
// coordinates.
func readEditorInput() editor.Input {
	pressed := inpututil.IsKeyJustPressed
	held := func(keys ...ebiten.Key) bool {
		for _, k := range keys {
			if ebiten.IsKeyPressed(k) {
				return true
			}
		}
		return false
	}
	cmd := held(ebiten.KeyMeta, ebiten.KeyControl)
	shift := held(ebiten.KeyShift)
	mx, my := ebiten.CursorPosition()
	wx, wy := ebiten.Wheel()
	in := editor.Input{
		MouseX: mx, MouseY: my,
		Paint: ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft),
		Erase: ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight),
		Shift: shift, Alt: held(ebiten.KeyAlt),
		WheelX: wx, WheelY: wy,
		Settings:  pressed(ebiten.KeyTab),
		Back:      pressed(ebiten.KeyEscape),
		Enter:     pressed(ebiten.KeyEnter) || pressed(ebiten.KeyNumpadEnter),
		Backspace: repeating(ebiten.KeyBackspace),
	}
	arrows := map[ebiten.Key]sim.Dir{
		ebiten.KeyArrowUp: sim.DirUp, ebiten.KeyArrowDown: sim.DirDown,
		ebiten.KeyArrowLeft: sim.DirLeft, ebiten.KeyArrowRight: sim.DirRight,
	}
	if cmd {
		in.Undo = pressed(ebiten.KeyZ) && !shift
		in.Redo = pressed(ebiten.KeyZ) && shift || pressed(ebiten.KeyY)
		in.Save = pressed(ebiten.KeyS)
		for k, d := range arrows {
			switch {
			case pressed(k) && shift:
				in.Shrink = d
			case pressed(k):
				in.Grow = d
			}
		}
		return in
	}
	in.Up = held(ebiten.KeyArrowUp, ebiten.KeyW)
	in.Down = held(ebiten.KeyArrowDown, ebiten.KeyS)
	in.Left = held(ebiten.KeyArrowLeft, ebiten.KeyA)
	in.Right = held(ebiten.KeyArrowRight, ebiten.KeyD)
	in.Playtest = pressed(ebiten.KeyP)
	in.Grid = pressed(ebiten.KeyG)
	for i, k := range brushKeys {
		if pressed(k) {
			in.Brush = i + 1
		}
	}
	in.Chars = ebiten.AppendInputChars(nil)
	return in
}

// repeating reports a key press, then repeats it every 3 updates once the key
// has been held for half a second, like text-field key repeat.
func repeating(k ebiten.Key) bool {
	d := inpututil.KeyPressDuration(k)
	return d == 1 || d >= game.UpdatesPerSecond/2 && d%3 == 0
}

// buildVersion reports the VCS revision Go embedded at build time.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	rev, dirty := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "dev"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}
