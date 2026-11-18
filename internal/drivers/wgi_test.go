package drivers

import "testing"

func TestWGIPercent(t *testing.T) {
	cases := []struct {
		status, full, rem int32
		known             bool
		want              int
		ok                bool
	}{
		{1, 1000, 850, true, 85, true},   // discharging
		{3, 1000, 400, true, 40, true},   // charging
		{2, 1000, 1010, true, 100, true}, // idle, slightly over full: clamped
		{0, 1000, 500, true, 0, false},   // no battery
		{1, 0, 500, true, 0, false},      // unknown capacity
		{1, 1000, 500, false, 0, false},  // null references
	}
	for _, c := range cases {
		got, ok := wgiPercent(c.status, c.full, c.rem, c.known)
		if got != c.want || ok != c.ok {
			t.Errorf("wgiPercent(%d,%d,%d,%v) = %d,%v; want %d,%v", c.status, c.full, c.rem, c.known, got, ok, c.want, c.ok)
		}
	}
}

// Activates the real Windows.Gaming.Input factory: proves the COM plumbing (ids, slots,
// HSTRING) works on this machine even without a controller attached.
func TestWGIActivation(t *testing.T) {
	list := wgiControllers()
	if wgiStatics == 0 {
		t.Fatal("RawGameController statics could not be activated")
	}
	for _, c := range list {
		t.Logf("controller %04X:%04X %q battery=%v %d%% charging=%v", c.vid, c.pid, c.name, c.hasReport, c.percent, c.charging)
	}
}
