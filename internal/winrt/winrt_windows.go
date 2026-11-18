// Package winrt holds the few raw WinRT/COM helpers the drivers need: apartment init,
// activation factories, HSTRINGs and vtable calls. Interface ids and vtable slots used by
// callers are taken from the Windows SDK headers (winrt/*.h).
package winrt

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	combase                    = windows.NewLazySystemDLL("combase.dll")
	procRoInitialize           = combase.NewProc("RoInitialize")
	procRoGetActivationFactory = combase.NewProc("RoGetActivationFactory")
	procWindowsCreateString    = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString    = combase.NewProc("WindowsDeleteString")
	procWindowsGetStringRaw    = combase.NewProc("WindowsGetStringRawBuffer")
)

// Vtable slots shared by every WinRT interface (IUnknown + IInspectable).
const (
	SlotQueryInterface = 0
	SlotRelease        = 2
	// FirstMethod is the first slot of an interface's own methods.
	FirstMethod = 6
)

// InitMTA joins the calling thread to the multithreaded apartment (safe to call repeatedly).
// The caller should have locked the goroutine to its OS thread.
func InitMTA() error {
	const roInitMultithreaded = 1
	hr, _, _ := procRoInitialize.Call(roInitMultithreaded)
	if int32(hr) < 0 && uint32(hr) != 0x80010106 { // RPC_E_CHANGED_MODE: already STA, still usable for MTA objects via proxy
		return fmt.Errorf("RoInitialize: %#x", uint32(hr))
	}
	return nil
}

// Object is a raw COM interface pointer.
type Object uintptr

// Call invokes vtable slot with the given arguments and returns the HRESULT as an error.
func (o Object) Call(slot int, args ...uintptr) error {
	// o points at the object, whose first word points at the vtable.
	obj := *(*unsafe.Pointer)(unsafe.Pointer(&o))
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	hr, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(o)}, args...)...)
	if int32(hr) < 0 {
		return fmt.Errorf("HRESULT %#x", uint32(hr))
	}
	return nil
}

// Release drops one reference (no-op on nil).
func (o Object) Release() {
	if o != 0 {
		o.Call(SlotRelease)
	}
}

// QueryInterface returns the object's iid interface, or 0.
func (o Object) QueryInterface(iid *windows.GUID) Object {
	var out Object
	if o.Call(SlotQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))) != nil {
		return 0
	}
	return out
}

// Factory returns the activation factory of a runtime class as interface iid.
func Factory(class string, iid *windows.GUID) (Object, error) {
	hs, err := NewHString(class)
	if err != nil {
		return 0, err
	}
	defer DeleteHString(hs)
	var f Object
	hr, _, _ := procRoGetActivationFactory.Call(hs, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&f)))
	if int32(hr) < 0 {
		return 0, fmt.Errorf("RoGetActivationFactory(%s): %#x", class, uint32(hr))
	}
	return f, nil
}

// NewHString creates an HSTRING; free it with DeleteHString.
func NewHString(s string) (uintptr, error) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return 0, err
	}
	var hs uintptr
	hr, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&hs)))
	if int32(hr) < 0 {
		return 0, fmt.Errorf("WindowsCreateString: %#x", uint32(hr))
	}
	return hs, nil
}

// DeleteHString frees an HSTRING (no-op on 0).
func DeleteHString(hs uintptr) {
	if hs != 0 {
		procWindowsDeleteString.Call(hs)
	}
}

// HStringToString copies an HSTRING into a Go string and frees it.
func HStringToString(hs uintptr) string {
	if hs == 0 {
		return ""
	}
	defer DeleteHString(hs)
	var n uint32
	p, _, _ := procWindowsGetStringRaw.Call(hs, uintptr(unsafe.Pointer(&n)))
	if p == 0 || n == 0 {
		return ""
	}
	buf := *(**uint16)(unsafe.Pointer(&p)) // the HSTRING's own buffer, valid until it is deleted
	return windows.UTF16ToString(unsafe.Slice(buf, n))
}

// MustGUID parses a GUID literal (panics on a typo, at init time).
func MustGUID(s string) windows.GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(err)
	}
	return g
}
