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

## D-026: Phase 3 started before the Phase 2 Mac playtest (owner decision)

- The kickoff prompt and the roadmap said to begin water and gates only once Phase 2's physics tests and Mac
  playtesting were stable. On 2026-10-18 the owner asked for Phase 3 to start on a new branch anyway,
  before the caves 02/03 checklist had been reported.
- Consequence: Phase 2 tuning (fall speed, crystal crushes) may still change after that playtest. Any
  such change needs another rule version and a fixture review. Water runs on the gravity cadence, so
  retuning `GravityIntervalTicks` changes both together.
- The Phase 2 Mac checklist stays open in the roadmap.

## D-027: Water model (rule version 3, provisional tuning)

- **Fluid layer:** `Cell.Water` holds 0 to 4 units (`WaterCapacity`). A full cell is *deep*; 1 to 3 is
  *shallow*. Water may occupy open floor (empty terrain or an open gate) without a boulder or crystal; it
  shares cells with the player.
- **Cadence:** on gravity ticks, after gravity, so 10 water steps per second.
- **Fall:** rows bottom to top; each cell pours as much as fits into the cell below. Water moves one cell
  down per step, never more.
- **Spread:** every side-by-side pair whose levels, after the fall, differ by two or more passes one unit
  to the lower cell. All pairs are judged on the same levels and applied together.
- **Why this algorithm:** SPEC §6.3 asks for discrete units, down then sideways, deterministic
  tie-breaking, and no duplication or deletion.
  - Judging every pair on frozen levels removes the bias a left-to-right pass would have; a mirrored cave
    evolves into the mirror image (tested).
  - Every transfer is between two cells, so the total is exactly conserved.
  - Bounds hold by construction. A cell only receives from a side that is at least two units higher, and
    only gives to a side at least two units lower, so it never exceeds 4 or drops below 0.
- **Settling:** a fall lowers the water's height; a spread keeps the height and strictly lowers the sum of
  squared levels. A closed cave therefore always comes to rest. The test covers 20 random caves.
- **Deliberate limits for v0.1:**
  - There is no pressure, so water never rises: no U-tubes, no siphons.
  - A one-unit film on a flat floor never spreads further, so levels settle within one unit of their
    neighbours rather than perfectly flat. On a flat floor, water therefore settles as a slope and
    reaches at most three cells past the last full cell; it travels far only by falling. Cave design
    has to use drops, as cave 04 does.
  - There are no flow events; renderers read levels from snapshots.
- **Player:** shallow water can be entered, with no slowdown yet. Deep water blocks the move with a
  `too-deep` event and, like any blocked move, does not start the cooldown. Water never harms or moves the
  player. The player can end up standing in deep water, when water flows in or a push displaces it into
  their cell, and can always step out.

## D-028: Falling objects and water: one policy

- SPEC §6.3 (provisional) says water does not stop boulders falling, and displaced water is conserved
  where possible.
- **Policy:** boulders and crystals treat water as empty space when falling, rolling and being pushed.
  Water treats them as walls. When an object moves into a cell holding water, that water swaps into the
  cell the object just left.
- The swap always fits, because an object's cell never holds water. The cell it vacates is empty floor
  with room for a full cell. Conservation is exact, so there is no capacity or displacement limitation to
  document.
- **Effects:**
  - Objects sink through a pool one cell per gravity step while the water rises above them.
  - A boulder or crystal dams a channel.
  - A push into water leaves that water in the cell the player steps into.
- Water does not slow, float or carry objects in v0.1.

## D-029: Sluice gates, valves and the Interact action

- **Terrain:**
  - A closed gate blocks everything, like a wall.
  - An open gate is exactly empty floor for every rule: the player, objects and water.
  - Valves are never passable.
  - A valve's state is its terrain (shut or open) and always equals its gates' state.
- **Mechanisms** link each valve to its gates. They are immutable cave data, covered by the state hash and
  the cave checksum.
- **Interact** turns the valve next to the player; if several are adjacent, the first in the order up,
  down, left, right.
  - It shares the movement action window. A successful turn starts the cooldown.
  - It takes precedence over a move. With no valve in reach, the move applies.
- **Opening** always succeeds.
- **Closing** needs every gate of the mechanism clear: no occupant and no water. Otherwise `gate-jammed`
  names the first blocked gate, nothing changes and the cooldown does not start.
  - Why: a slab closing through water or an object has no conserving, readable outcome in v0.1. It would
    have to destroy water, or crush or move objects.
- **Trapdoors** follow from the rules: a closed gate under a boulder holds it, and opening the gate drops
  the boulder on the same tick's gravity step.
- **App:**
  - Space or Enter interacts. The session takes the press edge and latches it like a direction tap until
    the next action window, and uses it only if a valve is next to the player then. Holding the key turns
    the valve once.
  - The HUD shows "Space: open/close the sluice gate" beside a valve.
- **Replays:** commands gain the action `"interact"`. The replay format version stays 1: the file shape is
  unchanged, and older builds cannot run rule version 3 anyway.

## D-030: Rule version 3, hash layout v3 and the reviewed fixture changes

- `sim.RuleVersion` is 3, and the hash magic is `deep-shift/state-hash/v3`. Each cell gains a water byte,
  and the mechanism links are appended. `TestStateHashGolden` was re-pinned.
- **The five earlier fixtures** were re-run under rule 3 and their `replay -trace` output (every event
  line and grid) compared with the `master` build's (rule 2). All five were identical, 12,480 lines in all.
  Final tick, status and crystals collected were unchanged.
  - Only `rule_version`, `final_hash` and `trace_hash` changed.
  - Two descriptions that named rule version 2 were reworded; descriptions are not hashed.
- Replays recorded by Phase 2 builds are rejected by this build with a message naming both rule versions.

## D-031: Cave format version 3

- From `format_version` 3, rows may contain:
  - `~` for deep water and `1` to `3` for shallow amounts;
  - `G`/`g` for a closed/open gate;
  - `V`/`v` for a shut/open valve.
- Older versions reject these symbols and name the version needed.
- **New field `mechanisms`:** `[{"valve": {"x", "y"}, "gates": [{"x", "y"}, ...]}]`.
  - Every valve and gate must be listed exactly once, and states must agree.
  - Errors are located, for example `mechanisms[0].gates[1]: ...`.
- **Checksum:** it covers mechanisms only when present, so earlier caves keep their checksums and replays.
- **Dumps:** water in an open gate or under the player is not shown, the same lossy rule as other dumps.
  The state hash covers it.

## D-032: Example cave 04 "Reservoir"

- SPEC §8: "Opening a gate changes reachable space." The cave is 30×16, quota 7 of 9, with 44 units of
  water.
- **Lessons:**
  - Deep water blocks: the only way down is a two-wide shaft full of it.
  - Shallow water can be waded: a puddle near the start, and later the row above the basin.
  - The valve beside the shaft's mouth opens the two gates at its foot. The 40 units fall into the basin,
    whose floor row stays deep.
  - A crystal on the basin floor is taken from the shallow row above.
  - Earth holds water back; tunnels dug from the basin let it follow.
- **Evidence:**
  - With no input the reservoir stays at rest, and a flood fill without the valve cannot get below the
    wall band.
  - The completion fixture goes below only after the valve turns, and conserves water on every tick.
  - It joins the 100-run determinism test.
- It is deliberately simple, with no boulders. Cave 05 (Phase 4) combines water and gravity.

## D-014 addendum: Phase 3 headless capture (2026-10-18)

