// Command deepshift is the native desktop entrypoint: it opens one window on
// the title menu, plays the embedded caves and renders them with Ebitengine.
// No web view, no server. Each finished attempt (completed, crushed or
// caught) is saved as a replay under the user config directory, for bug
// reports.
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
	"github.com/townhill/deep-shift/internal/game"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/perf"
	"github.com/townhill/deep-shift/internal/render"
	"github.com/townhill/deep-shift/internal/replay"
)

func main() {
	cave := flag.String("cave", "", "start at this embedded cave id (cave_01 to cave_05, crawler_warren, glimmer_hollow or test_cave) or play this cave JSON file")
	replayDir := flag.String("replay-dir", defaultReplayDir(), "directory for replays of finished attempts (empty: do not save)")
	showVersion := flag.Bool("version", false, "print the build version and exit without opening a window")
	noAudio := flag.Bool("no-audio", false, "do not open an audio device (for machines without sound output)")
	showPerf := flag.Bool("perf", false, "start with the performance overlay on (toggle with ` or F3)")
	flag.Parse()

	build := buildVersion()
	if *showVersion {
		fmt.Println("deepshift", build)
		return
	}

	levels, campaign, start, err := loadLevels(*cave)
	if err != nil {
		log.Fatal(err)
	}
	shell, err := game.NewShell(levels, campaign, render.ViewTilesW, render.ViewTilesH)
	if err != nil {
		log.Fatal(err)
	}
	if start >= 0 {
		if err := shell.Play(start); err != nil {
			log.Fatal(err)
		}
	}

	ebiten.SetWindowTitle("Deep Shift")
	ebiten.SetWindowSize(render.ScreenWidth*5/4, render.ScreenHeight*5/4)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(game.UpdatesPerSecond)
	a := &app{
		shell: shell, renderer: render.New(build), replayDir: *replayDir,
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

// defaultReplayDir is <user config dir>/Deep Shift/replays, which is
// ~/Library/Application Support/Deep Shift/replays on macOS.
func defaultReplayDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Deep Shift", "replays")
}

// perfWindow is how many updates and draws the overlay averages over: four
// seconds at 60 per second.
const perfWindow = 4 * game.UpdatesPerSecond

// app adapts the shell to ebiten.Game.
type app struct {
	shell     *game.Shell
	renderer  *render.Renderer
	sound     *audio.Player // nil with -no-audio: silent
	replayDir string

	perfOn         bool
	updates, draws *perf.Window
}

func (a *app) Update() error {
	start := time.Now()
	defer func() { a.updates.Add(time.Since(start)) }()
	if inpututil.IsKeyJustPressed(ebiten.KeyBackquote) || inpututil.IsKeyJustPressed(ebiten.KeyF3) {
		a.perfOn = !a.perfOn
		if !a.perfOn {
			log.Print("perf: " + strings.Join(a.perfLines(), "; "))
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}
	toggleSound := inpututil.IsKeyJustPressed(ebiten.KeyM)
	if toggleSound {
		a.sound.SetMuted(!a.sound.Muted())
	}
	a.shell.Update(readControls())
	if a.shell.Screen() != game.ScreenPlaying {
		return nil
	}
	s := a.shell.Playlist().Session()
	switch {
	case toggleSound && a.sound == nil:
		s.Notify("No sound: started with -no-audio")
	case toggleSound && a.sound.Muted():
		s.Notify("Sound off (M: on)")
	case toggleSound:
		s.Notify("Sound on")
	}
	for _, cue := range s.TakeCues() {
		a.sound.Play(cue)
	}
	if r, ok := s.FinishedReplay(); ok && a.replayDir != "" {
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
	if a.shell.Screen() != game.ScreenPlaying {
		a.renderer.DrawMenu(screen, a.shell.Menu())
	} else {
		a.renderer.Draw(screen, a.shell.Playlist().Session().Frame())
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
	}
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
