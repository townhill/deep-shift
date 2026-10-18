// Package render draws game.Frames with Ebitengine.
//
// It only reads Frames (copies of simulation state plus presentation data)
// and never touches the simulation. Its output has not been observed in a
// real macOS window yet: compile-only, pending Mac playtest.
package render

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"

	"github.com/townhill/deep-shift/internal/game"
	"github.com/townhill/deep-shift/internal/sim"
	"github.com/townhill/deep-shift/internal/sprites"
)

// Layout constants, in logical pixels. Ebitengine scales the logical screen
// to the window, keeping the aspect ratio.
const (
	TileSize     = sprites.Size
	ViewTilesW   = 30
	ViewTilesH   = 18
	HUDHeight    = 64
	ScreenWidth  = ViewTilesW * TileSize           // 960
	ScreenHeight = ViewTilesH*TileSize + HUDHeight // 640

	textScale  = 2 // debug font is 6x16; drawn at 2x for legibility
	glyphW     = 6
	glyphH     = 16
	lineHeight = glyphH * textScale
)

var (
	backgroundColour   = color.RGBA{10, 9, 14, 0xff}
	hudColour          = color.RGBA{16, 18, 26, 0xff}
	hudRuleColour      = color.RGBA{60, 200, 210, 0xff}
	waterColour        = color.NRGBA{40, 110, 200, 150}
	waterSurfaceColour = color.NRGBA{170, 215, 245, 200}
)

// Renderer owns the GPU images used to draw frames.
type Renderer struct {
	tiles [sprites.KindCount]*ebiten.Image
	world *ebiten.Image // the play area, clipped to the view
	text  *ebiten.Image // scratch for 2x text
	pixel *ebiten.Image // 1x1 white, for tinted rectangles
	build string
}

// New creates a renderer. build is shown on the pause overlay so playtest
// reports can name the exact build.
func New(build string) *Renderer {
	r := &Renderer{
		world: ebiten.NewImage(ScreenWidth, ScreenHeight-HUDHeight),
		text:  ebiten.NewImage(ScreenWidth/textScale, glyphH),
		pixel: ebiten.NewImage(1, 1),
		build: build,
	}
	r.pixel.Fill(color.White)
	for k := sprites.Kind(0); k < sprites.KindCount; k++ {
		r.tiles[k] = ebiten.NewImageFromImage(sprites.Generate(k))
	}
	return r
}

// Draw renders one frame.
func (r *Renderer) Draw(screen *ebiten.Image, f game.Frame) {
	screen.Fill(backgroundColour)
	r.drawWorld(f)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(0, HUDHeight)
	screen.DrawImage(r.world, op)
	r.drawHUD(screen, f.HUD)
	if len(f.HUD.Overlay) > 0 {
		r.drawOverlay(screen, f.HUD.Overlay)
	}
}

func (r *Renderer) drawWorld(f game.Frame) {
	r.world.Fill(backgroundColour)
	snap := f.Snapshot
	// Whole-pixel camera offsets keep the pixel art crisp while scrolling.
	camX := math.Round(f.CameraX * TileSize)
	camY := math.Round(f.CameraY * TileSize)
	x0, y0 := int(math.Floor(f.CameraX)), int(math.Floor(f.CameraY))
	gliding := make(map[sim.Pos]bool, len(f.Gliding))
	for _, g := range f.Gliding {
		gliding[g.Cell] = true
	}
	op := &ebiten.DrawImageOptions{}
	for y := max(y0, 0); y <= min(y0+ViewTilesH, snap.Height-1); y++ {
		for x := max(x0, 0); x <= min(x0+ViewTilesW, snap.Width-1); x++ {
			p := sim.Pos{X: x, Y: y}
			cell := snap.Cell(p)
			op.GeoM.Reset()
			op.GeoM.Translate(float64(x*TileSize)-camX, float64(y*TileSize)-camY)
			r.world.DrawImage(r.tiles[terrainSprite(cell.Terrain, snap.ExitOpen)], op)
			if k, ok := objectSprite(cell.Occupant); ok && !gliding[p] {
				r.world.DrawImage(r.tiles[k], op)
			}
		}
	}
	// Moving objects and the player are drawn after all terrain, at their
	// interpolated positions, so the next row's tiles never cover them.
	for _, g := range f.Gliding {
		k, ok := objectSprite(g.Object)
		if !ok {
			continue
		}
		op.GeoM.Reset()
		op.GeoM.Translate(math.Round(g.X*TileSize-camX), math.Round(g.Y*TileSize-camY))
		r.world.DrawImage(r.tiles[k], op)
	}
	op.GeoM.Reset()
	if snap.Status == sim.StatusDead {
		// Crushed: squashed to the bottom of the cell and tinted red.
		op.GeoM.Scale(1, 0.45)
		op.GeoM.Translate(0, TileSize*0.55)
		op.ColorScale.Scale(1, 0.45, 0.45, 1)
	}
	op.GeoM.Translate(math.Round(f.PlayerX*TileSize-camX), math.Round(f.PlayerY*TileSize-camY))
	r.world.DrawImage(r.tiles[sprites.Player], op)
	r.drawWater(f, x0, y0, camX, camY)
}