- The by-hand `exp/vmhost` setup (Linux, Xvfb, Mesa software GL; nothing committed) replayed
  `cave_04_complete.json` into the unmodified app (`-cave cave_04 -replay-dir ""`). Key events were
  injected, with Space for `interact`.
- The frames showed, in order:
  - the "Too deep to wade" prompt and the waved deep shaft;
  - the valve hint;
  - the valve spokes turning from + to ×;
  - water easing down through the open gates into the basin;
  - the player wading in the shallow row;
  - the completion overlay.
- This is Linux/OpenGL evidence only, not a Mac test.

## D-033: Phase 4 started before the Mac playtests of Phases 2 and 3 (owner decision)

- On 2026-10-18 the owner asked for Phase 4 on a new branch. The Mac checklists for caves 02–04 had not
  been reported yet.
- Phase 4 changes no rules: `sim.RuleVersion` stays 3, and every earlier golden replay is untouched. Any
  tuning the playtests ask for (fall and drain speed, crystal crushes) still needs its own rule version and
  fixture review.
- SPEC §11 Phase 4's last acceptance item, known usability, audio and graphics issues documented after a
  hands-on Mac playtest, cannot be met from the cloud. It stays open in the roadmap.

## D-034: The campaign and the extras

- The campaign is SPEC §8's five starter caves in order, `cave_01` to `cave_05`, with the titles the spec
  gives. A test pins the ids and titles.
- The Phase 1 test cave "Proving Ground" moves out of the campaign into an **extras** list. It stays
  embedded and playable (the cave list, `-cave test_cave`, or N after cave 05) because its golden replays
  pin the rule-1 behaviour.
- `assets.PlayOrder` is the campaign followed by the extras; every embedded cave must appear in it exactly
  once.

## D-035: Cave 01 "First Descent"

- SPEC §8: "movement, dig, crystal, exit; complete cave without death". The cave is 30×14, quota 5 of 6.
- Nothing in it can hurt the player: there are no boulders and no water, and every crystal rests on wall,
  so no digging can make one fall. A test checks this.
- Its golden replay bumps the sealed exit on the way, so the "collect N more" message is part of the
  recorded tutorial.
- Phase 4's acceptance asks that every cave can be failed. Cave 01 cannot be, deliberately, as the spec
  requires; R restarts it at any time.

## D-036: Cave 05 "The Sluice" and its two routes

- SPEC §8 asks for at least two meaningful routes using water and gravity, demonstrated by replays rather
  than asserted. The cave is 30×16, quota 6 of 9.
- **One obstacle, two ways past it.** The exit side is reachable only across a seven-cell channel of deep
  water.
  - **Sluice route:** turn the valve in the left region's floor. The gate in the middle of the channel's
    floor opens, and the channel drains into the sump below, leaving a 3-2-1-0-1-2-3 slope that can be
    waded. The gate sits in the middle because a drain only lowers water within three cells of it
    (D-027).
  - **Boulder route:** dig the earth plug under the boulder perched over the channel's mouth, and step
    aside so it drops in. Then push it along the channel. Each push swaps the deep water ahead of the
    boulder into the cell behind it, the one the player steps into (D-028), so the player crosses while
    the channel is still flooded. The valve is never touched.
- The boulder route is not a new rule. It follows from the documented object-and-water policy, and the
  README's rules of thumb now mention it.
- **Probing for unintended routes:** digging out the entire left region beside the channel drains only its
  near end, because water only flows to a neighbour two units lower. The far five cells stay deep, so a
  side drain is not a third route. A test pins this.
- **Evidence:** a golden replay for each route. Tests show:
  - the sluice route pushes nothing and climbs out with no deep cell left;
  - the boulder route never turns the valve and climbs out with at least four channel cells still deep;
  - a flood fill cannot reach the exit side without crossing.
- Both routes collect the same crystals on the far side. They differ where the spec cares: how the main
  obstacle is overcome.

## D-037: Menus

- `game.Shell`, pure Go and tested headlessly, sits above the playlist. The app opens on a title menu:
  play the campaign, or choose a cave.
  - The cave list shows the campaign and the extras, marking caves completed during this run.
  - **Progress is not saved between runs.** Saved progress belongs with SPEC §11 Phase 5's "local
    preferences".
- From the pause, completion or death screen, C opens the cave list. In the list, Esc goes back to the
  title. `-cave` still starts a cave directly.
- **Input guard:** after any screen change, input is ignored until every key has been released. Without
  it, the Enter that chose a cave could arrive in the game as a fresh press and turn a valve next to the
  start, or a held arrow could walk.
- Menu keys reuse the game's controls (arrows/WASD, Space/Enter, Esc), so there is nothing new to learn.

## D-038: Sound

- **Sounds are synthesised in code** (`internal/sfx`) from oscillators, seeded noise and envelopes. There
  are no audio files to license or ship, and the same cue always gives the same bytes.
- **Ten cues:** crush, complete, exit open, collect, valve, gate, blocked, land, push and dig. Blocked
  covers the sealed exit, deep water and a jammed gate. Moving, falling and rolling are silent; they happen
  too often to be informative.
- **One sound per kind per tick:** each tick's events map to cues at most once each, in priority order,
  so ten boulders landing at once make one sound.
  - Rare, important cues are louder than frequent ones; digging is the quietest.
  - Every cue starts and ends at silence (no clicks) and peaks below half scale, leaving headroom for
    overlap.
- Sound repeats what the screen shows and is never required (SPEC §2).
- **Playback:** the session queues cues only from simulated ticks, so a paused cave is silent.
  `internal/audio` plays them through Ebitengine. M toggles sound; `-no-audio` skips the audio device for
  machines without one, because an audio device error would otherwise end the game loop. A full volume
  control is a follow-up (SPEC §4 asks for one).
- **Builds:** Ebitengine's audio backend uses AudioToolbox on macOS and ALSA or PulseAudio on Linux,
  without cgo, so the build needs no new headers.

## D-039: Measuring performance

- SPEC §11 Phase 4 asks for 60 FPS on a reference Apple Silicon Mac to be measured and reported, not
  asserted. The cloud cannot measure it.
- **The overlay:** the backquote key or F3 (or `-perf`) toggles an overlay showing:
  - Ebitengine's measured FPS and TPS;
  - CPU time per update, which includes every simulation tick;
  - CPU time per draw (submitting draw calls, not GPU time).

  Times are averages and p95 over the last four seconds (`internal/perf`). Turning the overlay off prints
  the same numbers to the terminal, for the playtest report.
- **Frame rate and simulation:** results cannot depend on frame rate by construction. The simulation runs
  on a fixed 60 updates per second, independent of the display's refresh rate, and the golden replays pin
  its outcomes.
- **Limitation:** under the headless capture driver, FPS and TPS read 0, because the host drives the
  guest's frames. The overlay's numbers are only meaningful in a real window.

## D-014 addendum: Phase 4 headless capture (2026-10-18)

- The by-hand `exp/vmhost` setup (Linux, Xvfb, Mesa software GL; nothing committed) drove the unmodified app.
  The driver now takes a small script (`shot`, `wait`, `tap`, `fixture`) and can read the guest's audio
  streams.
- **Menus:** injected keys walked from the title through the cave list into cave 05. All three screens
  rendered.
- **Boulder route:** `cave_05_boulder_route.json` was replayed with sound on. The guest opened six audio
  streams at 48 kHz, and each matched a synthesised cue sample for sample: dig, collect, land, push, exit
  open and complete. That is exactly this route's events (no valve or gate), each at its designed peak.
  This shows the app produces the intended sound data; it is not the Mac's speakers.
