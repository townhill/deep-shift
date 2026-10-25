# Roadmap and status

Phases follow [SPEC.md §11](SPEC.md). A box is ticked only when its acceptance evidence exists. **Cloud** =
verified by automated tests or tools in a cloud/CI environment. **Mac** = needs a human on a physical Mac.

## Phase 0 — Repository and repeatable pipeline

- [x] Go module (`go 1.25.0`, toolchain `go1.27.2`), Ebitengine `v2.10.5` pinned in `go.mod`/`go.sum`
- [x] Pure-Go simulation package `internal/sim` with no renderer dependency (enforced by a test and a CI check)
- [x] Native entrypoint `cmd/deepshift` (Ebitengine window; no web frontend, no server)
- [x] `go test ./...` and `go vet ./...` green (Cloud)
- [x] CI: Linux test/vet/validate/cross-compile job; macOS arm64 test/build/package job
- [x] Mac build script `scripts/build-macos-app.sh` producing an ad-hoc-signed development `.app`
- [x] README with exact local Mac build/run steps; SPEC, DECISIONS, PHYSICS, ROADMAP
- [x] **Mac:** a human follows the README on a physical Mac and the window opens. The owner ran
  `go run ./cmd/deepshift` and judged it "ok as a first pass" (2026-10-18); the detailed checklist below
  has not been reported yet.

## Phase 1 — Deterministic core (first slice delivered)

- [x] Versioned, validated cave format with located errors (`internal/level`)
- [x] Movement, earth digging, crystal collection, walls, cave bounds
- [x] Quota-sealed exit that opens when the quota is met and completes on entry
- [x] Fixed-tick loop (20 TPS; 3 updates per tick); movement cadence of one move per 4 ticks
- [x] Snapshot state hash, seeded PRNG, restart, ASCII state dumps (`cavetool show`, `Sim.ASCII`)
- [x] Tick-stamped replays with golden fixtures; 100-run determinism test with per-tick trace hashes
- [x] Pause/resume, restart from pause/completion, HUD (cave, quota, exit state, time, prompts)
- [x] Original placeholder art drawn in code; one original test cave ("Proving Ground")
- [ ] **Mac:** full playtest of the test cave (checklist below). A first pass was done by the owner on
  2026-10-18.

### Mac playtest checklist for the test cave

Record the build shown on the pause screen, the macOS version and the chip. File issues using the README's
bug-report template.

- [ ] `go run ./cmd/deepshift` opens a "Deep Shift" window; the cave and HUD are legible
- [ ] CI artifact: unzip, approve via Privacy & Security, launch from Finder
- [ ] Arrow keys and WASD each move one tile per tap; holding a key repeats at a steady 5 tiles/s
- [ ] Walking into walls or the cave edge does nothing; the player never leaves the map
- [ ] Digging leaves floor behind; crystals disappear and the count rises by exactly one
- [ ] Bumping the exit before 8 crystals shows "Exit sealed: collect N more crystals"
- [ ] Collecting the 8th crystal shows "Quota met" and the exit visibly changes (padlock → open arch)
- [ ] Entering the open exit shows the completion overlay; R restarts with all crystals back
- [ ] Esc pauses (nothing moves, keys are ignored) and resumes; R from pause restarts
- [ ] F toggles fullscreen and back; Esc in fullscreen still pauses (check for conflicts with macOS)
- [ ] Camera follows smoothly and shows more of the cave ahead; no shimmer while scrolling
- [ ] Window resize keeps the aspect ratio; Retina sharpness is acceptable for placeholder art
- [ ] Movement feel: is 200 ms per tile too slow or too fast? (`MoveIntervalTicks`, `SlideUpdates`)

## Phase 2 — Boulder physics (delivered in the cloud; Mac playtest pending)

Acceptance contract: SPEC §11 Phase 2. Rules are in [PHYSICS.md](PHYSICS.md) (rule version 2), and the
decisions are D-018 to D-025.

- [x] Unsupported boulder falls one cell per gravity tick, never skipping (Cloud)
- [x] Supported boulder stays at rest; rolls only off rounded blockers with both side cells clear, left
  before right (Cloud)