// drawWater draws translucent water over everything else in the world, so
// a wading player is tinted where they stand and an object sinking out of a
// cell passes under the water rising into it. The fill height follows the
// eased level; deep cells (exactly full in the snapshot) carry wave lines,
// the shape cue that the player cannot enter them.
func (r *Renderer) drawWater(f game.Frame, x0, y0 int, camX, camY float64) {
	snap := f.Snapshot
	if len(f.Water) != len(snap.Cells) {
		return
	}
	op := &ebiten.DrawImageOptions{}
	for y := max(y0, 0); y <= min(y0+ViewTilesH, snap.Height-1); y++ {
		for x := max(x0, 0); x <= min(x0+ViewTilesW, snap.Width-1); x++ {
			i := y*snap.Width + x
			level := f.Water[i]
			if level <= 0 {
				continue
			}
			left, top := float64(x*TileSize)-camX, float64(y*TileSize)-camY
			h := level / sim.WaterCapacity * TileSize
			r.fillRectF(r.world, left, top+TileSize-h, TileSize, h, waterColour)
			if y == 0 || f.Water[i-snap.Width] <= 0 {
				r.fillRectF(r.world, left, top+TileSize-h, TileSize, 2, waterSurfaceColour)
			}
			if snap.Cells[i].Deep() {
				op.GeoM.Reset()
				op.GeoM.Translate(left, top)
				r.world.DrawImage(r.tiles[sprites.Waves], op)
			}
		}
	}
}

// objectSprite returns the sprite for a boulder or crystal occupant.
func objectSprite(o sim.Occupant) (sprites.Kind, bool) {
	switch o {
	case sim.OccupantCrystal:
		return sprites.Crystal, true
	case sim.OccupantBoulder:
		return sprites.Boulder, true
	}
	return 0, false
}

func terrainSprite(t sim.Terrain, exitOpen bool) sprites.Kind {
	switch t {
	case sim.TerrainEarth:
		return sprites.Earth
	case sim.TerrainWall:
		return sprites.Wall
	case sim.TerrainExit:
		if exitOpen {
			return sprites.ExitOpen
		}
		return sprites.ExitSealed
	case sim.TerrainGateClosed:
		return sprites.GateClosed
	case sim.TerrainGateOpen:
		return sprites.GateOpen
	case sim.TerrainValveShut:
		return sprites.ValveShut
	case sim.TerrainValveOpen:
		return sprites.ValveOpen
	}
	return sprites.Empty
}

func (r *Renderer) drawHUD(screen *ebiten.Image, h game.HUD) {
	r.fillRect(screen, 0, 0, ScreenWidth, HUDHeight, hudColour)
	r.fillRect(screen, 0, HUDHeight-2, ScreenWidth, 2, hudRuleColour)
	r.drawText(screen, h.Status, 8, 0)
	r.drawText(screen, h.Prompt, 8, lineHeight-2)
}

func (r *Renderer) drawOverlay(screen *ebiten.Image, lines []string) {
	r.fillRect(screen, 0, HUDHeight, ScreenWidth, ScreenHeight-HUDHeight, color.RGBA{0, 0, 0, 0xb0})
	extra := 0
	if r.build != "" {
		extra = 1
	}
	top := HUDHeight + (ScreenHeight-HUDHeight-(len(lines)+extra)*lineHeight)/2
	for i, line := range lines {
		r.drawText(screen, line, (ScreenWidth-len(line)*glyphW*textScale)/2, top+i*lineHeight)
	}
	if r.build != "" {
		ebitenutil.DebugPrintAt(screen, "build "+r.build, 8, ScreenHeight-glyphH-4)
	}
}

// drawText draws one line of debug-font text at 2x, nearest-neighbour.
func (r *Renderer) drawText(screen *ebiten.Image, s string, x, y int) {
	r.text.Clear()
	ebitenutil.DebugPrintAt(r.text, s, 0, 0)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(textScale, textScale)
	op.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(r.text, op)
}

func (r *Renderer) fillRect(screen *ebiten.Image, x, y, w, h int, c color.Color) {
	r.fillRectF(screen, float64(x), float64(y), float64(w), float64(h), c)
}

func (r *Renderer) fillRectF(screen *ebiten.Image, x, y, w, h float64, c color.Color) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(w, h)
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(c)
	screen.DrawImage(r.pixel, op)
}
