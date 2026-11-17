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

func TestTaskbarRendering(t *testing.T) {
	items := []BarItem{
		{Glyph: GlyphHeadset, Text: "40%"},
		{Glyph: GlyphMouse, Text: "85%", Charging: true},
	}
	one := PreviewBar(items[:1], 48, false)
	two := PreviewBar(items, 48, false)
	if one.Bounds().Dx() < 30 || two.Bounds().Dx() <= one.Bounds().Dx() {
		t.Fatalf("widths: one item %d px, two items %d px", one.Bounds().Dx(), two.Bounds().Dx())
	}
	var opaque, green int
	for y := range 48 {
		for x := range two.Bounds().Dx() {
			c := two.NRGBAAt(x, y)
			if c.A > 128 {
				opaque++
				if c.G > 150 && c.R < 120 {
					green++ // charging bolt
				}
			}
		}
	}
	if opaque < 100 || green == 0 {
		t.Fatalf("opaque=%d green(bolt)=%d", opaque, green)
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