- **A real fix came from it:** a player standing in deep water behind the pushed boulder was nearly
  invisible, because water was drawn over the player. Water is now drawn first, and only the player's legs
  are tinted.
- **Other runs:**
  - `-no-audio` runs with no audio streams;
  - the performance overlay draws legibly (its numbers are meaningless under the driver, D-039).

## D-040: Phase 5 starts with the creature only (owner decision)

- SPEC §11 Phase 5 lists a cave editor, a wall-following creature, structural supports, optional level
  selection, local preferences and a visual theme, "prioritised on prototype feedback". On 2026-10-18 the
  owner chose the **creature** for this branch, as one focused change.
- Level selection already exists (the Phase 4 cave list, D-037). The editor, supports, preferences and
  theme are not started.
- The Mac playtests of Phases 2 to 4 are still pending (D-033). Tuning they ask for still needs its own
  rule version.

## D-041: Crawlers (rule version 4)

SPEC §6.5 asks for "a deterministic wall-following organism or simple patrol", implemented after gravity
and water are stable, with tests for movement order, collision and reset.

- **Left-hand rule.** Each crawler step tries left, ahead, right, behind relative to its facing and takes
  the first open cell. It is the simplest rule that follows walls, needs no memory beyond a facing, and
  is readable: a player can predict where it goes next. Open space makes it turn in a 2×2 square, which is
  also predictable. It never draws from the PRNG.
- **Cadence:** a step every `CrawlerIntervalTicks = 6` ticks (3.3 cells/s), slower than the player's 4, so
  the player can outpace and follow one. It is a multiple of the gravity interval, so crawler steps happen
  inside world steps. Provisional, to tune in the Mac playtest.
- **Order:** player, gravity, crawlers, water, drowning, quota.
  - Gravity first: a boulder falling into a cell a crawler wanted is already there.
  - Crawlers judge water as the last water step left it, and never step into any.
  - Drowning after the water phase, so water arriving this step drowns this step.
  - Crawlers are scanned row by row from the top. A cell a crawler moved into is skipped by the rest of the
    scan, so each moves at most once. Of two crawlers wanting one cell, the first in the scan gets it.
- **Danger only on contact.** A crawler catches the player by stepping into their cell; walking into a
  crawler is equally fatal. Standing next to one is safe. The rule makes threats visible (a crawler must
  be heading at you) and gives safe spots: openings on a crawler's right are only taken when its left and
  ahead are blocked.
- **A catch ends the crawler step,** so nothing else moves after the player is caught. The rest of the
  world step (water, drowning) still runs, as after a crush.
- **Crushing follows the player's rule:** only a falling object crushes, and it falls on into the cell. A
  crawler under a resting boulder holds it up.
- **Water:** crawlers refuse any water, even one unit, and drown when water reaches them. Flooding becomes
  a tool against them, the "demonstrated interaction with a pre-existing system" SPEC §7 asks for.
- **No gravity for crawlers:** they cling to walls, so vertical patrols (shafts) work like corridors.
- **State:** a crawler's facing is a new cell field, hashed (layout v4, one more byte per cell). It is
  needed to replay a crawler's path, so it is part of the state.
- **Fixture review:** all nine earlier golden replays have no crawlers. Traced on master and on this
  build, they agree on every tick's events and grid. Only `rule_version` and the two hashes changed, from
  the new layout and rule version.

## D-042: Cave format version 4

- Rows may contain a crawler, written as the arrow of the way it starts facing: `↑` `↓` `←` `→`. Arrows put
  the facing, part of the initial state, in the grid where a designer sees it. The dumps (`cavetool show`,
  traces) use the same arrows, so a cave file and its dump read alike.
- These are the first non-ASCII symbols. Cave files are UTF-8 JSON, and widths count symbols (runes), not
  bytes, as they already did.
- An arrow in a file below version 4 is rejected with the version it needs. Older files are unchanged.
- `cavetool validate` reports crawler counts.

## D-043: Crawler Warren, the example cave

- SPEC §7: every new system needs documented behaviour, deterministic tests, at least one example cave and
  a demonstrated interaction with an existing system. SPEC §8 fixes the five starter caves, so the
  example is an **extra**, listed before the Proving Ground. The campaign is unchanged.
- **Three lessons, one crawler each,** 30×16 so it fits on one screen, quota 5 of 6:
  1. **The ring:** a crawler circles a pillar anticlockwise, the pillar on its left. Three crystals sit in
     pockets on its right, safe places to wait while it passes. The way in, dug from above, is also on its
     right. The quota needs at least two of these crystals.
  2. **The drop:** a crawler paces a one-wide shaft, the only way down. A boulder pushed in falls three
     times as fast as the crawler moves and always crushes it. The only refusal is when the crawler is in
     the top cell, blocking the push.
  3. **The flood:** a crawler paces a five-cell corridor in front of the exit. The valve drops nine units
     of water into its middle. On a flat floor, water settles as a slope of one unit per cell (D-027), so
     nine units become `12321`: the whole corridor gets wet, so the crawler drowns, and no cell is deep, so
     the player can wade through. More water would leave a deep cell under the gate and block the way; a
     longer corridor would leave dry ends where a crawler could survive.
- **Evidence:** a completion replay and a death-by-crawler replay, both in the 100-run determinism check.
  - Tests sweep the push and the valve turn over a whole patrol: every timing crushes, or drowns, the
    crawler, and the flood always settles `12321`.
  - A flood fill that may dig any earth and pass any boulder, but avoids the shaft, cannot reach the lower
    half.
- The first draft flooded an eleven-cell corridor from 24 units. It left a deep cell under the gate and
  blocked the exit. The planner found this before any replay was recorded.

## D-044: Presenting crawlers

- **Sprite:** an original six-legged cave beetle, drawn in code like every other sprite, in violet so it
  stands apart from the teal player, cyan crystals and grey boulders. Its head, glowing eyes and feelers
  point the way it faces: one drawing, turned four ways, which a test checks. The shape (legs, segments)
  differs from every other sprite, not only the colour.
- **Movement:** a crawler glides between cells over its whole step (18 updates), so a patrol reads as
  continuous. A crushed or drowned crawler stops being drawn at once. The same fix applies to a crystal
  collected while still gliding into its cell, which could briefly linger before (a test now checks that
  every gliding sprite is heading for a cell that really holds it, through three whole fixtures).
- **Death screen:** deaths now have a cause. CAUGHT says whether the player walked into a crawler or a
  crawler caught them, and repeats the left-hand rule. The caught player is drawn shrunk and violet
  rather than squashed. Logged replays say "was caught by a crawler".
- **Prompts:** a crawler cave opens with "Crawlers keep the wall on their left: stay out of their way".
  Crushed and drowned crawlers are announced.
- **Sound:** two new cues, a chittering bite for a catch and a wet pop when a crawler is crushed or
  drowned. Walking crawlers are silent, like falling objects: they move too often for a sound to carry
  information. So an approaching crawler can only be seen, not heard (follow-up F-21).

## D-014 addendum: Phase 5 headless capture (2026-10-18)

- The by-hand `exp/vmhost` setup (Linux, Xvfb, Mesa software GL; nothing committed) drove the unmodified app
  through both Crawler Warren fixtures. The driver had to be rebuilt in a fresh container. Its fixture
  playback now waits one update first, because the shell ignores input until every key is released
  (D-037), and a key held on the very first update was being dropped.
- **Death by crawler:** the crawler glides along the corridor facing the player; the CAUGHT overlay shows
  the left-hand hint and the violet, shrunk player. Audio: dig, collect and the new caught cue, each
  matching its synthesised samples exactly.
