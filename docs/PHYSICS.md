# Deep Shift rules reference — rule version 2

This file describes the rules **as implemented** in `internal/sim` (`sim.RuleVersion = 2`). The code and its
tests are authoritative; keep this file in step with them.

- **Rule version 1** covered movement, digging, crystals and the quota-sealed exit.
- **Rule version 2** adds **boulders**: falling, rolling, pushing and crushing.

Caves without boulders behave exactly as under rule version 1: the re-recorded test-cave fixtures have the
same outcomes and final grids. Water, gates and creatures are not implemented yet.

## Grid and coordinates

- The cave is a rectangle of `width × height` cells (each side 1..256). Cell `(x, y)` has origin at the
  top-left, and `y` grows downward, the direction of gravity.
- Each cell has a **terrain** and at most one **occupant**:

| Terrain | Symbol | Passable? | Notes |
|---|---|---|---|
| empty | ` ` (space) | yes | open floor |
| earth | `.` | yes, by digging | becomes empty when the player enters |
| wall | `#` | never | permanent |
| exit | `E` (sealed) / `X` (open, dumps only) | only when open | entering it while open completes the cave |

| Occupant | Symbol | Rounded? | Notes |
|---|---|---|---|
| player | `@` | no | exactly one; starts on empty terrain |
| crystal | `*` | yes | collected when the player enters its cell; **does not fall** in rule version 2 |
| boulder | `O` | yes | always on empty terrain; needs cave `format_version` 2 |

- A boulder cell also carries a **falling** flag. It is set when the boulder moved (fell or rolled) on the
  most recent gravity step, and cleared when it comes to rest. Cave files always start boulders at rest.
- Anything outside the cave behaves as a wall. "Empty" below means open floor with no occupant.

## Tick model

