// Package sim is the deterministic, renderer-independent rules engine for
// Deep Shift.
//
// The package is deliberately pure Go: it must not import Ebitengine, GUI,
// audio, OS event-loop, file-system or wall-clock packages. A test
// (TestImportsArePure) enforces an allow-list of standard-library imports.
//
// # Determinism contract
//
// A simulation is fully defined by (initial Cave, seed, RuleVersion, ordered
// per-tick Inputs). Running the same tuple twice yields identical StateHash
// values after every tick. To keep that true:
//
//   - Step advances exactly one fixed logical tick; there is no frame delta.
//   - All randomness comes from the explicitly seeded SplitMix64 held in the
//     state (and therefore included in the hash).
//   - Cells are stored in a row-major slice and traversed in index order; no
//     Go map iteration influences outcomes.
//   - Renderers read Snapshot copies and Events; they cannot mutate state.
//
// # Tick numbering
//
// A fresh or Reset simulation is at tick 0. Step applies the Input for the
// current tick T, then the tick becomes T+1. "The hash at tick N" means the
// hash after N Steps. Once the cave is complete, Step is a no-op and the tick
// counter stops advancing. The same holds once the player is dead.
//
// # Update order (rule version 2)
//
//  1. Player phase: movement cooldown, then the move (dig, collect, push,
//     exit). Entering the open exit ends the tick's updates.
//  2. Gravity phase, on ticks that are multiples of GravityIntervalTicks:
//     boulders and crystals on open floor fall, roll, land or crush,
//     scanned bottom-to-top and left-to-right, each at most once.
//  3. Quota check, which may open the exit (skipped if the player died).
//  4. Tick counter increments.
//
// docs/PHYSICS.md is the human-readable rules reference and must be kept in
// step with this package.
package sim
