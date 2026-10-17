package game

import "github.com/townhill/deep-shift/internal/sim"

// Camera presentation constants (provisional; tune after Mac playtesting).
const (
	// LookaheadTiles shifts the view toward the facing direction so more of
	// the cave ahead of the player is visible.
	LookaheadTiles = 3.0
	// CameraEase is the fraction of the remaining distance the camera covers
	// per update. Counted in updates, never wall-clock time.
	CameraEase = 0.15
)

// Camera tracks the top-left corner of the view, in tile units. It is pure
// presentation state: nothing in the simulation reads it.
type Camera struct {
	X, Y         float64
	ViewW, ViewH float64 // view size in tiles
	placed       bool
}

// Target returns where the camera wants to be to frame a focus point (tile
// coordinates of the player's centre) on a mapW x mapH cave: centred on the
// focus plus lookahead, clamped so the view never shows past the cave edges.
// A cave smaller than the view is centred instead.
func (c *Camera) Target(focusX, focusY float64, facing sim.Dir, mapW, mapH int) (x, y float64) {
	dx, dy := facing.Delta()
	return clampAxis(focusX+LookaheadTiles*float64(dx), c.ViewW, float64(mapW)),
		clampAxis(focusY+LookaheadTiles*float64(dy), c.ViewH, float64(mapH))
}

func clampAxis(centre, view, size float64) float64 {
	if size <= view {
		return (size - view) / 2 // negative: the cave is centred in the view
	}
	return min(max(centre-view/2, 0), size-view)
}

// Follow eases the camera toward Target. The first call (or the first after
// Snap) jumps straight there.
func (c *Camera) Follow(focusX, focusY float64, facing sim.Dir, mapW, mapH int) {
	tx, ty := c.Target(focusX, focusY, facing, mapW, mapH)
	if !c.placed {
		c.X, c.Y, c.placed = tx, ty, true
		return
	}
	c.X += (tx - c.X) * CameraEase
	c.Y += (ty - c.Y) * CameraEase
}

// Snap makes the next Follow jump straight to its target.
func (c *Camera) Snap() { c.placed = false }
