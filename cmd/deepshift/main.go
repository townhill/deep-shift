// Command deepshift is the native desktop entrypoint: it opens one window,
// runs the test cave and renders it with Ebitengine. No web view, no server.
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
	"runtime/debug"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/townhill/deep-shift/assets"
	"github.com/townhill/deep-shift/internal/game"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/render"
)

func main() {
	cavePath := flag.String("cave", "", "play this cave JSON file instead of the built-in test cave")
	showVersion := flag.Bool("version", false, "print the build version and exit without opening a window")
	flag.Parse()

	build := buildVersion()
	if *showVersion {
		fmt.Println("deepshift", build)
		return
	}

	l, err := loadLevel(*cavePath)
	if err != nil {
		log.Fatal(err)
	}
	session, err := game.NewSession(l, render.ViewTilesW, render.ViewTilesH)
	if err != nil {
		log.Fatal(err)
	}

	ebiten.SetWindowTitle("Deep Shift")
	ebiten.SetWindowSize(render.ScreenWidth*5/4, render.ScreenHeight*5/4)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(game.UpdatesPerSecond)
	if err := ebiten.RunGame(&app{session: session, renderer: render.New(build)}); err != nil {
		log.Fatal(err)
	}
}

func loadLevel(path string) (*level.Level, error) {
	if path == "" {
		return level.Load(assets.Levels, assets.TestCave)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return level.Parse(path, data)
}

// app adapts the session to ebiten.Game.
type app struct {
	session  *game.Session
	renderer *render.Renderer
}

func (a *app) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}
	a.session.Update(readControls())
	return nil
}

func (a *app) Draw(screen *ebiten.Image) {
	a.renderer.Draw(screen, a.session.Frame())
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
