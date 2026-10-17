# Deep Shift — Game Design & Technical Specification

**Version:** 0.1 (concept and MVP baseline)  
**Date:** 17 October 2026  
**Status:** Working specification for an AI coding agent; decisions labelled *provisional* may be revised after playtesting.  
**Target:** Standalone, native macOS 2D game. No browser, Electron, or web runtime.  
**Working name:** Deep Shift (not trademark-cleared).  
**Audience:** Claude Code or another implementation agent, reviewer, and human playtester.

---

## 1. Mission and intended result

Build an original, premium-feeling 2D cave-exploration and emergent-physics puzzle/adventure game inspired by the *design philosophy* of the classic 1984 Boulder Dash: simple local rules yielding complex, understandable chain reactions. Preserve tactile digging, falling/rolling boulders, collectables, hostile cave inhabitants, and tense movement, while creating original level layouts, visuals, characters, audio, terminology, mechanics, and progression. Do **not** copy the original game's code, artwork, music, branding, or level data.

**Core experience:** A player explores interconnected subterranean regions, collects crystals and resources, manipulates their surroundings, solves physics puzzles, avoids hazards, and discovers unconventional solutions. The environment is a simulated system, not scripted scenery.

**Primary success criterion:** Within a playable five-cave vertical slice, a player can predict and deliberately exploit gravity and water to accomplish objectives; the game feels responsive and visually coherent on an Apple Silicon Mac.

### Design pillars

1. **Readable cause and effect** — Players can understand *why* a boulder rolled, water flowed, or a barrier failed.
2. **Emergent problem solving** — The same physical rules must apply to all compatible objects; avoid one-off level-specific magic.
3. **Tactile arcade roots** — Digging, movement, collecting and danger must feel immediate even when the player takes time to think.
4. **A genuinely native Mac game** — A local `.app` with standard desktop behaviour, not a browser container.
5. **A testable simulation** — Game rules do not depend on the renderer, wall-clock timing, or live input devices.
6. **Expandable architecture** — New systems (electricity, lava, creatures) can be added incrementally without rewriting the core.

### Non-goals for version 0.1

- 3D, full fluid dynamics, rigid-body physics, online multiplayer, networking or accounts.
- A commercial launch, Steam integration, asset store, achievements, localization or cloud saves.
- Copying an exact Boulder Dash cave set or reproducing source/binary behaviour byte for byte.
- Procedural levels with formal solvability guarantees.
- A vast open-world campaign before the core mechanics are proven fun.

---

## 2. Target players, tone and camera

- **Audience:** People who enjoy cave exploration, light action, spatial puzzles, thoughtful chain reactions and classic arcade design. Accessible for casual players, rewarding for expert players.
- **Tone:** Atmospheric, curious, occasionally dangerous; avoid a grim survival-horror presentation.
- **Camera:** Orthographic top-down/side-on readable 2D cave grid (no perspective camera). Movement and topology are grid-based; visual movement interpolates smoothly.
- **Visual language:** Distinct silhouettes for dirt, rock, crystal, solid rock, gate, water and exit; animations and particle effects communicate state without obscuring it.
- **Audio:** Responsive, layered environmental effects with optional ambience/music; sound is informative but never required to understand a hazard.
- **Working art direction:** Stylised, high-resolution 2D tiles with subtle shadows, coloured mineral light and satisfying particles. Retro-inspired theme optional later.

### Provisional design choices

These are sensible defaults, not owner-approved final choices:

- Grid-based deterministic physics rather than fully continuous simulation.
- Campaign comprised of authored caves, with interconnected regions later; no seamless open world in the MVP.
- Hybrid exploration/puzzle/arcade pacing; no global cave timer in initial levels.
- Ebitengine v2.10+ in Go 1.25+; pure Go simulation; standard Go tooling.
- 60 x 40 tile demonstration cave, with smaller tutorial caves.
- Simulation rate of **20 fixed ticks per second** for the first implementation; graphics can interpolate independently.
- Native Apple Silicon build required; Intel build desirable, subject to testing.

Maintain `docs/DECISIONS.md` to record changed decisions, their rationale and the feature scope affected.

---

## 3. Game loop

1. Select a cave/expedition from the world map.
2. Enter a cave and inspect the hazards and objective(s).
3. Move on a grid, dig, collect crystals, shift boulders, and experiment with environmental mechanisms.
4. Reach the exit after meeting the crystal quota and any specific cave conditions.
5. Receive clear feedback (time, optional challenges, secrets found); unlock the next cave.
6. Replay for a cleaner or more creative solution, or return with a later-unlocked tool in the broader campaign.

