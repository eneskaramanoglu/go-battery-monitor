package drivers

// Xbox controllers (and headsets plugged into them) through XInput, which every Windows 10/11
// machine has (xinput1_4.dll). XInput only reports coarse levels: empty / low / medium / full.

import (
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
)

var (
	modXInput             = windows.NewLazySystemDLL("xinput1_4.dll")
	procXInputGetBattInfo = modXInput.NewProc("XInputGetBatteryInformation")
)

// XINPUT_BATTERY_INFORMATION and its constants (xinput.h).
type xinputBattery struct {
	Type  byte
	Level byte
}

const (
	xinputMaxUsers     = 4
	batteryDevGamepad  = 0
	batteryDevHeadset  = 1
	batteryTypeNone    = 0x00 // disconnected
	batteryTypeWired   = 0x01
	batteryTypeUnknown = 0xFF
)

type xinputDriver struct{}

func init() { device.Register(xinputDriver{}) }

func (xinputDriver) Name() string { return "xinput" }

// Poll asks Windows.Gaming.Input first (real percentages); XInput's coarse levels are only
// used for gamepads when it found no Xbox controller, and always for headsets plugged into one.
func (xinputDriver) Poll(ctx *device.PollContext) {
	xboxFromWGI := pollWGI(ctx)
	if modXInput.Load() != nil {
		return
	}
	for user := range xinputMaxUsers {
		for _, dev := range []uint32{batteryDevGamepad, batteryDevHeadset} {
			if dev == batteryDevGamepad && xboxFromWGI > 0 {
				continue
			}
			var b xinputBattery
			r, _, _ := procXInputGetBattInfo.Call(uintptr(user), uintptr(dev), uintptr(unsafe.Pointer(&b)))
			if r != 0 { // ERROR_DEVICE_NOT_CONNECTED or no such slot
				continue
			}
			if s, ok := xinputStatus(user, dev, b); ok {
				ctx.Results = append(ctx.Results, s)
			}
		}
	}
}

// xinputStatus converts one XInput battery reading; wired and absent devices are skipped.
func xinputStatus(user int, dev uint32, b xinputBattery) (device.Status, bool) {
	switch b.Type {
	case batteryTypeNone, batteryTypeWired, batteryTypeUnknown:
		return device.Status{}, false
	}
	level := [...]string{"Empty", "Low", "Medium", "Full"}
	if int(b.Level) >= len(level) {
		return device.Status{}, false
	}
	s := device.Status{
		Brand:   "xbox",
		Key:     "xinput:" + strconv.Itoa(user),
		Name:    "Xbox controller",
		Via:     "Xbox Wireless",
		Kind:    device.Gamepad,
		Percent: -1,
		Level:   level[b.Level],
	}
	if dev == batteryDevHeadset {
		s.Key += ":headset"
		s.Name = "Xbox headset"
		s.Kind = device.Headset
	}
	if user > 0 {
		s.Name += " " + strconv.Itoa(user+1)
	}
	return s, true
}
