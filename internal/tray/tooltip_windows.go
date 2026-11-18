package tray

// Hover tooltips on the taskbar text: one Windows tooltip control per taskbar window, whose
// text follows the device under the mouse.

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	comctl32                  = windows.NewLazySystemDLL("comctl32.dll")
	procInitCommonControlsEx  = comctl32.NewProc("InitCommonControlsEx")
	procSendMessage           = user32.NewProc("SendMessageW")
	procGetClientRect         = user32.NewProc("GetClientRect")
	commonControlsInitialised bool
)

const (
	wmMouseMove       = 0x0200
	wmMouseLeave      = 0x02A3
	ttmAddToolW       = 0x0432
	ttmNewToolRectW   = 0x0434
	ttmUpdateTipTextW = 0x0439
	ttmSetMaxTipWidth = 0x0418
	ttmSetDelayTime   = 0x0403
	ttfSubclass       = 0x0010
	ttsAlwaysTip      = 0x01
	ttsNoPrefix       = 0x02
	wsPopup           = 0x80000000
	wsExTopmost       = 0x00000008
	cwUseDefault      = 0x80000000
	iccWin95Classes   = 0x000000FF
	ttdtInitial       = 3
	ttdtAutoPop       = 2
)

// TOOLINFOW (commctrl.h), 72 bytes on 64-bit.
type toolInfo struct {
	Size     uint32
	Flags    uint32
	Hwnd     uintptr
	ID       uintptr
	Rect     rect
	Inst     uintptr
	Text     *uint16
	LParam   uintptr
	Reserved uintptr
}

func initCommonControls() {
	if commonControlsInitialised {
		return
	}
	icc := struct{ Size, ICC uint32 }{8, iccWin95Classes}
	procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))
	commonControlsInitialised = true
}

// createTooltip attaches a tooltip control to the bar window (one tool covering it all).
func (b *bar) createTooltip() {
	initCommonControls()
	cls, _ := windows.UTF16PtrFromString("tooltips_class32")
	inst, _ := getModuleHandle()
	h, _, _ := procCreateWindowEx.Call(wsExTopmost, uintptr(unsafe.Pointer(cls)), 0,
		wsPopup|ttsNoPrefix|ttsAlwaysTip, cwUseDefault, cwUseDefault, cwUseDefault, cwUseDefault,
		b.hwnd, 0, inst, 0)
	if h == 0 {
		return
	}
	b.tip = h
	procSendMessage.Call(h, ttmSetMaxTipWidth, 0, 480) // allow multi-line text
	procSendMessage.Call(h, ttmSetDelayTime, ttdtInitial, 300)
	procSendMessage.Call(h, ttmSetDelayTime, ttdtAutoPop, 15000)
	empty, _ := windows.UTF16PtrFromString(" ")
	ti := b.toolInfo(empty)
	procSendMessage.Call(h, ttmAddToolW, 0, uintptr(unsafe.Pointer(&ti)))
}

func (b *bar) toolInfo(text *uint16) toolInfo {
	ti := toolInfo{Flags: ttfSubclass, Hwnd: b.hwnd, ID: 1, Text: text}
	ti.Size = uint32(unsafe.Sizeof(ti))
	procGetClientRect.Call(b.hwnd, uintptr(unsafe.Pointer(&ti.Rect)))
	return ti
}

// resizeTooltip keeps the tool rectangle equal to the window after a redraw.
func (b *bar) resizeTooltip() {
	if b.tip == 0 {
		return
	}
	ti := b.toolInfo(nil)
	procSendMessage.Call(b.tip, ttmNewToolRectW, 0, uintptr(unsafe.Pointer(&ti)))
}

// hover updates the tooltip text to the item under the client point.
func (b *bar) hover(x, y int) {
	if b.tip == 0 {
		return
	}
	key := b.keyAt(x, y)
	if key == b.hoverKey {
		return
	}
	b.hoverKey = key
	text := " "
	for _, it := range b.items {
		if it.Key == key && it.Tooltip != "" {
			text = it.Tooltip
		}
	}
	t16, _ := windows.UTF16PtrFromString(text)
	ti := b.toolInfo(t16)
	procSendMessage.Call(b.tip, ttmUpdateTipTextW, 0, uintptr(unsafe.Pointer(&ti))) // the control copies the text
}

// invalidKey never matches a device key, so the next mouse move refreshes the tooltip.
const invalidKey = "\x00none"
