// Package hid enumerates Windows HID collections and talks to them through hid.dll.
// It uses only system DLLs (no cgo, no hidapi), so the app builds to a single static exe.
package hid

import (
	"errors"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Collection is one HID top-level collection. A single physical device (say a 2.4 GHz dongle)
// usually exposes several of these, one per interface / usage page.
type Collection struct {
	Path       string
	VendorID   uint16
	ProductID  uint16
	Release    uint16
	UsagePage  uint16
	Usage      uint16
	InputLen   uint16 // report sizes in bytes, including the leading report id byte
	OutputLen  uint16
	FeatureLen uint16
	Interface  int // USB interface number (&MI_xx in the path), -1 when absent
	Product    string
}

// ErrTimeout is returned by Read and Write when the device did not answer in time.
var ErrTimeout = errors.New("hid: timeout")

var (
	modHID = windows.NewLazySystemDLL("hid.dll")

	procGetAttributes     = modHID.NewProc("HidD_GetAttributes")
	procGetPreparsedData  = modHID.NewProc("HidD_GetPreparsedData")
	procFreePreparsedData = modHID.NewProc("HidD_FreePreparsedData")
	procGetCaps           = modHID.NewProc("HidP_GetCaps")
	procGetProductString  = modHID.NewProc("HidD_GetProductString")
	procGetFeature        = modHID.NewProc("HidD_GetFeature")
	procSetFeature        = modHID.NewProc("HidD_SetFeature")
	procGetInputReport    = modHID.NewProc("HidD_GetInputReport")
	procSetOutputReport   = modHID.NewProc("HidD_SetOutputReport")
	procFlushQueue        = modHID.NewProc("HidD_FlushQueue")
)

// GUID_DEVINTERFACE_HID
var hidGUID = windows.GUID{Data1: 0x4D1E55B2, Data2: 0xF16F, Data3: 0x11CF,
	Data4: [8]byte{0x88, 0xCB, 0x00, 0x11, 0x11, 0x00, 0x00, 0x30}}

type hiddAttributes struct {
	Size          uint32
	VendorID      uint16
	ProductID     uint16
	VersionNumber uint16
}

// HIDP_CAPS from hidpi.h (64 bytes).
type hidpCaps struct {
	Usage, UsagePage                                uint16
	InputReportByteLength                           uint16
	OutputReportByteLength                          uint16
	FeatureReportByteLength                         uint16
	Reserved                                        [17]uint16
	NumberLinkCollectionNodes                       uint16
	NumberInputButtonCaps, NumberInputValueCaps     uint16
	NumberInputDataIndices                          uint16
	NumberOutputButtonCaps, NumberOutputValueCaps   uint16
	NumberOutputDataIndices                         uint16
	NumberFeatureButtonCaps, NumberFeatureValueCaps uint16
	NumberFeatureDataIndices                        uint16
}

const hidpStatusSuccess = 0x00110000

// Enumerate lists every present HID collection. Opening with zero access rights is enough
// for attributes, caps and strings, and works even for keyboards and mice that Windows
// keeps open exclusively.
func Enumerate() ([]Collection, error) {
	paths, err := windows.CM_Get_Device_Interface_List("", &hidGUID, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
	if err != nil {
		return nil, err
	}
	out := make([]Collection, 0, len(paths))
	for _, p := range paths {
		if c, ok := describe(p); ok {
			out = append(out, c)
		}
	}
	return out, nil
}

// Report types (HIDP_REPORT_TYPE).
const (
	InputReport   = 0
	OutputReport  = 1
	FeatureReport = 2
)

// ValueCap describes one value field of a collection (from HidP_GetValueCaps).
type ValueCap struct {
	ReportType     int
	ReportID       byte
	UsagePage      uint16
	Usage          uint16 // first usage when the field is a range
	LinkCollection uint16
	BitSize        uint16
	ReportCount    uint16
	LogicalMin     int32
	LogicalMax     int32
}

// HIDP_VALUE_CAPS (hidpi.h), 72 bytes.
type hidpValueCaps struct {
	UsagePage         uint16
	ReportID          uint8
	IsAlias           uint8
	BitField          uint16
	LinkCollection    uint16
	LinkUsage         uint16
	LinkUsagePage     uint16
	IsRange           uint8
	IsStringRange     uint8
	IsDesignatorRange uint8
	IsAbsolute        uint8
	HasNull           uint8
	Reserved          uint8
	BitSize           uint16
	ReportCount       uint16
	Reserved2         [5]uint16
	UnitsExp          uint32
	Units             uint32
	LogicalMin        int32
	LogicalMax        int32
	PhysicalMin       int32
	PhysicalMax       int32
	Usage             uint16 // Range.UsageMin or NotRange.Usage
	Rest              [7]uint16
}

var (
	procGetValueCaps  = modHID.NewProc("HidP_GetValueCaps")
	procGetUsageValue = modHID.NewProc("HidP_GetUsageValue")
)

// preparsed holds a collection's report descriptor in Windows' parsed form.
type preparsed struct {
	pp   uintptr
	caps hidpCaps
}

func loadPreparsed(h windows.Handle) (*preparsed, bool) {
	p := &preparsed{}
	if r, _, _ := procGetPreparsedData.Call(uintptr(h), uintptr(unsafe.Pointer(&p.pp))); r == 0 {
		return nil, false
	}
	if st, _, _ := procGetCaps.Call(p.pp, uintptr(unsafe.Pointer(&p.caps))); st != hidpStatusSuccess {
		procFreePreparsedData.Call(p.pp)
		return nil, false
	}
	return p, true
}

func (p *preparsed) free() { procFreePreparsedData.Call(p.pp) }

func (p *preparsed) valueCaps() []ValueCap {
	var out []ValueCap
	for _, rt := range []struct {
		typ int
		n   uint16
	}{{InputReport, p.caps.NumberInputValueCaps}, {FeatureReport, p.caps.NumberFeatureValueCaps}} {
		if rt.n == 0 {
			continue
		}
		raw := make([]hidpValueCaps, rt.n)
		n := rt.n
		if st, _, _ := procGetValueCaps.Call(uintptr(rt.typ), uintptr(unsafe.Pointer(&raw[0])), uintptr(unsafe.Pointer(&n)), p.pp); st != hidpStatusSuccess {
			continue
		}
		for _, v := range raw[:n] {
			out = append(out, ValueCap{
				ReportType: rt.typ, ReportID: v.ReportID, UsagePage: v.UsagePage, Usage: v.Usage,
				LinkCollection: v.LinkCollection, BitSize: v.BitSize, ReportCount: v.ReportCount,
				LogicalMin: v.LogicalMin, LogicalMax: v.LogicalMax,
			})
		}
	}
	return out
}

// ValueCaps lists the input and feature value fields of a collection without opening it
// for reading or writing.
func ValueCaps(c Collection) []ValueCap {
	p16, err := windows.UTF16PtrFromString(c.Path)
	if err != nil {
		return nil
	}
	h, err := windows.CreateFile(p16, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)
	p, ok := loadPreparsed(h)
	if !ok {
		return nil
	}
	defer p.free()
	return p.valueCaps()
}

// ReadValue reads a report of the field's type and id (GetFeature / GetInputReport, both
// read-only requests) and extracts the field with HidP_GetUsageValue.
func (d *Device) ReadValue(v ValueCap) (uint32, error) {
	p, ok := loadPreparsed(d.h)
	if !ok {
		return 0, errors.New("hid: no preparsed data")
	}
	defer p.free()
	var buf []byte
	switch v.ReportType {
	case FeatureReport:
		buf = d.fbuf
		clear(buf)
		buf[0] = v.ReportID
		if r, _, err := procGetFeature.Call(uintptr(d.h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r == 0 {
			return 0, err
		}
	case InputReport:
		buf = d.rbuf
		clear(buf)
		buf[0] = v.ReportID
		if r, _, err := procGetInputReport.Call(uintptr(d.h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r == 0 {
			return 0, err
		}
	default:
		return 0, errors.New("hid: unsupported report type")
	}
	var val uint32
	st, _, _ := procGetUsageValue.Call(uintptr(v.ReportType), uintptr(v.UsagePage), uintptr(v.LinkCollection), uintptr(v.Usage),
		uintptr(unsafe.Pointer(&val)), p.pp, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if st != hidpStatusSuccess {
		return 0, errors.New("hid: usage value not in report")
	}
	return val, nil
}

// Unopenable lists HID interface paths that are present but could not be described
// (for --dump: such devices need another way in, or are held exclusively).
func Unopenable() []string {
	paths, err := windows.CM_Get_Device_Interface_List("", &hidGUID, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
	if err != nil {
		return nil
	}
	var bad []string
	for _, p := range paths {
		if _, ok := describe(p); !ok {
			bad = append(bad, p)
		}
	}
	return bad
}

func describe(path string) (Collection, bool) {
	c := Collection{Path: path, Interface: interfaceNumber(path)}
	p16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return c, false
	}
	h, err := windows.CreateFile(p16, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return c, false
	}
	defer windows.CloseHandle(h)

	attr := hiddAttributes{Size: uint32(unsafe.Sizeof(hiddAttributes{}))}
	if r, _, _ := procGetAttributes.Call(uintptr(h), uintptr(unsafe.Pointer(&attr))); r == 0 {
		return c, false
	}
	c.VendorID, c.ProductID, c.Release = attr.VendorID, attr.ProductID, attr.VersionNumber

	var pp uintptr
	if r, _, _ := procGetPreparsedData.Call(uintptr(h), uintptr(unsafe.Pointer(&pp))); r != 0 {
		var caps hidpCaps
		if st, _, _ := procGetCaps.Call(pp, uintptr(unsafe.Pointer(&caps))); st == hidpStatusSuccess {
			c.UsagePage, c.Usage = caps.UsagePage, caps.Usage
			c.InputLen, c.OutputLen, c.FeatureLen = caps.InputReportByteLength, caps.OutputReportByteLength, caps.FeatureReportByteLength
		}
		procFreePreparsedData.Call(pp)
	}

	var name [127]uint16
	if r, _, _ := procGetProductString.Call(uintptr(h), uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)*2)); r != 0 {
		c.Product = windows.UTF16ToString(name[:])
	}
	return c, true
}

func interfaceNumber(path string) int {
	lp := strings.ToLower(path)
	i := strings.Index(lp, "&mi_")
	if i < 0 || i+6 > len(lp) {
		return -1
	}
	n := 0
	for _, ch := range lp[i+4 : i+6] {
		switch {
		case ch >= '0' && ch <= '9':
			n = n*16 + int(ch-'0')
		case ch >= 'a' && ch <= 'f':
			n = n*16 + int(ch-'a'+10)
		default:
			return -1
		}
	}
	return n
}

// Device is an open HID collection. It is opened for overlapped I/O so reads can time out.
// Buffers and the OVERLAPPED block live on the heap inside Device, because Windows writes to
// them asynchronously and Go is free to move stack memory.
type Device struct {
	c    Collection
	h    windows.Handle
	ev   windows.Handle
	ov   *windows.Overlapped
	rbuf []byte
	wbuf []byte
	fbuf []byte
}

// Open opens a collection for reading and writing.
func Open(c Collection) (*Device, error) {
	p16, err := windows.UTF16PtrFromString(c.Path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p16, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, err
	}
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	d := &Device{c: c, h: h, ev: ev, ov: new(windows.Overlapped)}
	d.rbuf = make([]byte, max(int(c.InputLen), 1))
	d.wbuf = make([]byte, max(int(c.OutputLen), 1))
	d.fbuf = make([]byte, max(int(c.FeatureLen), 1))
	return d, nil
}

// Collection returns what the device was opened from.
func (d *Device) Collection() Collection { return d.c }

// Close releases the handle.
func (d *Device) Close() {
	windows.CloseHandle(d.ev)
	windows.CloseHandle(d.h)
}

// Flush drops input reports Windows has queued but nobody has read yet.
func (d *Device) Flush() { procFlushQueue.Call(uintptr(d.h)) }

// GetFeature reads a feature report. report[0] must hold the report id (0 when the device
// uses none). The result, including the report id byte, is copied back into report.
func (d *Device) GetFeature(report []byte) (int, error) {
	clear(d.fbuf)
	copy(d.fbuf, report)
	r, _, err := procGetFeature.Call(uintptr(d.h), uintptr(unsafe.Pointer(&d.fbuf[0])), uintptr(len(d.fbuf)))
	if r == 0 {
		return 0, err
	}
	return copy(report, d.fbuf), nil
}

// SetFeature sends a feature report (report[0] = report id), padded to the device's size.
func (d *Device) SetFeature(report []byte) error {
	clear(d.fbuf)
	copy(d.fbuf, report)
	if r, _, err := procSetFeature.Call(uintptr(d.h), uintptr(unsafe.Pointer(&d.fbuf[0])), uintptr(len(d.fbuf))); r == 0 {
		return err
	}
	return nil
}

// GetInputReport polls an input report over the control pipe (HidD_GetInputReport).
func (d *Device) GetInputReport(report []byte) (int, error) {
	clear(d.rbuf)
	copy(d.rbuf, report)
	if r, _, err := procGetInputReport.Call(uintptr(d.h), uintptr(unsafe.Pointer(&d.rbuf[0])), uintptr(len(d.rbuf))); r == 0 {
		return 0, err
	}
	return copy(report, d.rbuf), nil
}

// SetOutputReport sends an output report over the control pipe (HidD_SetOutputReport).
func (d *Device) SetOutputReport(report []byte) error {
	clear(d.wbuf)
	copy(d.wbuf, report)
	if r, _, err := procSetOutputReport.Call(uintptr(d.h), uintptr(unsafe.Pointer(&d.wbuf[0])), uintptr(len(d.wbuf))); r == 0 {
		return err
	}
	return nil
}

// Write sends an output report over the interrupt pipe, padded to the device's size.
func (d *Device) Write(report []byte, timeout time.Duration) error {
	clear(d.wbuf)
	copy(d.wbuf, report)
	_, err := d.overlapped(false, d.wbuf, timeout)
	return err
}

// Read waits up to timeout for the next input report and copies it into buf.
func (d *Device) Read(buf []byte, timeout time.Duration) (int, error) {
	n, err := d.overlapped(true, d.rbuf, timeout)
	if err != nil {
		return 0, err
	}
	return copy(buf, d.rbuf[:n]), nil
}

func (d *Device) overlapped(read bool, buf []byte, timeout time.Duration) (int, error) {
	*d.ov = windows.Overlapped{HEvent: d.ev}
	windows.ResetEvent(d.ev)
	var n uint32
	var err error
	if read {
		err = windows.ReadFile(d.h, buf, &n, d.ov)
	} else {
		err = windows.WriteFile(d.h, buf, &n, d.ov)
	}
	if err != nil && err != windows.ERROR_IO_PENDING {
		return 0, err
	}
	ev, _ := windows.WaitForSingleObject(d.ev, uint32(timeout.Milliseconds()))
	if ev != windows.WAIT_OBJECT_0 {
		windows.CancelIoEx(d.h, d.ov)
		windows.GetOverlappedResult(d.h, d.ov, &n, true) // wait for the cancel so buf is free again
		return 0, ErrTimeout
	}
	if err := windows.GetOverlappedResult(d.h, d.ov, &n, false); err != nil {
		return 0, err
	}
	return int(n), nil
}