**Fail state:** Crush/hazard/deadly creature, with instant restart. Optional rewind and checkpoints are future enhancements, not a reason to dilute the simulation's determinism.

**Scoring:** Later, offer collectible score, speed and no-rewind challenge badges; never force a score chase on exploratory players.

---

## 4. Player controls and presentation

### Version 0.1 controls

| Input | Action | Behaviour |
|---|---|---|
| Arrow keys / WASD | Move | Exactly one cardinal tile per movement event; bounded repeat for held key |
| Space / Enter | Interact | Activate nearby valves, switches or exit where applicable |
| Esc | Pause / resume | Stops simulation ticks and input consumption |
| R from pause or restart UI | Restart cave | Restores initial cave state and deterministic RNG seed |
| F (optional) | Toggle fullscreen | Standard native display behaviour |
| Mouse | UI / editor | No mouse dependency for standard cave navigation |

Movement repeat timing should be configurable independently of simulation rate; the first implementation may use a documented fixed cadence (for example, one movement every 4 simulation ticks). Do not allow operating-system key repeat to determine simulation behaviour. A queued command per eligible action window is acceptable.

### Interaction requirements

- Directions cannot move through solid rock or outside the cave.
- The player may dig ordinary soil by entering its cell.
- Entering a crystal cell collects it and increases the count once.
- Heavy boulders are not walkable; horizontal pushing is possible only where supported by the rules.
- Exit is inactive until the required quota is met, then visibly changes state.
- The HUD displays quota, collected count, current cave, and a concise prompt when interacting.
- Camera follows the player, clamps to map edges and exposes a visible margin in the intended movement direction.

### Accessibility and comfort

- Rebindable keyboard controls should be designed for, even if the MVP starts with one binding set.
- Motion effects and flashes have a reduced-effects mode planned.
- Distinguish objects with shapes and symbols, not colour alone.
- Offer volume controls and readable UI scaling. No obligatory timed gameplay in onboarding levels.

---

## 5. Cell model and simulation invariants

The simulation is the technical heart of the project. Keep it in a pure-Go package with **no Ebitengine imports**.

### Map representation

Use a rectangular grid of cells with clear layers rather than one overburdened tile enum:

- **Terrain:** empty, diggable earth, permanent wall, fragile/breakable wall, goal/exit foundation (as appropriate).
- **Occupant:** player, boulder, crystal, mechanical object, creature (at most one solid occupant per cell).
- **Fluid:** no water / water amount or presence (separate layer to allow player movement through shallow water and future fluid-on-terrain interaction).
- **Metadata:** cell-specific state (e.g. gate open, falling boulder, animation cue). Prefer typed structs/enums instead of maps of untyped strings in the simulation core.

Grid coordinates are integer X,Y with origin at upper-left; gravity points toward increasing Y. Cave bounds and one-player invariant must be validated before simulation begins.

### Determinism contract

A simulation is fully defined by `(initial cave data, random seed, rule version, ordered player commands)`. Running this twice produces identical state hashes at every tick.

- `Step(state, input)` updates by **one fixed logical tick**. No frame delta, wall-clock time, external files, GPU context or hidden global state.
- All randomness comes from a local explicitly seeded PRNG.
- Entity update order must be documented and deterministic. Use an `updatedAtTick` marker or equivalent to ensure no object is processed twice due to movement.
- Avoid nondeterminism from Go map iteration; when enumerating entities/events, sort or use explicit cell traversal.
- The renderer reads simulation snapshots and events but cannot change physics outcomes.
- Prefer serializable, versioned, JSON-compatible cave and replay formats. Snapshot hashes should be stable across supported platforms.
- Replays record initial cave identifier/checksum, seed, rule version and tick-stamped logical inputs; a replay test must verify resulting state hashes.

### Simulation cadence

- Logical tick rate: provisional **20 TPS**.
- Player action eligibility may be slower than physics ticks.
- Display frame rate is independent; visual interpolation must never alter authoritative cell positions.
- Do not tie falling speed to 60/120 FPS.

---

## 6. Rules catalogue: prototype

Rules below are **new game design decisions**, not claims about the exact 1984 game's implementation. Final balancing is subject to playtesting. Write unit tests for every rule before relying on it in a level.

