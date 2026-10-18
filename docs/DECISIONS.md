# Decision log

Significant decisions, clarified assumptions and deviations from [SPEC.md](SPEC.md). Each entry says what
was decided, why, and which scope it affects. "Provisional" means it should be revisited after Mac
playtesting. Newest entries go at the bottom.

---

## D-001: Spec and agent docs moved to their intended paths

- **Decision:** `docs/DEEP_SHIFT_GAME_SPEC.md` → `docs/SPEC.md`; `docs/CLAUDE.md` → `CLAUDE.md`;
  `docs/CLAUDE_CLOUD_KICKOFF.md` → `CLAUDE_CLOUD_KICKOFF.md`. Content unchanged (`git mv`).
- **Why:** `docs/README_FIRST.md`, `CLAUDE.md` and the kickoff all refer to `docs/SPEC.md` and a root
  `CLAUDE.md`. Moving the files made those references correct.
- **Scope:** documentation only.

## D-002: Toolchain and dependency pins

- **Decision:** `go.mod` declares `go 1.25.0` (the minimum language version) and `toolchain go1.27.2` (the
  exact toolchain used). Ebitengine is pinned at `v2.10.5`, the latest stable release on 2026-10-17
  (`v2.11.0-alpha` excluded). CI reads `go.mod` via `actions/setup-go` `go-version-file`.
- **Why:** The spec asks for Go 1.25+ and Ebitengine 2.10+. Ebitengine v2.10.5 itself requires `go 1.25.0`.
  Go 1.25 is no longer supported upstream since Go 1.27, so the pinned toolchain is a supported release while
  the floor stays at the spec's 1.25.
- **Consequence:** binaries built with Go 1.27 require macOS 13 Ventura or newer (the Go 1.27 linker stamps a
  13.0 minimum), so `Info.plist` sets `LSMinimumSystemVersion` to `13.0`.

## D-003: Fixed-step scheduling (provisional rates)

- **Decision:** The simulation runs at **20 ticks/s** (`sim.TicksPerSecond`). Ebitengine runs `Update` at
  **60/s** (`ebiten.SetTPS(60)`), and the session advances the simulation exactly once every **3 updates**.
- **Why:** Ebitengine calls `Update` at a fixed logical rate independent of display refresh and catches up
  after slow frames. Tying ticks to an update *count* (not wall-clock time) keeps the simulation identical on
  60 Hz and 120 Hz displays. Sampling input at 60 Hz rather than 20 Hz means short key taps are not missed.
  Presentation (player slide, camera easing) is also driven by update counts.

## D-004: Movement cadence and input model (provisional)

- **Decision:**
  - The cadence is a **rule** inside `internal/sim`: after a successful move the player waits
    `MoveIntervalTicks = 4` ticks (200 ms; 5 tiles/s) before the next move can start.
  - A move into a wall, the cave edge or a sealed exit does **not** use up the movement window.
  - Move input that arrives while the window is closed is dropped by the rules, deterministically.
  - The game session (not the rules) handles the keyboard. It latches one press until the next window, so
    a tap shorter than a tick still moves the player once. If several keys are held, the most recently
    pressed one wins. Operating-system key repeat is ignored; only held state and press edges are used.
  - The session sends a move command only when the window is open, so recorded commands are exactly the
    inputs that took effect.
- **Why:** Player speed relative to falling rocks (Phase 2) is a core rule, so it must be deterministic and
  replayable, which means it belongs in the simulation. Keyboard comfort (taps, key priority) is
  presentation, so it belongs in the testable session layer.

## D-005: Exit behaviour

- **Decision:** A sealed exit blocks like a wall and emits `exit-sealed` (the HUD shows how many crystals
  are still needed). The exit opens in the world phase of the tick on which the quota is met. **Entering**
  the open exit completes the cave; no separate "activate" key is used. A quota of 0 opens the exit before
  tick 0. After completion `Step` does nothing and the tick counter stops.
- **Why:** The spec asks for one consistent choice. Entering is the simplest and works with movement alone.
  Freezing on completion makes the final hash independent of how long a replay runs afterwards.

## D-006: Cell model for rule version 1

- **Decision:** Each cell has a `Terrain` (empty, earth, wall, exit) and an `Occupant` (none, player,
  crystal). Crystals are occupants, so a later phase can let them fall. Fluid and metadata layers will be
  added as new `Cell` fields when needed, not by overloading these enums. Enum values are append-only
  because they are part of the state-hash encoding.

