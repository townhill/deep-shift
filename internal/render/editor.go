package render

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/townhill/deep-shift/internal/editor"
	"github.com/townhill/deep-shift/internal/sim"
	"github.com/townhill/deep-shift/internal/sprites"
)

// EditorBarHeight is the height of the editor's bottom bar: the palette and
// the problem list.
const EditorBarHeight = 64

// EditorLayout is where the editor's parts sit on the logical screen. The
// editor hit-tests the mouse against the same layout.
func EditorLayout() editor.Layout {
	return editor.Layout{Tile: TileSize, Width: ScreenWidth, Height: ScreenHeight, Top: HUDHeight, Bottom: EditorBarHeight}
}

var (
	gridColour    = color.NRGBA{255, 255, 255, 56}
	slotColour    = color.NRGBA{150, 160, 175, 120}
	problemColour = color.NRGBA{240, 70, 70, 255}
	hoverColour   = color.NRGBA{255, 255, 255, 210}
	boxColour     = color.NRGBA{60, 200, 210, 255}
	boxFillColour = color.NRGBA{60, 200, 210, 50}
	linkColour    = color.NRGBA{235, 190, 60, 170}
	selectColour  = color.NRGBA{255, 215, 80, 255}
	areaColour    = color.NRGBA{255, 255, 255, 230}
	areaFill      = color.NRGBA{255, 255, 255, 36}
	shadeColour   = color.NRGBA{0, 0, 0, 150}
	unknownColour = color.NRGBA{200, 40, 200, 255}
	panelColour   = color.NRGBA{12, 14, 20, 235}
)

// ghostAlpha is how opaque the brush preview under the mouse is.
const ghostAlpha = 0.55

// DrawEditor renders the cave editor: the status bar, the cave with its
// grid, the reachability overlay, problem outlines, mechanism links with the
// selected valve ringed, the selection, the copy being pasted or the brush
// preview, the palette bar, and the settings panel when it is open.
func (r *Renderer) DrawEditor(screen *ebiten.Image, v editor.View) {
	screen.Fill(backgroundColour)
	r.drawEditorWorld(v)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(0, float64(v.Layout.Top))
	screen.DrawImage(r.world, op)
	r.fillRect(screen, 0, 0, ScreenWidth, HUDHeight, hudColour)
	r.fillRect(screen, 0, HUDHeight-2, ScreenWidth, 2, hudRuleColour)
	r.drawText(screen, v.Status, 8, 0)
	r.drawText(screen, v.Hint, 8, lineHeight-2)
	r.drawEditorBar(screen, v)
	if v.Settings != nil {
		r.drawSettings(screen, v.Settings)
	}
}

func (r *Renderer) drawEditorWorld(v editor.View) {
	r.world.Fill(backgroundColour)
	camX := math.Round(v.CameraX * TileSize)
	camY := math.Round(v.CameraY * TileSize)
	x0, y0 := int(math.Floor(v.CameraX)), int(math.Floor(v.CameraY))
	_, viewH := v.Layout.ViewTiles()
	x1 := min(x0+ViewTilesW, v.Width-1)
	y1 := min(y0+int(math.Ceil(viewH)), v.Height-1)
	cellPos := func(x, y int) (float64, float64) { return float64(x*TileSize) - camX, float64(y*TileSize) - camY }

	for y := max(y0, 0); y <= y1; y++ {
		for x := max(x0, 0); x <= x1; x++ {
			i := y*v.Width + x
			left, top := cellPos(x, y)
			if v.Unknown != nil && v.Unknown[i] {
				r.fillRectF(r.world, left, top, TileSize, TileSize, unknownColour)
				ebitenutil.DebugPrintAt(r.world, "?", int(left)+13, int(top)+8)
				continue
			}
			surface := y == 0 || v.Cells[i-v.Width].Water == 0
			r.drawCell(r.world, v.Cells[i], left, top, 1, surface)
		}
	}
	if v.Reach != nil {
		// The overlay shades the cells the player cannot reach. Walls are
		// left alone: they are never reachable, and shading them would hide
		// the unreachable floor and earth that matter.
		for y := max(y0, 0); y <= y1; y++ {
			for x := max(x0, 0); x <= x1; x++ {
				if i := y*v.Width + x; !v.Reach[i] && v.Cells[i].Terrain != sim.TerrainWall {
					left, top := cellPos(x, y)
					r.fillRectF(r.world, left, top, TileSize, TileSize, shadeColour)
				}
			}
		}
	}
	if v.Grid {
		l, t := cellPos(0, 0)
		w, h := float64(v.Width*TileSize), float64(v.Height*TileSize)
		for x := max(x0, 0); x <= min(x1+1, v.Width); x++ {
			r.fillRectF(r.world, l+float64(x*TileSize), t, 1, h, gridColour)
		}
		for y := max(y0, 0); y <= min(y1+1, v.Height); y++ {
			r.fillRectF(r.world, l, t+float64(y*TileSize), w, 1, gridColour)
		}
	}
	centre := func(p sim.Pos) (float32, float32) {
		left, top := cellPos(p.X, p.Y)
		return float32(left + TileSize/2), float32(top + TileSize/2)
	}
	for _, m := range v.Mechanisms {
		width, c := float32(2), color.Color(linkColour)
		if v.LinkingOK && m.Valve == v.Linking {
			width, c = 3, selectColour
		}
		vx, vy := centre(m.Valve)
		for _, g := range m.Gates {
			gx, gy := centre(g)
			vector.StrokeLine(r.world, vx, vy, gx, gy, width, c, true)
		}
	}
	if v.LinkingOK {
		left, top := cellPos(v.Linking.X, v.Linking.Y)
		vector.StrokeRect(r.world, float32(left)-2, float32(top)-2, TileSize+4, TileSize+4, 3, selectColour, false)
	}
	for _, p := range v.Problems {
		left, top := cellPos(p.X, p.Y)
		vector.StrokeRect(r.world, float32(left)+1, float32(top)+1, TileSize-2, TileSize-2, 2, problemColour, false)
	}
	if v.BoxOK {
		left, top := cellPos(v.Box.X0, v.Box.Y0)
		w, h := float64((v.Box.X1-v.Box.X0+1)*TileSize), float64((v.Box.Y1-v.Box.Y0+1)*TileSize)
		r.fillRectF(r.world, left, top, w, h, boxFillColour)
		vector.StrokeRect(r.world, float32(left), float32(top), float32(w), float32(h), 2, boxColour, false)
	}
	if v.SelectionOK {
		left, top := cellPos(v.Selection.X0, v.Selection.Y0)
		w := float64((v.Selection.X1 - v.Selection.X0 + 1) * TileSize)
		h := float64((v.Selection.Y1 - v.Selection.Y0 + 1) * TileSize)
		r.fillRectF(r.world, left, top, w, h, areaFill)
		vector.StrokeRect(r.world, float32(left), float32(top), float32(w), float32(h), 2, areaColour, false)
	}
	if v.Paste != nil && v.HoverOK {
		// The copy, faint, with its top-left corner on the cell under the
		// mouse; cells beyond the cave are not drawn.
		for i, c := range v.Paste {
			x, y := v.Hover.X+i%v.PasteW, v.Hover.Y+i/v.PasteW
			if x < v.Width && y < v.Height {
				left, top := cellPos(x, y)
				r.drawCell(r.world, c, left, top, ghostAlpha, true)
			}
		}
		left, top := cellPos(v.Hover.X, v.Hover.Y)
		vector.StrokeRect(r.world, float32(left), float32(top), float32(v.PasteW*TileSize), float32(len(v.Paste)/v.PasteW*TileSize), 1, areaColour, false)
	}
	if v.HoverOK {
		left, top := cellPos(v.Hover.X, v.Hover.Y)
		if v.GhostOK {
			r.drawCell(r.world, v.Ghost, left, top, ghostAlpha, true)
		}
		vector.StrokeRect(r.world, float32(left), float32(top), TileSize, TileSize, 1, hoverColour, false)
	}
}

