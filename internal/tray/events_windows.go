package tray

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procRegisterDeviceNotification = user32.NewProc("RegisterDeviceNotificationW")

const (
	wmDeviceChange          = 0x0219
	wmPowerBroadcast        = 0x0218
	dbtDeviceArrival        = 0x8000
	dbtDeviceRemoveComplete = 0x8004
	dbtDevtypDeviceIface    = 5
	pbtAPMResumeAutomatic   = 0x12
)

// DEV_BROADCAST_DEVICEINTERFACE_W
type devBroadcastDeviceInterface struct {
	Size       uint32
	DeviceType uint32
	Reserved   uint32
	ClassGUID  windows.GUID
	Name       [1]uint16
}

// registerHIDNotifications asks Windows to send WM_DEVICECHANGE when a HID interface
// (dongle plugged in, Bluetooth controller connected...) arrives or goes away.
func registerHIDNotifications(hwnd uintptr) {
	f := devBroadcastDeviceInterface{
		DeviceType: dbtDevtypDeviceIface,
		ClassGUID: windows.GUID{Data1: 0x4D1E55B2, Data2: 0xF16F, Data3: 0x11CF,
			Data4: [8]byte{0x88, 0xCB, 0x00, 0x11, 0x11, 0x00, 0x00, 0x30}},
	}
	f.Size = uint32(unsafe.Sizeof(f))
	procRegisterDeviceNotification.Call(hwnd, uintptr(unsafe.Pointer(&f)), 0) // DEVICE_NOTIFY_WINDOW_HANDLE
}

func call(f func()) {
	if f != nil {
		f()
	}
}