### 6.1 Digging and collecting

- Earth becomes empty when entered by the player, producing a short-lived digging event.
- A crystal is collected by entering its cell if that terrain is passable; the crystal is removed, count increments once.
- Permanent walls cannot be destroyed by ordinary movement.
- An active exit is won by the player entering/activating it (choose and document one consistent behaviour).

### 6.2 Falling and rolling boulders

- Gravity affects unsupported boulders and, when sensible, loose crystals.
- If directly below a boulder is free, the boulder falls one grid cell per applicable physics tick.
- A boulder resting above a rounded blocker may roll diagonally if the side and target-diagonal cells are both clear. Define and document a fixed tie-breaking rule, e.g. try left then right in v0.1, and test it.
- Falling objects crush a player occupying a destination cell and trigger a death event; later they may destroy other destructible entities.
- A rock should **not** teleport multiple cells in a single tick or receive duplicate updates from movement traversal.
- The model should distinguish a moving/falling rock from a resting rock where required for collision semantics.
- Pushing: player can push exactly one pushable boulder horizontally into an empty destination if the push rule allows it; no chain push in v0.1.
- Include a concise rules table and examples in `docs/PHYSICS.md` after implementation.

### 6.3 Water — first major extension

**Purpose:** Water creates new routes, hazards and puzzles. Keep v0.1 intentionally simple; do not build a realistic fluid solver.

- Water occupies a fluid layer and flows under gravity into eligible adjacent cells subject to terrain barriers.
- Baseline algorithm: discrete/limited water units; prefer down, then horizontal spreading using documented deterministic tie-breaking. No infinite water duplication or deletion.
- Flowing water cannot move through permanent walls, closed gates or filled earth until terrain is removed.
- Player can enter shallow-water cells; movement may be slowed later, but never inconsistently between keyboard and replay.
- Water alone does not kill the player in v0.1. Drowning, pressure, electricity and lava interactions are deferred.
- Boulders must have a clearly specified, tested interaction with water. Provisional: water does not prevent boulders from falling; any displaced water is conserved where possible, otherwise the rules must document capacity and displacement limitations.
- A closed gate can hold back water; opening it starts a deterministic flow sequence.
- Key test: draining a small reservoir produces the same final water distribution and input hashes across 100 repeat runs.

### 6.4 Basic mechanism

- Introduce an interactable **sluice gate** or valve. This is the MVP's first technology object.
- Opening/closing action changes a documented blocking property of one or more cells.
- Water-flow rules discover that change automatically, rather than using a level-specific script.
- The valve must be visually and mechanically understandable.

### 6.5 Creatures and danger

Not required for the first engine milestone. First non-player creature should be a deterministic wall-following organism or simple patrol, implemented after gravity/water are stable. Movement order, collision and reset behaviour must have tests.

---

## 7. Systems roadmap after v0.1

| Priority | System | Gameplay value | Prerequisites |
|---|---|---|---|
| Next | Structural supports | Hold rocks, create temporary paths | Gravity engine |
| Next | Creatures | Pattern learning, deliberate manipulation | Cell/entity rules |
| Next | Tool: drill | New diggable rock grades | Terrain types |
| Next | Tool: magnet | Push/pull magnetic material | Entity flags and force rules |
| Later | Pumps and pressure | Redirect water and unlock chambers | Stable water model |
| Later | Electricity | Circuits, conductive liquids, doors | Water + mechanism layers |
| Later | Fire, lava and heat | Melt/ignite, chain reactions | Thermal rules and hazard balancing |
| Later | Gas | Ventilation and gas-pocket puzzles | Pressure/flow model |
| Later | Fungi/amoeba ecology | Growing obstacles and resources | Controlled update rules |
| Later | Explosives | Destruction and emergent shortcuts | Damage/explosion events |
| Later | Rewind | Forgiving experimentation | State serialization/snapshots |
| Later | Procedural expeditions | Replayability | Robust authored-level toolkit |
| Later | Campaign map | Regional progression and secrets | Save data and content pipeline |

Every new system must satisfy four conditions before release: documented behaviour, deterministic tests, at least one tutorial/example cave, and demonstrated interaction with a pre-existing system.

---

## 8. Game world and level design

### Proposed regions (future campaign)

