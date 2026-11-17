package main

import (
	"strings"
	"testing"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/tray"
)

func TestIconFor(t *testing.T) {
	cases := []struct {
		s     device.Status
		text  string
		style tray.Style
	}{
		{device.Status{Key: "a", Name: "Mouse", Percent: 85}, "85", tray.Normal},
		{device.Status{Key: "b", Name: "Mouse", Percent: 15}, "15", tray.Low},
		{device.Status{Key: "c", Name: "Mouse", Percent: 10, Charging: true}, "10", tray.Charging},
		{device.Status{Key: "d", Name: "Mouse", Percent: 60, Asleep: true}, "60", tray.Dim},
		{device.Status{Key: "e", Name: "Pad", Percent: -1, Level: "Low"}, "L", tray.Low},
		{device.Status{Key: "f", Name: "Pad", Percent: -1}, "?", tray.Normal},
	}
	for _, c := range cases {
		ic := iconFor(c.s, 15)
		if ic.Text != c.text || ic.Style != c.style || ic.Key != c.s.Key {
			t.Errorf("iconFor(%+v) = %+v, want text %q style %v", c.s, ic, c.text, c.style)
		}
		if !strings.HasPrefix(ic.Tooltip, c.s.Name) {
			t.Errorf("tooltip %q does not start with the name", ic.Tooltip)
		}
	}
}
