// Package pnp reads Plug and Play device properties through cfgmgr32 (the same data
// Device Manager shows on its "Details" tab).
package pnp

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Key is a DEVPROPKEY: a property set GUID plus a property id.
type Key struct {
	Fmt windows.GUID
	PID uint32
}

// Property types (DEVPROP_TYPE_*).
const (
	TypeByte    = 0x03
	TypeUint16  = 0x05
	TypeUint32  = 0x07
	TypeGUID    = 0x0D
	TypeBoolean = 0x11
	TypeString  = 0x12

	typeModList = 0x2000
)

func guid(s string) windows.GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(err)
	}
	return g
}

// Well-known keys (devpkey.h, bthdevpkey.h).
var (
	KeyFriendlyName = Key{guid("{A45C254E-DF1C-4EFD-8020-67D146A850E0}"), 14}
	KeyDeviceDesc   = Key{guid("{A45C254E-DF1C-4EFD-8020-67D146A850E0}"), 2}
	KeyContainerID  = Key{guid("{8C7ED206-3F8A-4827-B3AB-AE9E1FAEFC6C}"), 2}
	// KeyIsConnected is false while a paired Bluetooth device is out of range or switched off.
	KeyIsConnected = Key{guid("{83DA6326-97A6-4088-9453-A1923F573B29}"), 15}

	// KeyContainerCategory lists categories such as "{Input.Mouse}" or "{Audio.Headset}".
	KeyContainerCategory = Key{guid("{78C34FC8-104A-4ACA-9EA4-524D52996E57}"), 90}

	KeyBluetoothBattery       = Key{guid("{104EA319-6EE2-4701-BD47-8DDBF425BBE5}"), 2}
	KeyBluetoothVID           = Key{guid("{2BD67D8B-8BEB-48D5-87E0-6CDA3428040A}"), 7}
	KeyBluetoothPID           = Key{guid("{2BD67D8B-8BEB-48D5-87E0-6CDA3428040A}"), 8}
	KeyBluetoothClassOfDevice = Key{guid("{2BD67D8B-8BEB-48D5-87E0-6CDA3428040A}"), 10}
)

var (
	modCfgmgr32        = windows.NewLazySystemDLL("cfgmgr32.dll")
	procGetIDListSize  = modCfgmgr32.NewProc("CM_Get_Device_ID_List_SizeW")
	procGetIDList      = modCfgmgr32.NewProc("CM_Get_Device_ID_ListW")
	procLocateDevNode  = modCfgmgr32.NewProc("CM_Locate_DevNodeW")
	procGetDevNodeProp = modCfgmgr32.NewProc("CM_Get_DevNode_PropertyW")
)

const (
	crSuccess              = 0
	crBufferSmall          = 0x1A
	cmGetIDListEnumerator  = 0x1
	cmGetIDListPresent     = 0x100
	cmLocateDevNodePhantom = 0x1
)

// DeviceIDs lists the present device instance ids under one enumerator ("BTHENUM", "USB", ...).
func DeviceIDs(enumerator string) ([]string, error) {
	filter, err := windows.UTF16PtrFromString(enumerator)
	if err != nil {
		return nil, err
	}
	flags := uintptr(cmGetIDListEnumerator | cmGetIDListPresent)
	for range 3 { // the list can grow between the size and the list call
		var n uint32
		if cr, _, _ := procGetIDListSize.Call(uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(filter)), flags); cr != crSuccess {
			return nil, fmt.Errorf("CM_Get_Device_ID_List_Size(%s): CR %d", enumerator, cr)
		}
		if n <= 1 {
			return nil, nil
		}
		buf := make([]uint16, n)
		cr, _, _ := procGetIDList.Call(uintptr(unsafe.Pointer(filter)), uintptr(unsafe.Pointer(&buf[0])), uintptr(n), flags)
		if cr == crBufferSmall {
			continue
		}
		if cr != crSuccess {
			return nil, fmt.Errorf("CM_Get_Device_ID_List(%s): CR %d", enumerator, cr)
		}
		var ids []string
		for start := 0; start < len(buf) && buf[start] != 0; {
			end := start
			for end < len(buf) && buf[end] != 0 {
				end++
			}
			ids = append(ids, windows.UTF16ToString(buf[start:end]))
			start = end + 1
		}
		return ids, nil
	}
	return nil, fmt.Errorf("CM_Get_Device_ID_List(%s): list kept changing", enumerator)
}

