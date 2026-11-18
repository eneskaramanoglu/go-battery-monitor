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
	wmLButtonDown     = 0x0201
	wmRButtonDown     = 0x0204
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

// hover highlights the item under the client point and points the tooltip at it.
func (b *bar) hover(x, y int) {
	if !b.tracking {
		tme := trackMouseEvent{Flags: tmeLeave, Hwnd: b.hwnd}
		tme.Size = uint32(unsafe.Sizeof(tme))
		procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
		b.tracking = true
	}
	if i := b.indexAt(x, y); i != b.hoverIdx {
		b.hoverIdx, b.pressed = i, false
		b.redraw()
	}
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

var procTrackMouseEvent = user32.NewProc("TrackMouseEvent")

const tmeLeave = 0x00000002

// TRACKMOUSEEVENT (winuser.h).
type trackMouseEvent struct {
	Size      uint32
	Flags     uint32
	Hwnd      uintptr
	HoverTime uint32
}

// leave removes the highlight when the mouse leaves the window.
func (b *bar) leave() {
	b.tracking = false
	if b.hoverIdx != -1 || b.pressed {
		b.hoverIdx, b.pressed = -1, false
		b.redraw()
	}
	b.hoverKey = invalidKey
}

// press shows the pressed state while a button is down on an item.
func (b *bar) press(down bool) {
	if b.pressed != down && b.hoverIdx >= 0 {
		b.pressed = down
		b.redraw()
	}
}

// redraw re-renders with the geometry of the last draw (hover / press changes).
func (b *bar) redraw() {
	if b.hwnd != 0 && b.lastTB != (rect{}) {
		key := b.hoverKey
		b.draw(b.lastTB, b.lastNotify)
		b.hoverKey = key // the items did not change: keep the tooltip as it is
	}
}

// highlight paints a rounded, translucent plate behind one item (premultiplied BGRA), like
// the hover state of taskbar buttons: white on dark taskbars, black on light ones; stronger
// while pressed.
func highlight(px []byte, width, height int, s span, vertical bool, dpi int, light, pressed bool) {
	pad := 4 * dpi / 96
	r := 4 * dpi / 96
	x0, x1, y0, y1 := s.x0-pad, s.x1+pad, height*16/100, height-height*16/100
	if vertical {
		x0, x1, y0, y1 = pad, width-pad, s.x0-pad, s.x1+pad
	}
	x0, y0 = max(0, x0), max(0, y0)
	x1, y1 = min(width, x1), min(height, y1)
	c := byte(255)
	ab := 30
	if light {
		c, ab = 0, 18
	}
	if pressed {
		ab = ab * 3 / 2
	}
	inCorner := func(x, y int) bool {
		cx, cy := -1, -1
		switch {
		case x < x0+r && y < y0+r:
			cx, cy = x0+r, y0+r
		case x >= x1-r && y < y0+r:
			cx, cy = x1-r-1, y0+r
		case x < x0+r && y >= y1-r:
			cx, cy = x0+r, y1-r-1
		case x >= x1-r && y >= y1-r:
			cx, cy = x1-r-1, y1-r-1
		}
		return cx >= 0 && (x-cx)*(x-cx)+(y-cy)*(y-cy) > r*r
	}
	bg := byte(int(c) * ab / 255) // premultiplied channel
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if inCorner(x, y) {
				continue
			}
			i := (y*width + x) * 4
			a := int(px[i+3])
			if a <= 1 {
				px[i], px[i+1], px[i+2], px[i+3] = bg, bg, bg, byte(ab)
				continue
			}
			// existing (text) pixel over the plate
			k := 255 - a
			for j := 0; j < 3; j++ {
				px[i+j] = byte(int(px[i+j]) + int(bg)*k/255)
			}
			px[i+3] = byte(a + ab*k/255)
		}
	}
}
