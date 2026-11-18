package tray

// Taskbar text: a transparent layered child window placed inside the taskbar (Shell_TrayWnd),
// just left of the notification area, showing "🎧 40%  🖱 85%" with Windows' own icon font.
// It is drawn with UpdateLayeredWindow from a premultiplied 32-bit bitmap, so the taskbar
// (including transparent / acrylic styles) shows through.

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procFindWindow          = user32.NewProc("FindWindowW")
	procFindWindowEx        = user32.NewProc("FindWindowExW")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procIsWindow            = user32.NewProc("IsWindow")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procSetTimer            = user32.NewProc("SetTimer")
	procGetDpiForWindow     = user32.NewProc("GetDpiForWindow")
)

const (
	wsChild          = 0x40000000
	wsVisible        = 0x10000000
	wsClipSiblings   = 0x04000000
	wsExLayered      = 0x00080000
	wsExNoActivate   = 0x08000000
	swpNoActivate    = 0x0010
	swpNoZOrder      = 0x0004
	swpShowWindow    = 0x0040
	ulwAlpha         = 0x2
	wmTimer          = 0x0113
	wmMouseActivate  = 0x0021
	maNoActivate     = 3
	barTimerID       = 1
	barTimerInterval = 2000
)

// Glyphs from Segoe Fluent Icons (Windows 11) / Segoe MDL2 Assets (Windows 10).
const (
	GlyphMouse    = ''
	GlyphHeadset  = ''
	GlyphKeyboard = ''
	GlyphGamepad  = ''
	GlyphDevice   = ''
	glyphCharging = ''
)

// BarItem is one device on the taskbar: a glyph and its level text. Key identifies the
// device in menu callbacks (right-clicking an item passes its key to Config.Menu).
type BarItem struct {
	Key      string
	Glyph    rune
	Text     string
	Style    Style
	Charging bool
	// Accent, when set, colours the glyph and a thin level bar under the text (brand colours).
	Accent *Accent
	// Fill is the level drawn in the bar, 0..100; -1 draws no bar.
	Fill int
	// Tooltip is shown when the mouse hovers the item.
	Tooltip string
	// Caption is a tiny label (up to 3 characters) drawn at the glyph's lower right, to tell
	// two devices of the same kind apart.
	Caption string
}

// Accent is a colour with a shade for dark and for light taskbars.
type Accent struct{ Dark, Light RGB }

func (a *Accent) pick(light bool) RGB {
	if light {
		return a.Light
	}
	return a.Dark
}

// span is the horizontal extent of one item in the taskbar window (client pixels).
type span struct{ x0, x1 int }

type bar struct {
	hwnd               uintptr
	host               taskbarHost // the taskbar this window lives in
	items              []BarItem
	spans              []span
	lastTB, lastNotify rect    // geometry used for the last draw
	pointerButton      int     // button pressed in the current pointer gesture: 1 left, 2 right
	vertical           bool    // column layout (spans hold y ranges)
	tip                uintptr // tooltip control
	hoverKey           string  // item the tooltip currently describes
	hoverIdx           int     // item under the mouse (highlighted), -1 none
	pressed            bool    // a mouse button is down on the hovered item
	tracking           bool    // TrackMouseEvent armed for WM_MOUSELEAVE
}

// keyAt returns the key of the item under the client point (the nearest item when between
// two): x on a horizontal taskbar, y on a vertical one.
func (b *bar) keyAt(x, y int) string {
	i := b.indexAt(x, y)
	if i < 0 {
		return ""
	}
	return b.items[i].Key
}

// indexAt returns the index of the item nearest to the client point, or -1.
func (b *bar) indexAt(x, y int) int {
	if b.vertical {
		x = y
	}
	best, bestDist := -1, 1<<30
	for i, s := range b.spans {
		d := 0
		switch {
		case x < s.x0:
			d = s.x0 - x
		case x > s.x1:
			d = x - s.x1
		}
		if d < bestDist {
			best, bestDist = i, d
		}
	}
	if best < 0 || best >= len(b.items) {
		return -1
	}
	return best
}

