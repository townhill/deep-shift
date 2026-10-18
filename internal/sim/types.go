package sim

import "fmt"

// RuleVersion identifies the rule set implemented by this package. Bump it
// whenever a change could alter the outcome of any existing replay, and
// record the change in docs/DECISIONS.md and docs/PHYSICS.md.
const RuleVersion uint32 = 1

// TicksPerSecond is the provisional logical simulation rate (SPEC §5).
const TicksPerSecond = 20

// MoveIntervalTicks is the player's movement cadence: after a successful
// move, the next move may start this many ticks later (one move every 4 ticks
// = 5 tiles per second at 20 TPS). Provisional; tune after Mac playtesting.
const MoveIntervalTicks = 4

// MaxDimension bounds cave width and height.
const MaxDimension = 256

// Terrain is the static ground layer of a cell.
type Terrain uint8

// Terrain values. The numeric values are part of the state-hash encoding;
// append new values, never renumber.
const (
	TerrainEmpty Terrain = iota // open floor
	TerrainEarth                // diggable soil; becomes empty when entered
	TerrainWall                 // permanent wall; never passable
	TerrainExit                 // exit foundation; passable only once open
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
	}
	return fmt.Sprintf("terrain(%d)", uint8(t))
}

// Occupant is the solid object, if any, in a cell. At most one per cell.
type Occupant uint8

// Occupant values. The numeric values are part of the state-hash encoding;
// append new values, never renumber.
const (
	OccupantNone Occupant = iota
	OccupantPlayer
	OccupantCrystal
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
	}
	return fmt.Sprintf("occupant(%d)", uint8(o))
}

// Cell is one grid square. Further layers (fluid, metadata) will be added as
// separate fields in later phases rather than overloading these enums.
type Cell struct {
	Terrain  Terrain
	Occupant Occupant
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

// Input is the logical player input applied during one tick.
type Input struct {
	// Move requests a one-tile move. It is only acted on when the player's
	// movement window is open (see Sim.MoveReady); otherwise it is ignored.
	Move Dir
}

// Status is the overall state of the cave attempt.
type Status uint8

// Status values. The numeric values are part of the state-hash encoding.
const (
	StatusPlaying Status = iota
	StatusComplete
)

func (s Status) String() string {
	switch s {
	case StatusPlaying:
		return "playing"
	case StatusComplete:
		return "complete"
	}
	return fmt.Sprintf("status(%d)", uint8(s))
}