- **Completion:** the boulder falls down the shaft as "A falling boulder crushed a crawler" shows; the
  sluice water drops into the corridor as "The water drowned a crawler" shows, and settles as a visible
  1-2-3-2-1 pool; CAVE COMPLETE reads 6 crystals in 0:39, the fixture's tick 797. Audio: nine streams,
  each an exact cue match: dig, collect, push, crawler gone, land, exit open, valve, gate and complete.
- **Menus:** the cave list shows Crawler Warren under Extras, before the Proving Ground.
- Linux/OpenGL evidence only, not a Mac test.

## D-045: The cave editor, milestone 1 (owner decision)

- On 2026-10-25 the owner chose the **cave editor** as the next Phase 5 item (SPEC §8 "Level editor", §11
  Phase 5). They accepted a three-milestone plan and asked for playtesting to be part of the first:
  1. paint, validate, save and **playtest** (this change);
  2. the player's own caves in the cave list, and replays of playtests;
  3. valves, gates and their links, plus design aids.
- **Built into the game app,** not a separate program. This matches SPEC §14's "native integrated cave
  editor, quick playtesting", needs no second `.app`, and reuses the renderer, the sprites and
  `game.Session`.
- **Offered to everyone** from the title menu ("Cave editor"), and with `-edit FILE|ID|new` for
  development.
- **SPEC §8's minimum is met:** painting terrain; placing the player, boulders, crystals, the exit and
  water; save and load with validation. Crawlers are paintable too, since they are only arrows in the
  rows. Valves and gates are not yet, because they need linking into mechanisms (milestone 3).
- Like D-033 and D-040, this goes ahead before the pending Mac playtests. No rule changes: rule version 4,
  cave format version 4, and every golden replay unchanged.

## D-046: Canonical cave layout, `cavetool fmt`, and drafts

- **Writing caves.** The editor must write caves back to JSON. `level.Draft` is a cave file's content
  before validation, and `Draft.Encode` writes the layout the hand-made caves already used:
  - fields in documentation order;
  - one row per line, so the grid reads as a picture in a diff;
  - one mechanism per line;
  - metadata keys sorted;
  - no `mechanisms` or `metadata` field when there are none.
- **The embedded caves are canonical.** Two caves wrote their mechanisms over several lines, so `cavetool
  fmt` reformatted them (cave 05, Crawler Warren).
  - `Checksum` hashes parsed values, not text, so their checksums are unchanged and every golden replay
    still matches.
  - A test and a CI step (`cavetool fmt -check`) keep the embedded caves canonical.
  - So the editor opens and saves any embedded cave byte for byte, which is tested for all seven.
- **Work in progress.** `level.DecodeDraft` reads a cave checking only the JSON shape, so the editor can
  reopen a cave saved with problems and fix it. `level.Parse`, which the game uses, is unchanged.
- **Located problems.** `level.Problem.At` lists the cells a problem concerns, so the editor can outline
  them:
  - unknown symbols, and symbols too new for the file's format version;
  - extra player starts or exits;
  - wrong or unlisted valve and gate cells, and gate state mismatches.
- **One validator.** The editor validates by encoding the cave and running `level.Parse`, the game's own
  loader. Validation lives in one place, and the editor and the game cannot disagree.

## D-047: A cave keeps its format version

- A cave's `format_version` is part of its checksum, so changing it breaks the cave's golden replays.
- The editor **keeps the version it opened**. Painting a symbol the version cannot hold raises it to the
  first version that can, and says that replays of the old version will not match. For example, water in
  the version 1 Proving Ground raises it to version 3.
- The version is never lowered. A new cave uses the current version.
- A file with no version, or one too old, opens as the current version; such a file never loaded in the
  game, so no replay depends on it. A file newer than this build does not open.

## D-048: Editing, saving and the editor's keys

- **Layout.** A status bar (cave, size, quota, problems, unsaved), a 30×16-tile view, and a bottom bar
  with the palette and the first problems. `editor.Layout` describes it for both hit-testing and drawing.
  It is pure Go (like `Camera`, D-012), so the mouse mapping is unit-tested.
- **Brushes (1 to 9):** wall, earth, floor, crystal, boulder, player start, exit, water, crawler.
  - Pressing a brush's key again, or clicking its slot, moves to the next variant: water from deep to 1,
    and crawlers turning clockwise.
  - A new cave starts as one screen of earth in a wall, with the player start and the exit in opposite
    corners and no quota.
- **The mouse:**
  - left drag paints along the pointer's path, with no gaps however fast it moves;
  - right drag erases to floor;
  - Shift-drag fills a box on release;
  - Alt-click picks the brush of a cell;
  - Esc cancels a stroke in progress.
- **Moving the view:** arrows, WASD or the wheel scroll it. A cave smaller than the view is centred.
- **An edit changes only the cells painted, with two exceptions:**
  - The player start and the exit **move** rather than duplicate, since a cave needs exactly one of each.
    A box of either places one, where the drag ended.
  - Painting over a valve or gate **takes it out of its mechanism**. A removed valve takes its mechanism
    with it, and its gates are outlined as unlisted until they are removed. Nothing else changes behind
    the designer's back.
- **Resizing.** Cmd+arrow adds a column or row on that side; Cmd+Shift+arrow removes one.
  - New cells are wall, the neutral fill at a cave's edge.
  - Mechanism positions move with their cells, and the view stays on the same cells.
  - The size limits are 1 to 256 each way.
- **Undo and settings.**
  - Undo (Cmd+Z) and redo (Cmd+Shift+Z) take one step per stroke, box, resize or settings change, 200
    steps deep.
  - Tab opens the settings: title, id, seed and quota, edited as text. Ids accept only valid characters;
    the seed and quota take digits, and the seed is range-checked.
- **Saving (Cmd+S):**
  - A file opened from disk saves back to it.
  - A new cave or a copy saves as `<id>.json` in `~/Library/Application Support/Deep Shift/caves/`
    (`-caves-dir`). It never replaces an existing file without a second Cmd+S.
  - Writes are atomic: a temporary file, then a rename.
  - **A cave with problems still saves,** with a warning, so work is never lost; the game will not play it
    until they are fixed.
  - Esc with unsaved changes asks first.
  - A copy of a built-in cave gets `_copy` and ` (copy)` added to its id and title, and
    `{"based_on": <id>}` as metadata. Its layout, and so its checksum, is the original's.