// drawCell draws one cell as the cave starts: terrain, then any occupant,
// then water, at the given opacity (water is always translucent).
func (r *Renderer) drawCell(dst *ebiten.Image, c sim.Cell, left, top float64, alpha float32, surface bool) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(left, top)
	op.ColorScale.ScaleAlpha(alpha)
	dst.DrawImage(r.tiles[terrainSprite(c.Terrain, false)], op)
	if c.Occupant == sim.OccupantPlayer {
		dst.DrawImage(r.tiles[sprites.Player], op)
	} else if k, ok := occupantSprite(c.Occupant, c.Facing); ok {
		dst.DrawImage(r.tiles[k], op)
	}
	if c.Water > 0 {
		r.drawWaterCell(dst, left, top, float64(c.Water), surface, c.Deep())
	}
}

func (r *Renderer) drawEditorBar(screen *ebiten.Image, v editor.View) {
	l := v.Layout
	barTop := l.Height - l.Bottom
	r.fillRect(screen, 0, barTop, ScreenWidth, l.Bottom, hudColour)
	r.fillRect(screen, 0, barTop, ScreenWidth, 2, hudRuleColour)
	for i, s := range v.Palette {
		x, y := l.Slot(i)
		r.drawCell(screen, s.Cell, float64(x), float64(y), 1, true)
		// A frame on every slot, so dark tiles such as empty floor still
		// read as a brush.
		vector.StrokeRect(screen, float32(x), float32(y), TileSize, TileSize, 1, slotColour, false)
		if s.Selected {
			vector.StrokeRect(screen, float32(x)-3, float32(y)-3, TileSize+6, TileSize+6, 2, hudRuleColour, false)
		}
		ebitenutil.DebugPrintAt(screen, s.Key, x+TileSize/2-glyphW/2, y+TileSize)
	}
	tx := l.TextX()
	maxChars := (ScreenWidth - tx - 8) / glyphW
	for i, line := range v.Info {
		ebitenutil.DebugPrintAt(screen, clip(line, maxChars), tx, barTop+6+i*glyphH)
	}
}

// drawSettings draws the cave settings panel over the middle of the cave.
func (r *Renderer) drawSettings(screen *ebiten.Image, lines []editor.Line) {
	const w = 720
	h := len(lines)*lineHeight + 32
	x, y := (ScreenWidth-w)/2, HUDHeight+(ScreenHeight-HUDHeight-EditorBarHeight-h)/2
	r.fillRect(screen, x, y, w, h, panelColour)
	vector.StrokeRect(screen, float32(x), float32(y), w, float32(h), 2, hudRuleColour, false)
	for i, line := range lines {
		ly := y + 16 + i*lineHeight
		if line.Selected {
			r.fillRect(screen, x+8, ly-2, w-16, lineHeight, menuSelectColour)
		}
		r.drawText(screen, clip(line.Text, (w-32)/(glyphW*textScale)), x+16, ly)
	}
}

// clip shortens s to at most n characters, marking the cut.
func clip(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	if n <= 3 {
		return string(rs[:max(n, 0)])
	}
	return string(rs[:n-3]) + "..."
}
