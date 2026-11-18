package drivers

// Generic HID battery: any device whose report descriptor declares a standard battery field
// (USB HID Usage Tables): Generic Device Controls 0x06 / Battery Strength 0x20, or Battery
// System 0x85 / Relative State Of Charge 0x64, Absolute State Of Charge 0x65, Remaining
// Capacity 0x66 (when its logical range is a percentage). Reads use only GetFeature /
// GetInputReport. Runs after the vendor drivers ("zy_") and before the Bluetooth fallback.

import (
	"strings"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
)

type hidBatteryDriver struct{}

func init() { device.Register(hidBatteryDriver{}) }

func (hidBatteryDriver) Name() string { return "hid-battery" }

// batteryUsage reports whether a value field is a standard battery level, best first.
func batteryUsage(v hid.ValueCap) (rank int, ok bool) {
	switch {
	case v.UsagePage == 0x06 && v.Usage == 0x20:
		return 0, true
	case v.UsagePage == 0x85 && v.Usage == 0x65:
		return 1, true
	case v.UsagePage == 0x85 && v.Usage == 0x64:
		return 2, true
	case v.UsagePage == 0x85 && v.Usage == 0x66 && v.LogicalMax == 100:
		return 3, true // remaining capacity is only a percentage when declared 0..100
	}
	return 0, false
}

// pickBatteryCap returns the best battery field of a collection.
func pickBatteryCap(caps []hid.ValueCap) (hid.ValueCap, bool) {
	best, bestRank, found := hid.ValueCap{}, 99, false
	for _, v := range caps {
		if r, ok := batteryUsage(v); ok && r < bestRank && v.LogicalMax > v.LogicalMin {
			best, bestRank, found = v, r, true
		}
	}
	return best, found
}

// scalePercent maps a raw value in the field's logical range to 0..100.
func scalePercent(raw uint32, v hid.ValueCap) (int, bool) {
	lo, hi := int64(v.LogicalMin), int64(v.LogicalMax)
	val := int64(raw)
	if lo < 0 && val > hi { // sign-extend fields with a negative logical minimum
		val -= int64(1) << v.BitSize
	}
	if val < lo || val > hi || hi <= lo {
		return 0, false
	}
	return int((val - lo) * 100 / (hi - lo)), true
}

func (hidBatteryDriver) Poll(ctx *device.PollContext) {
	for _, c := range ctx.Collections {
		if ctx.Claimed(c.VendorID, c.ProductID) {
			continue
		}
		v, ok := pickBatteryCap(hid.ValueCaps(c))
		if !ok {
			continue
		}
		d, err := hid.Open(c)
		if err != nil {
			continue
		}
		raw, err := d.ReadValue(v)
		d.Close()
		if err != nil {
			continue
		}
		pct, ok := scalePercent(raw, v)
		if !ok {
			continue
		}
		via := "USB"
		if isBluetoothPath(c.Path) {
			via = "Bluetooth"
		}
		name := strings.TrimSpace(c.Product)
		if name == "" {
			name = "HID device " + hex4(c.VendorID) + ":" + hex4(c.ProductID)
		}
		ctx.Add(device.Status{
			Key:     "hid:" + hex4(c.VendorID) + ":" + hex4(c.ProductID),
			Name:    name,
			Via:     via,
			Kind:    kindOfProduct(ctx.Collections, c.VendorID, c.ProductID),
			Percent: pct,
		}, c.VendorID, c.ProductID)
	}
}

// kindOfProduct guesses the device kind from the top-level collections the product exposes.
func kindOfProduct(cols []hid.Collection, vid, pid uint16) device.Kind {
	kind := device.Other
	for _, c := range cols {
		if c.VendorID != vid || c.ProductID != pid || c.UsagePage != 0x01 {
			continue
		}
		switch c.Usage {
		case 0x04, 0x05: // joystick, gamepad
			return device.Gamepad
		case 0x02:
			kind = device.Mouse
		case 0x06:
			if kind == device.Other {
				kind = device.Keyboard
			}
		}
	}
	return kind
}
