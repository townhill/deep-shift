# Claude Code project instructions — Deep Shift

## Source of truth
Read `docs/SPEC.md` before every significant change. `docs/DECISIONS.md` records deviations or clarified assumptions. `docs/ROADMAP.md` tracks progress.

## Architecture constraints
- Native macOS game. Do not introduce Electron, a browser frontend, a backend server, a database or web-only deployment.
- Language Go, Ebitengine for graphics, **pure Go `internal/sim`** with no renderer or system UI dependencies.
- Game state and rules deterministic for given level, seed, rule version and tick-stamped command sequence.
- Commit small scope-focused changes with tests. CI passing is mandatory; GUI claims require actual GUI observation.
- Assets, levels, narrative, brand and code are original. No copied Boulder Dash materials.

## First priority
Follow `CLAUDE_CLOUD_KICKOFF.md` for the first session only. Do not expand the first PR into the entire game.

## Quality gates
- `go test ./...`
- `go vet ./...`
- Deterministic replay tests and explicit cave parser validation
- macOS arm64 CI compile/build, followed by human Mac smoke test for native GUI

## Progress communication
For each pull request summarize the task, changed files, test evidence, Mac GUI test limitations, decisions made, remaining risks and suggested next single milestone.