## D-007: Cave file format version 1

- **Decision:** JSON in `assets/levels/*.json` (embedded in the binary), with fields `format_version`,
  `id`, `title`, `width`, `height`, `seed`, `required_crystals`, `rows` and optional `metadata`.
  - The grid field is named **`rows`**, not the spec sketch's `terrain_rows`, because rows also place the
    player and crystals. The spec's `entities` and `mechanisms` are not part of v1.
  - Legend: `#` wall, `.` earth, space = empty, `*` crystal, `@` player start, `E` exit. The ASCII dumps
    use the same legend, plus `X` for an open exit (dump only).
  - Exactly one player start and exactly one exit. `required_crystals` must be between 0 and the crystal
    count. Each side must be 1..256 tiles. The seed must be ≤ 2^53−1 so every JSON tool keeps it exact.
  - Outer walls are **not** required, because the rules treat the cave edge as a wall.
  - Unknown fields are rejected so that typos surface. All problems are reported together with JSON paths
    and x/y positions; syntax errors give line:column.
  - The **checksum** (SHA-256) covers only gameplay fields: version, size, seed, quota and rows. Renaming
    a cave or editing its title/metadata does not invalidate replays.
- **Why:** The format is human-editable and versioned. Validation errors point at the exact tile, and
  presentation fields are kept apart from rules.

## D-008: Seeded PRNG

- **Decision:** Randomness comes from an in-house SplitMix64 whose whole state (one `uint64`) is part of the
  simulation state and hash. Rule version 1 does not consume randomness yet.
- **Why:** The spec requires an explicit, local, seeded PRNG. An in-repo implementation fixes the sequence
  regardless of Go version (checked against the published reference vector) and is trivially serialisable.

## D-009: State hash

- **Decision:** `StateHash` is SHA-256 over an explicit little-endian encoding that starts with the magic
  string `deep-shift/state-hash/v1`. It covers the rule version, seed, tick, size, quota, collected count,
  player position, exit/status/cooldown, PRNG state and every cell. A golden test pins the layout. Replays
  also compute a **trace hash**: SHA-256 over the tick-0 hash followed by every subsequent tick's hash, so
  matching trace hashes mean the runs agreed at *every* tick.
- **Rule:** any change that alters a golden hash needs a deliberate `RuleVersion` or hash-magic bump, a
  PHYSICS.md update and a fixture review. Hashes are never regenerated wholesale.
