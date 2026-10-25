# Deep Shift

Deep Shift is an original, native macOS 2D underground physics/puzzle adventure: dig through a cave, collect
crystals, meet the quota and reach the exit, using environmental physics (falling boulders and flowing
water). It is written in Go with [Ebitengine](https://ebitengine.org) and runs as a local `.app`. There is no
browser, Electron, server or database.

The game takes design inspiration from the *rules-driven* arcade cave games of the 1980s. All of its code,
caves, art, names and rules are original to this project.

> **Status: Phases 0–4, Phase 5's creature and the first milestone of the cave editor delivered in the
> cloud: a five-cave vertical slice, crawlers and an editor, all still needing their Mac playtest.**
> Movement, digging, crystals, walls and a quota-sealed exit, plus:
>
> - **gravity:** boulders and crystals fall, roll and crush, and boulders can be pushed;
> - **water:** it falls and spreads, is conserved exactly, and is too deep to wade when a cell is full;
> - **sluice gates** that a valve opens and closes;
> - **crawlers:** creatures that follow the wall on their left and catch you, but can be crushed by a
>   falling boulder or drowned.
>
> The campaign has five caves, plus two extras, "Crawler Warren" (crawlers) and the "Proving Ground" test
> cave:
>
> 1. "First Descent" (movement, digging, crystals, the exit);
> 2. "Unstable Ground" (falling boulders);
> 3. "The Roll" (rolling and pushing);
> 4. "Reservoir" (water and a sluice gate);
> 5. "The Sluice" (water and gravity, two ways through).
>
> There's a title menu and a cave list, pause/restart, a HUD, sound effects, arcade music, a performance
> overlay, and automatic replays of finished attempts. A **cave editor** paints, validates, saves and
> playtests caves, and the caves you make appear in the cave list under "My caves". Progress is not saved between runs. See [docs/ROADMAP.md](docs/ROADMAP.md) and the rules in [docs/PHYSICS.md](docs/PHYSICS.md).

## What has and has not been verified

| Behaviour | How it was checked | Status |
|---|---|---|
| Rules: movement, bounds, walls, digging, collecting, exit quota, restart | `go test ./...` (table-driven, headless) | Verified on Linux; CI also runs them on macOS arm64 |
| Gravity: boulders and crystals fall, roll (left first) and crush; boulders push (no chains); at most one move per tick | `go test ./...`, randomised invariant runs, golden fixtures | Verified on Linux; CI runs them on macOS arm64 |
| Water: falls then spreads with no left/right bias, conserved every tick, contained by every barrier, displaced by objects; gates and valves | `go test ./...` (hand-computed cases, mirror test, randomised invariants), cave 04 fixture | Verified on Linux; CI runs them on macOS arm64 |
| Crawlers: left-hand rule, one move per step, catching both ways, crushed by falling objects only, drowned by water, reset | `go test ./...` (hand-computed paths, randomised invariants, mutation-checked), Crawler Warren fixtures and timing sweeps | Verified on Linux; CI runs them on macOS arm64 |
| Determinism: 100 identical runs → identical hashes at every tick, including deaths by crush and by crawler, a reservoir drain, a boulder pushed through water, and crawlers crushed and drowned | `internal/replay` test + golden fixtures | Verified (Linux); CI checks the same golden hashes on macOS arm64 |
| Every cave can be completed; cave 05 has two distinct routes; cave 01 cannot hurt the player | Golden replays plus tests on each route's events and water state | Verified (Linux); CI runs them on macOS arm64 |
| Menus: title, cave list, keys that chose an entry never leak into the game | `internal/game` shell tests; headless capture | Verified (Linux) |
| Sound: twelve original synthesised cues, one per kind of event per tick | `internal/sfx` tests; in a headless capture the app's audio streams matched the synthesised cues sample for sample | Verified as data (Linux VM guest); **not yet heard on a Mac** |
| Music: an original 32 s arcade loop; B turns it off and on, M mutes it with everything else, a paused cave or playtest holds it | `internal/sfx` tests (loop length, silent seam, peak below the cues), `internal/game` tests of the keys; in a headless capture the app's music stream matched the loop sample for sample, wrapped at 32 s, and B, M and Esc paused and resumed it | Verified as data (Linux VM guest); **not yet heard on a Mac** |
| A finished attempt is saved as a replay that reproduces it exactly | Session tests; app run headlessly (below) | Verified (Linux) |
| Cave files validate; broken caves give useful errors | `internal/level` tests, `cavetool validate` | Verified |
| Cave editor: painting, boxes, undo, resizing, settings, saving (never replacing a file without asking), the format version kept, embedded caves saved byte for byte | `internal/editor` tests (random-input invariants, mutation-checked), `cavetool fmt -check` in CI | Verified (Linux); CI runs them on macOS arm64 |
| Editor playtests play the cave in a real session; P goes back and forth | `internal/game` shell tests (cave 01's golden replay completes a copy in a playtest); headless capture | Verified (Linux); **not yet used on a Mac** |
| "My caves" in the cave list; caves that cannot be played say why; playtests of saved caves are recorded and re-run against the cave file | `internal/game` shell tests (mutation-checked); headless capture with `cavetool replay -cave` | Verified (Linux); **not yet used on a Mac** |
| macOS arm64 / amd64 executables compile | Cross-compile from Linux; native build on CI's macOS runner | Compile-only |
| `DeepShift.app` bundle builds and is ad-hoc signed | `scripts/build-macos-app.sh` on CI's macOS runner | Packaging checked in CI; **launch from Finder not yet verified** |
| Window, keyboard, rendering, HUD on a real Mac | Owner ran `go run ./cmd/deepshift` on a Mac: "ok as a first pass" (Phase 1 build) | First pass only; **menus, sound, music, caves 01–05, boulders, water and crawlers not yet played on a Mac** |
| 60 FPS on a reference Apple Silicon Mac | The in-game performance overlay (`` ` `` or F3) measures it | **Not yet measured** |
| Rendering + key and mouse handling of the real app on Linux (Xvfb, Mesa software GL), editor included | Ebitengine `exp/vmhost` headless capture, run by hand | Observed in the cloud sessions; not macOS, not in CI ([D-014](docs/DECISIONS.md#d-014-ebitengine-expvmhost-headless-capture-assessed-not-adopted)) |

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

The game opens on the title menu: choose "Play the campaign" or pick a cave from the list.

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
| Arrow keys / WASD | Move one tile, digging earth and collecting crystals. Holding a key repeats one move per 4 ticks (200 ms). In menus: choose. |
| Space / Enter | Turn the valve next to you, opening or closing its sluice gates. One turn per press. In menus: select. |
| Esc | Pause / resume. Simulation and input stop while paused. In the cave list: back to the title. |
| R | Restart the cave, from the pause, completion or death screen. |
| N | Next cave, from the pause, completion or death screen (wraps after the last). |
| C | The cave list, from the pause, completion or death screen. |
| M | Sound on/off, music included |
| B | Music on/off |
| F | Toggle fullscreen |
| `` ` `` or F3 | Performance overlay on/off (frame rate, update and draw times) |
| Cmd+Q / close window | Quit |

WASD uses physical key positions, so it stays under the same fingers on non-US layouts.

N plays the caves in order: `cave_01` to `cave_05`, then the `crawler_warren` and `test_cave` extras. To skip the menu and start a
cave directly, run `go run ./cmd/deepshift -cave cave_03`. To play your own cave file, run
`go run ./cmd/deepshift -cave path/to/cave.json`; validation errors are printed to the terminal.

Other options:

- `-no-audio` starts without opening an audio device, for a machine without sound output;
- `-no-music` starts with the music off (B turns it on);
- `-perf` starts with the performance overlay on.

Cave 01 is deliberately safe: nothing in it can fall on you. Every other cave can be lost, and R always
restarts.

**Rules of thumb for gravity:**

- Boulders **and crystals** fall when there's nothing under them.
- A boulder or crystal resting above you won't hurt you, but one that is falling will.
- When you dig out from under a boulder, step **sideways**, never straight down.
- You can push one boulder left or right into open floor.
- Boulders and crystals roll off each other.
- You can still grab a falling crystal by walking into it from the side.

**Rules of thumb for water:**

- Water falls, then spreads sideways. It never rises and never disappears.
- You can wade through shallow water, but a full cell (with wave lines) is too deep.
- A valve opens or closes its gates. An open gate is just floor; a closed one is a wall.
- A gate only closes when nothing (and no water) is in it.
- Boulders and crystals sink through water, and the water moves up into the space they leave.
- Earth holds water back until you dig it.
- A boulder pushed into water shoves the water behind it, so you can follow it through a flooded tunnel.

**Rules of thumb for crawlers:**

- A crawler keeps the wall on its **left**. Watch it go round once and you know its path.
- It catches you by stepping into your cell, and walking into it is just as bad. Standing **next** to one
  is safe.
- A pocket or tunnel on a crawler's right is a safe place to wait: it only turns right when it must.
- A falling boulder or crystal crushes it; a resting one doesn't.
- It never steps into water, and water that reaches it drowns it.

The full rules are in [docs/PHYSICS.md](docs/PHYSICS.md).

**Replays:** every finished attempt, completed, crushed or caught, is saved to
`~/Library/Application Support/Deep Shift/replays/`. The HUD shows the file name. Run
`go run ./cmd/cavetool replay <file>` to re-run one exactly. `-replay-dir <dir>` changes the folder, and
`-replay-dir ""` turns saving off.

### Make your own caves

Choose **Cave editor** on the title menu. Start a new cave, open one of yours, or edit a copy of a built-in
cave. To jump straight in, run `go run ./cmd/deepshift -edit new`. You can also pass `-edit` an embedded
cave id (edited as a copy) or a cave file (saved back to it).

| Input | Action |
|---|---|
| 1–9, 0, - | Choose a brush: wall, earth, floor, crystal, boulder, player start, exit, water, crawler, valve, gate. Press again for the next variant (shallower water, the crawler turned clockwise). Clicking a palette slot does the same. |
| Left drag | Paint. The player start and the exit move rather than duplicate. A valve becomes the selected valve; gates join the selected valve. |
| Right drag | Erase to floor |
| Shift + drag | Fill a box |
| Cmd + click | Flood-fill the area of matching cells (Cmd + right-click fills it with floor) |
| Option (Alt) + click | Pick the brush of a cell; on a valve or gate, select its valve and the gate brush |
| O | Open or close the valve or gate under the pointer, with all its linked gates: how the cave starts |
| R | Reach overlay on/off: shades what the player cannot reach by walking and digging alone |
| Arrows / WASD / wheel | Scroll |
| Cmd+Z / Cmd+Shift+Z | Undo / redo |
| Cmd+arrow / Cmd+Shift+arrow | Add / remove a column or row on that side |
| Tab | Cave settings: title, id, seed and quota (Enter to change one, Esc to cancel) |
| G | Grid on/off |
| P | Playtest; P again comes back to the editor |
| Cmd+S | Save |
| Esc | Back to the list (it asks first if there are unsaved changes) |

The editor checks the cave after every change, using the same loader as the game. Problems are listed in
the bottom bar, and the cells they concern are outlined in red. A cave with problems can still be saved,
but it cannot be playtested until they are fixed. New caves are saved as
`~/Library/Application Support/Deep Shift/caves/<id>.json`; `-caves-dir <dir>` changes the folder. They
appear in **Choose a cave** under "My caves", after the built-in caves; one that cannot be played yet is
listed as "(cannot play)" and says why.

**Sluices:** paint a valve (0), then its gates (-): each gate joins the selected valve, ringed in yellow,
and a line links them. To add gates to another valve, Option-click it first. A valve with no gates, or a
gate with no valve, is a problem until it is linked. O sets whether a sluice starts open.

**Reach (R)** is a first look at a layout, not a solver: it follows walking and digging only. Falling and
pushed boulders, turned valves and flowing water can make a cave easier or harder than it shows.

A playtest of a saved, unchanged cave is saved as a replay like any finished attempt; a playtest of a cave
with unsaved changes is not. Replays of your own caves name the cave file. Re-run one with
`go run ./cmd/cavetool replay -cave <cave file> <replay>`.

## Develop and test (any OS, no display needed)

```sh
go test ./...                     # all rules, level, replay and session tests; no window or GPU
go vet ./...
go run ./cmd/cavetool validate    # validate the embedded caves
go run ./cmd/cavetool fmt -check assets/levels/*.json  # caves are in the canonical layout the editor writes
go run ./cmd/cavetool show assets/levels/test_cave.json
go run ./cmd/cavetool replay testdata/replays/*.json   # re-run golden replays and check their hashes
go run ./cmd/cavetool replay -trace testdata/replays/cave_02_crushed.json   # events + grid per tick
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
cmd/cavetool/         headless CLI: validate and format caves, print ASCII state, run/check replays
internal/sim/         pure-Go deterministic rules engine: no Ebitengine, GUI, OS, file or clock imports
internal/level/       versioned cave file format, validation with located errors, checksum, canonical writer
internal/replay/      tick-stamped replay format, runner, golden comparison
internal/game/        pure-Go session and menus: tick scheduling, keyboard intent, pause/restart, camera, HUD text, editor screens and playtests
internal/editor/      pure-Go cave editor: brushes, strokes, undo, resizing, validation, saving, layout and view model
internal/sprites/     original placeholder tiles drawn in code (pure Go, unit-tested)
internal/sfx/         original sound cues and music synthesised in code, and which events trigger the cues (pure Go, tested)
internal/perf/        rolling timing statistics for the performance overlay (pure Go, tested)
internal/render/      Ebitengine drawing of game frames, menus, HUD and the cave editor
internal/audio/       Ebitengine playback of the sound cues and the looping music
assets/levels/        authored caves (embedded into the binary)
testdata/replays/     golden replay fixtures with expected hashes and explanations
packaging/macos/      Info.plist template
scripts/              build-macos-app.sh
docs/                 SPEC, ROADMAP, DECISIONS, PHYSICS
```

## Documentation

- [docs/SPEC.md](docs/SPEC.md) — the game design and technical specification (source of truth)
- [docs/PHYSICS.md](docs/PHYSICS.md) — the rules as actually implemented (rule version 4)
- [docs/DECISIONS.md](docs/DECISIONS.md) — decisions and deviations, with rationale
- [docs/ROADMAP.md](docs/ROADMAP.md) — phase status, playtest checklist and follow-ups
- [CLAUDE.md](CLAUDE.md) — working rules for coding agents

## Reporting a playtest bug

Include:

- the build (the pause screen shows `build <revision>`);
- your macOS version and chip;
- the cave id and seed (from the cave file);
- steps, expected and actual;
- a screenshot or video if it's visual;
- whether it reproduces.

**Attach the replay file** the game saved for that attempt (see Replays above), so it can be re-run exactly.
