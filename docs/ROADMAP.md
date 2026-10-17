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
- [ ] **Mac:** a human follows the README on a physical Mac and the window opens

## Phase 1 — Deterministic core (first slice delivered)

- [x] Versioned, validated cave format with located errors (`internal/level`)
- [x] Movement, earth digging, crystal collection, walls, cave bounds
- [x] Quota-sealed exit that opens when the quota is met and completes on entry
- [x] Fixed-tick loop (20 TPS; 3 updates per tick); movement cadence of one move per 4 ticks
- [x] Snapshot state hash, seeded PRNG, restart, ASCII state dumps (`cavetool show`, `Sim.ASCII`)
- [x] Tick-stamped replays with golden fixtures; 100-run determinism test with per-tick trace hashes
- [x] Pause/resume, restart from pause/completion, HUD (cave, quota, exit state, time, prompts)
- [x] Original placeholder art drawn in code; one original test cave ("Proving Ground")
- [ ] **Mac:** playtest of the test cave (checklist below)

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

## Phase 2 — Boulder physics (next)

Acceptance contract: SPEC §11 Phase 2. Falling (one cell per tick, never skipping), rolling with
documented tie-breaking, single horizontal push without chains, crush/death with events, the
`updatedAtTick` guarantee, two example caves, and replay fixtures that include a death. Start only after the
Phase 1 Mac playtest is done.

## Phase 3 — Water and sluice gate

Not started. Begin only after Phase 2's physics tests and Mac playtesting are stable.

## Phase 4 — Playable native Mac vertical slice

Not started.

## Follow-ups recorded in this PR

| ID | Follow-up | Why |
|---|---|---|
| F-1 | Hands-on Mac playtest of Phase 0/1 (checklist above) | The GUI has not been observed on macOS |
| F-2 | Developer ID signing and notarisation in CI | Remove the Gatekeeper "Open Anyway" step; needs Apple credentials as secrets |
| F-3 | App icon (`.icns`) generated from original art | The bundle currently has no icon |
| F-4 | Retina-crisp rendering (`LayoutF`/device scale) and a real UI font | Placeholder text uses the 2× debug font |
| F-5 | In-app replay export (for example on completion or via a key) | Bug reports should carry a replay; `Session.Replay` already builds one |
| F-6 | `tools/vmcapture` plus an optional CI job using Ebitengine `exp/vmhost` | Repeatable headless frame captures ([D-014](DECISIONS.md#d-014-ebitengine-expvmhost-headless-capture-assessed-not-adopted)) |
| F-7 | Intel / universal (`lipo`) build and test | macOS amd64 is "desired"; currently compile-only |
| F-8 | Rebindable keys, reduced-effects mode, UI scaling | SPEC §4 accessibility; designed for but not built |
| F-9 | Record simulation tick timing (avg/p95) on a reference Mac | SPEC §12 performance gate; CI records a Linux benchmark only |