1. **Old Mines** — dirt, rocks, collectables, basic hazards. Teaches causal physics.
2. **Crystal Caverns** — reflective minerals, brittle surfaces and magnetic puzzles.
3. **Fungal Depths** — spreading organisms and vibration-responsive wildlife.
4. **Flooded Works** — reservoirs, gates, pumps, electrical hazards (after relevant rules exist).
5. **Volcanic Fault** — heat, lava, steam and chain-reaction hazards.
6. **Ancient Machines** — conveyors, switches, controlled infrastructure and composite puzzles.

Five starter caves must *teach*, not overwhelm:

| Cave | Teaching focus | Must demonstrate |
|---|---|---|
| 01: First Descent | Movement, dig, crystal, exit | Complete cave without death |
| 02: Unstable Ground | Falling boulder | Dig safely or be crushed |
| 03: The Roll | Roll and push | A deliberate path cleared by moving one boulder |
| 04: Reservoir | Water and barrier | Opening a gate changes reachable space |
| 05: The Sluice | Combined mechanics | At least two meaningful solution routes using water and gravity |

Caves should have understandable prerequisites, optional secrets where suitable, and clear completion conditions. At least two solutions to Cave 05 should be *demonstrated* in scripted replay fixtures, not merely asserted in prose.

### Cave file format

Prefer human-readable, versioned `JSON` in `assets/levels/*.json`. Suggested fields:

```json
{
  "format_version": 1,
  "id": "cave_01",
  "title": "First Descent",
  "width": 30,
  "height": 20,
  "seed": 12345,
  "required_crystals": 3,
  "terrain_rows": ["..."],
  "entities": [{"type": "player", "x": 2, "y": 2}],
  "mechanisms": [],
  "metadata": {"difficulty": "tutorial"}
}
```

The example is deliberately schematic, not an implementation-complete schema. The agent must define exact tile symbols, parse-time validation, dimensions, object state fields, one-player and exit assertions, and helpful error messages. Do not treat JSON as a reason to tightly couple rules to presentation.

### Level editor

A minimal editor should allow painting terrain and placing the player, rocks, crystals, exit and water, plus save/load with validation. An in-game visual editor is the longer-term goal; a developer-oriented JSON/ASCII editing utility is acceptable in the initial prototype if it expedites physics validation. No external database is required.

---

## 9. Technical architecture

### Stack

- **Language:** Go 1.25+ (choose a documented supported version in `go.mod` and CI).
- **Desktop game library:** Ebitengine v2.10+ (`github.com/hajimehoshi/ebiten/v2`). Verify latest stable compatible patch during setup, then pin via `go.mod`.
- **Graphics:** 2D sprites/tile atlas; 60 FPS presentation goal with high-DPI scaling; optional 120 FPS monitor support without changing physics speed.
- **Storage:** Local JSON files for levels, preferences, replay inputs and later saves. No SQL database required for the initial release.
- **Source control:** Git + GitHub; pull requests and CI required for agent work.
- **Packaging:** Standalone macOS `.app`, with resource bundle and Info.plist; sign/notarize later if distributing publicly.
- **Platforms:** macOS arm64 required, macOS amd64 desired. Other operating systems may compile for CI but are not the user-facing target.

### Proposed Go repository layout

```text
deep-shift/
  CLAUDE.md                     # Working instructions for coding agents
  README.md
  go.mod
  go.sum
  cmd/deepshift/main.go         # Native desktop application entrypoint
  internal/sim/                # Pure Go deterministic rules (NO Ebitengine import)
  internal/level/              # File formats, parsing and validation
  internal/game/               # High-level game orchestration and input queue
  internal/render/             # Ebitengine sprites, particles, HUD and camera
  internal/audio/              # Audio adapter (later if appropriate)
  internal/replay/             # Tick-stamped command fixtures and hashes
  internal/editor/             # Cave authoring tools (scope in stages)
  assets/levels/               # Original authored level content
  assets/sprites/              # Original/licensed assets
  docs/SPEC.md                 # This specification
  docs/DECISIONS.md            # Decision log
  docs/PHYSICS.md              # Actual physics rules
  docs/ROADMAP.md              # Phase/status checklist
  .github/workflows/ci.yml     # Tests + Mac builds
  scripts/                     # Explicit, repeatable build/package helpers
```

Use simple dependencies, explicit interfaces and standard library facilities by default. Introduce ECS or third-party physics only if a measured need arises. Avoid overengineering before five caves are fun.