- **Shortcuts in text fields.** F, M and `` ` `` are not shortcuts while a settings field is being typed
  into.
- **Cmd or Control.** Cmd shortcuts accept Command or Control, for keyboards without a Command key. On a
  Mac, Ctrl+arrows would switch Spaces, so Cmd is the one to use there.
- **Not yet built:**
  - valve and gate brushes (done in milestone 3: D-052);
  - editing metadata;
  - "save as" (a cave keeps the file name of its first save, even if its id changes);
  - a full problem list (the bar shows the first one or two, and outlines every cell concerned).

## D-049: Playtests

- **P plays the cave as it stands.**
  - The editor validates it with `level.Parse`. The shell starts an ordinary `game.Session` on the
    resulting level, so a playtest is the game, not an imitation: cave 01's golden replay completes a copy
    of cave 01 in a playtest, with the same final hash (tested).
  - A cave with problems cannot be playtested, and the bar says how many problems are left to fix.
- **P returns to the editor at any time,** with every edit, the view and the undo history intact.
  - The overlays say "P: back to the editor" instead of the playlist's N and C, which do nothing in a
    playtest (`Session.SetNavKeys`).
  - The shell feeds exactly one of its inputs per update, chosen by the screen at the update's start, and
    the editor ignores input until every key is released. So the P that ends a playtest never starts
    another.
- **Playtests are not saved as replays.** The cave may not be saved, and a replay is only useful beside its
  cave file. Replays of saved, unchanged caves are milestone 2 (done: D-051).
- **Playtests are not campaign progress.** They never mark a cave "(done)".

## D-012 addendum: `internal/editor` (2026-10-25)

- `internal/editor` now exists, scoped as D-045 to D-049 describe. It is pure Go, like `internal/game`. A
  test allows it only the standard library and the pure `internal/level` and `internal/sim` packages.
- `internal/render` draws its `View`, and `cmd/deepshift` turns the mouse and keyboard into its `Input`.

## D-014 addendum: cave editor headless capture (2026-10-25)

- The by-hand `exp/vmhost` setup (Linux, Xvfb, Mesa software GL; nothing committed) drove the unmodified
  app. The driver gained an `-args` flag for the guest's flags, and scripted mouse, key and typed-text
  input. Four sessions were run:
  - **The title menu** shows "Cave editor". Its list shows New cave, the copies of the built-in caves, and
    "(more below)" where the list scrolls.
  - **A new cave (`-edit new`).** Each of these drew as intended:
    - crystals painted in a drag;
    - the Shift-drag box preview, then the boulders it filled;
    - deep and shallow water;
    - a crawler, and the translucent preview of the crawler brush under the mouse;
    - a wall over the exit, giving "1 problem" and "rows: no exit ('E') found" in the bar, then Cmd+Z;
    - the settings panel, and a title typed into it.
  - **A playtest of that cave.** It started with "Playtest: P returns to the editor". Its pause overlay
    read "Esc: resume   R: restart   P: back to the editor", and P returned to the editor with every edit.
    Cmd+S showed "Saved new_cave.json", and Esc showed the list with the saved cave selected under "Your
    caves".
  - **A copy of Crawler Warren** showed the valve's link to its gate. Painting over the valve outlined the
    orphaned gate in red, with the problem text in the bar.
  - **The 40×24 Proving Ground (a copy of its file)** scrolled to its edge and grew by two columns. Esc
    with unsaved changes asked first, and the file was not touched.
- **The capture found four faults, now fixed:**
  - growing the cave left the new columns out of view;
  - the grid was too faint to see;
  - saved caves were private (0600) rather than 0644;
  - the floor brush's slot was hard to see against the bar, so every slot now has a frame.
- This is Linux/OpenGL evidence only, not a Mac test. The trackpad, Retina scaling, the Command key and
  wheel direction on macOS still need a Mac.

## D-050: My caves in the cave list (editor milestone 2)

- **What it is.** The cave list gains a "My caves" section after the extras: the caves in the editor's
  folder (`-caves-dir`). The folder is read afresh each time the list opens, so a cave saved a moment ago is
  there.
- **Playable caves play like any other.**
  - N moves through the built-in caves, then the player's own, then wraps.
  - Completing one marks it "(done)" for the run, as for any cave. The campaign count is unchanged.
  - C from the pause, completion or death screen returns to the list with the cave just played selected.
    The match is by file, since the list is rebuilt.
- **A cave that cannot be played is still listed,** as "(cannot play)", so a cave saved with problems is not
  silently missing. Choosing it says why:
  - the file cannot be read;
  - it has problems to fix in the Cave editor (with the count);
  - its id is taken by a built-in cave, or by an earlier file in the folder.
- **Why ids must be unique.** Completion marks and replays both name caves by id, so two caves sharing one
  would be confused. The first in file-name order plays, and later ones say which file has the id.
- **Validation.** A file is validated the same way as everywhere else: `level.Parse` of its canonical
  encoding. That gives the same result and checksum as parsing the file itself (D-046).

## D-051: Replays of playtests and of the player's caves

- **Playtests are recorded when the cave is saved and unchanged.** Then a cave file matches the replay
  exactly, and it can be re-run. A playtest of a new cave, a copy not yet saved, or a cave changed since its
  last save is not recorded: the app says "Playtest not recorded: save the cave first (Cmd+S)". This
  completes what D-049 left for milestone 2.
- **The shell decides.** `Shell.FinishedAttempt` replaces the platform layer's own playtest rule, so the
  rule is tested without a display.
- **Replays of the player's caves name the cave file,** whether played from the list or playtested.
  `cavetool replay` finds only built-in caves by id, so it needs the file as `-cave`.
  - A playtest replay starts "Recorded in an editor playtest".
  - Tests re-run both kinds with `replay.Run` against the saved file and check their expect blocks.
  - In the headless capture, `cavetool replay -cave` matched a playtest replay the app saved.

## D-014 addendum: editor milestone 2 headless capture (2026-10-25)

- The by-hand `exp/vmhost` setup (Linux, Xvfb, Mesa software GL; nothing committed) used a scratch caves
  folder. It held a copy of cave 01 under a new id, a cave with an impossible quota, and a one-step cave.
- **The cave list:**
  - "My caves" follows the extras, with the cave with problems marked "(cannot play)";
  - choosing it showed "Cannot play wip.json: 1 problem; fix it in the Cave editor";
  - choosing the copy played it, its title in the HUD.
- **Playtests:**
  - a playtest of the saved one-step cave showed "Replay saved: one_step-complete-….json", and `cavetool
    replay -cave` matched the file;
  - after an edit, the next playtest showed "Playtest not recorded: save the cave first (Cmd+S)", and no
    second file was written.
- This is Linux/OpenGL evidence only, not a Mac test.

## D-052: Valves, gates and their links (editor milestone 3)

- **Two brushes:** Valve (0) and Gate (-). Every valve and gate must belong to exactly one mechanism, and a
  gate must be open exactly when its valve is (D-029, D-031). The editor maintains that as it goes, so
  linking needs no separate mode.
  - **A painted valve** is shut and gets a mechanism of its own. It becomes the **selected valve**, ringed
    in yellow, with its links drawn bold. Until it has a gate, validation reports it, outlined.
  - **A painted gate** joins the selected valve's mechanism, open or closed to match it. A gate already
    linked to another valve moves to the selected one.
  - **With no valve selected,** a new gate is left unlinked: it is reported and outlined, and the bar says
    to paint or select a valve. Repainting an existing gate with no valve selected leaves it as it was.
  - **Alt-click on a valve or gate** selects its valve and the gate brush, so gates can be added to an
    existing mechanism, such as cave 04's.
  - **O on a valve or gate** opens or closes the whole mechanism at once. That is how the cave starts,
    as if the valve were turned before the first tick; no current cave starts open.
- **Removing.** Painting over a valve or gate unlinks it, as in milestone 1. A removed valve takes its
  mechanism with it, and the selection goes too.
- **Boxes and fills of valves place one,** at the end of the gesture, like the player start and exit. A box
  of valves, each with no gates, is never wanted.
- **Format version.** Valves and gates need format version 3, so painting one raises an older cave's
  version, with the usual warning (D-047).
- **Evidence:**
  - the random-input invariants now check every mechanism after every step: valves are valves, gates are
    gates, each linked once, and gates match their valve;
  - a test builds a sluice in the editor and turns its valve in the simulation;
  - a shell test playtests one and opens its gate with Space.

## D-053: The reach overlay

- **R shades the cells the player cannot reach** from the start by walking and digging alone, with nothing
  else moving. The bar gives a summary: reachable crystals against the quota, and whether the exit is
  reached.
- **The rules it follows:**
  - **entered:** floor, open gates, shallow water, earth, crystals and crawlers (a crawler moves on);
  - **blocked:** walls, closed gates, valves, deep water, boulders and unknown symbols;
  - **the exit** is reached, but not passed, since entering it ends the cave.
- **It is a first look, not a solver.** Boulders fall and can be pushed, valves turned and water drained,
  so a cave can be solvable when the overlay says not, and the reverse. Cave 04, for example, shows 3 of 9
  crystals and no exit until its sluice is used. A solver over the real rules is a separate, larger
  piece of work, like the planners used to design the campaign.
- **Shading.** Walls are not shaded: they are never reachable, and shading them hid the floor and earth
  that matter (seen in the headless capture).

## D-054: Flood fill, and copy and paste left for later

- **Cmd-click flood-fills** the 4-connected area of cells matching the clicked cell with the brush, as one
  undo step. Cmd-right-click fills it with floor. Filling with a player start, exit or valve places one,
  like a box.
- **Cmd or Control.** Cmd-click accepts Control-click too, like the other Cmd shortcuts (D-048).
- **Copy and paste is not in this milestone.** It needs a selection separate from the Shift-drag box, and
  copied valves and gates must keep their links, joined to a copied valve or to the selected one. That
  deserves its own change: follow-up F-27.

## D-014 addendum: editor milestone 3 headless capture (2026-10-25)

- The by-hand `exp/vmhost` setup (Linux, Xvfb, Mesa software GL; nothing committed) drove `-edit new`:
  - a painted valve, ringed and selected, reported as "a valve must operate at least one gate";
  - three gates painted with the gate brush, linked to it by bold lines, and the problem gone;
  - O opened the valve and its gates;
  - R shaded a walled-off room, and Cmd-click (Control under the driver) filled it with floor;
  - in a playtest, Space at the valve opened all three gates: "Sluice gate open".
- **A copy of cave 04:** Alt-click on a gate selected the valve, the gate brush and the ringed valve.
- **One fault found and fixed:** the overlay shaded walls too, which hid what it is for.
- This is Linux/OpenGL evidence only, not a Mac test.

## D-055: Arcade music (owner decision)

Numbered D-055 because the open cave-editor pull requests already use D-045 to D-054 (and follow-ups F-23
to F-28); this one merged first.

On 2026-10-25 the owner asked for arcade music that can be turned off and on. From a written plan they
chose a separate B key, music on at launch, and Esc pausing it. SPEC §2 already allows "optional
ambience/music".

- **Synthesised in code, like the cues** (`internal/sfx/music.go`, D-038). There is no audio file to
  license or ship, and the loop always gives the same bytes. The tune was written from scratch for this
  game; it is not taken or adapted from Boulder Dash or any other game.
- **The tune:** 16 bars of 4/4 at 120 BPM (32 s) in A minor. An eight-bar theme over Am F C G, Am F G E is
  answered by eight syncopated bars over Dm Am F C, Dm Am E E, whose last bar leads back to the start.
  Four parts in the style of an early arcade sound chip:
  - a pulse-wave lead (25% duty), with a gentle vibrato on long notes;
  - a thinner pulse arpeggio (12.5%), rising through the chord one tone per sixteenth;
  - a triangle bass jumping octaves in eighth notes;
  - noise drums: kick on 1 and 3, snare on 2 and 4, hi-hats between, and a fill every eighth bar.

  Pulse edges are smoothed (PolyBLEP), so high notes do not alias into harsh extra tones.
- **Seamless loop:** a sixteenth note is exactly 6,000 samples, so the loop ends on a bar line. Every note
  fades out within its own steps, so the loop starts and ends in silence and repeats with no click. A test
  pins both.
- **Quieter than the cues:** the music peaks at half a cue's peak (0.25 of full scale). Cues stay the
  loudest sounds, so the music does not mask one the player relies on (SPEC §2), and music plus the
  loudest cue cannot clip. Provisional: the balance needs the Mac playtest (and F-17, volume control).
- **Keys:**
  - M still turns all sound off and on, music included. It was already documented and is in the Mac
    checklists.
  - B turns the music alone off and on. Pressed while sound is off, B turns the music on for when M brings
    sound back, and says so.
  - With `-no-audio`, both keys say there is no sound.
- **Where it plays:** the title, the cave list and the caves, from launch.
  - It holds while a cave is paused, because a paused cave is silent (D-038), and resumes where it stopped.
  - The cave list opened from a pause is a menu, so the music plays there.
  - It keeps playing on the completion and death screens; their cues play over it.
- **Settings last one run.** There are no saved preferences yet (F-16); `-no-music` starts with the music
  off.
- **Code:** the rules and their notices are a pure-Go `game.Sound`, tested headlessly; M's handling moved
  there from `cmd/deepshift`. `internal/audio` loops the PCM with Ebitengine's `InfiniteLoop`. The loop
  takes 6 MB of memory and about 90 ms to generate at startup on the cloud runner, before the window opens.
- **The simulation is untouched:** the music reads no game state. Rule version, cave format, replays and
  hashes are unchanged.
- **HUD:** the controls hint now names B. To fit, its gaps shrank to two spaces. It is 78 of the 79
  characters a HUD line holds (a test checks the hint and both menu footers), so another key needs a
  shorter hint or a real UI font (F-4).

## D-014 addendum: arcade music headless capture (2026-10-25)

- A fresh by-hand `exp/vmhost` driver ran the unmodified app (build `bbbf36c`, guest tag
  `ebitenginevmguest`) under Xvfb with Mesa software GL. Nothing was committed. The host pulled each audio
  stream at real-time pace (800 frames per update) and compared it with `sfx.MusicPCM()` exactly.
- **The loop:** a single 48 kHz stream opened on the first update, on the title, and played on into a
  cave.
  - All 1,840,000 frames read (38 s) matched the loop. The stream starts at the loop's first sample and
    wraps at exactly 32 s.
  - The loop's first and last samples are exactly zero, so the seam has no step at all.
- **B, Esc and M** each stopped the stream outright: zero frames for a second. Pressing the key again
  resumed it from the frame where it stopped, not from the start, both on the title and in cave 01.
  - Esc paused and resumed the music with the cave.
  - B and M showed "Music off (B: on)", "Music on", "Sound off (M: on)" and "Sound on" in the HUD.
  - M pauses the music player rather than turning its volume down.
- **Flags:**
  - `-no-music` opened no stream until B, then the loop from its start.
  - `-no-audio` opened no streams at all, and B and M both said "No sound: started with -no-audio".
- **Frames:** the title footer and the cave's hint line, the longest at 78 characters, draw in full, with
  10 px and 17 px to spare.
- **Seen, not changed:**
  - On the title and cave list, M and B give no text notice, because notices are a HUD prompt; the music
    starting or stopping is the feedback.
  - The loop's largest sample-to-sample step (0.14, at 19.5 s) is a pulse-wave edge at a note's start.
    Steps that large are common across the loop, as square waves have, rather than a lone click.
- **Limits:** the host sets the pace of the audio and the guest has no output buffer. So this shows the
  app never rewinds or seeks the music on resume, but not how a real device sounds at a pause. No cue
  played in these runs, so M was checked against the music only. Linux/OpenGL evidence only; the music has
  not been heard on a Mac.

## D-055 addendum: the music and the cave editor (2026-10-25)

The music (D-055) reached master before the editor, so the editor branch took it in with a merge. Where
the two meet:

- **B is not a shortcut in a text field,** like F, M and `` ` `` (D-048): while a settings field is being
  typed into, B is a letter.
