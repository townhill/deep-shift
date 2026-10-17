# Deep Shift

Deep Shift is an original, native macOS 2D underground physics/puzzle adventure: dig through a cave, collect
crystals, meet the quota and reach the exit, with environmental physics (boulders, then water) to come. It is
written in Go with [Ebitengine](https://ebitengine.org) and runs as a local `.app`. There is no browser,
Electron, server or database.

The game takes design inspiration from the *rules-driven* arcade cave games of the 1980s. All of its code,
caves, art, names and rules are original to this project.

> **Status: Phase 0 + the first slice of Phase 1.** One test cave ("Proving Ground") has movement, digging,
> crystals, walls, a quota-sealed exit, pause/restart and a HUD. There are no boulders, water or creatures yet.
> See [docs/ROADMAP.md](docs/ROADMAP.md).

## What has and has not been verified

| Behaviour | How it was checked | Status |
|---|---|---|
| Rules: movement, bounds, walls, digging, collecting, exit quota, restart | `go test ./...` (table-driven, headless) | Verified on Linux; CI also runs them on macOS arm64 |
| Determinism: 100 identical runs → identical hashes at every tick | `internal/replay` test + golden fixtures | Verified (Linux); CI checks the same golden hashes on macOS arm64 |
| Cave files validate; broken caves give useful errors | `internal/level` tests, `cavetool validate` | Verified |
| macOS arm64 / amd64 executables compile | Cross-compile from Linux; native build on CI's macOS runner | Compile-only |
| `DeepShift.app` bundle builds and is ad-hoc signed | `scripts/build-macos-app.sh` on CI's macOS runner | Packaging checked in CI; **launch from Finder not yet verified** |
| Window, keyboard, rendering, HUD on a real Mac | — | **Not yet tested: pending a hands-on Mac playtest** |
| Rendering + key handling of the real app on Linux (Xvfb, Mesa software GL) | Ebitengine `exp/vmhost` headless capture, run once by hand | Observed in the cloud session; not macOS, not in CI ([D-014](docs/DECISIONS.md#d-014-ebitengine-expvmhost-headless-capture-assessed-not-adopted)) |

## Play it on a Mac (from source)

Requirements: macOS 13 Ventura or newer on Apple Silicon (Intel Macs should work but are untested), the Xcode
Command Line Tools, and Go 1.25 or newer.

```sh
xcode-select --install            # once; skip if already installed (Homebrew needs these too)
brew install go                   # or the installer from https://go.dev/dl/
git clone https://github.com/townhill/deep-shift.git
cd deep-shift
go run ./cmd/deepshift            # first run fetches Ebitengine (and Go 1.27.2 if yours is older)
```

`go.mod` pins the toolchain to `go1.27.2`. With any Go ≥ 1.25 installed, the `go` command fetches that exact
toolchain automatically (`GOTOOLCHAIN=auto`, the default).

To try a branch before it is merged, run `git switch <branch>` before `go run`.

### Build the `.app` yourself

```sh
scripts/build-macos-app.sh        # → dist/DeepShift.app and dist/DeepShift-macos-arm64.zip
open dist/DeepShift.app
```

A locally built app is not quarantined, so it opens without Gatekeeper prompts.

### Use the CI-built `.app`

Each CI run uploads `DeepShift-macos-arm64-unsigned-dev`. It is an **unsigned development build**: it has an
ad-hoc code signature only, with no Developer ID and no notarisation.

1. Download the artifact from the workflow run's *Summary* page. GitHub wraps it in a zip, so unzip that to
   get `DeepShift-macos-arm64.zip`.
2. Double-click `DeepShift-macos-arm64.zip` (or run `ditto -x -k DeepShift-macos-arm64.zip .`) to get
   `DeepShift.app` with its executable bit intact.
3. The first launch is blocked because the app was downloaded and is not notarised. Open it once, then go to
   **System Settings → Privacy & Security** and click **Open Anyway** for Deep Shift. Alternatively, clear
   the download quarantine on this one app only: `xattr -dr com.apple.quarantine DeepShift.app`.

Do **not** disable Gatekeeper system-wide (`spctl --master-disable`). Signing and notarisation are a later
release milestone.

### Controls

| Key | Action |
|---|---|
| Arrow keys / WASD | Move one tile, digging earth and collecting crystals. Holding a key repeats one move per 4 ticks (200 ms). |
| Esc | Pause / resume. Simulation and input stop while paused. |
| R | Restart the cave, from the pause screen or the completion screen. |
| F | Toggle fullscreen |
| Cmd+Q / close window | Quit |

WASD uses physical key positions, so it stays under the same fingers on non-US layouts. Space/Enter are
reserved for mechanisms (valves, gates) in later phases.

To play a different cave file: `go run ./cmd/deepshift -cave path/to/cave.json`. Validation errors are
printed to the terminal.

## Develop and test (any OS, no display needed)

```sh
go test ./...                     # all rules, level, replay and session tests; no window or GPU
go vet ./...
go run ./cmd/cavetool validate    # validate the embedded caves
go run ./cmd/cavetool show assets/levels/test_cave.json
go run ./cmd/cavetool replay testdata/replays/*.json   # re-run golden replays and check their hashes
go test -run '^$' -bench . ./internal/sim              # tick/hash benchmarks on a 60x40 cave
```

On Linux, `go test` and `go vet` work out of the box with `CGO_ENABLED=0`. A default cgo build of the GUI
packages needs the X11/GL headers Ebitengine uses:

```sh
sudo apt-get install libgl1-mesa-dev libxcursor-dev libxi-dev libxinerama-dev libxrandr-dev libxxf86vm-dev
```

Cross-compile the Mac binary from Linux as a compile check (this is not a Mac test):
`CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./cmd/deepshift`.

## Repository layout

```text
cmd/deepshift/        native desktop entrypoint (the only place Ebitengine starts)
cmd/cavetool/         headless CLI: validate caves, print ASCII state, run/check replays
internal/sim/         pure-Go deterministic rules engine: no Ebitengine, GUI, OS, file or clock imports
internal/level/       versioned cave file format, validation with located errors, checksum
internal/replay/      tick-stamped replay format, runner, golden comparison
internal/game/        pure-Go session: tick scheduling, keyboard intent, pause/restart, camera, HUD text
internal/sprites/     original placeholder tiles drawn in code (pure Go, unit-tested)
internal/render/      Ebitengine drawing of game frames and HUD
assets/levels/        authored caves (embedded into the binary)
testdata/replays/     golden replay fixtures with expected hashes and explanations
packaging/macos/      Info.plist template
scripts/              build-macos-app.sh
docs/                 SPEC, ROADMAP, DECISIONS, PHYSICS
```

## Documentation

- [docs/SPEC.md](docs/SPEC.md) — the game design and technical specification (source of truth)
- [docs/PHYSICS.md](docs/PHYSICS.md) — the rules as actually implemented (rule version 1)
- [docs/DECISIONS.md](docs/DECISIONS.md) — decisions and deviations, with rationale
- [docs/ROADMAP.md](docs/ROADMAP.md) — phase status, playtest checklist and follow-ups
- [CLAUDE.md](CLAUDE.md) — working rules for coding agents

## Reporting a playtest bug

Include: build (the pause screen shows `build <revision>`), macOS version and chip, cave id, seed (from the
cave file), steps, expected, actual, screenshot/video if visual, and whether it reproduces.
