// iconpreview writes a PNG strip of tray icons in every style and theme, enlarged 4x so
// the rendering can be checked by eye: go run ./tools/iconpreview out.png
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/tray"
)

func main() {
	out := "iconpreview.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	const size, zoom, pad = 24, 4, 8
	texts := []string{"85", "100", "7", "15", "L", "?"}
	styles := []tray.Style{tray.Normal, tray.Low, tray.Charging, tray.Dim}
	themes := []struct {
		light bool
		bg    color.NRGBA
	}{{false, color.NRGBA{0x20, 0x20, 0x20, 255}}, {true, color.NRGBA{0xEE, 0xEE, 0xEE, 255}}}

	cell := size*zoom + pad
	barItems := []tray.BarItem{
		{Glyph: tray.GlyphHeadset, Text: "40%"},
		{Glyph: tray.GlyphMouse, Text: "85%", Charging: true},
		{Glyph: tray.GlyphGamepad, Text: "12%", Style: tray.Low},
		{Glyph: tray.GlyphKeyboard, Text: "Medium", Style: tray.Dim},
	}
	const barH, barZoom = 48, 2
	barW := tray.PreviewBar(barItems, barH, false).Bounds().Dx() * barZoom
	gridH := len(styles) * len(themes) * cell
	img := image.NewNRGBA(image.Rect(0, 0, max(len(texts)*cell, barW), gridH+2*(barH*barZoom+pad)))
	for ti, th := range themes {
		bar := tray.PreviewBar(barItems, barH, th.light)
		oy := gridH + ti*(barH*barZoom+pad)
		for y := range barH * barZoom {
			for x := range bar.Bounds().Dx() * barZoom {
				c := bar.NRGBAAt(x/barZoom, y/barZoom)
				a := int(c.A)
				blend := func(f, b uint8) uint8 { return uint8((int(f)*a + int(b)*(255-a)) / 255) }
				img.SetNRGBA(x, oy+y, color.NRGBA{blend(c.R, th.bg.R), blend(c.G, th.bg.G), blend(c.B, th.bg.B), 255})
			}
		}
	}
	row := 0
	for _, th := range themes {
		for _, st := range styles {
			for col, txt := range texts {
				ic := tray.Preview(txt, st, th.light, size)
				for y := range size * zoom {
					for x := range size * zoom {
						c := ic.NRGBAAt(x/zoom, y/zoom)
						a := int(c.A)
						blend := func(f, b uint8) uint8 { return uint8((int(f)*a + int(b)*(255-a)) / 255) }
						img.SetNRGBA(col*cell+x, row*cell+y, color.NRGBA{blend(c.R, th.bg.R), blend(c.G, th.bg.G), blend(c.B, th.bg.B), 255})
					}
				}
			}
			row++
		}
	}
	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}