- **A paused playtest holds the music,** like a paused cave. `Shell.Paused` covers both, and a test pauses
  a playtest and checks.
- **The music plays in the edit list and the editor,** as on the other menus, and B turns it off. The edit
  list's footer names M and B; the editor's key bar has no room for them.
- **No text notice outside caves and playtests:** in the editor, as on the menus, M and B change the sound
  without a message.

## D-056: Glimmers and shattering (rule version 5, owner decisions)

- **Request (owner, 2026-10-25):** in the 1984 game, a rock falling on a butterfly turned it into
  diamonds; add that mechanic. The plan offered five choices with recommendations, and the owner took all
  five:
  1. **A 3×3 shattering that destroys nothing.** Only empty, dry floor in the block becomes crystal. A
     faithful explosion would destroy terrain and kill a player in range, which needs damage rules first:
     SPEC §7 lists explosives as "Later", with "damage/explosion events" as the prerequisite.
  2. **An original name, the glimmer.** SPEC §1 asks for original terminology.
  3. **The right-hand rule.** A glimmer is the crawler's mirror image, which is follow-up F-20's "right-hand
     crawler", and its path tells it apart from a crawler at a glance.
  4. **A drowned glimmer leaves nothing.** Crystals cannot share a cell with water (D-028), so crystals
     formed in water would break conservation. It also makes flood-or-crush a real choice.
  5. **A shattering never harms the player,** in line with D-041's "danger only on contact".
