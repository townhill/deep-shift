// Package sprites generates Deep Shift's original placeholder tiles in code.
//
// Every tile is drawn procedurally into an *image.RGBA from simple shapes and
// a deterministic integer hash, so there are no binary art files to license
// and the art is reproducible. Objects differ by silhouette as well as
// colour (SPEC §4 accessibility): blocky masonry for walls, speckled soil
// for earth, a faceted rhombus for crystals, a helmeted figure for the
// player, a barred door versus an open, chevron-marked doorway for the exit
// states, a round shaded rock for boulders, a riveted slab versus a raised
// one for sluice gates, a wheel whose spokes turn 45 degrees for valves,
// wave lines that mark deep water, a many-legged crawler whose head and
// feelers point the way it faces, and a broad-winged glimmer with a crystal
// shard on its back whose head and antennae do the same. Pure Go (no
// Ebitengine), so it is unit-tested headlessly.
package sprites

import (
	"image"
	"image/color"
	"math"
)

// Size is the edge length of every tile, in pixels.
const Size = 32

// Kind identifies a placeholder sprite.
type Kind int

// Sprite kinds. Terrain kinds are opaque; Crystal, Player, Boulder and the
// creatures are drawn over terrain and have transparent backgrounds; Waves
// is a translucent pattern drawn over deep water. The four crawler kinds are
// one drawing rotated to face up, down, left and right, and so are the four
// glimmer kinds.
const (
	Empty Kind = iota
	Earth
	Wall
	ExitSealed
	ExitOpen
	Crystal
	Player
	Boulder
	GateClosed
	GateOpen
	ValveShut
	ValveOpen
	Waves
	CrawlerUp
	CrawlerDown
	CrawlerLeft
	CrawlerRight
	GlimmerUp
	GlimmerDown
	GlimmerLeft
	GlimmerRight
	KindCount
)

func (k Kind) String() string {
	return [...]string{"empty", "earth", "wall", "exit-sealed", "exit-open", "crystal", "player", "boulder",
		"gate-closed", "gate-open", "valve-shut", "valve-open", "waves",
		"crawler-up", "crawler-down", "crawler-left", "crawler-right",
		"glimmer-up", "glimmer-down", "glimmer-left", "glimmer-right"}[k]
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
	case Boulder:
		drawBoulder(img)
	case GateClosed:
		drawGate(img, false)
	case GateOpen:
		drawGate(img, true)
	case ValveShut:
		drawValve(img, false)
	case ValveOpen:
		drawValve(img, true)
	case Waves:
		drawWaves(img)
	case CrawlerUp, CrawlerDown, CrawlerLeft, CrawlerRight:
		up := image.NewRGBA(img.Bounds())
		drawCrawler(up)
		rotate(img, up, int(k-CrawlerUp))
	case GlimmerUp, GlimmerDown, GlimmerLeft, GlimmerRight:
		up := image.NewRGBA(img.Bounds())
		drawGlimmer(up)
		rotate(img, up, int(k-GlimmerUp))
	}
	return img
}

