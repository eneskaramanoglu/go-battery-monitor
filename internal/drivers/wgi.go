package drivers

// Real battery percentages for game controllers through Windows.Gaming.Input: every
// RawGameController that offers a battery report (Xbox controllers on the Xbox Wireless
// adapter, USB or Bluetooth, and some third-party pads). Interface ids and vtable order are
// from the Windows SDK headers windows.gaming.input.h and windows.devices.power.h.

import (
	"runtime"
	"strconv"
	"sync"
	"unsafe"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/winrt"
)

var (
	iidRawGameControllerStatics  = winrt.MustGUID("{EB8D0792-E95A-4B19-AFC7-0A59F8BF759E}")
	iidRawGameController         = winrt.MustGUID("{7CAD6D91-A7E1-4F71-9A78-33E9C5DFEA62}")
	iidRawGameController2        = winrt.MustGUID("{43C0C035-BB73-4756-A787-3ED6BEA617BD}")
	iidGameControllerBatteryInfo = winrt.MustGUID("{DCECC681-3963-4DA6-955D-553F3B6F6161}")
)

// Vtable slots (after the 6 IInspectable slots), in SDK declaration order.
const (
	slotStaticsGetRawGameControllers = winrt.FirstMethod + 4 // IRawGameControllerStatics
	slotVectorGetAt                  = winrt.FirstMethod + 0 // IVectorView<T>
	slotVectorGetSize                = winrt.FirstMethod + 1
	slotRawGetHardwareProductID      = winrt.FirstMethod + 3 // IRawGameController
	slotRawGetHardwareVendorID       = winrt.FirstMethod + 4
	slotRaw2GetDisplayName           = winrt.FirstMethod + 2 // IRawGameController2
	slotBatteryInfoTryGetReport      = winrt.FirstMethod + 0 // IGameControllerBatteryInfo
	slotReportFullCharge             = winrt.FirstMethod + 2 // IBatteryReport
	slotReportRemaining              = winrt.FirstMethod + 3
	slotReportStatus                 = winrt.FirstMethod + 4
	slotReferenceGetValue            = winrt.FirstMethod + 0 // IReference<int>
)

// Windows.System.Power.BatteryStatus
const (
	batteryNotPresent = 0
	batteryCharging   = 3
)

// The statics object is kept for the life of the process: the controller list fills in
// asynchronously after the first activation, so a fresh factory on every poll would see
// an empty list.
var (
	wgiOnce    sync.Once
	wgiStatics winrt.Object
)

type wgiController struct {
	vid, pid  uint16
	name      string
	percent   int
	charging  bool
	hasReport bool
}

// wgiControllers lists the game controllers Windows.Gaming.Input knows, with their battery.
func wgiControllers() []wgiController {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if winrt.InitMTA() != nil {
		return nil
	}
	wgiOnce.Do(func() {
		wgiStatics, _ = winrt.Factory("Windows.Gaming.Input.RawGameController", &iidRawGameControllerStatics)
	})
	if wgiStatics == 0 {
		return nil
	}
	var list winrt.Object
	if wgiStatics.Call(slotStaticsGetRawGameControllers, uintptr(unsafe.Pointer(&list))) != nil || list == 0 {
		return nil
	}
	defer list.Release()
	var n uint32
	list.Call(slotVectorGetSize, uintptr(unsafe.Pointer(&n)))

	var out []wgiController
	for i := uint32(0); i < n; i++ {
		var item winrt.Object
		if list.Call(slotVectorGetAt, uintptr(i), uintptr(unsafe.Pointer(&item))) != nil || item == 0 {
			continue
		}
		out = append(out, readController(item))
		item.Release()
	}
	return out
}

func readController(item winrt.Object) wgiController {
	var c wgiController
	if raw := item.QueryInterface(&iidRawGameController); raw != 0 {
		raw.Call(slotRawGetHardwareVendorID, uintptr(unsafe.Pointer(&c.vid)))
		raw.Call(slotRawGetHardwareProductID, uintptr(unsafe.Pointer(&c.pid)))
		raw.Release()
	}
	if raw2 := item.QueryInterface(&iidRawGameController2); raw2 != 0 {
		var hs uintptr
		if raw2.Call(slotRaw2GetDisplayName, uintptr(unsafe.Pointer(&hs))) == nil {
			c.name = winrt.HStringToString(hs)
		}
		raw2.Release()
	}
	info := item.QueryInterface(&iidGameControllerBatteryInfo)
	if info == 0 {
		return c
	}
	defer info.Release()
	var report winrt.Object
	if info.Call(slotBatteryInfoTryGetReport, uintptr(unsafe.Pointer(&report))) != nil || report == 0 {
		return c
	}
	defer report.Release()
	var status int32
	report.Call(slotReportStatus, uintptr(unsafe.Pointer(&status)))
	full, okFull := referenceInt(report, slotReportFullCharge)
	rem, okRem := referenceInt(report, slotReportRemaining)
	if p, ok := wgiPercent(status, full, rem, okFull && okRem); ok {
		c.percent, c.charging, c.hasReport = p, status == batteryCharging, true
	}
	return c
}

// referenceInt reads an IReference<int> property (null when unknown).
func referenceInt(report winrt.Object, slot int) (int32, bool) {
	var ref winrt.Object
	if report.Call(slot, uintptr(unsafe.Pointer(&ref))) != nil || ref == 0 {
		return 0, false
	}
	defer ref.Release()
	var v int32
	if ref.Call(slotReferenceGetValue, uintptr(unsafe.Pointer(&v))) != nil {
		return 0, false
	}
	return v, true
}

// wgiPercent turns a battery report into a percentage. Pure function, unit tested.
func wgiPercent(status, full, remaining int32, known bool) (int, bool) {
	if !known || status == batteryNotPresent || full <= 0 || remaining < 0 {
		return 0, false
	}
	return min(int(int64(remaining)*100/int64(full)), 100), true
}

// pollWGI adds every controller with a battery report and returns how many Microsoft
// (Xbox) controllers it reported, so XInput's coarse levels can be skipped for them.
func pollWGI(ctx *device.PollContext) (xbox int) {
	for i, c := range wgiControllers() {
		if !c.hasReport || ctx.Claimed(c.vid, c.pid) {
			continue
		}
		name := c.name
		switch {
		case c.vid == 0x3537:
			name = "GameSir controller" // GIP reports a generic Xbox name for third-party pads
		case name == "":
			name = "Xbox controller"
		}
		brand := ""
		if c.vid == 0x045E {
			brand = "xbox"
		}
		ctx.Add(device.Status{
			Brand:    brand,
			Key:      "wgi:" + hex4(c.vid) + ":" + hex4(c.pid) + ":" + strconv.Itoa(i),
			Name:     name,
			Via:      "Xbox Wireless / Bluetooth",
			Kind:     device.Gamepad,
			Percent:  c.percent,
			Charging: c.charging,
		}, c.vid, c.pid)
		if c.vid == 0x045E {
			xbox++
		}
	}
	return xbox
}