- **Movement:** right, ahead, left, behind relative to its facing, and the first open cell wins. Otherwise
  everything is the crawler's: the cadence (`CrawlerIntervalTicks`), catching the player either way,
  refusing water, drowning, no gravity. Crawlers and glimmers share **one** scan, so of two creatures
  wanting a cell, the first in row-major order gets it, whichever kind it is.
- **Shattering:** a **falling** boulder or crystal landing on a glimmer removes it and leaves a crystal in
  its cell and in every empty, dry floor cell of the 3×3 block around it, in row-major order. Earth,
  walls, the exit, gates, valves, objects, creatures, the player and wet cells are left alone.
  - **The object stops** where it was and lands, rather than falling on through as it does after crushing
    a crawler: the glimmer's cell now holds a crystal.
  - **At least one crystal:** water drowns a glimmer in the step it arrives, so a glimmer's cell is always
    dry when gravity runs.
  - **New crystals rest and sit out the rest of that gravity step,** like a pushed boulder (D-018). No
    object moves twice in a tick (SPEC §5), and a crystal formed over the player's head cannot crush them
    on its first step. They fall and roll from the next step.
  - **Events:** `glimmer-shattered` (object → glimmer), `crystal-formed` for each crystal, then `landed`.
- **Quota:** a cave may count on up to `ShatterCrystals = 9` crystals per glimmer. That only rules out an
  exit that could never open; a completion replay shows a cave can be finished.
- **Events and state:** new events `glimmer-moved`, `glimmer-shattered`, `crystal-formed` and
  `glimmer-drowned`. `player-caught` now names the creature. Glimmers reuse the facing byte, so the hash
  layout stays v4; only the rule version changed.
- **Fixture review:** all eleven earlier golden replays have no glimmers. Traced on master and on this
  build, they agree on every tick's events and grid; only `rule_version` and the two hashes changed.
- **Evidence:**
  - a mirror test: 30 random caves, each run again mirrored with crawlers and glimmers swapped, match
    tick for tick;
  - a table of every kind of neighbour, for falling boulders and falling crystals;
  - randomised runs that check every shattering's crystals and the crystal count;
  - five mutations, each caught: new crystals not waiting a step, glimmers using the left-hand rule,
    crystals forming in water, resting objects shattering, and a block that wraps around the cave edge.

## D-057: Cave format version 5

- Rows may contain a glimmer, written as the triangle pointing the way it starts facing: `▲` `▼` `◀` `▶`.
  Triangles read as facings like the crawler arrows (D-042) and cannot be mistaken for them. Dumps use the
  same symbols.
- `required_crystals` may exceed the crystals in the rows by up to 9 per glimmer, matching the simulation.
  The error for a quota that is too high states the sum.
- A triangle in a file below version 5 is rejected with the version it needs; older files are unchanged.
  `cavetool validate` reports glimmer counts.

## D-058: Glimmer Hollow, the example cave