### Suggested interfaces (conceptual; agent chooses final API)

```go
type Command struct {
    Tick   uint64
    Action Action
}

type Simulation interface {
    Step([]Command) []Event
    Snapshot() Snapshot
    StateHash() [32]byte
    Reset() error
}
```

Events (dig, collect, roll, crush, gate toggled, cave complete) feed sound/particle/UI adapters; they never independently mutate world state. The authoritative state resides in simulation objects.

### Asset loading

Use `embed` for small default sprites and levels when practical, with user-level overrides in a documented app data directory later. Avoid absolute paths and dependence on repo working directory when launching the `.app` from Finder.

---

## 10. Cloud-first development with Claude Code

**Intended workflow:** Use Claude Code cloud sessions against a GitHub repository, with independent pull requests for narrowly scoped changes. Run core simulation tests in the cloud; use GitHub Actions macOS runners and human testing on a physical Mac to validate actual native behaviour.

### Cloud responsibilities

Cloud session can:

- Implement Go code, tests, fixtures, level files and documentation.
- Run `go test`, `go vet`, static validation and cross-compilation.
- Build program binaries for macOS from Linux on pure-Go dependencies (subject to any future library constraints).
- Open pull requests, iterate on CI feedback and maintain decisions/roadmap.

Do **not** claim a rendered native Mac window was observed in a headless cloud environment. Display/input/audio/Metal behaviour needs a suitable GUI test environment or a physical Mac. Ebitengine 2.10 provides opt-in application virtualisation for agent-driven graphics testing; assess it separately and document actual coverage instead of assuming that it replaces real macOS testing.

### GitHub Actions CI expectations

- Linux job: `go test ./...`, `go vet ./...`, cave-file validation, replay fixtures, deterministic stress tests, and a `go build`.
- macOS job: run `go test ./...` for simulation-only packages and compile/build the app on `macos-latest` or a pinned runner; package a `.app` as a downloadable workflow artifact.
- Optionally cross-compile from Linux using `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64` and `...=amd64` as an early compile check.
- CI must clearly distinguish **compilation successful** from **native GUI tested**. Do not falsely label cross-compilation as a Mac gameplay test.
- Automated screen captures/visual tests only after a repeatable visual harness has been demonstrated.
- If macOS runner signing/notarisation is not configured, mark the `.app` as an **unsigned development build** and provide local launch guidance and the associated Gatekeeper caveat. Never weaken system-wide security settings automatically.

### Cloud setup

1. Create a new GitHub repo and commit this specification to `docs/SPEC.md`, alongside `CLAUDE.md` and a `README.md`.
2. Connect the repository at `https://claude.ai/code` and start an isolated cloud session.
3. Authorise dependency downloads only from domains actually needed; pin major dependencies.
4. Give the agent the focused milestone prompt in `CLAUDE_CLOUD_KICKOFF.md`.
5. Review its PR and CI result, then test the resulting build on a Mac when graphical output is available.
6. File playtest observations as issues, with cave ID, version, replay seed/steps, expected and actual results.

### Agent operating rules

- Work on one meaningful milestone per PR; do not claim to have finished the whole game in one session.
- Read specification before coding. Respect non-goals, provisional choices and IP boundaries.
- Write failing tests for rule changes, then implement until green. Prefer simple, readable Go.
- Do not add Electron, a web app, a React frontend, a server or a database without an approved design change.
- Keep game logic renderer-independent; no GUI imports in `internal/sim/`.
- If an intended test environment is unavailable, record the limitation; never invent test results.
- Maintain `docs/ROADMAP.md` and `docs/DECISIONS.md` with each substantive PR.
- Include a PR summary: files changed, features, test commands/results, known gaps, steps to try locally.
- Avoid unreviewed sweeping refactors, unnecessary dependencies and auto-committed generated binaries.
- Do not copy protected Boulder Dash level files, graphics or decompiled source into the repository.
- Prefer explicit, testable decisions over repeated questions. Escalate only irreversible or product-shaping choices.

---

## 11. Implementation phases and acceptance criteria

### Phase 0 — Repository and repeatable pipeline

**Deliverables:** Project scaffold, `go.mod`, pure-Go simulation package, minimal Ebitengine entrypoint, README, spec, CI, Mac build script, decision log.

**Acceptance:** `go test ./...` passes; `go vet ./...` passes; Mac arm64 compilation succeeds on CI; build instructions reproducible; no browser requirement. App may display a placeholder window, not full gameplay.

