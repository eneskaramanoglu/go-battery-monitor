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
}

// span is the horizontal extent of one item in the taskbar window (client pixels).
type span struct{ x0, x1 int }

type bar struct {
	hwnd               uintptr
	items              []BarItem
	spans              []span
	lastTB, lastNotify rect // geometry used for the last draw
	pointerButton      int  // button pressed in the current pointer gesture: 1 left, 2 right
}

// keyAt returns the key of the item under client x (the nearest item when between two).
func (b *bar) keyAt(x int) string {
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
		return ""
	}
	return b.items[best].Key
}

// SetTaskbar shows items as text on the taskbar; nil removes it. It reports nothing when the
// taskbar cannot host it (vertical taskbar, Explorer not running): use TaskbarAvailable.
func (t *Tray) SetTaskbar(items []BarItem) {
	t.mu.Lock()
	t.wantBar = append([]BarItem(nil), items...)
	t.barSet = true
	t.mu.Unlock()
	procPostMessage.Call(t.hwnd, wmApply, 0, 0)
}

// TaskbarAvailable reports whether there is a horizontal taskbar to draw into.
func TaskbarAvailable() bool {
	_, _, ok := taskbarGeometry()
	return ok
}

// taskbarGeometry returns the taskbar, its notification area rects (screen coords) and
// whether the taskbar is horizontal.
func taskbarGeometry() (tb, notify rect, ok bool) {
	cls, _ := windows.UTF16PtrFromString("Shell_TrayWnd")
	h, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(cls)), 0)
	if h == 0 {
		return
	}
	ncls, _ := windows.UTF16PtrFromString("TrayNotifyWnd")
	n, _, _ := procFindWindowEx.Call(h, 0, uintptr(unsafe.Pointer(ncls)), 0)
	if n == 0 {
		return
	}
	procGetWindowRect.Call(h, uintptr(unsafe.Pointer(&tb)))
	procGetWindowRect.Call(n, uintptr(unsafe.Pointer(&notify)))
	ok = tb.Right-tb.Left > tb.Bottom-tb.Top
	return
}

var barClassRegistered bool

// applyBar creates, redraws, moves or removes the taskbar window. UI thread only.
func (t *Tray) applyBar() {
	t.mu.Lock()
	items, set := t.wantBar, t.barSet
	t.mu.Unlock()
	if !set {
		return
	}
	b := &t.bar
	b.items = items
	if len(items) == 0 {
		b.destroy()
		return
	}
	tb, notify, ok := taskbarGeometry()
	if !ok {
		b.destroy()
		return
	}
	if b.hwnd != 0 {
		if r, _, _ := procIsWindow.Call(b.hwnd); r == 0 {
			b.hwnd = 0 // Explorer restarted and took our child window with it
		}
	}
	if b.hwnd == 0 && !b.create() {
		return
	}
	b.draw(tb, notify)
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
	tcls, _ := windows.UTF16PtrFromString("Shell_TrayWnd")
	parent, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(tcls)), 0)
	if parent == 0 {
		return false
	}
	h, _, _ := procCreateWindowEx.Call(wsExLayered|wsExNoActivate, uintptr(unsafe.Pointer(cls)), 0,
		wsChild|wsVisible|wsClipSiblings, 0, 0, 1, 1, parent, 0, inst, 0)
	b.hwnd = h
	return h != 0
}

var procGetWindow = user32.NewProc("GetWindow")

// onTop reports whether our window is the topmost child of the taskbar (above the XAML layer).
func (b *bar) onTop() bool {
	const gwChild = 5
	cls, _ := windows.UTF16PtrFromString("Shell_TrayWnd")
	parent, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(cls)), 0)
	if parent == 0 {
		return false
	}
	first, _, _ := procGetWindow.Call(parent, gwChild)
	return first == b.hwnd
}

func (b *bar) destroy() {
	if b.hwnd != 0 {
		procDestroyWindow.Call(b.hwnd)
		b.hwnd = 0
	}
	b.lastTB, b.lastNotify = rect{}, rect{}
}

// draw renders the items, sizes the window to fit and places it left of the notification area.
func (b *bar) draw(tb, notify rect) {
	height := int(tb.Bottom - tb.Top)
	dpi, _, _ := procGetDpiForWindow.Call(b.hwnd)
	if dpi == 0 {
		dpi = 96
	}
	px, width, spans := renderBar(b.items, height, int(dpi), taskbarIsLight())
	if px == nil || width == 0 {
		return
	}
	margin := int(4 * dpi / 96)
	x := int(notify.Left-tb.Left) - width - margin
	if x < 0 {
		x = 0
	}
	// HWND_TOP (0): on Windows 11 the taskbar content is a XAML layer
	// (DesktopWindowContentBridge) that covers the whole taskbar; we must sit above it.
	procSetWindowPos.Call(b.hwnd, 0, uintptr(x), 0, uintptr(width), uintptr(height), swpNoActivate|swpShowWindow)
	b.lastTB, b.lastNotify, b.spans = tb, notify, spans

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
				t.bar.pointerButton = 2
			case flags&pointerFirstBtn != 0:
				t.bar.pointerButton = 1
			}
		}
		return 0
	case wmPointerUp:
		if t != nil {
			btn := t.bar.pointerButton
			t.bar.pointerButton = 0
			pt := struct{ X, Y int32 }{int32(int16(l)), int32(int16(l >> 16))} // screen coords
			procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
			t.barClick(btn, int(pt.X))
		}
		return 0
	case wmLButtonUp:
		if t != nil {
			t.barClick(1, int(int16(l)))
		}
		return 0
	case wmRButtonUp:
		if t != nil {
			t.barClick(2, int(int16(l)))
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

func (t *Tray) barClick(button, x int) {
	switch button {
	case 1:
		call(t.cfg.OnLeftClick)
	case 2:
		t.showMenuFor(t.bar.keyAt(x))
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
	x := gap
	for i, it := range items {
		spans[i].x0 = x
		add := func(s string, f uintptr, after int) {
			u, _ := windows.UTF16FromString(s)
			w := textWidth(hdc, f, u)
			parts = append(parts, part{u, f, x, w, i})
			x += w + after
		}
		add(string(it.Glyph), glyphFont, gap)
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
	width := x + gap
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
		procDrawText.Call(hdc, uintptr(unsafe.Pointer(&p.s[0])), ^uintptr(0), uintptr(unsafe.Pointer(&r)),
			dtVCenter|dtSingleLine|dtNoPrefix)
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
		c := palette(items[p.item].Style, light)
		if items[p.item].Charging && p.font == glyphFont && p.s[0] == glyphCharging {
			c = palette(Charging, light)
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
