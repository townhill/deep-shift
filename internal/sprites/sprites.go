// Package sprites generates Deep Shift's original placeholder tiles in code.
//
// Every tile is drawn procedurally into an *image.RGBA from simple shapes and
// a deterministic integer hash, so there are no binary art files to license
// and the art is reproducible. Objects differ by silhouette as well as
// colour (SPEC §4 accessibility): blocky masonry for walls, speckled soil
// for earth, a faceted rhombus for crystals, a helmeted figure for the
// player, and a barred door versus an open, chevron-marked doorway for the
// exit states. Pure Go (no Ebitengine), so it is unit-tested headlessly.
package sprites

import (
	"image"
	"image/color"
)

// Size is the edge length of every tile, in pixels.
const Size = 32

// Kind identifies a placeholder sprite.
type Kind int

// Sprite kinds. Terrain kinds are opaque; Crystal and Player are drawn over
// terrain and have transparent backgrounds.
const (
	Empty Kind = iota
	Earth
	Wall
	ExitSealed
	ExitOpen
	Crystal
	Player
	KindCount
)

func (k Kind) String() string {
	return [...]string{"empty", "earth", "wall", "exit-sealed", "exit-open", "crystal", "player"}[k]
}

// Generate draws sprite k.
func Generate(k Kind) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, Size, Size))
	switch k {
	case Empty:
		drawEmpty(img)
	case Earth:
		drawEarth(img)
	case Wall:
		drawWall(img)
	case ExitSealed:
		drawExit(img, false)
	case ExitOpen:
		drawExit(img, true)
	case Crystal:
		drawCrystal(img)
	case Player:
		drawPlayer(img)
	}
	return img
}

// noise is a deterministic per-pixel hash in [0, 256).
func noise(x, y, salt int) int {
	h := uint32(x)*0x8da6b343 ^ uint32(y)*0xd8163841 ^ uint32(salt)*0xcb1ab31f
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	return int(h & 0xff)
}

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 0xff} }

func shade(c color.RGBA, delta int) color.RGBA {
	adj := func(v uint8) uint8 { return uint8(min(max(int(v)+delta, 0), 255)) }
	return color.RGBA{adj(c.R), adj(c.G), adj(c.B), c.A}
}

func fill(img *image.RGBA, c color.RGBA) {
	for y := range Size {
		for x := range Size {
			img.SetRGBA(x, y, c)
		}
	}
}

var (
	floorColour  = rgb(22, 19, 28)
	earthColour  = rgb(112, 76, 48)
	stoneColour  = rgb(74, 82, 98)
	mortarColour = rgb(40, 44, 54)
)

func drawEmpty(img *image.RGBA) {
	fill(img, floorColour)
	for y := range Size {
		for x := range Size {
			if noise(x, y, 1) > 250 {
				img.SetRGBA(x, y, shade(floorColour, 14))
			}
		}
	}
}

func drawEarth(img *image.RGBA) {
	for y := range Size {
		for x := range Size {
			n := noise(x, y, 2)
			c := shade(earthColour, (n-128)/10)
			switch {
			case n > 244:
				c = shade(earthColour, 38) // light grit
			case n < 12:
				c = shade(earthColour, -40) // dark pores
			}
			img.SetRGBA(x, y, c)
		}
	}
	// A few pebbles give the soil a lumpy silhouette.
	for _, p := range [][3]int{{8, 9, 3}, {22, 6, 2}, {17, 21, 3}, {5, 25, 2}, {27, 24, 2}} {
		disc(img, p[0], p[1], p[2], shade(earthColour, -22))
		img.SetRGBA(p[0]-1, p[1]-1, shade(earthColour, 30))
	}
}

func drawWall(img *image.RGBA) {
	fill(img, mortarColour)
	const course = 8 // brick height
	for y := range Size {
		row := y / course
		offset := 0
		if row%2 == 1 {
			offset = 8
		}
		for x := range Size {
			bx := (x + offset) % 16
			by := y % course
			if bx == 0 || by == 0 {
				continue // mortar line
			}
			c := shade(stoneColour, (noise(x/2, y/2, 3+row)-128)/12)
			switch {
			case by == 1 || bx == 1:
				c = shade(c, 22) // top/left bevel highlight
			case by == course-1 || bx == 15:
				c = shade(c, -20) // bottom/right bevel shadow
			}
			img.SetRGBA(x, y, c)
		}
	}
}

