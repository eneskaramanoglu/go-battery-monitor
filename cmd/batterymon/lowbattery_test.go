package main

import (
	"testing"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
)

func TestLowTracker(t *testing.T) {
	l := newLowTracker()
	at := func(p int, charging bool) []device.Status {
		return []device.Status{{Key: "m", Name: "Mouse", Percent: p, Charging: charging}}
	}
	steps := []struct {
		st   []device.Status
		want int
	}{
		{at(40, false), 0},
		{at(15, false), 1}, // crosses the threshold: warn
		{at(14, false), 0}, // already warned
		{at(18, false), 0}, // above threshold but within the re-arm margin
		{at(15, false), 0}, // so no second warning
		{at(21, false), 0}, // above threshold + margin: re-armed
		{at(12, false), 1}, // warns again
		{at(12, true), 0},  // charging: re-armed, no warning
		{at(11, false), 1}, // unplugged while low: warn
	}
	for i, s := range steps {
		if got := len(l.check(s.st, 15)); got != s.want {
			t.Fatalf("step %d (%+v): %d notifications, want %d", i, s.st[0], got, s.want)
		}
	}
}

func TestLowTrackerCoarseLevels(t *testing.T) {
	l := newLowTracker()
	pad := func(level string) []device.Status {
		return []device.Status{{Key: "x", Name: "Pad", Percent: -1, Level: level}}
	}
	if n := len(l.check(pad("Medium"), 15)); n != 0 {
		t.Fatal("medium warned")
	}
	if n := len(l.check(pad("Low"), 15)); n != 1 {
		t.Fatal("low did not warn")
	}
	if n := len(l.check(pad("Low"), 15)); n != 0 {
		t.Fatal("low warned twice")
	}
	l.check(pad("Full"), 15)
	if n := len(l.check(pad("Empty"), 15)); n != 1 {
		t.Fatal("not re-armed after Full")
	}
}