- SPEC §7 again: the example cave is an **extra**, listed after Crawler Warren. It is 30×16 with a quota of
  7, and has three glimmers, one per lesson:
  1. **The ring:** a glimmer circles a pillar clockwise, the pillar on its right. Three crystals sit in
     pockets on its left, safe places to wait while it passes, and the way in is on its left too.
  2. **The vault:** a glimmer is boxed in under a closed sluice gate, with a boulder resting on the gate
     between two walls. The vault's valve opens the gate, the boulder drops into it, and on the next
     gravity step it shatters the glimmer into five crystals. Two rest beside the gate, two fall into a
     basin, and the fifth stays sealed under the boulder.
  3. **The flood:** a glimmer paces the corridor to the exit. A second valve drops nine units of water
     into its middle, which drowns it whatever the timing, leaves no crystals, and settles 1,2,3,2,1, so
     the player can wade through (D-043's corridor, reused).
- **Why a vault rather than a pushed boulder.** A pushed boulder catches a glimmer at the same moment
  only if the glimmer is confined to the column it falls down, and in a one-wide shaft the walls leave
  room for at most two crystals. Any open floor beside the glimmer is open to the glimmer too, so it would
  walk off. A boxed-in glimmer stays put, and the four cells diagonal to it can be open floor that it cannot
  reach. A gate is the one thing that stops a glimmer leaving yet lets a boulder in. **Gravity runs before
  creatures in every tick,** so the boulder always fills the opened gate before the glimmer could step into
  it: the vault pays out five crystals whenever it is opened. Its boulder sits between walls, so it cannot
  be pushed off the gate, which would make the cave impossible.
- **The quota needs both lessons:** the 5 placed crystals fall short, and so do the 2 outside the ring's
  pockets plus the 4 vault crystals the player can reach.
- **Evidence:** a completion replay and a caught-by-a-glimmer replay, both in the 100-run determinism
  check. Tests also:
  - follow the ring glimmer round a whole lap, clockwise;
  - check that the vault glimmer is boxed in and its boulder cannot be pushed;
  - open the vault on every tick of a 12-tick window: always the same five crystals;
  - open the sluice at every point of the corridor patrol: always drowned, no crystals, 1,2,3,2,1;
  - show that the quota needs both lessons and that the corridor is the only way to the exit.

  With the gravity and creature phases swapped, the vault test fails: the glimmer escapes through the gate.
- **Known way to lose:** digging down into the corridor before opening the sluice lets the corridor
  glimmer climb out up the player's tunnel, as in Crawler Warren. The sluice valve stands at the top of
  that tunnel, so flooding first is the natural order.

## D-059: Presenting glimmers

- **Sprite:** an original winged creature drawn in code, with amber wings, a dark body and a cyan crystal
  shard on its back: the crystals it holds. Its head and knobbed antennae point the way it faces, one
  drawing turned four ways like the crawler. Wide wings and no legs give it a different outline from the
  six-legged crawler (checked by the sprite shape test), not only a different colour.
- **Movement:** glimmers glide over a whole creature interval, like crawlers (D-044). A shattered or drowned
  glimmer stops being drawn at once, even mid-glide.
- **Prompts:**
  - a glimmer cave opens with "Glimmers keep the wall on their right: drop a rock on one for crystals",
    and a cave with both kinds of creature names both walls;
  - a shattering says how many crystals it left ("…shattered a glimmer into 5 crystals");
  - a drowning says the glimmer "left no crystals".
- **Death screen:** CAUGHT names the creature and the wall it keeps. The caught player is tinted amber for
  a glimmer and violet for a crawler, and logged replays say which creature caught them.
- **Sound:** a new cue, a glassy crack and a quick run of high chimes, for a shattering, reported once per
  tick with the landing thud. Crystals forming make no sound of their own. A drowned glimmer uses the
  crawler's wet pop, renamed `creature-gone`.

## D-014 addendum: glimmer frames captured (2026-10-25)

- A scratch program, not committed, ran the real `game.Session` and `render.Renderer` in an Ebitengine
  window under Xvfb with Mesa software GL. It drove both Glimmer Hollow fixtures with the same key-press
  timing as the session tests, and paused the simulation at chosen ticks to save the frame. Unlike the
  earlier captures it did not use `exp/vmhost`, and it captured no audio.
- **Observed:**
  - the opening prompt and all three glimmers facing their way;
  - the vault boulder gliding into the opened gate;
  - the five crystals around it after the shattering, with the "into 5 crystals" prompt;
  - the two lower crystals resting in the basin;
  - the corridor flooding, with the drowned-glimmer prompt;
  - the CAUGHT screen, naming the glimmer and the right-hand wall, with the player drawn shrunk and amber.
- Linux/OpenGL evidence only, not a Mac test. The shatter cue's sound has not been heard.

## D-060: Copy and paste (follow-up F-27)

Numbered D-060 because the arcade music (D-055) and the glimmers (D-056 to D-059) reached master first.

- **Selecting.**
  - Option-drag selects a rectangle. Option-click on a single cell still picks its brush, or selects a
    valve (D-052), so one modifier serves both, told apart by whether the pointer moved.
  - Cmd+A selects the whole cave, and Esc deselects (before it would leave).
  - Delete or Backspace clears the selection to floor.
  - The selection shrinks with the cave, or goes when nothing of it is left, so it never reaches outside
    the grid.
- **Copying.** Cmd+C copies the selection; Cmd+X copies it, then clears it to floor, as one undo step.
- **Pasting.** Cmd+V shows the copy following the pointer, faint, with its top-left corner on the cell
  under it.
  - Each click stamps it, as one undo step, so a pattern can be repeated.
  - Esc or a right-click stops; choosing a brush, the settings or a playtest stops it too.
  - Cells past the cave's edge are left out.
- **What a paste does:**
  - **Plain symbols** are painted as a brush paints them, so a pasted player start or exit moves the cave's
    own.
  - **A copied sluice** (a valve whose copy holds it) becomes a new mechanism among the pasted cells: the
    pasted valve gets a mechanism of its own and its copied gates join it, even while another valve is
    selected. It keeps the open or shut state it was copied with.
  - **A copied gate whose valve was not copied** joins the selected valve, as a painted gate does, or stays
    unlinked and reported. A valve copied without its gates pastes with none, also reported.
  - So pasting keeps every mechanism valid in the same way painting does. The random-input invariants
    (D-052) now include copying, cutting, pasting, selecting all and clearing.
- **Across caves.** The copy is kept by the shell, so it can be pasted into the next cave opened in the
  editor. It is not the system clipboard, and it lasts until the app quits.

## D-014 addendum: copy-and-paste headless capture (2026-10-25)

- The by-hand `exp/vmhost` setup (Linux, Xvfb, Mesa software GL; nothing committed) drove `-edit cave_04`:
  - Option-drag selected the sluice (3 × 6), with "Selected 3 x 6: Cmd+C copy…" in the bar;
  - after Cmd+C and Cmd+V (Control under the driver), the copy followed the pointer;
  - a click placed a second sluice, linked to its own valve;
  - Cmd+X cut an area to floor; it held the player start, so "no player start" was reported, as it should
    be.
- **One fault found and fixed:** the bar still said "Pasting" after Esc ended paste mode.
- This is Linux/OpenGL evidence only, not a Mac test.

## D-061: The glimmer brush (follow-up F-33)

- **A twelfth brush, Glimmer, on `=`,** after the gate. Every key from 1 to 9, 0 and - was taken (D-048), and
  `=` is the next key along the keyboard's top row, so the palette still reads in key order from left to
  right. It is wired like the others: `ebiten.KeyEqual` in `brushKeys`, and the slot is labelled `=`.
- **Its variants turn clockwise,** ▲ ▶ ▼ ◀, exactly as the crawler's do (D-048): pressing `=` again, or
  clicking its slot, turns it.
- **Not on the crawler brush.** A glimmer is a different creature, the crawler's mirror (D-056), with its own
  symbols and sprite. Eight facings on one key would put a facing up to seven presses away, and the slot
  and status line would have to switch between two creatures. With a brush each, every facing is at most
  three presses away, as for water and the crawler.
- **Everything else follows from the brush being in `Palette`:** Option-click on a glimmer picks the brush
  with that facing; a box or flood fill places a glimmer in every cell, since only the player start, the
  exit and valves place one (D-048, D-052); the preview under the pointer shows it; and painting one into
  a cave below format version 5 raises the version with the usual warning (D-047), as pasting one already
  did (D-060). Nothing in `editor.go` changed.
- **The reach overlay** already entered glimmer cells as it does crawlers' (D-053): both move on. Its
  comment now says so, and the reach test has a row with both creatures.
- **The bottom bar.** The text after the palette starts one slot (40 px) further right, which leaves about
  76 characters for the cell description and the problem list, from 82. Lines were already clipped with
  "..." at the bar's edge, so nothing overflows.
- **Keyboards other than US.** `KeyEqual` names the physical key right of `-` on a US layout. On other
  layouts that key may carry another label (on a German Mac keyboard it is `´`), so the slot label can
  disagree with the key cap there. Clicking the slot always works. Checking what the key is on a British,
  German or French keyboard is on the Mac checklist, alongside rebindable keys (F-8).
- **No rule or format change:** rule version 5 and cave format version 5 stay, and every golden replay still
  matches.
- **Evidence:** headless tests in `internal/editor`: `=` chooses a glimmer facing up and turns it through all
  four facings and back; a glimmer painted into a version 4 cave saves as version 5 and the saved file
  loads with `level.Parse`; Option-click on each facing picks that facing; a box and a flood fill of
  glimmers cover their area. Three mutations were each caught by a test: the key changed to `+`, the
  variants turned anticlockwise, and `level.SymbolVersion` no longer counting glimmers as version 5.

## D-014 addendum: glimmer brush headless capture (2026-10-26)

- The by-hand `exp/vmhost` setup (Linux, Xvfb, Mesa software GL; the driver copied into the ignored
  `.vmdriver/`, nothing committed) drove the unmodified app with `-edit new`, saving a frame after each
  step:
  - the bottom bar shows twelve slots, the new one labelled `=` and drawn with the glimmer sprite, and the
    help text beside it still fits, clipped with "..." at the bar's edge as before;
  - `=` ringed the twelfth slot, the top bar read "Brush =: glimmer facing up", and the faint preview under
    the pointer was a glimmer;
  - `=` again read "glimmer facing right"; a click painted it, and the preview followed the pointer to the
    next cell with "unsaved" in the status;
  - a Shift-drag with the brush facing down filled a 4×3 box with twelve glimmers, the cell under the
    pointer described as "glimmer facing down";
  - after choosing the wall, Alt-click on the first glimmer picked "glimmer facing right" and ringed the
    `=` slot again;
  - a click on the slot turned it to "facing down".
- No fault was found. Saving and reloading is covered by the headless test, not this capture.
- This is Linux/OpenGL evidence only, not a Mac test. Which key cap `KeyEqual` is on a non-US Mac keyboard
  is on the Mac checklist.
