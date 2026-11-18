package main

import (
	"strconv"
	"strings"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/i18n"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/tray"
)

// name is the device name as shown: generic names from drivers are translated, product
// names pass through unchanged.
func name(s device.Status) string { return i18n.T(s.Name) }

// levelText is the level as shown ("85%", or a translated coarse level such as "Düşük").
func levelText(s device.Status) string { return i18n.T(s.LevelText()) }

// iconFor turns a device reading into its tray icon; at or below threshold it is drawn red.
func iconFor(s device.Status, threshold int) tray.Icon {
	text := "?"
	switch {
	case s.Percent >= 0:
		text = strconv.Itoa(s.Percent)
	case s.Level != "":
		text = string([]rune(levelText(s))[:1]) // first letter of the (translated) level
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
	tip.WriteString(name(s))
	tip.WriteString("\n")
	tip.WriteString(levelText(s))
	if s.Charging {
		tip.WriteString(", " + i18n.T("charging"))
	}
	if s.Asleep {
		tip.WriteString(", " + i18n.T("asleep (last known)"))
	}
	if s.Detail != "" {
		tip.WriteString("\n")
		tip.WriteString(s.Detail)
	}
	if s.Via != "" {
		tip.WriteString("\n")
		tip.WriteString(i18n.T(s.Via))
	}
	return tray.Icon{Key: s.Key, Text: text, Tooltip: tip.String(), Style: style}
}

// barItemFor turns a device reading into its taskbar entry.
func barItemFor(s device.Status, threshold int, brandColors bool) tray.BarItem {
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
	style := ic.Style
	if style == tray.Charging {
		style = tray.Normal // the charging bolt carries the colour, the number stays readable
	}
	item := tray.BarItem{Key: s.Key, Glyph: glyph, Text: levelText(s), Style: style, Charging: s.Charging, Fill: -1,
		Tooltip: ic.Tooltip}
	if a := brandAccent(s.Brand, brandColors); a != nil {
		item.Accent = a
		item.Fill = s.Percent // -1 (coarse level only) draws no bar
	}
	return item
}

// summary is the app icon tooltip in taskbar mode: one line per device.
func summary(st []device.Status) string {
	var b strings.Builder
	b.WriteString(i18n.T("Battery monitor"))
	for _, s := range st {
		line := "\n" + name(s) + ": " + levelText(s)
		if s.Charging {
			line += " (" + i18n.T("charging") + ")"
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
	return s.Level == "Empty" || s.Level == "Critical" || s.Level == "Low"
}
