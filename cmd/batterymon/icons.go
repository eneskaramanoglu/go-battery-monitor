package main

import (
	"strconv"
	"strings"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/tray"
)

// iconFor turns a device reading into its tray icon; at or below threshold it is drawn red.
func iconFor(s device.Status, threshold int) tray.Icon {
	text := "?"
	switch {
	case s.Percent >= 0:
		text = strconv.Itoa(s.Percent)
	case s.Level != "":
		text = s.Level[:1] // E / L / M / F
	}

	style := tray.Normal
	switch {
	case s.Charging:
		style = tray.Charging
	case s.Asleep:
		style = tray.Dim
	case isLow(s, threshold):
		style = tray.Low
	}

	var tip strings.Builder
	tip.WriteString(s.Name)
	tip.WriteString("\n")
	tip.WriteString(s.LevelText())
	if s.Charging {
		tip.WriteString(", charging")
	}
	if s.Asleep {
		tip.WriteString(", asleep (last known)")
	}
	if s.Detail != "" {
		tip.WriteString("\n")
		tip.WriteString(s.Detail)
	}
	if s.Via != "" {
		tip.WriteString("\n")
		tip.WriteString(s.Via)
	}
	return tray.Icon{Key: s.Key, Text: text, Tooltip: tip.String(), Style: style}
}

// barItemFor turns a device reading into its taskbar entry.
func barItemFor(s device.Status, threshold int) tray.BarItem {
	ic := iconFor(s, threshold)
	glyph := map[device.Kind]rune{
		device.Mouse:    tray.GlyphMouse,
		device.Keyboard: tray.GlyphKeyboard,
		device.Headset:  tray.GlyphHeadset,
		device.Gamepad:  tray.GlyphGamepad,
	}[s.Kind]
	if glyph == 0 {
		glyph = tray.GlyphDevice
	}
	text := s.LevelText()
	style := ic.Style
	if style == tray.Charging {
		style = tray.Normal // the charging bolt carries the colour, the number stays readable
	}
	return tray.BarItem{Key: s.Key, Glyph: glyph, Text: text, Style: style, Charging: s.Charging}
}

// summary is the app icon tooltip in taskbar mode: one line per device.
func summary(st []device.Status) string {
	var b strings.Builder
	b.WriteString("Battery monitor")
	for _, s := range st {
		line := "\n" + s.Name + ": " + s.LevelText()
		if s.Charging {
			line += " (charging)"
		}
		if s.Detail != "" {
			line += " - " + s.Detail
		}
		if b.Len()+len(line) > 127 {
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

func isLow(s device.Status, threshold int) bool {
	if s.Percent >= 0 {
		return s.Percent <= threshold
	}
	return s.Level == "Empty" || s.Level == "Low"
}
