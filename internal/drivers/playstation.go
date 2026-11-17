package drivers

// Sony DualSense (054C:0CE6) and DualSense Edge (054C:0DF2).
//
// The controller streams input reports continuously; the battery byte is part of the full
// report: report 0x01 over USB (byte 53) and report 0x31 over Bluetooth (byte 54, one byte
// later because of a sequence byte). Low nibble = level 0..10, high nibble = 0 discharging,
// 1 charging, 2 full. Facts from the public community controller documentation.
//
// Over Bluetooth Windows receives the reduced report 0x01 (no battery) until something asks
// the controller for its full report mode. Switching modes ourselves would change what games
// using DirectInput see, so we stay passive: on Bluetooth we only read the battery when
// another program (e.g. Steam) has already enabled full reports.

import (
	"strings"
	"time"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
)

const sonyVID = 0x054C

var dualSenseModels = map[uint16]string{
	0x0CE6: "DualSense",
	0x0DF2: "DualSense Edge",
}

type playstationDriver struct{}

func init() { device.Register(playstationDriver{}) }

func (playstationDriver) Name() string { return "playstation" }

func (playstationDriver) Poll(ctx *device.PollContext) {
	gamepads := ctx.Find(sonyVID, func(c hid.Collection) bool {
		_, known := dualSenseModels[c.ProductID]
		return known && c.UsagePage == 0x01 && c.Usage == 0x05
	})
	for _, c := range gamepads {
		bt := isBluetoothPath(c.Path)
		s := device.Status{
			Key:     "ps:" + hex4(c.ProductID) + ":" + shortID(c.Path),
			Name:    dualSenseModels[c.ProductID],
			Via:     "USB cable",
			Kind:    device.Gamepad,
			Percent: -1,
		}
		if bt {
			s.Via = "Bluetooth"
		}
		percent, charging, ok := readDualSense(c, bt)
		if !ok {
			// Connected but no battery data (Bluetooth in reduced mode): claim it so the
			// generic Bluetooth driver does not show a stale value, but report nothing.
			ctx.Claim(c.VendorID, c.ProductID)
			continue
		}
		s.Percent, s.Charging = percent, charging
		ctx.Add(s, c.VendorID, c.ProductID)
	}
}

func readDualSense(c hid.Collection, bt bool) (percent int, charging, ok bool) {
	d, err := hid.Open(c)
	if err != nil {
		return 0, false, false
	}
	defer d.Close()
	d.Flush()
	buf := make([]byte, 128)
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		n, err := d.Read(buf, 100*time.Millisecond)
		if err != nil {
			if err == hid.ErrTimeout {
				continue
			}
			return 0, false, false
		}
		if p, ch, ok := parseDualSense(buf[:n], bt); ok {
			return p, ch, true
		}
		if bt && n > 0 && buf[0] == 0x01 {
			return 0, false, false // reduced Bluetooth report: no battery in it
		}
	}
	return 0, false, false
}

// parseDualSense extracts the battery from a full input report. Pure function, unit tested.
func parseDualSense(r []byte, bt bool) (percent int, charging, ok bool) {
	var b byte
	switch {
	case !bt && len(r) >= 54 && r[0] == 0x01:
		b = r[53]
	case bt && len(r) >= 55 && r[0] == 0x31:
		b = r[54]
	default:
		return 0, false, false
	}
	level, state := int(b&0x0F), b>>4
	switch state {
	case 0: // discharging
	case 1:
		charging = true
	case 2: // full, still on the cable
		return 100, false, true
	default: // 0xA / 0xB / 0xF: voltage or temperature error, charging stopped
		return 0, false, false
	}
	// Each level step covers 10 %; report the middle of the step.
	return min(level*10+5, 100), charging, true
}

// isBluetoothPath reports whether a HID path belongs to a Bluetooth device (HID profile
// service GUID 00001124-... for classic, 00001812-... for BLE HID over GATT).
func isBluetoothPath(path string) bool {
	p := strings.ToLower(path)
	return strings.Contains(p, "{00001124-0000-1000-8000-00805f9b34fb}") ||
		strings.Contains(p, "{00001812-0000-1000-8000-00805f9b34fb}")
}
