// Command deepshift is the native desktop entrypoint: it opens one window,
// plays the embedded caves in order and renders them with Ebitengine. No web
// view, no server. Each finished attempt (completed or crushed) is saved as
// a replay under the user config directory, for bug reports.
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
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/townhill/deep-shift/assets"
	"github.com/townhill/deep-shift/internal/game"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/render"
	"github.com/townhill/deep-shift/internal/replay"
)

func main() {
	cave := flag.String("cave", "", "start at this embedded cave id (test_cave, cave_02, cave_03) or play this cave JSON file")
	replayDir := flag.String("replay-dir", defaultReplayDir(), "directory for replays of finished attempts (empty: do not save)")
	showVersion := flag.Bool("version", false, "print the build version and exit without opening a window")
	flag.Parse()

	build := buildVersion()
	if *showVersion {
		fmt.Println("deepshift", build)
		return
	}

	levels, start, err := loadLevels(*cave)
	if err != nil {
		log.Fatal(err)
	}
	playlist, err := game.NewPlaylist(levels, start, render.ViewTilesW, render.ViewTilesH)
	if err != nil {
		log.Fatal(err)
	}

	ebiten.SetWindowTitle("Deep Shift")
	ebiten.SetWindowSize(render.ScreenWidth*5/4, render.ScreenHeight*5/4)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(game.UpdatesPerSecond)
	a := &app{playlist: playlist, renderer: render.New(build), replayDir: *replayDir}
	if err := ebiten.RunGame(a); err != nil {
		log.Fatal(err)
	}
}

// loadLevels returns the caves to play and where to start. arg is empty
// (all embedded caves from the first), an embedded cave id, or a file path
// (that cave alone).
func loadLevels(arg string) ([]*level.Level, int, error) {
	embedded, err := level.LoadDir(assets.Levels, assets.LevelDir)
	if err != nil {
		return nil, 0, err
	}
	ordered, err := game.OrderLevels(embedded, assets.PlayOrder)
	if err != nil {
		return nil, 0, err
	}
	if arg == "" {
		return ordered, 0, nil
	}
	if i, ok := game.IndexOf(ordered, arg); ok {
		return ordered, i, nil
	}
	data, err := os.ReadFile(arg)
	if err != nil {
		return nil, 0, fmt.Errorf("-cave %q is neither an embedded cave id nor a readable file: %w", arg, err)
	}
	l, err := level.Parse(arg, data)
	if err != nil {
		return nil, 0, err
	}
	return []*level.Level{l}, 0, nil
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

// app adapts the playlist to ebiten.Game.
type app struct {
	playlist  *game.Playlist
	renderer  *render.Renderer
	replayDir string
}

func (a *app) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}
	a.playlist.Update(readControls())
	s := a.playlist.Session()
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
	a.renderer.Draw(screen, a.playlist.Session().Frame())
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
		Up:      held(ebiten.KeyArrowUp, ebiten.KeyW),
		Down:    held(ebiten.KeyArrowDown, ebiten.KeyS),
		Left:    held(ebiten.KeyArrowLeft, ebiten.KeyA),
		Right:   held(ebiten.KeyArrowRight, ebiten.KeyD),
		Pause:   inpututil.IsKeyJustPressed(ebiten.KeyEscape),
		Restart: inpututil.IsKeyJustPressed(ebiten.KeyR),
		Next:    inpututil.IsKeyJustPressed(ebiten.KeyN),
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