- [x] Each boulder updates at most once per tick, including after a roll or a push. This is mutation-
  checked, and the randomised runs check it on every tick (Cloud)
- [x] Horizontal push of one resting boulder into empty floor; no chains, no vertical pushes (Cloud)
- [x] A falling boulder crushes the player: the status becomes `dead`, the cave freezes, and R/N are
  offered. The attempt is logged as a replay that reproduces the death (Cloud)
- [x] Death, reset and replay are deterministic, with 100 identical runs of the death fixture (Cloud)
- [x] Events for fall, roll, land, push and crush; boulder sprite and gliding animation
- [x] Two example caves with golden replays: `cave_02` Unstable Ground and `cave_03` The Roll
- [ ] **Mac:** playtest of caves 02 and 03 (checklist below)

### Mac playtest checklist for caves 02 and 03

Start with `go run ./cmd/deepshift -cave cave_02`, or press N on a pause or end screen to move on.

- [ ] Boulders look round and distinct from crystals; falling boulders and crystals move smoothly (no jumps)
- [ ] Proving Ground: the two loose crystals drop at the start; walking into a falling crystal collects it
- [ ] cave_02: taking the first crystal from the side, then stepping aside, lets the boulder drop behind
  you; moving straight down from under it crushes you
- [ ] Death shows CRUSHED with "R: try again   N: next cave" and a "Replay saved: …" prompt; the file
  exists under `~/Library/Application Support/Deep Shift/replays/`, and
  `go run ./cmd/cavetool replay <file>` reports "matches expect block"
- [ ] cave_02: the row of boulders collapses behind you as you tunnel under it; the shaft stack drops when
  you step back out of its side entrance
- [ ] cave_03: digging the plug and stepping back makes the perched boulder roll into the pit
- [ ] cave_03: pushing the boulder in the wall gap drops it into the pit, and the way east opens
- [ ] cave_03: two boulders in a row will not move
- [ ] Feel: is the fall speed (10 cells/s) fair? Do crystal crushes feel fair, or should a falling crystal
  be collected instead (D-019)?

## Phase 3 — Water and sluice gate (delivered in the cloud; Mac playtest pending)