- One `Step` is exactly one logical tick. The simulation runs at **20 ticks per second** (provisional). The
  app runs 3 display updates per tick ([D-003](DECISIONS.md#d-003-fixed-step-scheduling-provisional-rates)).
- Inputs are per tick. At most one move direction applies to a tick.
- **Update order within a tick:**
  1. **Player phase.** If the move cooldown is above zero, decrement it and ignore any move. Otherwise
     resolve at most one move, which may push a boulder (below). Entering the open exit ends the cave
     immediately, and the rest of the tick is skipped.
  2. **Gravity phase**, only on ticks that are multiples of `GravityIntervalTicks = 2` (provisional, so
     boulders move 10 cells/s). Boulders are scanned **bottom row to top row, and left to right within a
     row**. Each boulder updates at most once per tick: a cell a boulder moved into this step is skipped if
     the scan reaches it again, and a boulder pushed this tick sits out the step.
  3. **Quota.** If the exit is sealed and `collected ≥ required_crystals`, it opens and emits
     `exit-opened`. This is skipped if the player died this tick.
  4. The tick counter increments.
- Once the cave is complete **or the player is dead**, `Step` changes nothing and the tick counter stops.

## Player movement

| Destination | Result | Events (in order) | Cooldown |
|---|---|---|---|
| outside the cave, or a wall | blocked | — | not started |
| sealed exit | blocked | `exit-sealed` | not started |
| empty | move | `moved` | started |
| earth | dig it (earth → empty), move | `moved`, `dug` | started |
| crystal | collect it (+1, crystal removed), move | `moved`, `collected` | started |
| boulder, pushable (below) | push it one cell, move into its cell | `moved`, `boulder-pushed` | started |
| boulder, not pushable | blocked | — | not started |
| open exit | move; the cave is complete | `moved`, `cave-complete` | started |

- **Cadence:** a successful move sets the cooldown to `MoveIntervalTicks − 1 = 3`, so a held direction moves
  on ticks `t, t+4, t+8, …` (5 tiles/s). A blocked move does not start the cooldown. A move given during the
  cooldown is dropped, not queued.
- **Pushing:** only **left or right**, only a **resting** boulder (not falling), and only into **empty
  floor**. There are no chains: a second boulder, earth, a wall, a crystal, the exit or the cave edge beyond
  the boulder blocks the push. Pushes always succeed when allowed; there is no random resistance.
- The player is not affected by gravity and may stand over an empty pit.

## Boulder gravity

For each boulder, in scan order, the first matching rule applies:

| Situation | Result | Event | Falling afterwards |
|---|---|---|---|
| cell below is empty | fall one cell | `boulder-fell` (from → to) | yes |
| player below, boulder **falling** | the player is crushed; the boulder stops on them; status `dead` | `player-crushed` (boulder → player) | no |
| below is rounded (boulder or crystal), the **left** cell and the cell below it are both empty | roll one cell left | `boulder-rolled` | yes |
| as above but left is blocked, **right** cell and the cell below it both empty | roll one cell right | `boulder-rolled` | yes |
| anything else (earth, wall, exit, cave floor, a resting boulder on the player, blocked rolls) | stay | `boulder-landed` if it was falling | no |

- **Never teleports:** a fall moves exactly one cell down. A roll moves one cell sideways, and the boulder
  drops into the diagonal on the *next* gravity step.
- **Resting vs falling:** a resting boulder above the player is held up by the player and never crushes
  them. When the player steps out from under it, the boulder falls into the vacated cell and is then
  falling. So **step aside, never straight down**: moving down from under a boulder means it lands on you
  on the next gravity step, because you can't move again for 4 ticks.
- Boulders never enter earth, walls or the exit, and do not destroy crystals (they rest on or roll off them).
- Stacks fall together: the bottom-up scan frees each cell before the boulder above is considered.

## Examples

Grids are shown between `|` bars so that empty cells are visible.

**Falling** (`TestBoulderFallsOneCellPerGravityStep`): one cell per gravity tick, landing on the wall.
Each column is the state after the labelled tick.

```text
start   tick 0  tick 2  tick 4  tick 6
|O@|    | @|    | @|    | @|    | @|
|  |    |O |    |  |    |  |    |  |
|  |    |  |    |O |    |  |    |  |
|  |    |  |    |  |    |O |    |O |  boulder-landed
|##|    |##|    |##|    |##|    |##|
```

**Rolling, left before right** (`TestRollingCriteriaAndTieBreak`, `TestRolledBoulderFallsNextStep`):

```text
tick 0: rolled left       tick 2: fell
|  O  @|  -> | O   @|  ->  |     @|
|  O   |     |  O   |      | OO   |
|  #   |     |  #   |      |  #   |
```

**Crushed by running down** (fixture `cave_02_crushed.json`): the player takes the crystal from under a
boulder at tick 24, moves down at tick 28, and the boulder falls into the cell they left. At tick 30 it
is falling and lands on them.

```text
tick 24 (collect)   tick 28 (move down)   tick 30
|O|                 | |                   | |
|@|                 |O|  boulder-fell     |O|  player-crushed: status dead
|.|                 |@|                   |@|
```

**Push into a pit** (fixture `cave_03_complete.json`): at tick 140 the push moves the boulder into the
cell over the pit. It sits out that tick's gravity, falls from tick 142, and the player walks over the
pit behind it.

## Determinism contract

- The outcome depends only on **(cave, seed, rule version, per-tick inputs)**.
- Cells are processed in a documented order (row-major for the player and quota, bottom-to-top /
  left-to-right for gravity). No Go map iteration, wall-clock time, file I/O or global state affects the
  rules. `internal/sim` imports only `crypto/sha256`, `encoding/binary`, `encoding/hex`, `errors`, `fmt`
  and `strings`; `TestImportsArePure` and a `go list -deps` check in CI enforce this.
- **Randomness:** SplitMix64 seeded with the cave seed. Its state is hashed. Rule version 2 does not draw
  from it.
- **State hash** (`sim.StateHash`, layout v2): SHA-256 of a fixed little-endian layout starting with
  `deep-shift/state-hash/v2\0`. It then covers rule version (u32), seed (u64), tick (u64), width, height,
  required, collected, player x and player y (u32 each), exit-open, status and cooldown (u8 each), and the
  PRNG state (u64). Finally it covers each cell's terrain, occupant and falling flag (u8 each), in
  row-major order.
- **Trace hash** (replays): SHA-256 over the tick-0 state hash, then the state hash after each tick that
  advanced the simulation. Frozen ticks after completion or death add nothing.

## Verification

| Rule | Tests |
|---|---|
| Movement, bounds, walls, digging, collecting, cadence, exit quota | `internal/sim/sim_test.go` (rule-1 tests, unchanged and passing under rule 2) |
| Falls one cell per gravity tick, never skipping | `TestBoulderFallsOneCellPerGravityStep`, `TestStackFallsTogetherOneCellEach` |
| Supported boulders stay at rest | `TestSupportedBoulderStaysAtRest` (earth, wall, floor, exit, player, flat blockers, blocked diagonals) |
| Rolling criteria and left-first tie-break | `TestRollingCriteriaAndTieBreak`, `TestRolledBoulderFallsNextStep` |
| At most one update per boulder per tick | `TestBoulderUpdatesAtMostOncePerStep` (mutation-checked: fails without the marker), `TestPushedBoulderFallsOnTheNextGravityStep`, per-tick event checks in the randomised runs |
| Push: horizontal, resting, single, into empty | `TestPushRules`, `TestFallingBoulderCannotBePushed` |
| Crushing, death freeze, reset after death | `TestCrushing`, `TestDeathFreezesAndResetRestores`, `TestEnteringTheExitEndsTheTickBeforeGravity` |
| Invariants under random input with boulders (20 seeds × 3000 ticks) | `TestRandomisedRunsKeepInvariantsAndDeterminism` |
| 100 identical runs, including a death | `TestReplayIsDeterministicAcross100Runs` |
| Death ends the attempt and is logged as a reproducible replay | `TestCrushFixtureEndsInDeath`, `TestCrushEndsAttemptAndLogsReproducibleReplay` |
| Every cave is completable | `TestEveryEmbeddedCaveHasACompletionFixture` |
| Hash layout pinned | `TestStateHashGolden`, golden replay fixtures |
