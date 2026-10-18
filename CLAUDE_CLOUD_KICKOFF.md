# First Claude Code Cloud Session — Kickoff Prompt

**Usage:** Commit the spec to `docs/SPEC.md` in a new GitHub repository, then paste the text under **Prompt to Claude** into a session at https://claude.ai/code. This kick-off is deliberately limited to the foundation and first movement rules.

## Prompt to Claude

You are the principal Go developer for **Deep Shift**, an original native macOS 2D underground physics/puzzle adventure. This project is **not** a web game, Electron app or an emulation of Boulder Dash.

Read `docs/SPEC.md` completely before implementing. Treat its non-goals, determinism contract, IP/originality rules and cloud-vs-Mac testing distinction as requirements. Any choice labelled *provisional* may be used as a sensible default; log significant decisions in `docs/DECISIONS.md`.

### Scope: first PR only

Implement the repository scaffold and **Phase 0 plus a small, tested slice of Phase 1**:

1. Initialise a clean Go module using Go 1.25+ and Ebitengine v2.10+, with dependencies pinned by Go modules.
2. Create a minimum native window application entrypoint under `cmd/deepshift` that displays a simple placeholder grid; no web frontend and no network server.
3. Implement a **pure Go, renderer-independent** fixed-tick simulation package (`internal/sim`) and versioned level parser/validator (`internal/level`).
4. Support one playable test cave with movement (WASD/arrows), earth digging, collecting crystals, impassable walls, player start, exit unlocked by quota, pause/restart, and an on-screen status display. Keep visuals simple and original. If graphical input/rendering cannot be exercised in cloud, still implement it and clearly mark it as *compile-only, pending Mac playtest*.
5. Add deterministic seeded state hashing and basic tick-stamped input replay or an explicit first-pass replay fixture. A hundred identical runs of the same input sequence must yield the same hash.
6. Add table-driven Go tests for movement, bounds, digging, collecting, exit unlock, invalid maps, restart, and replay/hash determinism. **None of these unit tests should require a display or graphics context.**
7. Add a GitHub Actions workflow with a Linux testing job and a macOS build job. Produce a downloadable unsigned arm64 macOS `.app` artefact if packaging can be done reliably; otherwise produce a darwin-arm64 executable, document why full packaging is incomplete, and record it as a follow-up task. Don't claim the app has been graphically tested if you cannot actually launch it on a Mac.
8. Add `README.md`, `docs/ROADMAP.md`, `docs/DECISIONS.md`, and `docs/PHYSICS.md`; include exact local Mac build/run steps. Include simple original placeholder assets generated programmatically or in source code.

### Non-negotiable constraints

- Go only for the simulation; `internal/sim` must not import Ebitengine, GUI packages, audio libraries, or OS event loop code.
- Native macOS `.app` target, no browser-first architecture, no Electron, no React/Vite and no database.
- Deterministic tick simulation independent of frame rate and wall clock.
- Keep current PR strictly to the scoped foundation. Do **not** add water, creatures, electricity, lava, story scenes, level editor or assets beyond minimal placeholders.
- No copyrighted original Boulder Dash code, cave layouts, artwork or music.
- Make tests runnable in a cloud Linux shell. Avoid dependence on a graphical desktop in `go test ./...` (consider isolation of any Ebitengine initialization in the app executable).
- State honestly which build and runtime behaviours were tested and which remain unverified.

### Acceptance checklist for this first PR

- `go test ./...` is green; `go vet ./...` is green.
- At least one deterministic replay test executes 100 repeat simulations with identical hashes.
- Test cave validates; invalid maps fail with useful errors.
- Exit completion cannot occur until required crystals are collected.
- A testable fixed-step engine has no renderer dependency.
- CI compiles macOS arm64 target and records an artifact or an explicit packaging gap.
- Human can follow README instructions to build/run on macOS.
- PR description includes files, tests/results, assumptions, limitations and next phase.

**Do the work now in this session.** Make reasonable decisions within the scope; don't request confirmation for straightforward implementation choices. Finish by opening a PR for review, or, if GitHub PR creation is unavailable, make a branch and provide the commit and the exact steps needed to open one. Never falsely state that you visually tested the Mac application in the cloud.

## After the first PR

Once the Mac test cave is running, start a second cloud session for Phase 2 (falling/rolling/pushing boulders) using `docs/SPEC.md` section 11 as the acceptance contract. Start the water/gate system only after Phase 2 physics tests and Mac playtesting are stable.
