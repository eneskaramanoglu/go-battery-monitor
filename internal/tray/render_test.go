package tray

import "testing"

func TestRenderedTextIsVisibleAndCentred(t *testing.T) {
	for _, size := range []int{16, 24, 32} {
		for _, text := range []string{"7", "85", "100", "?"} {
			img := Preview(text, Normal, false, size)
			minX, maxX, minY, maxY, opaque := size, -1, size, -1, 0
			for y := range size {
				for x := range size {
					if img.NRGBAAt(x, y).A > 128 {
						opaque++
						minX, maxX = min(minX, x), max(maxX, x)
						minY, maxY = min(minY, y), max(maxY, y)
					}
				}
			}
			if opaque < size/2 {
				t.Errorf("size %d %q: only %d opaque pixels", size, text, opaque)
				continue
			}
			// Text must not be clipped at the edges by more than a pixel and roughly centred.
			if cx := (minX + maxX) / 2; cx < size/2-2 || cx > size/2+1 {
				t.Errorf("size %d %q: horizontal centre %d", size, text, cx)
			}
			if maxY-minY < size/3 {
				t.Errorf("size %d %q: text only %d px tall", size, text, maxY-minY+1)
			}
		}
	}
}

func TestPaletteColours(t *testing.T) {
	img := Preview("15", Low, false, 24)
	var r, g int
	for y := range 24 {
		for x := range 24 {
			c := img.NRGBAAt(x, y)
			if c.A > 200 {
				r += int(c.R)
				g += int(c.G)
			}
		}
	}
	if r <= g {
		t.Errorf("low battery icon is not red (r=%d g=%d)", r, g)
	}
}