// Node is a located device instance.
type Node struct {
	ID   string
	inst uint32
}

// Locate opens a device instance by id (also works for nodes that are not running).
func Locate(id string) (Node, error) {
	p, err := windows.UTF16PtrFromString(id)
	if err != nil {
		return Node{}, err
	}
	var inst uint32
	if cr, _, _ := procLocateDevNode.Call(uintptr(unsafe.Pointer(&inst)), uintptr(unsafe.Pointer(p)), cmLocateDevNodePhantom); cr != crSuccess {
		return Node{}, fmt.Errorf("CM_Locate_DevNode(%s): CR %d", id, cr)
	}
	return Node{ID: id, inst: inst}, nil
}

// Raw returns a property's type and bytes; ok is false when the node does not have it.
func (n Node) Raw(k Key) (typ uint32, data []byte, ok bool) {
	size := uint32(64)
	for range 2 {
		data = make([]byte, size)
		cr, _, _ := procGetDevNodeProp.Call(uintptr(n.inst), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&typ)),
			uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&size)), 0)
		switch cr {
		case crSuccess:
			return typ, data[:size], true
		case crBufferSmall:
			continue
		default:
			return 0, nil, false
		}
	}
	return 0, nil, false
}

// Uint returns BYTE / UINT16 / UINT32 properties as a number.
func (n Node) Uint(k Key) (uint32, bool) {
	typ, d, ok := n.Raw(k)
	if !ok {
		return 0, false
	}
	switch {
	case typ == TypeByte && len(d) >= 1:
		return uint32(d[0]), true
	case typ == TypeUint16 && len(d) >= 2:
		return uint32(binary.LittleEndian.Uint16(d)), true
	case typ == TypeUint32 && len(d) >= 4:
		return binary.LittleEndian.Uint32(d), true
	}
	return 0, false
}

// Bool returns a DEVPROP_TYPE_BOOLEAN property.
func (n Node) Bool(k Key) (bool, bool) {
	typ, d, ok := n.Raw(k)
	if !ok || typ != TypeBoolean || len(d) < 1 {
		return false, false
	}
	return d[0] != 0, true
}

// String returns a DEVPROP_TYPE_STRING property.
func (n Node) String(k Key) (string, bool) {
	typ, d, ok := n.Raw(k)
	if !ok || typ != TypeString || len(d) < 2 {
		return "", false
	}
	return windows.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&d[0])), len(d)/2)), true
}

// Strings returns a DEVPROP_TYPE_STRING_LIST property (a single string is returned as a list).
func (n Node) Strings(k Key) []string {
	typ, d, ok := n.Raw(k)
	if !ok || (typ != TypeString && typ != TypeString|typeModList) || len(d) < 2 {
		return nil
	}
	u := unsafe.Slice((*uint16)(unsafe.Pointer(&d[0])), len(d)/2)
	var out []string
	for start := 0; start < len(u) && u[start] != 0; {
		end := start
		for end < len(u) && u[end] != 0 {
			end++
		}
		out = append(out, windows.UTF16ToString(u[start:end]))
		start = end + 1
	}
	return out
}

// GUID returns a DEVPROP_TYPE_GUID property in its "{...}" string form.
func (n Node) GUID(k Key) (string, bool) {
	typ, d, ok := n.Raw(k)
	if !ok || typ != TypeGUID || len(d) < 16 {
		return "", false
	}
	return (*windows.GUID)(unsafe.Pointer(&d[0])).String(), true
}

// Name returns the friendly name, falling back to the device description.
func (n Node) Name() string {
	if s, ok := n.String(KeyFriendlyName); ok && s != "" {
		return s
	}
	s, _ := n.String(KeyDeviceDesc)
	return s
}
