# Deep Shift rules reference — rule version 5

This file describes the rules **as implemented** in `internal/sim` (`sim.RuleVersion = 5`). The code and its
tests are authoritative; keep this file in step with them.

- **Rule version 1** covered movement, digging, crystals and the quota-sealed exit.
- **Rule version 2** added **gravity** for boulders and crystals (falling, rolling and crushing), and
  pushing for boulders.
- **Rule version 3** added **water** (a conserved fluid layer that falls and spreads), **sluice gates**
  and the **valves** that open and close them, and the player's **Interact** action.
- **Rule version 4** added **crawlers**: creatures that follow the wall on their left, catch the player,
  are crushed by falling objects and drown in water.
- **Rule version 5** adds **glimmers**: the crawler's mirror image, following the wall on their right,
  which **shatter into crystals** when a falling object lands on them.

Caves without glimmers play exactly as under rule version 4.

## Grid and coordinates

- The cave is a rectangle of `width × height` cells (each side 1..256). Cell `(x, y)` has origin at the
  top-left, and `y` grows downward, the direction of gravity.
- Each cell has a **terrain**, at most one **occupant**, and a **water** level:

| Terrain | Symbol | Passable? | Notes |
|---|---|---|---|
| empty | ` ` (space) | yes | open floor |
| earth | `.` | yes, by digging | becomes empty when the player enters |
| wall | `#` | never | permanent |
| exit | `E` (sealed) / `X` (open, dumps only) | only when open | entering it while open completes the cave |
| closed gate | `G` | never | blocks the player, objects and water, like a wall |
| open gate | `g` | yes | exactly like empty floor for every rule |
| valve | `V` (shut) / `v` (open) | never | turned with Interact; its state is its gates' state |

