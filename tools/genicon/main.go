// genicon draws the application icon (a battery with a charge bolt) into winres/icon.png.
// Run it with: go run ./tools/genicon
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

const size = 256

func main() {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	body := rgba(0x2E, 0x3A, 0x46, 255)
	fill := rgba(0x3D, 0xD6, 0x8C, 255)
	bolt := rgba(0xFF, 0xFF, 0xFF, 255)

	// Supersample 4x4 per pixel for smooth edges.
	const ss = 4
	for y := range size {
		for x := range size {
			var acc [4]float64
			for sy := range ss {
				for sx := range ss {
					px := (float64(x) + (float64(sx)+0.5)/ss) / size
					py := (float64(y) + (float64(sy)+0.5)/ss) / size
					c := shade(px, py, body, fill, bolt)
					a := float64(c.A) / 255
					acc[0] += float64(c.R) * a
					acc[1] += float64(c.G) * a
					acc[2] += float64(c.B) * a
					acc[3] += a
				}
			}
			n := float64(ss * ss)
			if acc[3] == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(acc[0] / acc[3]), G: uint8(acc[1] / acc[3]), B: uint8(acc[2] / acc[3]),
				A: uint8(math.Round(acc[3] / n * 255)),
			})
		}
	}

	if err := os.MkdirAll("winres", 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create("winres/icon.png")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}

func rgba(r, g, b, a uint8) color.NRGBA { return color.NRGBA{r, g, b, a} }

var transparent = color.NRGBA{}

// shade returns the colour at (x, y) in 0..1 icon space.
func shade(x, y float64, body, fill, bolt color.NRGBA) color.NRGBA {
	// Upright battery: body 0.22..0.78 wide, 0.16..0.94 tall, terminal cap on top.
	const l, r, t, b, rad = 0.22, 0.78, 0.16, 0.94, 0.09
	cap := x > 0.39 && x < 0.61 && y > 0.06 && y < 0.18 && roundRect(x, y, 0.39, 0.61, 0.06, 0.20, 0.035)
	if !roundRect(x, y, l, r, t, b, rad) && !cap {
		return transparent
	}
	if cap {
		return body
	}
	// Inner cavity with a 0.05 wall; the lower 70% is the charge.
	const w = 0.055
	inside := roundRect(x, y, l+w, r-w, t+w, b-w, rad-w)
	if !inside {
		return body
	}
	if inBolt(x, y) {
		return bolt
	}
	if y > t+w+(b-t-2*w)*0.30 {
		return fill
	}
	return rgba(0x1B, 0x24, 0x2D, 255)
}

func roundRect(x, y, l, r, t, b, rad float64) bool {
	if x < l || x > r || y < t || y > b {
		return false
	}
	cx := math.Max(l+rad, math.Min(x, r-rad))
	cy := math.Max(t+rad, math.Min(y, b-rad))
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= rad*rad
}

// inBolt tests a lightning-bolt polygon centred in the battery.
func inBolt(x, y float64) bool {
	poly := [][2]float64{{0.56, 0.30}, {0.36, 0.58}, {0.49, 0.58}, {0.43, 0.82}, {0.65, 0.50}, {0.52, 0.50}, {0.58, 0.30}}
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		xi, yi, xj, yj := poly[i][0], poly[i][1], poly[j][0], poly[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}