// SetTaskbar shows items as text on the taskbar(s); nil removes it. allMonitors also puts a
// copy on every secondary monitor's taskbar.
func (t *Tray) SetTaskbar(items []BarItem, allMonitors bool) {
	t.mu.Lock()
	t.wantBar = append([]BarItem(nil), items...)
	t.barAll = allMonitors
	t.barSet = true
	t.mu.Unlock()
	procPostMessage.Call(t.hwnd, wmApply, 0, 0)
}

// TaskbarAvailable reports whether there is a main taskbar to host the text (horizontal
// taskbars get a row, vertical ones a column).
func TaskbarAvailable() bool {
	for _, h := range taskbarHosts(false) {
		_, _, _, ok := h.geometry()
		return ok
	}
	return false
}

// taskbarHost is one taskbar window: the main one (Shell_TrayWnd) or a secondary monitor's
// (Shell_SecondaryTrayWnd).
type taskbarHost struct {
	hwnd    uintptr
	primary bool
}

// taskbarHosts lists the main taskbar and, when all is set, every secondary taskbar.
func taskbarHosts(all bool) []taskbarHost {
	var out []taskbarHost
	cls, _ := windows.UTF16PtrFromString("Shell_TrayWnd")
	if h, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(cls)), 0); h != 0 {
		out = append(out, taskbarHost{h, true})
	}
	if !all {
		return out
	}
	sec, _ := windows.UTF16PtrFromString("Shell_SecondaryTrayWnd")
	for h := uintptr(0); ; {
		h, _, _ = procFindWindowEx.Call(0, h, uintptr(unsafe.Pointer(sec)), 0)
		if h == 0 {
			break
		}
		out = append(out, taskbarHost{h, false})
	}
	return out
}

// secondaryClockReserve: a secondary taskbar has no notification-area window (its clock is
// drawn inside the XAML layer), so this much room is left free at its right end, at 96 DPI.
const secondaryClockReserve = 120

// geometry returns the taskbar rect and the rect our text must stay clear of (screen
// coordinates): left of it on a horizontal taskbar, above it on a vertical one.
func (h taskbarHost) geometry() (tb, notify rect, vertical, ok bool) {
	if r, _, _ := procIsWindow.Call(h.hwnd); r == 0 {
		return
	}
	procGetWindowRect.Call(h.hwnd, uintptr(unsafe.Pointer(&tb)))
	vertical = tb.Right-tb.Left <= tb.Bottom-tb.Top
	if h.primary {
		ncls, _ := windows.UTF16PtrFromString("TrayNotifyWnd")
		n, _, _ := procFindWindowEx.Call(h.hwnd, 0, uintptr(unsafe.Pointer(ncls)), 0)
		if n == 0 {
			return
		}
		procGetWindowRect.Call(n, uintptr(unsafe.Pointer(&notify)))
	} else {
		dpi, _, _ := procGetDpiForWindow.Call(h.hwnd)
		if dpi == 0 {
			dpi = 96
		}
		reserve := int32(secondaryClockReserve * dpi / 96)
		notify = rect{tb.Right - reserve, tb.Top, tb.Right, tb.Bottom}
		if vertical {
			notify = rect{tb.Left, tb.Bottom - reserve, tb.Right, tb.Bottom}
		}
	}
	ok = true
	return
}

var barClassRegistered bool

// applyBar creates, redraws, moves or removes the taskbar windows. UI thread only.
func (t *Tray) applyBar() {
	t.mu.Lock()
	items, set, all := t.wantBar, t.barSet, t.barAll
	t.mu.Unlock()
	if !set {
		return
	}
	if t.bars == nil {
		t.bars = map[uintptr]*bar{}
	}
	wanted := map[uintptr]bool{}
	if len(items) > 0 {
		for _, h := range taskbarHosts(all) {
			tb, notify, vertical, ok := h.geometry()
			if !ok {
				continue
			}
			wanted[h.hwnd] = true
			b := t.bars[h.hwnd]
			if b == nil {
				b = &bar{host: h, hoverIdx: -1}
				t.bars[h.hwnd] = b
			}
			b.vertical = vertical
			b.items = items
			if b.hwnd != 0 {
				if r, _, _ := procIsWindow.Call(b.hwnd); r == 0 {
					b.hwnd = 0 // Explorer restarted and took our child window with it
				}
			}
			if b.hwnd == 0 && !b.create() {
				continue
			}
			b.draw(tb, notify)
		}
	}
	for k, b := range t.bars {
		if !wanted[k] {
			b.destroy()
			delete(t.bars, k)
		}
	}
}

