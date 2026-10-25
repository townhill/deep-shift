package sprites

import (
	"bytes"
	"testing"
)

func TestSpritesAreDeterministicAndSized(t *testing.T) {
	for k := Kind(0); k < KindCount; k++ {
		a, b := Generate(k), Generate(k)
		if a.Bounds().Dx() != Size || a.Bounds().Dy() != Size {
			t.Errorf("%s is %v, want %dx%d", k, a.Bounds(), Size, Size)
		}
		if !bytes.Equal(a.Pix, b.Pix) {
			t.Errorf("%s differs between generations", k)
		}
	}
}

func TestTerrainIsOpaqueAndOverlaysAreCutOut(t *testing.T) {
	for _, k := range []Kind{Empty, Earth, Wall, ExitSealed, ExitOpen, GateClosed, GateOpen, ValveShut, ValveOpen} {
		img := Generate(k)
		for i := 3; i < len(img.Pix); i += 4 {
			if img.Pix[i] != 0xff {
				t.Fatalf("%s has a transparent pixel; terrain must be opaque", k)
			}
		}
	}
	for _, k := range []Kind{Crystal, Player, Boulder, CrawlerUp, CrawlerDown, CrawlerLeft, CrawlerRight,
		GlimmerUp, GlimmerDown, GlimmerLeft, GlimmerRight} {
		img := Generate(k)
		if img.RGBAAt(0, 0).A != 0 || img.RGBAAt(Size-1, 0).A != 0 {
			t.Errorf("%s corners are not transparent", k)
		}
		if img.RGBAAt(Size/2, Size/2).A != 0xff {
			t.Errorf("%s centre is not opaque", k)
		}
	}
	// Waves go over water, which is itself translucent: never opaque.
	waves := Generate(Waves)
	drawn := 0
	for i := 3; i < len(waves.Pix); i += 4 {
		if waves.Pix[i] == 0xff {
			t.Fatal("waves has an opaque pixel")
		}
		if waves.Pix[i] > 0 {
			drawn++
		}
	}
	if drawn < Size*3 {
		t.Fatalf("waves draws only %d pixels", drawn)
	}
}

// Sprites must differ in silhouette, not only colour (SPEC §4). Compare
// light/dark masks: a hue swap alone would leave them identical.
func TestSpritesDifferInShape(t *testing.T) {
	mask := func(k Kind) []bool {
		img := Generate(k)
		var m []bool
		for i := 0; i < len(img.Pix); i += 4 {
			p := img.Pix[i : i+4]
			lum := (int(p[0])*3 + int(p[1])*6 + int(p[2])) / 10
			m = append(m, p[3] > 0 && lum > 100)
		}
		return m
	}
	differ := func(a, b []bool) int {
		n := 0
		for i := range a {
			if a[i] != b[i] {
				n++
			}
		}
		return n
	}
	kinds := []Kind{Empty, Earth, Wall, ExitSealed, ExitOpen, Crystal, Player, Boulder, GateClosed, GateOpen, ValveShut, ValveOpen,
		CrawlerUp, CrawlerDown, CrawlerLeft, CrawlerRight, GlimmerUp, GlimmerDown, GlimmerLeft, GlimmerRight}
	for i, a := range kinds {
		for _, b := range kinds[i+1:] {
			if n := differ(mask(a), mask(b)); n < 40 {
				t.Errorf("%s and %s differ in only %d mask pixels", a, b, n)
			}
		}
	}
}

// Each creature's kinds are one drawing turned to face each way: its head
// (the topmost rows facing up) ends up on the side it faces.
func TestCreaturesAreOneDrawingTurned(t *testing.T) {
	for _, first := range []Kind{CrawlerUp, GlimmerUp} {
		up := Generate(first)
		const m = Size - 1
		turned := map[Kind]func(x, y int) (int, int){
			first + 1: func(x, y int) (int, int) { return m - x, m - y }, // down
			first + 2: func(x, y int) (int, int) { return y, m - x },     // left
			first + 3: func(x, y int) (int, int) { return m - y, x },     // right
		}
		for k, to := range turned {
			img := Generate(k)
			for y := range Size {
				for x := range Size {
					if tx, ty := to(x, y); img.RGBAAt(tx, ty) != up.RGBAAt(x, y) {
						t.Fatalf("%s at (%d,%d) is not %s's (%d,%d) turned", k, tx, ty, first, x, y)
					}
				}
			}
		}
		// Facing up, the head is at the top: the top third has the eyes.
		eyes := 0
		for y := range Size / 3 {
			for x := range Size {
				if c := up.RGBAAt(x, y); c.R > 240 && c.G > 220 {
					eyes++
				}
			}
		}
		if eyes == 0 {
			t.Fatalf("%s has no eyes in its top third", first)
		}
	}
}
