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
	img := image.NewNRGBA(image.Rect(0, 0, len(texts)*cell, len(styles)*len(themes)*cell))
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