// barFor finds the bar that owns a window handle (for input handling).
func (t *Tray) barFor(hwnd uintptr) *bar {
	for _, b := range t.bars {
		if b.hwnd == hwnd {
			return b
		}
	}
	return nil
}

func (b *bar) create() bool {
	inst, _ := getModuleHandle()
	cls, _ := windows.UTF16PtrFromString("GoBatteryMonitorBar")
	if !barClassRegistered {
		wc := wndClassEx{WndProc: syscall.NewCallback(barProc), Instance: inst, ClassName: cls}
		wc.Size = uint32(unsafe.Sizeof(wc))
		procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		barClassRegistered = true
	}
	h, _, _ := procCreateWindowEx.Call(wsExLayered|wsExNoActivate, uintptr(unsafe.Pointer(cls)), 0,
		wsChild|wsVisible|wsClipSiblings, 0, 0, 1, 1, b.host.hwnd, 0, inst, 0)
	b.hwnd = h
	if h != 0 {
		b.createTooltip()
	}
	return h != 0
}

var procGetWindow = user32.NewProc("GetWindow")

// onTop reports whether our window is the topmost child of the taskbar (above the XAML layer).
func (b *bar) onTop() bool {
	const gwChild = 5
	first, _, _ := procGetWindow.Call(b.host.hwnd, gwChild)
	return first == b.hwnd
}

func (b *bar) destroy() {
	// The tooltip's owner is the taskbar itself (a child window cannot own a popup), so it
	// is not destroyed with our window: destroy it explicitly.
	if b.tip != 0 {
		procDestroyWindow.Call(b.tip)
	}
	if b.hwnd != 0 {
		procDestroyWindow.Call(b.hwnd)
		b.hwnd = 0
	}
	b.tip, b.hoverKey = 0, ""
	b.lastTB, b.lastNotify = rect{}, rect{}
}