- **Amended by [D-020](#d-020-rule-version-2-hash-layout-v2-and-the-reviewed-fixture-changes):** the hash
  layout is now v2, and the trace hash stops at the final tick.

## D-010: Replay format version 1

- **Decision:** JSON with the cave id and checksum, seed, rule version, number of ticks, **sparse
  tick-stamped move commands** (each the input for exactly one tick, with ticks strictly increasing), a
  human `description`, and an optional `expect` block (final tick, status, collected, final hash, trace
  hash). Fixtures live in `testdata/replays/`, one command per line; a test enforces that layout.
- **Authoring:** plan the inputs (the first fixtures came from a route planner), run
  `cavetool replay -print-expect`, review the final ASCII state and outcome, and commit the result.
  The app session can already package an attempt as a replay (`Session.Replay`); in-app export is a
  follow-up.

## D-011: Restart semantics

- **Decision:** R restarts only from the pause screen or the completion screen, as the spec's control table
  says. Restart restores the simulation exactly (cells, counters, tick, PRNG seed) and clears the recorded
  commands.

## D-012: Package layout deviations from SPEC §9

- `Camera` lives in `internal/game` (pure, unit-tested) instead of `internal/render`, so camera clamping and
  lookahead can be tested without a graphics context.
- Added `internal/sprites` (procedural placeholder tiles, pure Go), `assets` (a Go package that embeds the
  levels so the `.app` never depends on the working directory) and `cmd/cavetool` (headless CLI).
- `internal/audio` and `internal/editor` are not created yet; they are out of this PR's scope.

## D-013: Rendering defaults (provisional)

- **Decision:** a fixed logical screen of 960×640: a 30×18-tile view of 32 px tiles under a 64 px HUD.
  Ebitengine scales it to the window (default 1200×800, resizable, F toggles fullscreen). HUD text uses
  Ebitengine's built-in debug font at 2×, so no font asset is needed. The camera is snapped to whole
  pixels, eases at 0.15 per update and looks 3 tiles ahead in the facing direction. The player sprite
  glides between cells over 6 updates (100 ms), which is always shorter than the move interval.
- **Follow-up:** crisper Retina output (`LayoutF` / device scale), a real font, and tuning of the camera and
  slide feel after playtesting.

## D-014: Ebitengine exp/vmhost headless capture assessed, not adopted

The spec asked for Ebitengine 2.10's agent-oriented virtualisation to be assessed separately.

- **What it is:** `github.com/hajimehoshi/ebiten/v2/exp/vmhost` (experimental). The app is built with
  `-tags ebitenginevmguest` and runs as a guest without opening a window. A small host program steps its
  updates, injects keyboard/mouse input and reads back rendered frames. The host still needs a real
  graphics context: on headless Linux that means Xvfb or EGL.
- **What was run (2026-10-17, cloud session, by hand):** the template driver shipped in the module
  (`skills/run-ebitengine-app-headless/_driver`) was copied into an ignored `.vmdriver/` directory, with
  `xvfb-run` and Mesa's software OpenGL. The unmodified `cmd/deepshift` was driven:
  1. through start, sealed-exit bump, digging (arrow keys and WASD) and the pause overlay;
  2. through the entire `test_cave_complete.json` fixture, translated into key press/release events.

  The captured frames showed the HUD, tiles, the sealed-exit prompt, the quota-met prompt, camera scrolling
  and a **CAVE COMPLETE — 8 crystals in 0:42** overlay. That matches the fixture's final tick, 846.
- **What this does *not* cover:** macOS, Metal, Retina scaling, real window management/fullscreen, real
  keyboards and OS key repeat, frame pacing, or Finder launch. It is evidence that the render/input wiring
  works on Linux/OpenGL, not a Mac test.
- **Why not adopted yet:** the API is experimental; upstream notes occasional flaky launches; it needs Xvfb
  on CI; and SPEC §10 says automated captures should wait until a repeatable harness has been demonstrated
  and reviewed. No tooling or images from it are committed.
- **Follow-up:** add a small `tools/vmcapture` driver plus an optional, non-gating CI job that uploads
  frames for review. Assess whether the host works on GitHub's macOS runners.
- **Reproduce:** copy the driver into `.vmdriver/`, edit its `INPUT SCRIPT` block, then run
  `xvfb-run -a go run ./.vmdriver -pkg ./cmd/deepshift -w 960 -h 640 -ticks 1 -out /tmp/frame.png`.

## D-015: macOS packaging

- **Decision:** CI builds natively on a pinned `macos-15` arm64 runner with
  `scripts/build-macos-app.sh`, which produces `DeepShift.app`:
  - `Contents/MacOS/deepshift` and `Info.plist`, with bundle id `io.github.townhill.deepshift`, version
    0.1.0, the build number from the CI run, and the git revision in `DeepShiftGitRevision`;
  - an **ad-hoc** signature over the whole bundle (`codesign -s -`). Apple Silicon requires at least
    this to run code, and signing the bundle binds `Info.plist`;
  - packed with `ditto -c -k --keepParent`, which preserves permissions inside GitHub's artifact zip.

  The script exports `MACOSX_DEPLOYMENT_TARGET=13.0` for native builds. With cgo, Go hands linking to the
  system linker and passes no minimum-version flag, so the binary would otherwise require the build
  machine's macOS (15 on CI). The same value fills `LSMinimumSystemVersion`. CI checks the architecture
  (`lipo`), the plist (`plutil -lint`), that the binary's `LC_BUILD_VERSION` `minos` equals the plist
  minimum, and the signature (`codesign --verify --strict`). It also starts the bundled binary with
  `-version`, which opens no window.
- **Not done:** Developer ID signing, notarisation, an app icon (`.icns`) and a universal build. The artifact
  is labelled an *unsigned development build*, and the README gives per-app Gatekeeper steps. Nothing
  weakens system-wide security.
- **Status:** packaging is scripted and checked in CI. Launching it from Finder on a real Mac is **not yet
  verified**.

## D-016: Linux build dependencies

- CI's Linux job installs the X11/GL headers Ebitengine's default cgo build uses (`libgl1-mesa-dev`,
  `libxcursor-dev`, `libxi-dev`, `libxinerama-dev`, `libxrandr-dev`, `libxxf86vm-dev`). No display is
  opened. With `CGO_ENABLED=0`, `go test ./...` and `go vet ./...` also pass on Linux without those headers
  (observed with Ebitengine v2.10.5). The macOS cross-compile check uses `CGO_ENABLED=0`, because
  Ebitengine's darwin backend is pure Go via purego.

## D-017: Originality of content

- The test cave "Proving Ground" was laid out by hand for this project. Placeholder art is drawn
  procedurally in `internal/sprites`. Tile symbols, terminology ("crystals", "sealed exit", "quota") and
  rules were chosen independently. No external art, audio, fonts (beyond Ebitengine's built-in debug
  font) or level data are included.

## D-018: Boulder rules (rule version 2, provisional tuning)

These gravity rules apply to crystals too ([D-019](#d-019-crystals-fall-owner-decision)); only pushing is
boulder-specific.

- **Gravity cadence:** gravity runs on ticks that are multiples of `GravityIntervalTicks = 2`, so objects
  move 10 cells/s, twice the player's walking speed. That makes falls dangerous but readable. Tune after
  playtesting.
- **Scan order:** rows bottom to top, and left to right within a row. A supported stack therefore drops
  together when its support goes. A per-step *updated* marker on destination cells stops a boulder that
  rolled right from being processed again. A boulder pushed this tick sits out that tick's gravity step.
  Together these guarantee each boulder moves at most one cell per tick (SPEC §6.2).
- **Moving vs resting:** a per-cell `Falling` flag records that the boulder moved on the last gravity step.
  Only a falling boulder crushes the player. A resting one is held up by the player, which makes "dig
  under it, then step aside" a safe, learnable move and "run straight down" fatal.
- **Crush:** the boulder stops on the player, `player-crushed` is emitted and the status becomes `dead`.
  The simulation then freezes like completion. The boulder does not take the player's cell, so the
  one-player invariant still holds.
- **Rolling:** off rounded occupants only (boulders, crystals); earth, walls, the exit and the player are
  flat. A roll needs both the side cell and the cell below it empty, and tries **left before right**. It
  moves one cell sideways; the drop into the diagonal happens on the next gravity step, so no move ever
  covers two cells.
- **Pushing:** horizontal only, resting boulders only, into empty floor only, one boulder only (no
  chains). It always succeeds when allowed, with no random resistance: the PRNG stays unused, and pushing
  should feel deterministic.
- **Order within a tick:** entering the open exit ends the tick immediately, so nothing can fall on a player
  who has just finished. The quota check is skipped on the tick the player dies.

## D-019: Crystals fall (owner decision)

- **Decision (owner, 2026-10-18):** crystals obey the same gravity as boulders. They fall, roll off
  rounded objects and land, and a **falling crystal crushes the player**, following SPEC §6.2 ("Falling
  objects crush a player"). The player still collects a crystal by walking into its cell, even while it is
  falling. Crystals are never pushed. Only objects on open floor fall, so a crystal embedded in earth
  (hand-built caves only) stays put.
- **History:** the first Phase 2 draft kept crystals fixed (provisional, "when sensible"). The owner
  reversed that before merge. Rule version 2 was therefore amended in place rather than bumped, because it
  had never been merged.
- **Consequence:** caves with loose crystals now play differently from rule version 1. In `test_cave`, two
  crystals drop one cell at tick 0, and the test-cave fixtures were re-planned (D-020).
- **Open option:** if playtesting shows crystal crushes feel unfair, "a falling crystal is collected
  instead of crushing" is a one-line rule change, and would need its own decision.

## D-020: Rule version 2, hash layout v2 and the reviewed fixture changes

- `sim.RuleVersion` is now 2, and the state-hash magic is `deep-shift/state-hash/v2` (one extra byte per
  cell for the falling flag). `TestStateHashGolden` was re-pinned for that.
- **Test-cave fixtures, first pass (crystals fixed):** both were re-recorded because the rule version is
  part of every hash and of the replay header. Final tick, status, crystal count and final grid were
  identical to rule version 1.
- **Test-cave fixtures, second pass (crystals fall, D-019):** the old routes no longer worked. Two loose
  crystals drop at tick 0, so the static route collected only 7 of 8 (and 6 of 7 for the sealed-exit
  fixture). Both routes were re-planned with a throwaway planner that re-plans on the real simulator after
  every move and rejects moves leading to a crush. The opening (dropped input, sealed-exit bump) is
  unchanged. Outcomes:
  - completion: 8 crystals, exits at tick 846, as before;
  - sealed exit: 7 crystals, still in play, ending at tick 819 instead of 779.

  The cave_02/cave_03 fixtures were unaffected: every crystal there is supported and their routes never
  undermine one.
- **Trace hash semantics:** the trace now covers ticks 0 to the final tick only. Ticks after completion or
  death leave the state frozen and are no longer hashed. Before this change, a fixture that idled after the
  end and the app's own recording of the same attempt had different traces even though every real tick
  matched. Only the `trace_hash` of the four fixtures with idle tails changed; their final hashes did not.

## D-021: Cave format version 2

- **Decision:** `format_version` 2 adds the boulder symbol `O`. The reader accepts versions 1 and 2. A
  version-1 file containing `O` is rejected with a specific message ("boulder(s) 'O' at x=… need
  format_version 2 or later"), not "unknown symbol".
- **Why:** files declare what they need, and older caves stay valid byte for byte. The test cave is still
  version 1, so its checksum is unchanged. New caves use version 2.

## D-022: Death and the end of an attempt

- A crush (by a falling boulder or crystal; the overlay names which) ends the attempt like completion: the
  world freezes and Esc does nothing. **R** restarts and
  **N** moves to the next cave, from the pause, completion and death screens (D-011 extended). The crushed
  player is drawn squashed and tinted red under a "CRUSHED" overlay.

## D-023: Replays of finished attempts are saved automatically

- **Decision:** when an attempt ends (completed or crushed), the app writes it as a replay to
  `<user config dir>/Deep Shift/replays/<cave>-<status>-<YYYYMMDD-HHMMSS>.json`. On macOS that is
  `~/Library/Application Support/Deep Shift/replays/`. Existing files are never overwritten. The HUD names
  the file, and the terminal logs the full path. `-replay-dir` changes the folder; `-replay-dir ""`
  disables saving.
- **Self-verifying:** the session keeps a live trace hash, so each saved replay carries an expect block,
  and `cavetool replay <file>` confirms it reproduces exactly. This meets SPEC §11 Phase 2 ("logs a
  reproducible replay") and gives bug reports a replay to attach. A manual "export now" key remains a
  follow-up.
- **Not in the simulation:** file I/O and wall-clock timestamps live only in `cmd/deepshift` and
  `replay.Save`.

## D-024: Cave order and selection

- `assets.PlayOrder` lists every embedded cave (`test_cave`, `cave_02`, `cave_03`). `game.OrderLevels` fails
  if a cave is missing from it, so a new cave cannot be silently left out. The app starts at the first
  cave. `-cave` takes an embedded id (start there) or a file path (play that file alone). A menu or world
  map is Phase 4 scope.

## D-025: Example caves 02 and 03

- **cave_02 "Unstable Ground"** (SPEC §8: "dig safely or be crushed"), 30×14, quota 6 of 8. Its lessons:
  - take a crystal from under a boulder and step aside;
  - tunnel under a row of boulders that collapses behind you;
  - a mid-air boulder that falls the moment the cave starts;
  - empty a walled shaft from its side entrance.
- **cave_03 "The Roll"** (SPEC §8: "a deliberate path cleared by moving one boulder"), 30×14, quota 5 of 8.
  Its lessons:
  - dig the earth plug beside a perched boulder and step back, so it rolls into a pit and frees its
    crystal;
  - push the boulder in the only gap of a full-height wall into the pit beyond it. Only 4 crystals lie
    west of that wall, so the push is required;
  - two boulders in a row cannot be pushed, so their crystal must be reached another way.
- Both layouts are original. Each has a golden completion replay, and cave_02 also has a death replay.

## D-014 addendum: Phase 2 headless capture (2026-10-18)

- The same by-hand `exp/vmhost` setup (Linux, Xvfb, Mesa software GL; nothing committed) replayed
  `cave_02_crushed.json` and `cave_03_complete.json` through injected key events into the unmodified app.
  The frames showed boulders mid-fall, the crushed player, the CRUSHED overlay with the "Replay saved"
  prompt, and the push into the pit.
- The replay files the app wrote matched the golden fixtures' final and trace hashes exactly. This is
  still Linux/OpenGL evidence, not a Mac test.