func drawExit(img *image.RGBA, open bool) {
	drawWall(img)
	// Doorway: an arch-topped opening in the masonry.
	inside := func(x, y int) bool {
		if x < 7 || x > 24 || y < 6 || y > 30 {
			return false
		}
		if y < 14 { // arch
			dx, dy := float64(x)-15.5, float64(y)-14
			return dx*dx+dy*dy <= 8.5*8.5
		}
		return true
	}
	for y := range Size {
		for x := range Size {
			if !inside(x, y) {
				continue
			}
			if open {
				// Warm light, brighter toward the bottom centre.
				dx := x - 15
				if dx < 0 {
					dx = -dx
				}
				glow := 190 + y*2 - dx*4
				img.SetRGBA(x, y, color.RGBA{uint8(min(255, glow+40)), uint8(min(255, glow+10)), uint8(max(0, glow-90)), 0xff})
			} else {
				c := rgb(38, 30, 26)
				if y%5 == 0 || x == 7 || x == 24 {
					c = rgb(120, 110, 96) // iron bars and frame
				}
				img.SetRGBA(x, y, c)
			}
		}
	}
	if open {
		// Upward chevron: "go through here".
		for i := range 6 {
			for t := range 2 {
				img.SetRGBA(15-i, 12+i+t, rgb(255, 255, 255))
				img.SetRGBA(16+i, 12+i+t, rgb(255, 255, 255))
			}
		}
	} else {
		// Padlock silhouette on the bars.
		for y := 19; y <= 25; y++ {
			for x := 12; x <= 19; x++ {
				img.SetRGBA(x, y, rgb(200, 170, 70))
			}
		}
		ring(img, 15.5, 18, 3.2, 1.2, rgb(200, 170, 70))
		img.SetRGBA(15, 22, rgb(60, 45, 20))
		img.SetRGBA(16, 22, rgb(60, 45, 20))
		img.SetRGBA(15, 23, rgb(60, 45, 20))
		img.SetRGBA(16, 23, rgb(60, 45, 20))
	}
}

func drawCrystal(img *image.RGBA) {
	const cx, cy, rx, ry = 15.5, 15.5, 11.0, 14.0
	light, mid, dark := rgb(170, 250, 245), rgb(60, 200, 210), rgb(20, 110, 140)
	for y := range Size {
		for x := range Size {
			fx, fy := float64(x)-cx, float64(y)-cy
			d := abs(fx)/rx + abs(fy)/ry
			if d > 1 {
				continue
			}
			c := mid
			switch {
			case d > 0.86:
				c = dark // outline
			case fx < 0 && fy < 0:
				c = light // lit facet
			case fx >= 0 && fy >= 0:
				c = dark
			}
			if abs(fx) < 0.6 || abs(fy) < 0.6 {
				c = shade(c, 25) // facet ridges
			}
			img.SetRGBA(x, y, c)
		}
	}
	img.SetRGBA(10, 9, rgb(255, 255, 255)) // sparkle
	img.SetRGBA(11, 10, rgb(255, 255, 255))
	img.SetRGBA(10, 10, rgb(255, 255, 255))
}

func drawPlayer(img *image.RGBA) {
	suit, suitDark := rgb(46, 120, 112), rgb(28, 78, 74)
	helmet, skin := rgb(236, 168, 44), rgb(226, 190, 150)
	// Boots and legs.
	rect(img, 10, 26, 14, 30, suitDark)
	rect(img, 17, 26, 21, 30, suitDark)
	// Body.
	rect(img, 9, 15, 22, 26, suit)
	rect(img, 9, 15, 10, 26, shade(suit, 25))
	// Arms.
	rect(img, 6, 16, 8, 23, suitDark)
	rect(img, 23, 16, 25, 23, suitDark)
	// Head and helmet with a headlamp.
	disc(img, 15, 11, 5, skin)
	for y := 3; y <= 10; y++ {
		for x := 8; x <= 23; x++ {
			dx, dy := float64(x)-15.5, float64(y)-10
			if dx*dx/(7.5*7.5)+dy*dy/(6.5*6.5) <= 1 && y <= 9 {
				img.SetRGBA(x, y, helmet)
			}
		}
	}
	rect(img, 7, 9, 24, 10, shade(helmet, -40)) // brim
	disc(img, 15, 6, 2, rgb(255, 250, 220))     // lamp
	img.SetRGBA(13, 12, rgb(30, 30, 30))        // eyes
	img.SetRGBA(17, 12, rgb(30, 30, 30))
}

func rect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func disc(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func ring(img *image.RGBA, cx, cy, r, width float64, c color.RGBA) {
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			d := dx*dx + dy*dy
			if d <= r*r && d >= (r-width)*(r-width) && float64(y) <= cy {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