// draw renders the items, sizes the window to fit and places it left of (horizontal) or
// above (vertical) the notification area.
func (b *bar) draw(tb, notify rect) {
	dpi, _, _ := procGetDpiForWindow.Call(b.hwnd)
	if dpi == 0 {
		dpi = 96
	}
	margin := int(4 * dpi / 96)
	var px []byte
	var width, height, x, y int
	var spans []span
	if b.vertical {
		width = int(tb.Right - tb.Left)
		px, height, spans = renderColumn(b.items, width, int(dpi), taskbarIsLight())
		y = max(0, int(notify.Top-tb.Top)-height-margin)
	} else {
		height = int(tb.Bottom - tb.Top)
		px, width, spans = renderBar(b.items, height, int(dpi), taskbarIsLight())
		x = max(0, int(notify.Left-tb.Left)-width-margin)
	}
	if px == nil || width == 0 || height == 0 {
		return
	}
	if b.hoverIdx >= 0 && b.hoverIdx < len(spans) {
		highlight(px, width, height, spans[b.hoverIdx], b.vertical, int(dpi), taskbarIsLight(), b.pressed)
	}
	// HWND_TOP (0): on Windows 11 the taskbar content is a XAML layer
	// (DesktopWindowContentBridge) that covers the whole taskbar; we must sit above it.
	procSetWindowPos.Call(b.hwnd, 0, uintptr(x), uintptr(y), uintptr(width), uintptr(height), swpNoActivate|swpShowWindow)
	b.lastTB, b.lastNotify, b.spans = tb, notify, spans
	b.resizeTooltip()
	b.hoverKey = invalidKey // items may have changed: refresh the text on the next mouse move

	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(mem)
	bi := bitmapInfoHeader{Width: int32(width), Height: -int32(height), Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	bmp, _, _ := procCreateDIBSection.Call(mem, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 {
		return
	}
	defer procDeleteObject.Call(bmp)
	copy(unsafe.Slice((*byte)(bits), len(px)), px)
	old, _, _ := procSelectObject.Call(mem, bmp)
	defer procSelectObject.Call(mem, old)

	size := struct{ CX, CY int32 }{int32(width), int32(height)}
	src := struct{ X, Y int32 }{}
	blend := [4]byte{0, 0, 255, 1} // AC_SRC_OVER, 0, opaque, AC_SRC_ALPHA
	procUpdateLayeredWindow.Call(b.hwnd, screen, 0, uintptr(unsafe.Pointer(&size)), mem,
		uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
}

const (
	wmContextMenu     = 0x007B
	wmPointerDown     = 0x0246
	wmPointerUp       = 0x0247
	wmPointerActivate = 0x024B
	paNoActivate      = 3
	pointerFirstBtn   = 0x10 // POINTER_MESSAGE_FLAG_FIRSTBUTTON (high word of wParam)
	pointerSecondBtn  = 0x20 // POINTER_MESSAGE_FLAG_SECONDBUTTON
)

var procScreenToClient = user32.NewProc("ScreenToClient")

// barProc handles input on the taskbar text. On Windows 11 25H2 clicks arrive as ordinary
// WM_xBUTTONDOWN/UP (seen in the debug log). Our window shares Explorer's input queue, so
// WM_POINTER* messages are handled as well in case a build runs that queue in pointer mode.
func barProc(hwnd uintptr, m uint32, w, l uintptr) uintptr {
	t := current
	debugLog(m, w, l)
	switch m {
	case wmMouseActivate:
		return maNoActivate // never steal focus from the taskbar
	case wmPointerActivate:
		return paNoActivate
	case wmPointerDown:
		if t != nil {
			flags := uint32(w >> 16)
			switch {
			case flags&pointerSecondBtn != 0:
				if b := t.barFor(hwnd); b != nil {
					b.pointerButton = 2
				}
			case flags&pointerFirstBtn != 0:
				if b := t.barFor(hwnd); b != nil {
					b.pointerButton = 1
				}
			}
		}
		return 0
	case wmPointerUp:
		if t != nil {
			btn := 0
			if b := t.barFor(hwnd); b != nil {
				btn, b.pointerButton = b.pointerButton, 0
			}
			pt := struct{ X, Y int32 }{int32(int16(l)), int32(int16(l >> 16))} // screen coords
			procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
			t.barClick(hwnd, btn, int(pt.X), int(pt.Y))
		}
		return 0
	case wmMouseLeave:
		if t != nil {
			if b := t.barFor(hwnd); b != nil {
				b.leave()
			}
		}
		return 0
	case wmLButtonDown, wmRButtonDown:
		if t != nil {
			if b := t.barFor(hwnd); b != nil {
				b.press(true)
			}
		}
		return 0
	case wmMouseMove:
		if t != nil {
			if b := t.barFor(hwnd); b != nil {
				b.hover(int(int16(l)), int(int16(l>>16)))
			}
		}
		return 0
	case wmLButtonUp:
		if t != nil {
			t.barClick(hwnd, 1, int(int16(l)), int(int16(l>>16)))
		}
		return 0
	case wmRButtonUp:
		if t != nil {
			t.barClick(hwnd, 2, int(int16(l)), int(int16(l>>16)))
		}
		return 0
	case wmContextMenu:
		if t != nil {
			t.showMenuFor("")
		}
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(m), w, l)
	return r
}

func (t *Tray) barClick(hwnd uintptr, button, x, y int) {
	if b := t.barFor(hwnd); b != nil {
		b.press(false)
	}
	switch button {
	case 1:
		call(t.cfg.OnLeftClick)
	case 2:
		key := ""
		if b := t.barFor(hwnd); b != nil {
			key = b.keyAt(x, y)
		}
		t.showMenuFor(key)
	}
}

// debugLog appends input messages to %TEMP%\batterymon-debug.log when the environment
// variable BATTERYMON_DEBUG is set (to diagnose taskbar input on new Windows builds).
func debugLog(m uint32, w, l uintptr) {
	if !debugEnabled {
		return
	}
	if (m >= 0x0200 && m <= 0x020E) || (m >= 0x0240 && m <= 0x0251) || m == wmContextMenu || m == wmMouseActivate {
		if f, err := os.OpenFile(filepath.Join(os.TempDir(), "batterymon-debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintf(f, "%s msg=%#04x w=%#x l=%#x\n", time.Now().Format("15:04:05.000"), m, w, l)
			f.Close()
		}
	}
}

var debugEnabled = os.Getenv("BATTERYMON_DEBUG") != ""

// renderBar draws the items into premultiplied BGRA pixels of the given height and returns
// them with the width used. Text is white-on-black first; brightness becomes alpha, and each
// item gets its own colour. The background keeps alpha 1 so the whole strip is clickable.
func renderBar(items []BarItem, height, dpi int, light bool) ([]byte, int, []span) {
	hdc, _, _ := procCreateCompatibleDC.Call(0)
	if hdc == 0 {
		return nil, 0, nil
	}
	defer procDeleteDC.Call(hdc)

	textPx := max(height*30/100, 11*dpi/96)
	textFont := createFont("Segoe UI Semibold", textPx, 600)
	glyphFont := createFont("Segoe Fluent Icons", textPx+textPx/6, 400)
	captionFont := createFont("Segoe UI Semibold", max(textPx*55/100, 8*dpi/96), 600)
	defer procDeleteObject.Call(captionFont)
	if !fontHasGlyphs(hdc, glyphFont) {
		procDeleteObject.Call(glyphFont)
		glyphFont = createFont("Segoe MDL2 Assets", textPx+textPx/6, 400)
	}
	defer procDeleteObject.Call(textFont)
	defer procDeleteObject.Call(glyphFont)

	type part struct {
		s    []uint16
		font uintptr
		x, w int
		item int
	}
	gap := textPx / 4
	itemGap := textPx
	var parts []part
	spans := make([]span, len(items))
	edge := max(gap, 6*dpi/96) // room for the hover plate at both ends
	x := edge
	for i, it := range items {
		spans[i].x0 = x
		add := func(s string, f uintptr, after int) {
			u, _ := windows.UTF16FromString(s)
			w := textWidth(hdc, f, u)
			parts = append(parts, part{u, f, x, w, i})
			x += w + after
		}
		if it.Caption != "" {
			add(string(it.Glyph), glyphFont, 0)
			add(it.Caption, captionFont, gap) // drawn low, like a subscript
		} else {
			add(string(it.Glyph), glyphFont, gap)
		}
		if it.Charging {
			add(it.Text, textFont, 1)
			add(string(glyphCharging), glyphFont, 0)
		} else {
			add(it.Text, textFont, 0)
		}
		if i < len(items)-1 {
			x += itemGap
		}
	}
	width := x + edge
	for _, p := range parts {
		spans[p.item].x1 = max(spans[p.item].x1, p.x+p.w)
	}

	bi := bitmapInfoHeader{Width: int32(width), Height: -int32(height), Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	bmp, _, _ := procCreateDIBSection.Call(hdc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 {
		return nil, 0, nil
	}
	defer procDeleteObject.Call(bmp)
	old, _, _ := procSelectObject.Call(hdc, bmp)
	procSetTextColor.Call(hdc, 0x00FFFFFF)
	procSetBkMode.Call(hdc, transparentBk)
	for _, p := range parts {
		procSelectObject.Call(hdc, p.font)
		r := rect{int32(p.x), 0, int32(p.x + p.w + 2), int32(height)}
		flags := uintptr(dtVCenter | dtSingleLine | dtNoPrefix)
		if p.font == captionFont {
			r.Bottom = int32(height/2 + textPx*3/4) // sit on the glyph's lower edge
			flags = dtBottom | dtSingleLine | dtNoPrefix
		}
		procDrawText.Call(hdc, uintptr(unsafe.Pointer(&p.s[0])), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags)
	}
	procGdiFlush.Call()
	procSelectObject.Call(hdc, old)

	// Colour per column: each item's span gets its style colour.
	colAt := make([]RGB, width)
	def := palette(Normal, light)
	for i := range colAt {
		colAt[i] = def
	}
	for _, p := range parts {
		it := items[p.item]
		c := palette(it.Style, light)
		switch {
		case it.Charging && p.font == glyphFont && p.s[0] == glyphCharging:
			c = palette(Charging, light)
		case it.Accent != nil && it.Style == Normal && (p.font == glyphFont || p.font == captionFont):
			c = it.Accent.pick(light) // brand-coloured device glyph
		}
		for cx := max(0, p.x-1); cx < min(width, p.x+p.w+2); cx++ {
			colAt[cx] = c
		}
	}
	src := unsafe.Slice((*[4]uint8)(bits), width*height)
	out := make([]byte, width*height*4)
	for i, s := range src {
		a := max(s[0], s[1], s[2])
		c := colAt[i%width]
		if a == 0 {
			out[i*4+3] = 1 // invisible but hit-testable
			continue
		}
		out[i*4], out[i*4+1], out[i*4+2], out[i*4+3] = mul(c.B, a), mul(c.G, a), mul(c.R, a), a
	}

	// Level bars: a faint track under each accented item's text, filled to its level.
	thick := max(2, 2*dpi/96)
	y0 := min(height-thick, height/2+textPx*62/100)
	for _, p := range parts {
		it := items[p.item]
		if it.Accent == nil || it.Fill < 0 || p.font != textFont {
			continue
		}
		c := it.Accent.pick(light)
		if it.Style != Normal {
			c = palette(it.Style, light)
		}
		filled := p.x + p.w*min(it.Fill, 100)/100
		for y := y0; y < y0+thick; y++ {
			for x := p.x; x < min(width, p.x+p.w); x++ {
				a := byte(70)
				if x < filled {
					a = 255
				}
				i := (y*width + x) * 4
				out[i], out[i+1], out[i+2], out[i+3] = mul(c.B, a), mul(c.G, a), mul(c.R, a), a
			}
		}
	}
	return out, width, spans
}

// PreviewBar renders taskbar text as an image (tests and tools/iconpreview).
func PreviewBar(items []BarItem, height int, light bool) *image.NRGBA {
	px, w, _ := renderBar(items, height, 96, light)
	img := image.NewNRGBA(image.Rect(0, 0, w, height))
	for i := 0; i+3 < len(px); i += 4 {
		a := px[i+3]
		if a <= 1 {
			continue
		}
		un := func(v byte) byte { return byte(min(255, int(v)*255/int(a))) }
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = un(px[i+2]), un(px[i+1]), un(px[i]), a
	}
	return img
}

func createFont(face string, px, weight int) uintptr {
	f16, _ := windows.UTF16PtrFromString(face)
	f, _, _ := procCreateFont.Call(^uintptr(px-1), 0, 0, 0, uintptr(weight), 0, 0, 0, 1 /*DEFAULT_CHARSET*/, 0, 0,
		antialiasQuality, 0, uintptr(unsafe.Pointer(f16)))
	return f
}

func textWidth(hdc, font uintptr, s []uint16) int {
	old, _, _ := procSelectObject.Call(hdc, font)
	defer procSelectObject.Call(hdc, old)
	var sz struct{ CX, CY int32 }
	procGetTextExtent.Call(hdc, uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)-1), uintptr(unsafe.Pointer(&sz)))
	return int(sz.CX)
}

var procGetGlyphIndices = gdi32.NewProc("GetGlyphIndicesW")

// fontHasGlyphs checks that the icon font really exists (GDI silently substitutes another
// font, which has no glyph for our private-use code points).
func fontHasGlyphs(hdc, font uintptr) bool {
	old, _, _ := procSelectObject.Call(hdc, font)
	defer procSelectObject.Call(hdc, old)
	s := []uint16{GlyphMouse, 0}
	var idx [1]uint16
	const ggiMarkNonexistingGlyphs = 1
	procGetGlyphIndices.Call(hdc, uintptr(unsafe.Pointer(&s[0])), 1, uintptr(unsafe.Pointer(&idx[0])), ggiMarkNonexistingGlyphs)
	return idx[0] != 0xFFFF
}
