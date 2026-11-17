package drivers

import (
	"testing"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
)

func boolp(b bool) *bool { return &b }

func noneClaimed(uint16, uint16) bool { return false }

func TestCollectBluetoothGroupsByContainer(t *testing.T) {
	nodes := []btNode{
		// BLE mouse: name and ids on the device node, battery on a service node.
		{ID: `BTHLE\DEV_AABBCCDDEEFF\7&1&0&AABBCCDDEEFF`, Container: "{c1}", Name: "MX Anywhere", Battery: -1, Connected: boolp(true), VID: 0x046D, PID: 0xB025, Category: []string{"{Input.Mouse}"}},
		{ID: `BTHLEDEVICE\{0000180F-0000-1000-8000-00805F9B34FB}_DEV_VID&02046D_PID&B025\8&1`, Container: "{c1}", Name: "Battery service", Battery: 70},
		// Headset reporting through hands-free.
		{ID: `BTHENUM\DEV_112233445566\7&2`, Container: "{c2}", Name: "WH-1000XM5", Battery: -1, CoD: 0x240404},
		{ID: `BTHHFENUM\BthHFPAudio\8&3`, Container: "{c2}", Name: "WH-1000XM5 Hands-Free", Battery: 40},
		// Paired controller that is switched off: stale level must be hidden.
		{ID: `BTHENUM\DEV_88034C145F39\A&3`, Container: "{c3}", Name: "DualSense Wireless Controller", Battery: 90, Connected: boolp(false), VID: 0x054C, PID: 0x0CE6, CoD: 0x2508},
		// Device without any battery level.
		{ID: `BTHENUM\DEV_000000000001\A&4`, Container: "{c4}", Name: "Speaker", Battery: -1},
	}
	got := collectBluetooth(nodes, noneClaimed)
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %+v", len(got), got)
	}
	m := got[0].status
	if m.Name != "MX Anywhere" || m.Percent != 70 || m.Kind != device.Mouse || got[0].vid != 0x046D {
		t.Errorf("mouse = %+v", got[0])
	}
	h := got[1].status
	if h.Name != "WH-1000XM5" || h.Percent != 40 || h.Kind != device.Headset {
		t.Errorf("headset = %+v", h)
	}
}

func TestCollectBluetoothSkipsClaimed(t *testing.T) {
	nodes := []btNode{{ID: `BTHENUM\DEV_1\x`, Container: "{c}", Name: "Pad", Battery: 50, Connected: boolp(true), VID: 0x054C, PID: 0x0CE6}}
	got := collectBluetooth(nodes, func(v, p uint16) bool { return v == 0x054C && p == 0x0CE6 })
	if len(got) != 0 {
		t.Fatalf("claimed device reported: %+v", got)
	}
}

func TestBtKind(t *testing.T) {
	cases := []struct {
		cat  []string
		cod  uint32
		name string
		want device.Kind
	}{
		{[]string{"{Input.Gaming}"}, 0, "", device.Gamepad},
		{[]string{"{Input.Keyboard}"}, 0, "Keychron K3", device.Keyboard},
		{[]string{"{Audio.Headphone}"}, 0, "", device.Headset},
		{nil, 0x2508, "", device.Gamepad},  // peripheral / gamepad (DualSense)
		{nil, 0x2580, "", device.Mouse},    // peripheral / pointing
		{nil, 0x2540, "", device.Keyboard}, // peripheral / keyboard
		{nil, 0x240404, "", device.Headset},
		{nil, 0, "Galaxy Buds2 Pro", device.Headset},
		{nil, 0, "Keychron K3", device.Other},
		{nil, 0, "Magic Keyboard", device.Keyboard},
	}
	for _, c := range cases {
		if got := btKind(c.cat, c.cod, c.name); got != c.want {
			t.Errorf("btKind(%v, %#x, %q) = %v, want %v", c.cat, c.cod, c.name, got, c.want)
		}
	}
}
