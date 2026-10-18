package sim

import "fmt"

// RuleVersion identifies the rule set implemented by this package. Bump it
// whenever a change could alter the outcome of any existing replay, and
// record the change in docs/DECISIONS.md and docs/PHYSICS.md.
//
//   - 1: movement, digging, crystals, quota-sealed exit.
//   - 2: boulders and crystals fall, roll and crush the player; boulders can
//     be pushed.
//   - 3: water flows (falls, then spreads) and is conserved; sluice gates
//     opened and closed by valves; deep water blocks the player.
//   - 4: crawlers, wall-following creatures that catch the player, are
//     crushed by falling objects and drown in water.
const RuleVersion uint32 = 4

// TicksPerSecond is the provisional logical simulation rate (SPEC §5).
const TicksPerSecond = 20

// MoveIntervalTicks is the player's movement cadence: after a successful
// move, the next move may start this many ticks later (one move every 4 ticks
// = 5 tiles per second at 20 TPS). Provisional; tune after Mac playtesting.
const MoveIntervalTicks = 4

// GravityIntervalTicks is the gravity cadence: gravity runs on every tick
// whose number is a multiple of this (10 cells per second at 20 TPS, twice
// the player's walking speed). Provisional; tune after Mac playtesting.
const GravityIntervalTicks = 2

// CrawlerIntervalTicks is the crawlers' cadence: they step on every tick
// whose number is a multiple of this (one cell per 6 ticks, about 3.3 cells
// per second, slower than the player). It is a multiple of
// GravityIntervalTicks, so crawler steps fall on world-phase ticks.
// Provisional; tune after Mac playtesting.
const CrawlerIntervalTicks = 6

// WaterCapacity is the most water one cell holds. A full cell is deep water,
// which the player cannot wade into. Provisional; see docs/PHYSICS.md.
const WaterCapacity = 4

// MaxDimension bounds cave width and height.
const MaxDimension = 256

// Terrain is the static ground layer of a cell.
type Terrain uint8

// Terrain values. The numeric values are part of the state-hash encoding;
// append new values, never renumber.
const (
	TerrainEmpty      Terrain = iota // open floor
	TerrainEarth                     // diggable soil; becomes empty when entered
	TerrainWall                      // permanent wall; never passable
	TerrainExit                      // exit foundation; passable only once open
	TerrainGateClosed                // sluice gate, closed: blocks like a wall
	TerrainGateOpen                  // sluice gate, open: behaves as open floor
	TerrainValveShut                 // valve whose gates are closed; never passable
	TerrainValveOpen                 // valve whose gates are open; never passable
	terrainCount
)

func (t Terrain) String() string {
	switch t {
	case TerrainEmpty:
		return "empty"
	case TerrainEarth:
		return "earth"
	case TerrainWall:
		return "wall"
	case TerrainExit:
		return "exit"
	case TerrainGateClosed:
		return "closed gate"
	case TerrainGateOpen:
		return "open gate"
	case TerrainValveShut:
		return "shut valve"
	case TerrainValveOpen:
		return "open valve"
	}
	return fmt.Sprintf("terrain(%d)", uint8(t))
}

// Gate reports whether t is a sluice gate, open or closed.
func (t Terrain) Gate() bool { return t == TerrainGateClosed || t == TerrainGateOpen }

// Valve reports whether t is a valve, shut or open.
func (t Terrain) Valve() bool { return t == TerrainValveShut || t == TerrainValveOpen }

// Floor reports whether t is open floor: empty ground or an open gate.
// Objects rest, fall and roll on floor, water flows through it and the
// player walks on it.
func (t Terrain) Floor() bool { return t == TerrainEmpty || t == TerrainGateOpen }

// Occupant is the solid object, if any, in a cell. At most one per cell.
type Occupant uint8

// Occupant values. The numeric values are part of the state-hash encoding;
// append new values, never renumber.
const (
	OccupantNone Occupant = iota
	OccupantPlayer
	OccupantCrystal
	OccupantBoulder
	OccupantCrawler
	occupantCount
)

func (o Occupant) String() string {
	switch o {
	case OccupantNone:
		return "none"
	case OccupantPlayer:
		return "player"
	case OccupantCrystal:
		return "crystal"
	case OccupantBoulder:
		return "boulder"
	case OccupantCrawler:
		return "crawler"
	}
	return fmt.Sprintf("occupant(%d)", uint8(o))
}