### Phase 1 — Deterministic core

**Deliverables:** Validated level loader; movement, digging, collectibles, quota/exit, wall collision; fixed tick loop; snapshot state hash, restart, replay test; ASCII state debugging.

**Acceptance tests:**

- Given the same cave, seed and command sequence, 100 separate runs yield identical final hashes.
- Digging removes exactly one dirt tile; collecting exactly one crystal increments once.
- Walls/bounds prevent movement; player cannot occupy invalid map coordinates.
- Exit cannot complete before quota, but can complete afterwards.
- Restart restores all state, counters and seed.
- CLI/`go test` exercises logic with no display available.

### Phase 2 — Boulder physics

**Deliverables:** Falling, rolling, horizontal push, crush/death, events, unit tests and two cave examples.

**Acceptance tests:**

- Unsupported rock falls one cell on the defined update tick and never skips a cell.
- Supported rock stays at rest; rolling only occurs when side/diagonal route meets documented criteria.
- Each rock updates at most once per physics tick.
- Horizontal push obeys obstruction and no-chain rules.
- Falling rock collision kills the player, pauses/ends cave correctly, and logs a reproducible replay.
- Death, reset and replay remain deterministic.

### Phase 3 — First water and gate system

**Deliverables:** Conserving bounded water model, sluice gate, explicit update order, visuals, tests, example caves.

**Acceptance tests:**

- Closed gate blocks water; opening it allows flow without level-specific scripts.
- Water does not pass walls, leak out of map bounds or duplicate in closed systems.
- Falling rock/water interactions follow and document a single consistent policy.
- Identical replay inputs yield identical water layouts and hashes across 100 runs.
- Cave 04 has an understandable use of the new mechanic.

### Phase 4 — Playable native Mac vertical slice

**Deliverables:** Five authored caves, menus/pause/restart/HUD, scrolling camera, placeholder polished sprites, audio cues, full-screen/window controls, Mac `.app`, cave validation and CI artifact.

**Acceptance tests:**

- Player can start, finish, fail and restart all five caves without developer tools.
- Cave 05 demonstrates at least two distinct valid solution routes with evidence or replay fixtures.
- At 60 FPS presentation on a reference Apple Silicon Mac, scrolling and movement are visually smooth, with simulation results unchanged by frame rate. Measure and report rather than assert untested performance.
- User can launch a packaged unsigned dev `.app` on a suitable Mac using documented installation steps; no browser/server required.
- All simulation tests and macOS CI build checks pass.
- Known usability/audio/graphics issues are documented after hands-on Mac playtest.

### Phase 5 — First content and tools iteration (not part of MVP)

Implement a basic cave editor, a deterministic wall-following creature, structural supports, optional level selection UI, local preferences and one original visual theme. Prioritise on the basis of prototype feedback.

---

## 12. Test strategy and failure reporting

### Unit and property tests

- Test terrain/entity/fluid invariants; no invalid double occupancy or out-of-bounds references.
- Table-driven cases for rock falls/rolls/pushes, crush collisions, dig/collect/exit, fluid conservation, and gates.
- Randomised seeded simulation tests assert state invariants after thousands of fixed ticks, with seed included on failure.
- Keep tests fast and independent of GPU, sound devices, display or keyboard.

### Golden replay fixtures

Store short scripted input sequences under `testdata/replays/`; include a seed, level checksum and expected stable state hash, plus human explanation. Fixture changes require justification and review; don't silently regenerate all expected hashes after a physics refactor.

### Performance gates

- Design for 60x40 maps first; record average/p95 simulation tick processing on CI and Mac.
- Rendering should target 60 FPS at native Retina resolutions on the actual target Mac. Do not declare this achieved from cloud compilation alone.
- Memory use should remain bounded for the first five caves; no long-running tick/replay memory growth.
- Introduce a benchmark before optimising; no speculative multithreading in deterministic simulation.

### Bug-report template

`Build/commit`, `macOS version/architecture`, `cave ID`, `seed`, `tick or replay file`, `steps`, `expected`, `actual`, `screenshot/video if visual`, `reproducible?`.

---

## 13. Packaging, distribution and local run

