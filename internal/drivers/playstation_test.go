package drivers

import "testing"

func usbReport(battery byte) []byte {
	r := make([]byte, 64)
	r[0], r[53] = 0x01, battery
	return r
}

func btReport(battery byte) []byte {
	r := make([]byte, 78)
	r[0], r[54] = 0x31, battery
	return r
}

func TestParseDualSense(t *testing.T) {
	cases := []struct {
		name     string
		report   []byte
		bt       bool
		percent  int
		charging bool
		ok       bool
	}{
		{"usb discharging 70%", usbReport(0x07), false, 75, false, true},
		{"usb charging 30%", usbReport(0x13), false, 35, true, true},
		{"usb full", usbReport(0x2A), false, 100, false, true},
		{"usb level 10 capped", usbReport(0x0A), false, 100, false, true},
		{"usb charge error", usbReport(0xA5), false, 0, false, false},
		{"bt full report", btReport(0x04), true, 45, false, true},
		{"bt reduced report", []byte{0x01, 0x80, 0x80, 0x80, 0x80, 0x08, 0, 0, 0, 0}, true, 0, false, false},
		{"usb report read as bt", usbReport(0x07), true, 0, false, false},
		{"short report", []byte{0x01, 0x02}, false, 0, false, false},
	}
	for _, c := range cases {
		p, ch, ok := parseDualSense(c.report, c.bt)
		if p != c.percent || ch != c.charging || ok != c.ok {
			t.Errorf("%s: got (%d, %v, %v), want (%d, %v, %v)", c.name, p, ch, ok, c.percent, c.charging, c.ok)
		}
	}
}

func TestIsBluetoothPath(t *testing.T) {
	bt := `\\?\hid#{00001124-0000-1000-8000-00805f9b34fb}_vid&0002054c_pid&0ce6#a&1#{4d1e55b2-f16f-11cf-88cb-001111000030}`
	usb := `\\?\hid#vid_054c&pid_0ce6&mi_03#8&1#{4d1e55b2-f16f-11cf-88cb-001111000030}`
	if !isBluetoothPath(bt) || isBluetoothPath(usb) {
		t.Fatal("isBluetoothPath misclassified")
	}
}
