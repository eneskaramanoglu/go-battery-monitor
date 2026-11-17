package drivers

import (
	"testing"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
)

func TestXInputStatus(t *testing.T) {
	const nimh = 0x03
	s, ok := xinputStatus(0, batteryDevGamepad, xinputBattery{Type: nimh, Level: 1})
	if !ok || s.Level != "Low" || s.Percent != -1 || s.Kind != device.Gamepad || s.Name != "Xbox controller" {
		t.Errorf("gamepad = %+v, %v", s, ok)
	}
	s, ok = xinputStatus(1, batteryDevHeadset, xinputBattery{Type: nimh, Level: 3})
	if !ok || s.Level != "Full" || s.Kind != device.Headset || s.Name != "Xbox headset 2" || s.Key != "xinput:1:headset" {
		t.Errorf("headset = %+v, %v", s, ok)
	}
	for _, typ := range []byte{batteryTypeNone, batteryTypeWired, batteryTypeUnknown} {
		if _, ok := xinputStatus(0, batteryDevGamepad, xinputBattery{Type: typ, Level: 3}); ok {
			t.Errorf("battery type %#x should be skipped", typ)
		}
	}
	if _, ok := xinputStatus(0, batteryDevGamepad, xinputBattery{Type: nimh, Level: 9}); ok {
		t.Error("invalid level accepted")
	}
}
