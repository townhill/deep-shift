# Deep Shift rules reference — rule version 1

This file describes the rules **as implemented** in `internal/sim` (`sim.RuleVersion = 1`). The code and its
tests are authoritative; keep this file in step with them. Boulders, water, gates and creatures are **not**
in rule version 1. Phase 2 (boulders) will add to this file and bump the rule version.

## Grid and coordinates

- The cave is a rectangle of `width × height` cells (each side 1..256). Cell `(x, y)` has origin at the
  top-left. `y` grows downward, which will be the direction of gravity.
- Each cell has a **terrain** and at most one **occupant**:

| Terrain | Symbol | Passable? | Notes |
|---|---|---|---|
| empty | ` ` (space) | yes | open floor |
| earth | `.` | yes, by digging | becomes empty when the player enters |
| wall | `#` | never | permanent in v1 |
| exit | `E` (sealed) / `X` (open, dumps only) | only when open | entering it while open completes the cave |

| Occupant | Symbol | Notes |
|---|---|---|
| player | `@` | exactly one; starts on empty terrain |
| crystal | `*` | collected when the player enters its cell |

- Anything outside the cave behaves as a wall.

## Tick model

- One `Step` is exactly one logical tick. The simulation rate is **20 ticks per second** (provisional). The
  app runs 3 display updates per tick; see [DECISIONS D-003](DECISIONS.md#d-003-fixed-step-scheduling-provisional-rates).
- Inputs are per tick. At most one move direction (`up`, `down`, `left`, `right`) applies to a tick.
- **Update order within a tick:**
  1. **Player phase.** If the move cooldown is above zero, decrement it and ignore any move. Otherwise
     resolve at most one move (below).
  2. **World phase.** If the exit is sealed and `collected ≥ required_crystals`, open it and emit
     `exit-opened` for the exit cell.
  3. The tick counter increments.
- After the cave is complete, `Step` changes nothing and the tick counter stops.

## Movement

| Destination | Result | Events (in order) | Cooldown |
|---|---|---|---|
| outside the cave | blocked | — | not started |
| wall | blocked | — | not started |
| sealed exit | blocked | `exit-sealed` | not started |
| empty | move | `moved` | started |
| earth | dig the cell (earth → empty), move | `moved`, `dug` | started |
| crystal | collect (+1, crystal removed), move | `moved`, `collected` | started |
| open exit | move, cave complete | `moved`, `cave-complete` | started |

- **Cadence:** a successful move sets the cooldown to `MoveIntervalTicks − 1 = 3`, so a held direction
  moves on ticks `t, t+4, t+8, …` (5 tiles/s).
- A blocked move does not start the cooldown, so another direction can be tried on the very next tick.
- A move given during the cooldown is dropped, not queued. The game session separately latches one key
  press until the window opens (see [D-004](DECISIONS.md#d-004-movement-cadence-and-input-model-provisional)).
- A crystal is counted exactly once: its cell becomes empty when collected.
- The quota check runs after the player phase. On the tick the last required crystal is collected, the
  events are `moved`, `collected`, `exit-opened`, and the earliest the player can enter the exit is the
  next movement window.
- `required_crystals = 0` means the exit is open before tick 0 (no `exit-opened` event).

## Examples

Grids are shown between `|` bars so empty cells are visible. This is the scenario of
`TestExitCannotCompleteBeforeQuota` (quota 2). Ticks not listed are cooldown ticks or have no input.

```text
tick  input  grid   result
 0    right  |*@E|  blocked by the sealed exit: exit-sealed; cooldown not started
             |*  |
 1    left   |@ E|  moved, collected (1/2)
             |*  |
 5    down   |  X|  moved, collected (2/2), exit-opened (X = open exit)
             |@  |
 9    right  |  X|  moved
             | @ |
13    up     | @X|  moved
             |   |
17    right  |  @|  moved, cave-complete; later Steps change nothing
             |   |
```

Digging changes exactly one tile, the one the player enters:

```text
tick  input  before  after
 0    right  |@..|   | @.|   moved, dug
```

## Determinism contract

- The full outcome is a function of **(cave, seed, rule version, per-tick inputs)**.
- Cells are processed in row-major index order. No Go map iteration, wall-clock time, file I/O or global
  state affects the rules. `internal/sim` imports only `crypto/sha256`, `encoding/binary`, `encoding/hex`,
  `errors`, `fmt` and `strings` (enforced by `TestImportsArePure`, and by a `go list -deps` check in CI).
- Randomness: SplitMix64 seeded with the cave seed. Its state is hashed. Rule version 1 never draws from it.
- **State hash** (`sim.StateHash`): SHA-256 of a fixed little-endian layout starting with
  `deep-shift/state-hash/v1\0`, then: rule version (u32), seed (u64), tick (u64), width, height,
  required, collected, player x, player y (u32 each), exit-open, status and cooldown (u8 each), PRNG
  state (u64), then each cell's terrain and occupant (u8 each) in row-major order.
- **Trace hash** (replays): SHA-256 over the tick-0 state hash followed by the state hash after each tick.

## Verification

| Rule | Tests |
|---|---|
| Movement in all directions; blocked moves change nothing | `TestMoveIntoEmptyCell`, `TestBlockedMoves` |
| Bounds and corners | `TestBlockedMoves` (edge and corner cases) |
| Digging removes exactly one earth tile | `TestDiggingRemovesExactlyOneEarthTile`, `TestDugTunnelIsWalkableWithoutDiggingAgain` |
| Collecting increments exactly once | `TestCollectingCrystalIncrementsOnce` |
| Cadence and dropped cooldown input | `TestMovementCadence`, `TestMoveInputDuringCooldownIsIgnored`, `TestBlockedMoveLeavesWindowOpenForAnotherDirection` |
| Exit sealed until quota; completes after | `TestExitCannotCompleteBeforeQuota`, `TestQuotaBelowTotalOpensEarly`, `TestZeroQuotaExitStartsOpen`, `test_cave_sealed_exit.json` |
| Completion freezes state | `TestCompletedCaveIsFrozen` |
| Restart restores everything, including the seed | `TestResetRestoresAllState`, `TestRestartOnlyFromPauseOrCompletion` |
| Invariants under random input (20 seeds × 3000 ticks) | `TestRandomisedRunsKeepInvariantsAndDeterminism` |
| 100 identical runs → identical final and trace hashes | `TestReplayIsDeterministicAcross100Runs` |
| Hash layout pinned | `TestStateHashGolden`, golden replay fixtures |
