package tray

// Draws a number ("85") into a small icon with GDI. Text is drawn white-on-black into a 32-bit
// DIB with grayscale anti-aliasing; the brightness of each pixel becomes its alpha, so the
// result is a smooth, transparent icon in any colour.

import (
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	gdi32 = windows.NewLazySystemDLL("gdi32.dll")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procCreateBitmap       = gdi32.NewProc("CreateBitmap")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procCreateFont         = gdi32.NewProc("CreateFontW")
	procSetTextColor       = gdi32.NewProc("SetTextColor")
	procSetBkMode          = gdi32.NewProc("SetBkMode")
	procGetTextExtent      = gdi32.NewProc("GetTextExtentPoint32W")
	procGdiFlush           = gdi32.NewProc("GdiFlush")

	procDrawText           = user32.NewProc("DrawTextW")
	procCreateIconIndirect = user32.NewProc("CreateIconIndirect")
	procDestroyIcon        = user32.NewProc("DestroyIcon")
	procGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
)

type bitmapInfoHeader struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, SizeImage       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ClrUsed, ClrImportant        uint32
}

type iconInfo struct {
	FIcon              int32
	XHotspot, YHotspot uint32
	HbmMask, HbmColor  uintptr
}

type rect struct{ Left, Top, Right, Bottom int32 }

// RGB colour.
type RGB struct{ R, G, B uint8 }

const (
	smCxSmIcon       = 49
	dtCenter         = 0x1
	dtVCenter        = 0x4
	dtBottom         = 0x8
	dtSingleLine     = 0x20
	dtNoPrefix       = 0x800
	transparentBk    = 1
	antialiasQuality = 4
	fwBold           = 700
)

// smallIconSize is the tray icon size in pixels at the current DPI (16 at 100 %, 24 at 150 %...).
func smallIconSize() int {
	s, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
	if s < 16 {
		return 16
	}
	return int(s)
}

// renderTextIcon returns an HICON showing text in colour c; the caller owns it (DestroyIcon).
func renderTextIcon(text string, c RGB) uintptr {
	size := smallIconSize()
	px := renderTextPixels(text, c, size)
	if px == nil {
		return 0
	}
	hdc, _, _ := procCreateCompatibleDC.Call(0)
	if hdc == 0 {
		return 0
	}
	defer procDeleteDC.Call(hdc)
	bi := bitmapInfoHeader{Width: int32(size), Height: -int32(size), Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	color, _, _ := procCreateDIBSection.Call(hdc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if color == 0 {
		return 0
	}
	defer procDeleteObject.Call(color)
	copy(unsafe.Slice((*byte)(bits), len(px)), px)

	mask, _, _ := procCreateBitmap.Call(uintptr(size), uintptr(size), 1, 1, 0)
	defer procDeleteObject.Call(mask)
	ii := iconInfo{FIcon: 1, HbmMask: mask, HbmColor: color}
	h, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	return h
}

// Preview renders an icon as an image, for tests and tools/iconpreview.
func Preview(text string, s Style, light bool, size int) *image.NRGBA {
	px := renderTextPixels(text, palette(s, light), size)
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := 0; i+3 < len(px); i += 4 {
		a := px[i+3]
		if a == 0 {
			continue
		}
		un := func(v byte) byte { return byte(min(255, int(v)*255/int(a))) }
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = un(px[i+2]), un(px[i+1]), un(px[i]), a
	}
	return img
}

// renderTextPixels draws text into size x size premultiplied BGRA pixels.
func renderTextPixels(text string, c RGB, size int) []byte {
	hdc, _, _ := procCreateCompatibleDC.Call(0)
	if hdc == 0 {
		return nil
	}
	defer procDeleteDC.Call(hdc)

	bi := bitmapInfoHeader{Width: int32(size), Height: -int32(size), Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	color, _, _ := procCreateDIBSection.Call(hdc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if color == 0 {
		return nil
	}
	defer procDeleteObject.Call(color)
	old, _, _ := procSelectObject.Call(hdc, color)

	t16, _ := windows.UTF16FromString(text)
	font := fitFont(hdc, t16, size)
	oldFont, _, _ := procSelectObject.Call(hdc, font)
	procSetTextColor.Call(hdc, 0x00FFFFFF)
	procSetBkMode.Call(hdc, transparentBk)
	r := rect{0, 0, int32(size), int32(size)}
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(&t16[0])), ^uintptr(0), uintptr(unsafe.Pointer(&r)),
		dtCenter|dtVCenter|dtSingleLine|dtNoPrefix)
	procGdiFlush.Call()
	procSelectObject.Call(hdc, oldFont)
	procDeleteObject.Call(font)
	procSelectObject.Call(hdc, old)

	// Brightness -> alpha, then premultiplied colour (BGRA).
	src := unsafe.Slice((*[4]uint8)(bits), size*size)
	out := make([]byte, size*size*4)
	for i, p := range src {
		a := max(p[0], p[1], p[2])
		out[i*4], out[i*4+1], out[i*4+2], out[i*4+3] = mul(c.B, a), mul(c.G, a), mul(c.R, a), a
	}
	return out
}

func mul(c, a uint8) uint8 { return uint8(uint16(c) * uint16(a) / 255) }

// fitFont creates the largest bold font (up to the icon height) whose text fits the icon width.
func fitFont(hdc uintptr, text []uint16, size int) uintptr {
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	for h := size + 2; h > 6; h-- {
		f, _, _ := procCreateFont.Call(^uintptr(h-1), 0, 0, 0, fwBold, 0, 0, 0, 0, 0, 0, antialiasQuality, 0,
			uintptr(unsafe.Pointer(face)))
		old, _, _ := procSelectObject.Call(hdc, f)
		var sz struct{ CX, CY int32 }
		procGetTextExtent.Call(hdc, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)-1), uintptr(unsafe.Pointer(&sz)))
		procSelectObject.Call(hdc, old)
		if int(sz.CX) <= size+1 {
			return f
		}
		procDeleteObject.Call(f)
	}
	f, _, _ := procCreateFont.Call(^uintptr(6), 0, 0, 0, fwBold, 0, 0, 0, 0, 0, 0, antialiasQuality, 0, uintptr(unsafe.Pointer(face)))
	return f
}

// taskbarIsLight reports whether the taskbar uses the light theme (dark text needed).
func taskbarIsLight() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("SystemUsesLightTheme")
	return err == nil && v == 1
}

// palette returns the colour for a style on the current taskbar theme.
func palette(s Style, light bool) RGB {
	switch s {
	case Low:
		return RGB{0xE8, 0x11, 0x23}
	case Charging:
		if light {
			return RGB{0x0E, 0x7A, 0x0D}
		}
		return RGB{0x3D, 0xD6, 0x8C}
	case Dim:
		return RGB{0x8A, 0x8A, 0x8A}
	}
	if light {
		return RGB{0x1A, 0x1A, 0x1A}
	}
	return RGB{0xFF, 0xFF, 0xFF}
}
