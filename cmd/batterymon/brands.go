package main

import "github.com/eneskaramanoglu/go-battery-monitor/internal/tray"

// brandAccents are the colours used for known makers when "Brand colors" is on: a brighter
// shade for dark taskbars and a deeper one for light taskbars. Our own picks, close to each
// maker's familiar colour.
var brandAccents = map[string]*tray.Accent{
	"logitech":    {Dark: tray.RGB{R: 0x00, G: 0xB8, B: 0xFC}, Light: tray.RGB{R: 0x00, G: 0x7A, B: 0xB8}},
	"playstation": {Dark: tray.RGB{R: 0x4C, G: 0x8D, B: 0xFF}, Light: tray.RGB{R: 0x00, G: 0x50, B: 0xD0}},
	"xbox":        {Dark: tray.RGB{R: 0x4C, G: 0xD1, B: 0x4C}, Light: tray.RGB{R: 0x10, G: 0x7C, B: 0x10}},
	"apple":       {Dark: tray.RGB{R: 0xD0, G: 0xD0, B: 0xD6}, Light: tray.RGB{R: 0x55, G: 0x55, B: 0x5C}},
}

// brandAccent returns the accent for a brand, or nil when unknown or disabled.
func brandAccent(brand string, enabled bool) *tray.Accent {
	if !enabled {
		return nil
	}
	return brandAccents[brand]
}
