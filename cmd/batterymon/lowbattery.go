package main

import "github.com/eneskaramanoglu/go-battery-monitor/internal/device"

// rearmMargin: after a low warning, the device must climb this many points above the
// threshold (or start charging) before it can warn again, so a level hovering around the
// threshold does not spam notifications.
const rearmMargin = 5

// lowTracker decides which devices just became low.
type lowTracker struct {
	warned map[string]bool
}

func newLowTracker() *lowTracker { return &lowTracker{warned: map[string]bool{}} }

// check returns the devices that should get a low battery notification now.
func (l *lowTracker) check(st []device.Status, threshold int) []device.Status {
	var out []device.Status
	for _, s := range st {
		switch {
		case s.Charging:
			delete(l.warned, s.Key)
		case s.Asleep:
			// last known value, not a fresh reading: leave the state alone
		case isLow(s, threshold):
			if !l.warned[s.Key] {
				l.warned[s.Key] = true
				out = append(out, s)
			}
		case s.Percent >= 0 && s.Percent > threshold+rearmMargin:
			delete(l.warned, s.Key)
		case s.Percent < 0 && s.Level != "":
			delete(l.warned, s.Key) // coarse level back above Low
		}
	}
	return out
}