// Rounded reports whether falling objects roll off this occupant. Boulders
// and crystals are rounded; the player, walls, earth and the exit are flat.
func (o Occupant) Rounded() bool { return o == OccupantBoulder || o == OccupantCrystal }

// Falls reports whether gravity acts on this occupant: boulders and crystals.
func (o Occupant) Falls() bool { return o == OccupantBoulder || o == OccupantCrystal }

// Cell is one grid square: a terrain, at most one occupant and a fluid
// layer. Further layers are added as separate fields rather than by
// overloading these enums.
type Cell struct {
	Terrain  Terrain
	Occupant Occupant
	// Falling is metadata for boulders and crystals: the object moved (fell
	// or rolled) on the most recent gravity step. Only a falling object
	// crushes the player, and only a resting boulder can be pushed. Always
	// false for other occupants.
	Falling bool
	// Water is the fluid layer: 0..WaterCapacity units. Water shares a cell
	// with the player but never with a boulder or crystal. Water reaching a
	// crawler drowns it by the end of the tick.
	Water uint8
	// Facing is metadata for crawlers: the direction the crawler last moved
	// (or starts facing). DirNone for every other occupant.
	Facing Dir
}

// Empty reports whether a falling object could move into c: open floor with
// no occupant. Water does not matter; an object entering water displaces it.
func (c Cell) Empty() bool { return c.Terrain.Floor() && c.Occupant == OccupantNone }

// HoldsWater reports whether water may occupy c: open floor without a
// boulder or crystal.
func (c Cell) HoldsWater() bool { return c.Terrain.Floor() && !c.Occupant.Falls() }

// Deep reports whether c is full of water, too deep for the player to wade
// into.
func (c Cell) Deep() bool { return c.Water >= WaterCapacity }

// OpenToCrawler reports whether a crawler may move into c: dry open floor
// with no occupant other than the player (whom it catches).
func (c Cell) OpenToCrawler() bool {
	return c.Terrain.Floor() && c.Water == 0 && (c.Occupant == OccupantNone || c.Occupant == OccupantPlayer)
}

// Pos is an integer grid coordinate. The origin is the upper-left cell and Y
// grows downward (the direction of gravity).
type Pos struct {
	X, Y int
}

func (p Pos) String() string { return fmt.Sprintf("(%d,%d)", p.X, p.Y) }

// Add returns p offset by d.
func (p Pos) Add(d Dir) Pos {
	dx, dy := d.Delta()
	return Pos{p.X + dx, p.Y + dy}
}

// Dir is a cardinal direction, or DirNone.
type Dir uint8

// Dir values. The numeric values are part of the state-hash encoding.
const (
	DirNone Dir = iota
	DirUp
	DirDown
	DirLeft
	DirRight
)

// Delta returns the unit grid offset for d.
func (d Dir) Delta() (dx, dy int) {
	switch d {
	case DirUp:
		return 0, -1
	case DirDown:
		return 0, 1
	case DirLeft:
		return -1, 0
	case DirRight:
		return 1, 0
	}
	return 0, 0
}

func (d Dir) String() string {
	switch d {
	case DirNone:
		return "none"
	case DirUp:
		return "up"
	case DirDown:
		return "down"
	case DirLeft:
		return "left"
	case DirRight:
		return "right"
	}
	return fmt.Sprintf("dir(%d)", uint8(d))
}

// ParseDir is the inverse of Dir.String for the four cardinal directions.
func ParseDir(s string) (Dir, bool) {
	switch s {
	case "up":
		return DirUp, true
	case "down":
		return DirDown, true
	case "left":
		return DirLeft, true
	case "right":
		return DirRight, true
	}
	return DirNone, false
}

// Input is the logical player input applied during one tick. Moving and
// interacting share the player's action window (see Sim.MoveReady); input
// given while it is closed is ignored.
type Input struct {
	// Move requests a one-tile move.
	Move Dir
	// Interact turns the valve next to the player (see Sim.AdjacentValve).
	// It takes precedence over Move; with no valve adjacent, Move applies.
	Interact bool
}

// Status is the overall state of the cave attempt.
type Status uint8

// Status values. The numeric values are part of the state-hash encoding.
const (
	StatusPlaying Status = iota
	StatusComplete
	StatusDead
)

func (s Status) String() string {
	switch s {
	case StatusPlaying:
		return "playing"
	case StatusComplete:
		return "complete"
	case StatusDead:
		return "dead"
	}
	return fmt.Sprintf("status(%d)", uint8(s))
}
