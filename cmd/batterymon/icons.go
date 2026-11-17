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
	if s.Via != "" {
		tip.WriteString("\n")
		tip.WriteString(s.Via)
	}
	return tray.Icon{Key: s.Key, Text: text, Tooltip: tip.String(), Style: style}
}

func isLow(s device.Status, threshold int) bool {
	if s.Percent >= 0 {
		return s.Percent <= threshold
	}
	return s.Level == "Empty" || s.Level == "Low"
}