// rotate copies src into dst turned for facing f: 0 as drawn (up), 1 a half
// turn (down), 2 a quarter turn anticlockwise (left), 3 a quarter turn
// clockwise (right).
func rotate(dst, src *image.RGBA, f int) {
	const m = Size - 1
	for y := range Size {
		for x := range Size {
			c := src.RGBAAt(x, y)
			switch f {
			case 0:
				dst.SetRGBA(x, y, c)
			case 1:
				dst.SetRGBA(m-x, m-y, c)
			case 2:
				dst.SetRGBA(y, m-x, c)
			case 3:
				dst.SetRGBA(m-y, x, c)
			}
		}
	}
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

func drawBoulder(img *image.RGBA) {
	const cx, cy, r = 15.5, 16.0, 13.5
	base := rgb(132, 116, 100)
	for y := range Size {
		for x := range Size {
			dx, dy := float64(x)-cx, float64(y)-cy
			d2 := dx*dx + dy*dy
			if d2 > r*r {
				continue
			}
			// Light from the upper left: shade by the dot product with it.
			light := (-dx - dy) / (r * 1.41)
			c := shade(base, int(light*46)+(noise(x/2, y/2, 9)-128)/16)
			if d2 > (r-1.6)*(r-1.6) {
				c = rgb(52, 44, 38) // outline
			}
			img.SetRGBA(x, y, c)
		}
	}
	// A crack and a highlight make the rock read as stone, not a ball.
	for i, p := range [][2]int{{18, 9}, {19, 10}, {19, 11}, {20, 12}, {21, 13}, {21, 14}, {20, 15}} {
		img.SetRGBA(p[0], p[1], shade(base, -58+i))
	}
	disc(img, 10, 10, 2, shade(base, 60))
}

var (
	steelColour = rgb(92, 104, 118)
	brassColour = rgb(206, 172, 74)
)

// drawGate draws a sluice gate in a masonry frame: closed, a riveted steel
// slab fills the opening down to a hazard-striped edge; open, the slab is
// raised into the top of the frame and the floor shows through.
func drawGate(img *image.RGBA, open bool) {
	drawEmpty(img)
	rect(img, 0, 0, 3, Size-1, stoneColour) // frame posts
	rect(img, Size-4, 0, Size-1, Size-1, stoneColour)
	rect(img, 3, 0, 3, Size-1, shade(stoneColour, -24))
	rect(img, Size-4, 0, Size-4, Size-1, shade(stoneColour, -24))
	bottom := Size - 1
	if open {
		bottom = 7
	}
	rect(img, 4, 0, Size-5, bottom, steelColour)
	for y := 8; y < bottom-4; y += 9 {
		rect(img, 4, y, Size-5, y, shade(steelColour, -30)) // plate seams
		img.SetRGBA(7, y+3, shade(steelColour, 60))         // rivets
		img.SetRGBA(Size-8, y+3, shade(steelColour, 60))
	}
	for y := bottom - 3; y <= bottom; y++ {
		for x := 4; x <= Size-5; x++ {
			c := rgb(232, 190, 40)
			if (x+y)/3%2 == 0 {
				c = rgb(30, 28, 26)
			}
			img.SetRGBA(x, y, c) // hazard stripes on the moving edge
		}
	}
	if open {
		// Downward chevron under the raised slab: the way is clear.
		for i := range 5 {
			img.SetRGBA(15-i, 22-i, rgb(220, 230, 240))
			img.SetRGBA(16+i, 22-i, rgb(220, 230, 240))
		}
	}
}

// drawValve draws a handwheel on the wall. Shut, its spokes form a '+' and
// the hub is red; open, they form an 'x' and the hub is green.
func drawValve(img *image.RGBA, open bool) {
	drawWall(img)
	const cx, cy = 15.5, 15.5
	angle := 0.0
	hub := rgb(200, 50, 40)
	if open {
		angle = math.Pi / 4
		hub = rgb(60, 190, 80)
	}
	for y := range Size {
		for x := range Size {
			dx, dy := float64(x)-cx, float64(y)-cy
			d := math.Hypot(dx, dy)
			switch {
			case d <= 13.5 && d >= 10.5:
				img.SetRGBA(x, y, shade(brassColour, int((-dx-dy)*2))) // rim
			case d < 10.5:
				img.SetRGBA(x, y, mortarColour) // backplate
				for k := range 2 {
					a := angle + float64(k)*math.Pi/2
					// Distance from the spoke line through the centre at angle a.
					if math.Abs(dx*math.Sin(a)-dy*math.Cos(a)) < 1.5 {
						img.SetRGBA(x, y, brassColour)
					}
				}
			}
		}
	}
	disc(img, 15, 15, 3, hub)
	img.SetRGBA(14, 14, shade(hub, 70))
}

var (
	shellColour = rgb(170, 70, 152)
	legColour   = rgb(146, 62, 132)
)

// drawCrawler draws the crawler facing up, seen from above: a head with
// glowing eyes and two feelers, a thorax and a segmented abdomen, and three
// pairs of jointed legs, so its outline reads as a creature at a glance.
func drawCrawler(img *image.RGBA) {
	outline := shade(shellColour, -96)
	// Legs first, so the body covers their roots: three pairs, each a thigh
	// out to the side and a shin bent towards the tail.
	for i, y := range []int{13, 18, 23} {
		for _, side := range []int{-1, 1} {
			kx := 15 + side*(9+i%2) // knee
			for t := range 9 {
				x := 15 + side*t
				img.SetRGBA(x+min(side, 0)+1, y-t/3, legColour)
				img.SetRGBA(x+min(side, 0)+1, y-t/3+1, legColour)
			}
			for t := range 5 {
				img.SetRGBA(kx+side*(t/2)+min(side, 0)+1, y-2+t, shade(legColour, -36))
			}
		}
	}
	// Feelers: two curving lines ahead of the head.
	for t := range 6 {
		img.SetRGBA(13-t/2, 4-t/2+1, legColour)
		img.SetRGBA(18+t/2, 4-t/2+1, legColour)
	}
	body := func(cx, cy, rx, ry float64) {
		for y := range Size {
			for x := range Size {
				dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
				d := dx*dx + dy*dy
				if d > 1 {
					continue
				}
				c := shade(shellColour, int((-dx-dy)*26))
				if d > 0.72 {
					c = outline
				}
				img.SetRGBA(x, y, c)
			}
		}
	}
	body(15.5, 23, 7.5, 7)         // abdomen
	body(15.5, 15, 6, 4.5)         // thorax
	body(15.5, 8.5, 4.5, 4)        // head
	for y := 19; y <= 28; y += 3 { // abdomen segments
		rect(img, 11, y, 20, y, shade(shellColour, -50))
	}
	rect(img, 15, 17, 16, 29, shade(shellColour, -34)) // wing-case seam
	eye := rgb(250, 238, 120)
	rect(img, 12, 7, 13, 8, eye)
	rect(img, 18, 7, 19, 8, eye)
}

var (
	wingColour  = rgb(226, 160, 58)
	glimmerBody = rgb(92, 60, 40)
	shardColour = rgb(120, 230, 240)
)

// drawGlimmer draws the glimmer facing up, seen from above: four broad
// wings with a dark band along their edges, a slender body, a head with
// glowing eyes and two knobbed antennae, and a crystal shard on its back,
// the crystals it holds. Its outline (wide wings, no legs) reads as a
// different creature from the crawler.
func drawGlimmer(img *image.RGBA) {
	outline := shade(wingColour, -110)
	wing := func(cx, cy, rx, ry float64) {
		for y := range Size {
			for x := range Size {
				dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
				d := dx*dx + dy*dy
				switch {
				case d > 1:
				case d > 0.78:
					img.SetRGBA(x, y, outline)
				case d > 0.5:
					img.SetRGBA(x, y, shade(wingColour, -46)) // band
				default:
					img.SetRGBA(x, y, shade(wingColour, int((-dx-dy)*24)))
				}
			}
		}
	}
	wing(8.5, 12, 7, 6)    // forewings
	wing(22.5, 12, 7, 6)   //
	wing(10, 22.5, 6, 5.5) // hindwings
	wing(21, 22.5, 6, 5.5) //
	// Antennae: thin lines from the head to a knob on each side.
	for t := range 5 {
		img.SetRGBA(14-t, 5-t, glimmerBody)
		img.SetRGBA(17+t, 5-t, glimmerBody)
	}
	disc(img, 9, 1, 1, glimmerBody)
	disc(img, 22, 1, 1, glimmerBody)
	// Body and head.
	for y := range Size {
		for x := range Size {
			dx, dy := (float64(x)-15.5)/3, (float64(y)-18)/10
			if dx*dx+dy*dy <= 1 {
				img.SetRGBA(x, y, shade(glimmerBody, int(-dx*30)))
			}
		}
	}
	disc(img, 15, 7, 3, glimmerBody)
	disc(img, 16, 7, 3, glimmerBody)
	eye := rgb(250, 238, 120)
	img.SetRGBA(14, 6, eye)
	img.SetRGBA(17, 6, eye)
	// The shard: a small rhombus on the back, lit on its upper left facet.
	for y := 12; y <= 22; y++ {
		half := 3 - abs(float64(y)-17)*3/5
		for x := 13; x <= 18; x++ {
			if dx := abs(float64(x) - 15.5); dx <= half {
				c := shardColour
				if float64(x) > 15.5 || y > 17 {
					c = shade(shardColour, -60)
				}
				img.SetRGBA(x, y, c)
			}
		}
	}
}

// drawWaves draws translucent wave lines, the shape cue for deep water.
func drawWaves(img *image.RGBA) {
	for _, base := range []int{6, 16, 26} {
		for x := range Size {
			off := x % 8
			if off > 4 {
				off = 8 - off
			}
			y := base + off - 2
			img.SetRGBA(x, y, color.RGBA{150, 200, 235, 150})
			img.SetRGBA(x, y+1, color.RGBA{60, 110, 170, 110})
		}
	}
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