- For development, use `go run ./cmd/deepshift` on a Mac with appropriate Go and OS dependencies.
- CI should produce an appropriately structured `.app` (`Contents/MacOS`, `Contents/Info.plist`, resource files) and zip it without losing executable permissions.
- Verify launch via Finder and native window creation on a real Mac.
- Support Apple Silicon as the minimum target. Intel/universal builds may follow; if producing a universal build, package/test both architectures and use an appropriate macOS toolchain.
- Signing/notarisation and installation convenience are separate release milestones; use secure, explicit steps rather than asking users to disable Gatekeeper globally.
- Save files should eventually use the macOS user data directory rather than writing alongside the application bundle. Keep user-generated content outside the signed bundle.

---

## 14. Future gameplay concept backlog

These are ideas, **not MVP commitments**:

- Drill for stronger surfaces, supports for temporary structures, magnetic manipulator, controlled explosives and environmental scanner.
- Distinct terrain grades; falling debris, fragile supports, conveyor belts, switches, mechanisms and moving platforms.
- Fungi/amoeba ecology and deterministic cave-dwelling enemies whose behaviour can be exploited for puzzle solving.
- Water pumps/pressure, electricity/conductivity, lava/heat/steam, ventilation/toxic gases.
- Hand-authored campaign of several dozen caves across multiple biomes; secrets and optional challenge routes.
- Procedural expedition mode seeded for replay; initially heuristic checks, then graph/search-based validations for limited mechanic subsets.
- Rewind implemented via periodic snapshots plus re-simulation of tick-stamped input; replay integrity is a prerequisite.
- A native integrated drag-and-drop cave editor, quick playtesting, JSON export/import and eventually cave sharing.
- Controller support, configurable input/accessibility preferences, high-score/challenge modes and cosmetic themes.

Avoid building systems merely because they sound impressive. Every candidate feature should specify a new player decision and at least one playable cave that demonstrates it.

---

## 15. Risks and product guardrails

| Risk | Mitigation |
|---|---|
| Scope creep from 7+ interacting systems | Implement gravity then water; stage everything else |
| Sim rules hard to reason about | Determinism contract, rule docs, replay fixtures, explicit update order |
| Cloud builds mistaken for Mac testing | Separate compile, headless test and real Mac GUI verification |
| Performance issues | Fixed-tick benchmarks and visible camera culling before complex rendering |
| Poor game feel despite correct code | Playtest five authored caves before expanding world |
| Unclear originality / rights | New brand, characters, assets, levels and code; use prior games only as conceptual inspiration |
| LLM agent runs ahead of spec | Narrow milestone PRs, tests, review, decision log, no extra frameworks |
| Cross-platform packaging surprises | Keep Mac runner build and physical Mac smoke test in release checklist |

---

## 16. Definition of done for the initial project

The initial **playable vertical slice** is done when all of the following hold:

- [ ] A physical Mac launches the standalone Deep Shift `.app` without a browser or server.
- [ ] Five *original* caves work end-to-end (start, objective, exit, death, restart).
- [ ] Digging, pushing, falling, rolling, crushing and collecting are implemented to a documented rule set.
- [ ] Water/gate creates at least one meaningful new puzzle in the first five caves.
- [ ] Physics is deterministic and renderer-independent; replay fixtures and tests pass.
- [ ] Caves are stored in editable, versioned files and checked by a validator.
- [ ] Smooth visuals, readable HUD and usable native keyboard controls pass human playtesting.
- [ ] macOS arm64 binary/application builds in CI; unsigned dev distribution caveats documented.
- [ ] Build instructions, test commands, known limitations and next-step roadmap are in the repo.
- [ ] No copied Boulder Dash source/art/levels or unlicensed external art assets are present.

### What to do first

**Start with Phase 0 and the essential core of Phase 1 only.** The first cloud session should not attempt rendering polish, water, five caves or the entire campaign. Use `CLAUDE_CLOUD_KICKOFF.md` as the actual first-agent prompt. Each subsequent session should take one of the acceptance-tested phases.

---

## 17. Reference links (verify at setup time)

- Claude cloud sessions: https://support.claude.com/en/articles/12618689-use-claude-code-in-the-cloud
- Claude Code: https://claude.ai/code
- Ebitengine 2.10 announcement and headless/virtualization feature: https://ebitengine.org/en/blog/v2.10.0.html
- Ebitengine cross-compilation FAQ: https://ebitengine.org/en/documents/faq.html
- Ebitengine project: https://github.com/hajimehoshi/ebiten

These external links are technical references, not game assets or licensing grants.