Acceptance contract: SPEC §11 Phase 3. Rules are in [PHYSICS.md](PHYSICS.md) (rule version 3), and the
decisions are D-026 to D-032. The owner asked for this phase to start before the Phase 2 Mac playtest
([D-026](DECISIONS.md#d-026-phase-3-started-before-the-phase-2-mac-playtest-owner-decision)).

- [x] Conserving, bounded water: 0..4 units per cell, falls then spreads with no left/right bias;
  explicit update order after gravity (Cloud)
- [x] Closed gate blocks water; a valve opens it and the water rules find the opening by themselves,
  with no level scripts (Cloud)
- [x] Water never passes walls, earth, closed gates, valves or the exit, never leaves the map, and the
  total never changes: checked every tick in randomised runs and in the cave 04 fixture (Cloud)
- [x] Falling objects and water follow one policy: objects ignore water and swap places with it
  ([D-028](DECISIONS.md#d-028-falling-objects-and-water-one-policy)) (Cloud)
- [x] Identical replay inputs give identical water layouts and hashes at every tick across 100 runs,
  including the cave 04 reservoir drain (Cloud)
- [x] Cave 04 "Reservoir": the only way down is a flooded shaft that the valve drains; tests show it is
  unreachable without the valve (Cloud)
- [x] Visuals: translucent, eased water with wave lines on deep cells; gate and valve tiles; HUD hints.
  Observed in a Linux headless capture only (D-014 addendum)
- [ ] **Mac:** playtest of cave 04 (checklist below)

### Mac playtest checklist for cave 04

Start with `go run ./cmd/deepshift -cave cave_04`, or press N on a pause or end screen to move on.

- [ ] Deep water (full, with wave lines) and shallow water (partly filled) are easy to tell apart
- [ ] Walking into the flooded shaft shows "Too deep to wade"; the puddle near the start can be waded
- [ ] Standing on the valve shows "Space: open the sluice gate"; Space and Enter both turn it, once per
  press, and the wheel's spokes visibly turn
- [ ] The water drains smoothly through the open gates into the basin (no flicker or jumps)
- [ ] The shaft can then be climbed down; the basin's top row can be waded and its floor cannot
- [ ] The crystal on the basin floor can be taken from above; water follows into dug tunnels
- [ ] After the drain, Space at the valve closes the gates again (they are clear) and the prompt changes
- [ ] The saved replay of a completed attempt reports "matches expect block" in `cavetool replay`
- [ ] Feel: is the drain speed (10 steps/s) right? Is the one-unit slope of settled water readable?

## Phase 4 — Playable native Mac vertical slice (delivered in the cloud; Mac playtest pending)

Acceptance contract: SPEC §11 Phase 4. The decisions are D-033 to D-039. The owner asked for this phase
to start before the Phase 2 and 3 Mac playtests
([D-033](DECISIONS.md#d-033-phase-4-started-before-the-mac-playtests-of-phases-2-and-3-owner-decision)).
No rule changed: rule version 3.

- [x] Five authored campaign caves, SPEC §8's table in order: 01 First Descent, 02 Unstable Ground, 03 The
  Roll, 04 Reservoir, 05 The Sluice; the Phase 1 test cave stays as an extra (Cloud)
- [x] Every cave has a golden completion replay. Cave 01 cannot hurt the player, by design
  ([D-035](DECISIONS.md#d-035-cave-01-first-descent)) (Cloud)
- [x] Cave 05 has two distinct routes across its flooded channel, each with a replay. Tests show one
  drains it and pushes nothing, and the other never turns the valve and crosses while it is still
  flooded ([D-036](DECISIONS.md#d-036-cave-05-the-sluice-and-its-two-routes)) (Cloud)
- [x] Title menu and cave list (tested headlessly); pause, restart, next cave and the cave list from the
  pause, completion or death screens
- [x] Sound: ten original synthesised cues for game events, with M to mute (cues tested as data; the app's
  audio streams matched them in a headless capture)
- [x] Fullscreen (F), resizable window, Cmd+Q; unsigned dev `.app` built and uploaded by CI
- [x] Performance overlay to measure frame rate and timings on a Mac
  ([D-039](DECISIONS.md#d-039-measuring-performance))
- [ ] **Mac:** start, finish, fail and restart all five caves without developer tools (checklist below)
- [ ] **Mac:** 60 FPS with smooth scrolling and movement on a reference Apple Silicon Mac, measured
- [ ] **Mac:** launch the CI-built `.app` from Finder using the README's steps
- [ ] **Mac:** usability, audio and graphics issues found in the playtest written up

### Mac playtest checklist for the vertical slice

Run `go run ./cmd/deepshift` with no flags, or open the CI-built `.app`. Record the build shown on the
pause and menu screens, your macOS version and chip, and your display's refresh rate.

- [ ] The title menu appears; arrows choose, Space/Enter select; "Play the campaign" starts cave 01
- [ ] The cave list shows caves 1–5 and the extras (Crawler Warren and the Proving Ground); Esc goes back;
  a cave you finish shows "(done)"
- [ ] Choosing a cave with Enter never moves the player or turns a valve on arrival
- [ ] In each cave: finish it; lose it where possible (cave 01 has no way to lose); restart with R; reach
  the cave list with C
- [ ] Cave 05, sluice: the valve in the floor at the bottom left drains the channel, which can then be
  waded
- [ ] Cave 05, boulder: drop the boulder over the channel's mouth into it, push it along, and follow it
  through the water; you stay visible while wading
- [ ] Sound: digging, crystals, landings, pushes, the valve and gates, refusals (sealed exit, deep water),
  the exit opening, completion and crushing each have a cue that matches what happened. Nothing is
  painfully loud, nothing clicks, and M mutes and unmutes
- [ ] Press `` ` `` (or F3) and play for a minute in cave 04 or 05. Note FPS, TPS, and update and draw
  avg/p95; toggling off prints them to the terminal. FPS should hold 60 in a window and fullscreen
- [ ] On a 120 Hz display, movement and falling run at the same speed as at 60 Hz

## Phase 5 — First content and tools iteration: the creature (delivered in the cloud; Mac playtest pending)

SPEC §11 Phase 5 is a menu to prioritise from. The owner chose the creature first
([D-040](DECISIONS.md#d-040-phase-5-starts-with-the-creature-only-owner-decision)). Rule version 4; the
decisions are D-040 to D-044.

- [x] A deterministic wall-following creature, the **crawler**: left-hand rule, catches the player, crushed
  by falling objects, drowned by water ([D-041](DECISIONS.md#d-041-crawlers-rule-version-4)). Movement
  order, collision and reset have tests, mutation-checked (Cloud)
- [x] Randomised invariants include crawlers; the earlier nine golden replays are unchanged except their
  hashes, shown by trace diffs against master (Cloud)
- [x] Cave format version 4 writes crawlers as arrows
  ([D-042](DECISIONS.md#d-042-cave-format-version-4)) (Cloud)
- [x] An example cave, Crawler Warren, an extra showing crawlers with boulders and with water, with a
  completion replay and a death replay, both in the 100-run determinism check
  ([D-043](DECISIONS.md#d-043-crawler-warren-the-example-cave)) (Cloud)
- [x] App: crawler sprite by facing, gliding movement, CAUGHT death screen, two sound cues
  ([D-044](DECISIONS.md#d-044-presenting-crawlers)); frames and audio checked in a headless capture
  (Linux/OpenGL only)
- [x] Level selection UI: done in Phase 4 ([D-037](DECISIONS.md#d-037-menus))
- [x] Basic cave editor, milestone 1: see the next section
- [ ] Structural supports, local preferences, one original visual theme: not started
- [ ] **Mac:** play Crawler Warren (checklist below)

### Mac playtest checklist for Crawler Warren

Choose it under Extras in the cave list, or run `go run ./cmd/deepshift -cave crawler_warren`.

- [ ] The opening prompt says crawlers keep the wall on their left
- [ ] Three violet crawlers glide smoothly one cell at a time, head first; each sprite faces the way it is
  going
- [ ] The ring crawler goes round its pillar anticlockwise and never enters the pockets beside it; you can
  wait in a pocket while it passes
- [ ] Stepping into a crawler's path gets you caught: CAUGHT, a bite sound, and the left-hand hint. Walking
  into one says "You walked into a crawler". R restarts
- [ ] Pushing the boulder into the shaft crushes the crawler there, with a pop and a prompt, whenever you
  push it
- [ ] Turning the valve drowns the corridor crawler, with a pop and a prompt; the water settles shallow
  enough to wade to the exit
- [ ] Feel: is the crawler speed (one cell per 0.3 s) right? Can you read where a crawler will go next?

## Phase 5 — The cave editor, milestone 1 (delivered in the cloud; Mac playtest pending)

SPEC §8 "Level editor". The owner chose the editor next and asked for playtesting in the first milestone
([D-045](DECISIONS.md#d-045-the-cave-editor-milestone-1-owner-decision)). The decisions are D-045 to D-049.
No rule changes: rule version 4, cave format version 4, and every golden replay unchanged.

- [x] Caves are written back in the canonical layout. Every embedded cave reads and writes byte for byte,
  and CI checks it (`cavetool fmt -check`). Random caves round-trip
  ([D-046](DECISIONS.md#d-046-canonical-cave-layout-cavetool-fmt-and-drafts)) (Cloud)
- [x] The editor validates with the game's own loader after every change, and problems point at their
  cells (Cloud)
- [x] Paint terrain; place the player start, boulders, crystals, the exit, water and crawlers. The player
  start and exit move rather than duplicate (Cloud)
- [x] Strokes with no gaps, boxes, erasing, picking, undo/redo, resizing, and settings for title, id, seed
  and quota. Random input keeps the editor's invariants, and undoing everything restores the cave exactly;
  key behaviours are mutation-checked (Cloud)
- [x] Save and reopen, including caves with problems. A file the editor did not open is never replaced
  without asking twice. A cave keeps its format version
  ([D-047](DECISIONS.md#d-047-a-cave-keeps-its-format-version)) (Cloud)
- [x] Playtest with P and return with P. A playtest is a real session: cave 01's golden replay completes a
  copy of it ([D-049](DECISIONS.md#d-049-playtests)) (Cloud)
- [x] Title menu entry, edit list (new cave, your caves, copies of the built-in caves) and `-edit`. Drawn
  in a headless capture (Linux/OpenGL only, [D-014 addendum](DECISIONS.md#d-014-addendum-cave-editor-headless-capture-2026-10-25))
- [ ] **Mac:** use the editor (checklist below)

### Mac playtest checklist for the cave editor

Choose "Cave editor" on the title menu, or run `go run ./cmd/deepshift -edit new`.

- [ ] The edit list shows New cave, then "Copy a built-in cave" with all seven caves; it scrolls; Esc goes
  back to the title
- [ ] New cave opens one screen of earth in a wall; the status bar reads "EDIT New cave 30x16 Quota 0 of 0
  OK"
- [ ] The cell under the pointer is outlined, with a faint preview of the brush; the bar names the cell
- [ ] Left-drag paints with no gaps however fast you move; right-drag (or a two-finger click and drag on a
  trackpad) erases to floor; Shift-drag fills a box; Option-click picks a cell's brush
- [ ] Keys 1–9 choose brushes; pressing 8 again makes water shallower, and 9 again turns the crawler;
  clicking a palette slot does the same
- [ ] Painting a second player start or exit moves it
- [ ] Cmd+Z and Cmd+Shift+Z undo and redo whole strokes; G toggles the grid
- [ ] Arrows, WASD and the trackpad or wheel scroll a large cave (try a copy of the Proving Ground); the
  scroll direction feels natural
- [ ] Cmd+arrows grow the cave and Cmd+Shift+arrows shrink it, with nothing clashing with macOS
- [ ] Tab opens the settings; typing a title, id, seed and quota works; F, M and B type letters there
  rather than toggling fullscreen, sound or music
- [ ] Covering the exit shows "1 problem" and the reason; P then refuses, saying what to fix
- [ ] P playtests; "P: back to the editor" is on the pause, death and completion screens; P returns with
  every edit intact
- [ ] Cmd+S saves to `~/Library/Application Support/Deep Shift/caves/new_cave.json`; the list then shows
  it under "Your caves"; reopening it shows no unsaved changes
- [ ] A second new cave saved under the same id asks before replacing the first
- [ ] Esc with unsaved changes asks first; Esc again leaves
- [ ] `go run ./cmd/deepshift -edit assets/levels/cave_03_the_roll.json`, then Cmd+S without changes: `git
  status` shows no change
- [ ] Copy Crawler Warren: the valve's link to its gate is drawn; painting over the valve outlines the gate
  in red

## Phase 5 — The cave editor, milestone 2 (delivered in the cloud; Mac playtest pending)

The decisions are D-050 and D-051. No rule changes: rule version 4, cave format version 4, and every golden
replay unchanged.

- [x] The cave list shows "My caves" from the editor's folder, read afresh each time. Playable caves join
  the playlist after the built-in ones, and completing one marks it done
  ([D-050](DECISIONS.md#d-050-my-caves-in-the-cave-list-editor-milestone-2)) (Cloud)
- [x] Caves that cannot be played are listed, and say why: unreadable, problems, or an id that is taken.
  Mutation-checked (Cloud)
- [x] A playtest is recorded only when its cave is saved and unchanged. Replays of the player's caves name
  the cave file, and re-run against it
  ([D-051](DECISIONS.md#d-051-replays-of-playtests-and-of-the-players-caves)) (Cloud)
- [x] Seen in a headless capture (Linux/OpenGL only,
  [D-014 addendum](DECISIONS.md#d-014-addendum-editor-milestone-2-headless-capture-2026-10-25))
- [ ] **Mac:** the checklist below

### Mac playtest checklist for editor milestone 2

- [ ] Save a new cave in the editor, then open "Choose a cave": it is under "My caves"
- [ ] Choose it: it plays; N afterwards goes on through your caves and wraps to cave 01; finishing it marks
  it "(done)"
- [ ] Save a cave with a problem (cover the exit): it is listed "(cannot play)", and choosing it says what to
  fix
- [ ] Playtest a saved cave to the end: "Replay saved: …" appears. `go run ./cmd/cavetool replay -cave
  <cave file> <replay>` reports "matches expect block"
- [ ] Change the cave, playtest it to the end: "Playtest not recorded: save the cave first (Cmd+S)"

## Phase 5 — The cave editor, milestone 3 (delivered in the cloud; Mac playtest pending)

The decisions are D-052 to D-054. No rule changes: rule version 4, cave format version 4, and every golden
replay unchanged.

- [x] Valve (0) and gate (-) brushes. Gates join the selected valve and match its state; Alt-click selects a
  valve; O opens or closes a mechanism. Mechanism invariants are checked under random input, and the key
  rules are mutation-checked
  ([D-052](DECISIONS.md#d-052-valves-gates-and-their-links-editor-milestone-3)) (Cloud)
- [x] A sluice built in the editor works: its valve opens its gates in the simulation and in a playtest
  (Cloud)
- [x] The reach overlay (R), with a crystal and exit summary ([D-053](DECISIONS.md#d-053-the-reach-overlay))
  (Cloud)
- [x] Cmd-click flood fill ([D-054](DECISIONS.md#d-054-flood-fill-and-copy-and-paste-left-for-later))
  (Cloud)
- [x] Seen in a headless capture (Linux/OpenGL only,
  [D-014 addendum](DECISIONS.md#d-014-addendum-editor-milestone-3-headless-capture-2026-10-25))
- [ ] Copy and paste: follow-up F-27
- [ ] **Mac:** the checklist below

### Mac playtest checklist for editor milestone 3

- [ ] 0 and - choose the valve and gate brushes; their palette slots read clearly
- [ ] A painted valve is ringed in yellow; gates painted next are linked to it by lines; the problem about
  a valve with no gates goes once the first gate is down
- [ ] Option-click a valve or gate in a copy of Reservoir: its valve is ringed and new gates join it
- [ ] A gate painted with no valve selected says to select one, and is outlined until linked
- [ ] O over a valve or gate opens or closes all of them together; Cmd+Z undoes it
- [ ] Playtest a sluice you built: Space at the valve opens its gates
- [ ] R shades what cannot be reached by walking and digging; the bar's summary matches what you see
- [ ] Cmd-click fills an area; Cmd-right-click fills it with floor; one Cmd+Z undoes the whole fill. Check
  that Cmd-click is not taken by macOS

## Arcade music (owner request; delivered in the cloud, Mac playtest pending)

The owner asked for arcade music that can be turned off and on, and chose the key, the default and the
pause behaviour ([D-055](DECISIONS.md#d-055-arcade-music-owner-decision)). No rule changed: rule version 4.

- [x] An original 32 s loop synthesised in code: pulse lead, arpeggio, triangle bass and noise drums. Tests
  pin its length, its silent seam and its peak below the cues (Cloud)
- [x] B turns the music off and on; M mutes it with everything else; a paused cave holds it; `-no-music`
  starts with it off. The rules and their notices are tested headlessly (Cloud)
- [x] In a headless capture, the app's music stream matched the loop sample for sample and wrapped at 32 s;
  B, M and Esc stopped it and resumed it where it stopped (Linux/OpenGL only; D-014 addendum)
- [x] The HUD hint and the menu footers name M and B, and fit their lines (Cloud)
- [ ] **Mac:** listen to it (checklist below)

### Mac playtest checklist for the music

Run `go run ./cmd/deepshift` with no flags.

- [ ] Music starts on the title screen and keeps going into the cave list and a cave
- [ ] It loops with no click, gap or jump every 32 s
- [ ] Every cue (digging, crystals, landings, the valve, completion) is still easy to hear over it, and it
  never crackles or clips, even when several cues sound together
- [ ] B turns it off ("Music off (B: on)") and back on ("Music on") in a cave, the title and the cave list
- [ ] M silences everything; M again brings the music back, unless B turned it off
- [ ] Esc pauses the music with the cave; Esc again resumes it where it stopped
- [ ] `-no-music` starts silent until B is pressed
- [ ] Feel: is it pleasant over a long cave, or wearing? Is the tempo right? Too loud or too quiet?

## Follow-ups

| ID | Follow-up | Why |
|---|---|---|
| F-1 | Full hands-on Mac playtest (checklists above) | A first pass was done; the detailed checks and the Phase 2 and 3 caves are not yet confirmed on macOS |
| F-2 | Developer ID signing and notarisation in CI | Remove the Gatekeeper "Open Anyway" step; needs Apple credentials as secrets |
| F-3 | App icon (`.icns`) generated from original art | The bundle currently has no icon |
| F-4 | Retina-crisp rendering (`LayoutF`/device scale) and a real UI font | Placeholder text uses the 2× debug font |
| F-5 | Manual "export replay now" key | Replays of finished attempts are now saved automatically (D-023); mid-attempt export is still missing |
| F-6 | `tools/vmcapture` plus an optional CI job using Ebitengine `exp/vmhost` | Repeatable headless frame captures ([D-014](DECISIONS.md#d-014-ebitengine-expvmhost-headless-capture-assessed-not-adopted)) |
| F-7 | Intel / universal (`lipo`) build and test | macOS amd64 is "desired"; currently compile-only |
| F-8 | Rebindable keys, reduced-effects mode, UI scaling | SPEC §4 accessibility; designed for but not built |
| F-9 | Record update and draw timing (avg/p95) on a reference Mac | SPEC §12 performance gate; the overlay measures it (D-039), and CI records a Linux benchmark only |
| F-10 | Tune `GravityIntervalTicks`; confirm crystal crushes feel fair | Provisional values from D-018 and D-019; needs Mac playtest feedback |
| F-11 | ~~Sound cues for land, push and crush events~~ | Done in Phase 4 (D-038) |
| F-12 | Move `actions/upload-artifact` to a Node 24 release | CI warns that the v4 action targets Node 20 |
| F-13 | Tune water: capacity, cadence, how far it spreads | Provisional values (D-027); needs the cave 04 Mac playtest |
| F-14 | A splash cue when an object enters water | Valve and gate cues are done (D-038); a splash needs a new event, so a rule version |
| F-15 | Slow the player in shallow water? | SPEC §6.3 allows it later; it must be a rule (same in replays), not a renderer effect |
| F-16 | Save campaign progress and settings locally | Progress, sound and music settings are per run only (D-037, D-055); SPEC §11 Phase 5 "local preferences" |
| F-17 | Volume control beyond mute, with separate music and effects levels | SPEC §4 accessibility; M and B only toggle (D-038, D-055) |
| F-18 | Reduce-motion option for the water easing and glides | SPEC §4 accessibility, with F-8 |
| F-19 | Tune `CrawlerIntervalTicks` | Provisional (D-041); needs the Crawler Warren Mac playtest |
| F-20 | Crawlers in campaign caves, and more kinds (a right-hand crawler, a fixed patrol) | Crawler Warren is the only cave with them; SPEC §8's regions call for creatures |
| F-21 | An audible cue for a nearby crawler | Walking crawlers are silent (D-044), so a threat out of view cannot be heard; needs care not to become noise |
| F-22 | Remaining Phase 5 items: structural supports, local preferences, a visual theme | SPEC §11 Phase 5; not started (D-040). The editor's first milestone is done (D-045) |
| F-23 | ~~Editor milestone 2: your caves in the cave list ("My caves"); replays of playtests of saved, unchanged caves~~ | Done (D-050, D-051) |
| F-24 | ~~Editor milestone 3: valve and gate brushes with linking; a "reachable from the start" overlay; flood fill~~ | Done (D-052 to D-054); copy and paste moved to F-27 |
| F-25 | Editor: "save as" / rename, metadata editing, and a full, readable problem list | D-048: a cave keeps its first file name, and the bar shows only the first problems |
| F-26 | Move the headless editor capture into `tools/vmcapture` (F-6) | The editor sessions were scripted by hand (D-014 addendum) |
| F-27 | Editor: copy and paste a selection, keeping valve and gate links | D-054: needs a selection separate from the box fill, and rules for copied mechanisms |
| F-28 | A solver-backed reach check that follows the real rules (falling, pushing, valves, water) | D-053: the overlay is walking and digging only |
| F-29 | More music: a longer loop, or a tune per region or for the title | One 32 s loop (D-055) may wear in long caves; judge in the Mac playtest |
