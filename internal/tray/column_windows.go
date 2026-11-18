package tray

import "image"

// Column layout for vertical taskbars (left / right): one device per row, stacked above the
// notification area. A row that does not fit the taskbar's width is split into the glyph on
// top and the level below it.

// renderColumn draws items into premultiplied BGRA pixels of the given width and returns
// them with the height used and each item's vertical extent (spans hold y ranges).
func renderColumn(items []BarItem, width, dpi int, light bool) ([]byte, int, []span) {
	rowH := 32 * dpi / 96
	type block struct {
		px   []byte
		w, h int
		item int
	}
	var blocks []block
	for i, it := range items {
		px, w, _ := renderBar([]BarItem{it}, rowH, dpi, light)
		if w <= width {
			blocks = append(blocks, block{px, w, rowH, i})
			continue
		}
		top := it
		top.Text, top.Charging, top.Fill = "", false, -1
		bottom := it
		bottom.Glyph, bottom.Caption = ' ', ""
		for _, part := range []BarItem{top, bottom} {
			px, w, _ := renderBar([]BarItem{part}, rowH*3/4, dpi, light)
			blocks = append(blocks, block{px, w, rowH * 3 / 4, i})
		}
	}

	gap := 4 * dpi / 96
	height := gap
	for _, b := range blocks {
		height += b.h
	}
	height += gap * len(items)

	out := make([]byte, width*height*4)
	for i := 3; i < len(out); i += 4 {
		out[i] = 1 // invisible but hit-testable
	}
	spans := make([]span, len(items))
	for i := range spans {
		spans[i].x0 = -1
	}
	y := gap
	last := -1
	for _, b := range blocks {
		if b.item != last && last >= 0 {
			y += gap
		}
		last = b.item
		if spans[b.item].x0 < 0 {
			spans[b.item].x0 = y
		}
		x0 := max(0, (width-b.w)/2)
		for row := 0; row < b.h && y+row < height; row++ {
			for col := 0; col < b.w && x0+col < width; col++ {
				src := (row*b.w + col) * 4
				if b.px[src+3] <= 1 {
					continue
				}
				dst := ((y+row)*width + x0 + col) * 4
				copy(out[dst:dst+4], b.px[src:src+4])
			}
		}
		y += b.h
		spans[b.item].x1 = y
	}
	return out, height, spans
}

// PreviewColumn renders the vertical-taskbar layout as an image (tools/iconpreview).
func PreviewColumn(items []BarItem, width int, light bool) *image.NRGBA {
	px, h, _ := renderColumn(items, width, 96, light)
	img := image.NewNRGBA(image.Rect(0, 0, width, h))
	for i := 0; i+3 < len(px); i += 4 {
		a := px[i+3]
		if a <= 1 {
			continue
		}
		un := func(v byte) byte { return byte(min(255, int(v)*255/int(a))) }
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = un(px[i+2]), un(px[i+1]), un(px[i]), a
	}
	return img
}