| Occupant | Symbol | Rounded? | Notes |
|---|---|---|---|
| player | `@` | no | exactly one; starts on empty terrain; may stand in water |
| crystal | `*` | yes | falls and rolls like a boulder; collected when the player enters its cell, even while it is falling; never pushed |
| boulder | `O` | yes | always on open floor; can be pushed; needs cave `format_version` 2 |
| crawler | `↑` `↓` `←` `→` | no | the arrow is the way it faces; never falls or is pushed; needs cave `format_version` 4 ([Crawlers](#crawlers)) |
| glimmer | `▲` `▼` `◀` `▶` | no | the triangle is the way it faces; never falls or is pushed; needs cave `format_version` 5 ([Glimmers](#glimmers)) |

| Water | Symbol | Notes |
|---|---|---|
| 1 to 3 units | `1` `2` `3` | shallow: the player can wade in |
| 4 units (`WaterCapacity`) | `~` | deep: the player cannot enter |

- **Open floor** means empty terrain or an open gate. Objects rest, fall and roll on it, water flows
  through it, and the player walks on it.
- Boulders and crystals are the **falling objects**. Their cell also carries a **falling** flag. It is set
  when the object moved (fell or rolled) on the most recent gravity step, and cleared when it comes to rest
  or is collected. Cave files always start objects at rest.
- Only objects on open floor are subject to gravity. Level files cannot place a boulder inside earth.
- **Water** may only be in open floor without a boulder or crystal. It shares cells with the player.
  Water reaching a crawler or glimmer drowns it by the end of the world step.
- Crawlers and glimmers are the **creatures**. A creature's cell also carries its **facing**: the direction
  it last moved, or starts facing.
- Anything outside the cave behaves as a wall. "Empty" below means open floor with no occupant; water does
  not matter.
- Water, gates and valves need cave `format_version` 3. Each valve is linked to its gates by the cave's
  `mechanisms` list.

## Tick model

- One `Step` is exactly one logical tick. The simulation runs at **20 ticks per second** (provisional). The
  app runs 3 display updates per tick ([D-003](DECISIONS.md#d-003-fixed-step-scheduling-provisional-rates)).
- Inputs are per tick. At most one action applies to a tick: Interact or a move.
- **Update order within a tick:**
  1. **Player phase.** If the action cooldown is above zero, decrement it and ignore any input. Otherwise:
     if Interact is given and a valve is next to the player, turn it (below); else resolve at most one
     move, which may push a boulder. Entering the open exit ends the cave immediately, and the rest of
     the tick is skipped.
  2. **World phase**, only on ticks that are multiples of `GravityIntervalTicks = 2` (provisional, so
     objects and water move 10 cells/s):
     1. **Gravity:** boulders and crystals are scanned **bottom row to top row, and left to right
        within a row**. Each object updates at most once per tick: a cell an object moved into this step
        is skipped if the scan reaches it again, and a boulder pushed this tick sits out the step.
     2. **Creatures**, only on ticks that are multiples of `CrawlerIntervalTicks = 6` (provisional, so
        creatures move 3.3 cells/s against the player's 5): crawlers and glimmers together, scanned
        **top row to bottom row, left to right within a row**. Each moves at most once per step: a cell a
        creature moved into is skipped if the scan reaches it again. A catch ends the creature step.
     3. **Water:** falls, then spreads (below).
     4. **Drowning:** every creature whose cell now holds water is removed, in row-major order.
  3. **Quota.** If the exit is sealed and `collected ≥ required_crystals`, it opens and emits
     `exit-opened`. This is skipped if the player died this tick.
  4. The tick counter increments.
- Once the cave is complete **or the player is dead**, `Step` changes nothing and the tick counter stops.

## Player actions

| Destination | Result | Events (in order) | Cooldown |
|---|---|---|---|
| outside the cave, a wall, a closed gate or a valve | blocked | — | not started |
| sealed exit | blocked | `exit-sealed` | not started |
| deep water (4 units) | blocked | `too-deep` | not started |
| a crawler or glimmer | the player is caught and does not move; status `dead` | `player-caught` (creature → player) | — |
| empty or open gate, dry or shallow | move (the player wades in any water) | `moved` | started |
| earth | dig it (earth → empty), move | `moved`, `dug` | started |
| crystal (resting or falling) | collect it (+1, crystal removed), move | `moved`, `collected` | started |
| boulder, pushable (below) | push it one cell, move into its cell | `moved`, `boulder-pushed` | started |
| boulder, not pushable | blocked | — | not started |
| open exit | move; the cave is complete | `moved`, `cave-complete` | started |

- **Cadence:** a successful action sets the cooldown to `MoveIntervalTicks − 1 = 3`, so a held direction
  moves on ticks `t, t+4, t+8, …` (5 tiles/s). A blocked move does not start the cooldown. Input given
  during the cooldown is dropped, not queued.
- **Pushing:** only **left or right**, only a **resting** boulder (not falling), and only into **empty
  floor**, which may hold water. There are no chains: a second boulder, earth, a wall, a crystal, the exit,
  a closed gate or the cave edge beyond the boulder blocks the push. Pushes always succeed when allowed;
  there is no random resistance.
- The player is not affected by gravity and may stand over an empty pit or on the surface of a pool.
- **Interact** turns the valve orthogonally next to the player. If several are, the first in the order
  up, down, left, right is turned. With no valve in reach, Interact does nothing and the move (if any)
  applies.

## Gravity (boulders and crystals)

For each falling object, in scan order, the first matching rule applies. Events name the object (`boulder`
or `crystal`). Water never stops an object: cells below and beside are judged ignoring water.

| Situation | Result | Event | Falling afterwards |
|---|---|---|---|
| cell below is empty | fall one cell | `fell` (from → to) | yes |
| player below, object **falling** | the player is crushed; the object stops on them; status `dead` | `player-crushed` (object → player) | no |
| crawler below, object **falling** | the crawler is crushed and removed; the object falls into its cell | `crawler-crushed`, then `fell` | yes |
| glimmer below, object **falling** | the glimmer shatters into crystals ([Glimmers](#glimmers)); the object stops where it is | `glimmer-shattered`, a `crystal-formed` for each crystal, then `landed` | no |
| below is rounded (boulder or crystal), the **left** cell and the cell below it are both empty | roll one cell left | `rolled` | yes |
| as above but left is blocked, **right** cell and the cell below it both empty | roll one cell right | `rolled` | yes |
| anything else (earth, wall, exit, closed gate, valve, cave floor, a resting object on the player or a creature, blocked rolls) | stay | `landed` if it was falling | no |

- **Never teleports:** a fall moves exactly one cell down. A roll moves one cell sideways, and the object
  drops into the diagonal on the *next* gravity step.
- **Resting vs falling:** a resting object above the player is held up by the player and never crushes
  them. When the player steps out from under it, the object falls into the vacated cell and is then
  falling. So **step aside, never straight down**: moving down from under a boulder or crystal means it
  lands on you on the next gravity step, because you can't move again for 4 ticks.
- **Catching a crystal:** walking into a crystal's cell collects it even while it is falling. Standing
  *under* a falling crystal crushes you, like a boulder.
- Objects never enter earth, walls, closed gates, valves or the exit, and never destroy each other; they
  rest on or roll off.
- Stacks fall together: the bottom-up scan frees each cell before the object above is considered.
- **Displacement:** when an object moves (falls, rolls or is pushed) into a cell holding water, that water
  swaps into the cell the object just left. That cell never holds water while an object is in it, so the
  swap always fits and water is exactly conserved
  ([D-028](DECISIONS.md#d-028-falling-objects-and-water-one-policy)).

## Water

Each world step, after gravity:

1. **Fall.** Rows are scanned bottom to top. Each cell with water pours as much as fits into the cell
   below, if that cell can hold water. Water moves at most one cell down per step.
2. **Spread.** For every pair of side-by-side cells that can hold water, compare their levels after the
   fall: if one is at least **two units** higher, **one unit** flows to the lower cell. Every pair is
   judged on the same levels and all transfers are applied together, so there is no left/right bias.

- **Holds water:** open floor (empty terrain or an open gate) without a boulder or crystal. The player's
  cell holds water. Walls, earth, the exit, closed gates, valves, boulders, crystals and the cave edge all
  contain it.
- **Conserved:** every transfer moves units between two cells, so the total never changes. Nothing
  creates or destroys water: not digging, collecting, pushing, gates or the exit.
- **Settles:** in a closed cave, falls lower the water and spreads level it, so it always comes to rest.
  Levels end within one unit of their neighbours: a one-unit film on a flat floor stays put.
- **Slopes:** on a flat floor water settles as a slope that falls by at most one unit per cell. So it
  reaches at most three cells beyond the last full cell (4, 3, 2, 1). To move water far, drop it: falls
  are not limited.
- **No pressure:** water never rises. It does not climb U-bends or push up through openings.
- **Player:** shallow water (1 to 3 units) can be entered; deep water (4 units) cannot (`too-deep`).
  Water flows into the player's cell freely and never harms or moves the player.

## Sluice gates and valves

- A **mechanism** is a valve and the gates it operates. Its gates are always all open or all closed, and
  the valve shows which.
- **Opening** (a shut valve): the valve and all its gates open at once. Events: `valve-turned`, then
  `gate-opened` for each gate in the cave's listed order. The cooldown starts.
- **Closing** (an open valve) needs every gate clear: **no occupant and no water**. Then the valve and all
  gates close: `valve-turned`, then `gate-closed` for each gate. Otherwise `gate-jammed` names the first
  blocked gate, nothing changes, and the cooldown does not start.
- An open gate is ordinary floor, so water flows through it and objects fall through it: a closed gate
  under a boulder is a **trapdoor** that drops it on the same tick's gravity step.
- Water rules discover gate changes by themselves; caves contain no scripts.

## Crawlers

A crawler is a creature that keeps the wall on its left. It is not affected by gravity: it clings to
walls, so it can patrol up and down shafts as easily as along corridors.

- **Open to a crawler:** open floor (empty terrain or an open gate) holding **no water at all** and no
  occupant except the player. Earth, walls, closed gates, valves, the exit, boulders, crystals, other
  creatures, any water and the cave edge all block it.
- **Left-hand rule:** on its step a crawler tries the cell to its **left**, then **ahead**, then to its
  **right**, then **behind**, relative to its facing, and takes the first one open to it. Its facing becomes
  the direction it moved. With all four blocked it stays put and keeps its facing.
  - Along a wall it follows the wall; in a one-wide corridor it walks to the end and turns back; in open
    space, with nothing on its left, it turns in a tight square.
  - Openings on its **right** are only taken when its left and ahead are both blocked. A pocket or tunnel
    on a crawler's right is a safe place to wait while it passes.
- **Catching the player:** if the cell a crawler picks holds the player, it catches them: `player-caught`
  (crawler → player), status `dead`. The crawler stays where it is (it turns to face the player), and no
  other creature moves that tick. The rest of the world step still runs, as after a crush.
  - Walking into a crawler is fatal too (see Player actions): the player does not move.
  - **Standing next to a crawler is safe.** Only a crawler stepping into the player's cell, or the player
    stepping into the crawler's, catches.
- **Crushing:** a **falling** boulder or crystal landing on a crawler crushes it and falls on into its
  cell (`crawler-crushed`, then `fell`). A resting object over a crawler is held up and harms nothing, so a
  crawler may walk under a resting boulder; when it walks on, the boulder falls into the cell it left, as
  it would behind the player.
- **Drowning:** crawlers never step into water. Water reaching a crawler's cell, by falling or spreading
  in the water phase, drowns it at the end of that world step (`crawler-drowned`).
- Crawlers are never pushed or collected, and do not block or displace water: water may flow into a
  crawler's cell (and drown it).
- **Gates:** an open gate is floor to a crawler. A crawler standing in a gate jams it, like any occupant.
- **Validation:** a crawler must face up, down, left or right; it cannot start in water, earth, a wall, a
  closed gate, a valve or on the exit. Only creatures have a facing.

## Glimmers

A glimmer is the crawler's mirror image: it keeps the wall on its **right**. It holds crystals, and a
falling object landing on it shatters it into them.

- **Right-hand rule:** on its step a glimmer tries the cell to its **right**, then **ahead**, then to its
  **left**, then **behind**, and takes the first one open to it. Its facing becomes the direction it
  moved. With all four blocked it stays put and keeps its facing.
  - Round a pillar it goes clockwise, the pillar on its right; in open space it turns in a tight square
    to the right.
  - Openings on its **left** are only taken when its right and ahead are both blocked. A pocket or tunnel
    on a glimmer's left is a safe place to wait while it passes.
- **Everything else is the crawler's:** the same cells are open to it, it moves on the same ticks, in the
  same scan as crawlers (of two creatures wanting one cell, the first in the scan gets it), it catches the
  player the same two ways (`player-caught` names it), it never steps into water, and water reaching it
  drowns it (`glimmer-drowned`). Crawlers and glimmers block each other.
- **Shattering:** a **falling** boulder or crystal landing on a glimmer (directly above it, in the gravity
  phase) shatters it:
  1. The glimmer is removed (`glimmer-shattered`, object → glimmer).
  2. Its own cell, and every **empty, dry floor** cell of the 3×3 block around it, gets a **resting
     crystal**, in row-major order (`crystal-formed` for each). Cells outside the cave are skipped.
     Nothing else in the block changes: earth, walls, the exit, gates, valves, boulders, crystals,
     creatures, the player and cells holding any water stay as they were. So a shattering never hurts the
     player and never creates or destroys water.
  3. The object stops where it was (`landed`); it does not fall on through.
  - A glimmer's cell is always dry when gravity runs (water drowns it in the step it arrives), so a
    shattering leaves at least one crystal, and at most `ShatterCrystals = 9` (less the object's own cell,
    so 8 in practice).
  - **New crystals sit out the rest of the gravity step,** like a pushed boulder. From the next step they
    fall and roll by the usual rules. A crystal that forms over the player rests on them and never crushes
    them on that step.
  - A **resting** object over a glimmer is held up and shatters nothing, as with crawlers.
- **Drowning leaves nothing:** a drowned glimmer leaves no crystals.
- **Quota:** `required_crystals` may count on up to 9 crystals per glimmer (cave `format_version` 5). A
  completion replay is what shows a cave can really be finished.
- **Validation:** as for crawlers.

## Examples

Grids are shown between `|` bars so that empty cells are visible.

**Falling** (`TestBoulderFallsOneCellPerGravityStep`): one cell per gravity tick, landing on the wall.
Each column is the state after the labelled tick.

```text
start   tick 0  tick 2  tick 4  tick 6
|O@|    | @|    | @|    | @|    | @|
|  |    |O |    |  |    |  |    |  |
|  |    |  |    |O |    |  |    |  |
|  |    |  |    |  |    |O |    |O |  landed
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
|@|                 |O|  fell             |O|  player-crushed: status dead
|.|                 |@|                   |@|
```

**Push into a pit** (fixture `cave_03_complete.json`): at tick 140 the push moves the boulder into the
cell over the pit. It sits out that tick's gravity, falls from tick 142, and the player walks over the
pit behind it.

**Spreading** (`TestWaterSpreadsEvenlyToBothSides`): one deep cell on a flat floor gives one unit to each
side, then stops, because neighbours one unit apart do not flow.

```text
start           tick 0 and after
|@#   ~   #|    |@#  121  #|
|##########|    |##########|
```

**Sinking** (`TestObjectsSinkThroughWaterDisplacingIt`): a boulder above a flooded shaft falls into it,
swapping places with the water one cell per gravity step.

```text
start    tick 0   tick 2
|@#O#|   |@#~#|   |@#~#|
| #~#|   | #O#|   | #~#|
| #~#|   | #~#|   | #O#|
| ###|   | ###|   | ###|
```

**Pushing a boulder through water** (fixture `cave_05_boulder_route.json`): each push moves the boulder
into deep water and that water into the cell the boulder left, which the player steps into. The water
behind the player spreads back as a slope. The player follows the boulder along the channel and never needs
to drain it. Six cells of deep water hold 24 units, and the total stays 24.

```text
start          after push 1     after push 2
|@O~~~~~~#|    |1@O~~~~~#|      |23@O~~~~#|   (the player's cell holds water the dump does not show)
|#########|    |#########|      |#########|
```

**Draining a reservoir** (`TestClosedGateHoldsWaterUntilTheValveOpensIt`): the closed gate holds two
deep cells back indefinitely. Interact beside the valve opens it, and the water spreads into the channel
until no neighbours differ by two units, which takes until tick 7. The open gate holds 2 units that the
dump does not show (`g`), so the total is still 8. The water stops one cell past the gate: a slope of one
unit per cell.

```text
start (any number of ticks)     after Interact, settled
|@V    |                        |@v    |
|~~G   |                        |32g1  |
|######|                        |######|
```

**Following the wall** (`TestCrawlerFollowsTheWallOnItsLeft`): a crawler facing up in the top-left
corner of a corridor round a block has a wall to its left and ahead, so it turns right. From then on the
outer wall is always on its left, and it circles clockwise, one cell per crawler step:

```text
start          step 1         step 5         step 12
|#########|    |#########|    |#########|    |#########|
|#↑    #@#|    |# →   #@#|    |#     #@#|    |#↑    #@#|
|# ### ###|    |# ### ###|    |# ###↓###|    |# ### ###|
|#     ###|    |#     ###|    |#     ###|    |#     ###|
|#########|    |#########|    |#########|    |#########|
```

**Open space** (`TestCrawlerInOpenSpaceCirclesASquare`): with nothing on its left, a crawler turns left
every step, so it circles a 2×2 square for ever: from `(3,3)` facing up, it visits `(2,3)`, `(2,4)`,
`(3,4)`, `(3,3)` and back.

**Caught** (fixture `crawler_warren_caught.json`, columns 7 to 11 of rows 2 and 3): the player waits in
the hole dug above the ring corridor, which is on the crawler's right and so safe. At tick 120 the
crawler reaches the corridor's corner; at tick 121 the player steps down into the corridor, in its path.
At tick 126 the crawler moves next to the player (still safe) and at tick 132 into them.

```text
tick 121    tick 126    tick 132
|## ##|     |## ##|     |## ##|
|  @ ↑|     |  @← |     |  @← |  player-caught: status dead
```

**The drop** (fixture `crawler_warren_complete.json`, `TestCrawlerWarrenDropCrushesTheCrawlerWhateverTheTiming`):
a boulder pushed into a one-wide shaft falls three times as fast as the crawler pacing in it moves, and
the crawler cannot leave the shaft, so the boulder always catches it falling. Pushed at tick 581, it
crushes the crawler at tick 586 and comes to rest at the bottom.

**The flood** (`TestCrawlerWarrenFloodDrownsTheCrawlerWhateverTheTiming`): nine units of water dropped
into the middle of a five-cell corridor settle as `12321`. That covers all five cells, so the crawler
pacing there is drowned wherever it is, and no cell is deep, so the player can wade through afterwards.

**Shattering** (`TestFallingObjectShattersAGlimmer`): a falling boulder lands on a glimmer in a small
room. The glimmer's cell and the seven empty cells around it each get a resting crystal; the boulder's own
cell keeps the boulder, which stops there.

```text
start (boulder falling)   tick 0
|#######|                 |#######|
|# O #@#|                 |#*O*#@#|
|# ▲ ###|                 |#***###|
|#   ###|                 |#***###|
|#######|                 |#######|
```

**The vault** (fixture `glimmer_hollow_complete.json`, `TestGlimmerHollowVaultShattersWhateverTheTiming`,
columns 6 to 12 of rows 3 to 10): the vault glimmer is boxed in under a closed gate holding a boulder.
At tick 547 the player turns the valve; at tick 548 the boulder drops into the open gate; at tick 550,
falling, it shatters the glimmer into five crystals and stops. The two below the vault walls fall from
tick 552 and land in the basin at tick 556. Gravity runs before creatures, so the boulder always fills the
gate before the glimmer could leave through it.

```text
tick 547    tick 548    tick 550    tick 556
|#     #|   |#     #|   |#     #|   |#     #|
|v@#O# #|   |v@# # #|   |v@# # #|   |v # # #|
|#  g  #|   |#  O  #|   |# *O* #|   |# @O* #|
|###▼#.#|   |###▼#.#|   |###*#.#|   |###*#.#|
|## # .#|   |## # .#|   |##*#*.#|   |## # .#|
|## # .#|   |## # .#|   |## # .#|   |## # .#|
|##     |   |##     |   |##     |   |##* *  |
|#######|   |#######|   |#######|   |#######|
```

**Caught by a glimmer** (fixture `glimmer_hollow_caught.json`, columns 19 to 26 of rows 2 and 3): the
player steps from the dug entrance into the ring's top corridor at tick 145, with the glimmer three cells
west and heading east. It moves alongside at tick 156 (still safe) and into the player at tick 162.

```text
tick 145     tick 156     tick 162
|##### ##|   |##### ##|   |##### ##|
|  ▶  @  |   |    ▶@  |   |    ▶@  |  player-caught: status dead
```

## Determinism contract

- The outcome depends only on **(cave, seed, rule version, per-tick inputs)**.
- Cells are processed in a documented order (row-major for the player and quota, bottom-to-top /
  left-to-right for gravity, row-major for the crystals of a shattering, top-to-bottom / left-to-right for
  creatures and drowning, bottom-to-top for water falls). Water spreads are judged on frozen levels, so
  their order cannot matter. No Go map iteration, wall-clock time, file I/O or global state affects the
  rules. `internal/sim` imports only `crypto/sha256`, `encoding/binary`, `encoding/hex`, `errors`, `fmt`
  and `strings`; `TestImportsArePure` and a `go list -deps` check in CI enforce this.
- **Randomness:** SplitMix64 seeded with the cave seed. Its state is hashed. Rule versions 4 and 5 do not
  draw from it: creatures and shattering are fully determined by the cave.
- **Test-cave example:** in `test_cave` two crystals sit in open pockets, so they drop one cell at tick 0
  (`(9,21)->(9,22)` and `(25,5)->(25,6)`, visible with `cavetool replay -trace`).
- **State hash** (`sim.StateHash`, layout v4, unchanged by rule version 5, which added no state; glimmers
  use the occupant and facing bytes): SHA-256 of a fixed little-endian layout starting with
  `deep-shift/state-hash/v4\0`. It then covers rule version (u32), seed (u64), tick (u64), width, height,
  required, collected, player x and player y (u32 each), exit-open, status and cooldown (u8 each), and the
  PRNG state (u64). Next come each cell's terrain, occupant, falling flag, water and facing (u8 each), in
  row-major order. Last are the mechanism links: their count, then for each the valve's x and y, the gate count and
  each gate's x and y (u32 each).
- **Trace hash** (replays): SHA-256 over the tick-0 state hash, then the state hash after each tick that
  advanced the simulation. Frozen ticks after completion or death add nothing.

## Verification

| Rule | Tests |
|---|---|
| Movement, bounds, walls, digging, collecting, cadence, exit quota | `internal/sim/sim_test.go` (rule-1 tests, unchanged and passing under rule 4) |
| Falls one cell per gravity tick, never skipping | `TestBoulderFallsOneCellPerGravityStep`, `TestStackFallsTogetherOneCellEach`, `TestCrystalFallsLikeABoulder` |
| Crystals roll, crush and can be caught | `TestCrystalRollsOffRoundedObjects`, `TestFallingCrystalCrushesThePlayer`, `TestWalkingIntoAFallingCrystalCollectsIt`, `TestObjectEmbeddedInEarthDoesNotFall` |
| Supported boulders stay at rest | `TestSupportedBoulderStaysAtRest` (earth, wall, floor, exit, player, flat blockers, blocked diagonals) |
| Rolling criteria and left-first tie-break | `TestRollingCriteriaAndTieBreak`, `TestRolledBoulderFallsNextStep` |
| At most one update per boulder per tick | `TestBoulderUpdatesAtMostOncePerStep` (mutation-checked: fails without the marker), `TestPushedBoulderFallsOnTheNextGravityStep`, per-tick event checks in the randomised runs |
| Push: horizontal, resting, single, into empty | `TestPushRules`, `TestFallingBoulderCannotBePushed` |
| Crushing, death freeze, reset after death | `TestCrushing`, `TestDeathFreezesAndResetRestores`, `TestEnteringTheExitEndsTheTickBeforeGravity` |
| Water falls one cell per step, spreads to both sides, rests on the floor | `TestWaterFallsOneCellPerStep`, `TestWaterSpreadsEvenlyToBothSides`, `TestWaterRestsOnTheCaveFloor` |
| Water is contained by walls, earth, exit, closed gates, valves, objects and the cave edge | `TestWaterIsContainedByEveryBarrier` |
| No left/right bias; conservation; settling | `TestWaterIsMirrorSymmetricConservedAndSettles` (20 random caves; mutation-checked: a left-to-right live spread fails it) |
| Deep water blocks, shallow does not; water flows into the player's cell | `TestDeepWaterBlocksThePlayerAndShallowWaterDoesNot`, `TestWaterFlowsIntoThePlayersCell` |
| Objects displace water (fall, push, roll) | `TestObjectsSinkThroughWaterDisplacingIt`, `TestPushingABoulderIntoWaterDisplacesItBehind`, `TestBoulderRollsIntoWaterAndDisplacesIt` |
| Closed gate holds water; valve opens it; no level scripts | `TestClosedGateHoldsWaterUntilTheValveOpensIt`, `TestOpeningAGateUnderABoulderDropsIt` |
| Interact: adjacency order, cooldown, fall-through to a move | `TestInteractTurnsTheFirstAdjacentValveUpDownLeftRight`, `TestInteractDuringCooldownIsIgnored`, `TestInteractWithoutAValveMovesInstead` |
| Closing needs clear gates; gates block and pass the player | `TestClosingAGateNeedsItClear`, `TestPlayerCannotEnterAClosedGateButWalksThroughAnOpenOne` |
| Reset restores water, gates and valves | `TestResetRestoresGatesValvesAndWater` |
| Crawlers: left-hand rule, open-space square, boxed in, one move per step, crawler cadence | `TestCrawlerFollowsTheWallOnItsLeft`, `TestCrawlerInOpenSpaceCirclesASquare`, `TestBoxedInCrawlerStays`, `TestCrawlerMovesAtMostOncePerStep` |
| Crawlers catch the player both ways; a catch ends the crawler step; adjacency is safe | `TestCrawlerCatchesThePlayerByMovingIntoThem`, `TestWalkingIntoACrawlerIsFatal`, `TestCatchStopsTheCrawlerStep` |
| Falling objects crush crawlers, resting ones do not; water drowns them and they never enter it | `TestFallingObjectCrushesACrawler`, `TestRestingObjectDoesNotCrushACrawler`, `TestCrawlersNeverEnterWaterAndDrownWhenItReachesThem` |
| Crawlers are blocked by terrain, objects and closed gates, pass open gates; reset; validation; symbols | `TestCrawlersAreBlockedByTerrainAndObjects`, `TestResetRestoresCrawlers`, `TestCrawlerValidation`, `TestCrawlerSymbolsRoundTrip` |
| Glimmers: right-hand rule, the exact mirror image of a crawler, open-space square | `TestGlimmerFollowsTheWallOnItsRight`, `TestGlimmerIsTheMirrorImageOfACrawler` (30 random caves run mirrored with the kinds swapped; mutation-checked: glimmers on the left-hand rule fail it), `TestGlimmerInOpenSpaceCirclesASquare` |
| Glimmers catch both ways and name themselves; crawlers and glimmers share one scan and block each other; blocked by terrain, objects and water | `TestCreaturesCatchThePlayerEitherWay`, `TestCrawlersAndGlimmersShareOneScan`, `TestGlimmersAreBlockedByTerrainObjectsAndCreatures` |
| Shattering: which cells fill (every kind of neighbour, boulders and crystals, the cave edge), the object stops, the player is spared | `TestFallingObjectShattersAGlimmer` (mutation-checked: crystals forming in wet cells fail it), `TestShatteringAtTheCaveEdge` (mutation-checked: a block wrapping round the rows fails it), `TestShatteringSparesThePlayer` |
| New crystals wait for the next gravity step; resting objects shatter nothing; drowning leaves nothing; reset; validation and the quota bound; symbols | `TestNewCrystalsWaitForTheNextGravityStep` (mutation-checked: without the updated marks a crystal falls in the tick it formed), `TestRestingObjectDoesNotShatterAGlimmer` (mutation-checked: shattering on any object over a glimmer fails it), `TestDrownedGlimmerLeavesNoCrystals`, `TestResetRestoresGlimmers`, `TestGlimmerValidation`, `TestGlimmerSymbolsRoundTrip` |
| Invariants under random input with water, gates, valves, crawlers and glimmers (20 seeds × 3000 ticks) | `TestRandomisedRunsKeepInvariantsAndDeterminism` (water conserved and bounded, never in walls or objects, gates match valves; creatures have a facing, stand on dry floor, move one cell, and are only removed by a crush, a shattering or drowning; every shattering's crystals lie in its block, one in the glimmer's cell, and crystals are conserved counting those formed; no crystal moves in the tick it formed) |
| 100 identical runs, including deaths by crush, by crawler and by glimmer, a reservoir drain, crawlers crushed and drowned, and glimmers shattered and drowned | `TestReplayIsDeterministicAcross100Runs` |
| Death ends the attempt and is logged as a replay | `TestCrushFixtureEndsInDeath`, `TestCrushEndsAttemptAndLogsReproducibleReplay` |
| Cave 04 needs the sluice and conserves water | `TestCave04TheFloodedShaftIsTheOnlyWayDown`, `TestCave04FixtureDrainsTheShaftBeforeGoingDown` |
| Crawler Warren: the drop and the flood work whatever the timing; the shaft is the only way down; a catch is logged and reproducible | `TestCrawlerWarrenDropCrushesTheCrawlerWhateverTheTiming`, `TestCrawlerWarrenFloodDrownsTheCrawlerWhateverTheTiming`, `TestCrawlerWarrenTheShaftIsTheOnlyWayDown`, `TestCaughtFixtureEndsInDeath`, `TestCaughtEndsTheAttemptWithItsOwnOverlay` |
| Glimmer Hollow: the vault always gives the same five crystals and the sluice always drowns its glimmer, whatever the timing; the ring glimmer goes clockwise; the vault glimmer is boxed in; the quota needs the vault and the ring; the corridor is the only way out; a catch is logged and reproducible | `TestGlimmerHollowVaultShattersWhateverTheTiming` (mutation-checked: creatures moving before gravity fail it), `TestGlimmerHollowFloodDrownsTheGlimmerWhateverTheTiming`, `TestGlimmerHollowRingGlimmerGoesRoundClockwise`, `TestGlimmerHollowVaultGlimmerIsBoxedIn`, `TestGlimmerHollowQuotaNeedsTheVaultAndTheRing`, `TestGlimmerHollowTheCorridorIsTheOnlyWayToTheExit`, `TestGlimmerCaughtFixtureEndsInDeath`, `TestGlimmerCatchEndsTheAttemptWithItsOwnOverlay` |
| Every cave is completable | `TestEveryEmbeddedCaveHasACompletionFixture` |
| Hash layout pinned | `TestStateHashGolden`, golden replay fixtures |
