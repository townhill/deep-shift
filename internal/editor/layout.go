package editor

import (
	"math"

	"github.com/townhill/deep-shift/internal/sim"
)

// Layout places the editor on the logical screen, in pixels: a status bar
// at the top, the cave view, and a bar at the bottom holding the palette
// and the problem list. The renderer draws to it and the editor hit-tests
// the mouse against it, so both agree on where everything is.
type Layout struct {
	Tile          int // tile size
	Width, Height int // logical screen size
	Top, Bottom   int // heights of the top and bottom bars
}

// Palette slot geometry within the bottom bar.
const (
	slotPad   = 8  // margin before the first slot and above the tiles
	slotPitch = 40 // distance between slot left edges
)

// ViewTiles is the size of the cave view in tiles.
func (l Layout) ViewTiles() (w, h float64) {
	return float64(l.Width) / float64(l.Tile), float64(l.Height-l.Top-l.Bottom) / float64(l.Tile)
}

// InView reports whether the screen point (x, y) is over the cave view.
func (l Layout) InView(x, y int) bool {
	return x >= 0 && x < l.Width && y >= l.Top && y < l.Height-l.Bottom
}

// CellAt returns the cave cell under the screen point (x, y) with the view's
// top-left corner at (camX, camY) tiles. The cell may lie outside the cave.
func (l Layout) CellAt(x, y int, camX, camY float64) sim.Pos {
	fx := float64(x)/float64(l.Tile) + camX
	fy := float64(y-l.Top)/float64(l.Tile) + camY
	return sim.Pos{X: int(math.Floor(fx)), Y: int(math.Floor(fy))}
}

// Slot returns the top-left pixel of palette slot i's tile.
func (l Layout) Slot(i int) (x, y int) {
	return slotPad + i*slotPitch, l.Height - l.Bottom + slotPad
}

// SlotAt returns the palette slot under the screen point (x, y), if any.
func (l Layout) SlotAt(x, y int) (int, bool) {
	for i := range Palette {
		sx, sy := l.Slot(i)
		if x >= sx && x < sx+l.Tile && y >= sy && y < sy+l.Tile {
			return i, true
		}
	}
	return 0, false
}

// TextX is where the bottom bar's text starts, right of the palette.
func (l Layout) TextX() int { return slotPad + len(Palette)*slotPitch + slotPad }
