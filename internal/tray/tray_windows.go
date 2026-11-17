// Package tray is a minimal notification-area icon written directly against Win32:
// a message-only window, one tray icon with a tooltip, and a right-click menu.
package tray

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32  = windows.NewLazySystemDLL("user32.dll")
	shell32 = windows.NewLazySystemDLL("shell32.dll")

	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostMessage         = user32.NewProc("PostMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procLoadIcon            = user32.NewProc("LoadIconW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenu          = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procRegisterWindowMsg   = user32.NewProc("RegisterWindowMessageW")
	procShellNotifyIcon     = shell32.NewProc("Shell_NotifyIconW")
)

const (
	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmLButtonUp   = 0x0202
	wmRButtonUp   = 0x0205
	wmApp         = 0x8000
	wmTrayCB      = wmApp + 1
	wmSetTip      = wmApp + 2
	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nifMessage    = 0x1
	nifIcon       = 0x2
	nifTip        = 0x4
	mfString      = 0x0
	mfSeparator   = 0x800
	tpmReturnCmd  = 0x100
	tpmRightAlign = 0x8
	hwndMessage   = ^uintptr(2) // (HWND)-3
	idiApp        = 32512
)

type wndClassEx struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	IconSm                             uintptr
}

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type notifyIconData struct {
	Size             uint32
	Wnd              uintptr
	ID, Flags        uint32
	CallbackMessage  uint32
	Icon             uintptr
	Tip              [128]uint16
	State, StateMask uint32
	Info             [256]uint16
	Version          uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	GuidItem         windows.GUID
	BalloonIcon      uintptr
}

// MenuItem is one entry of the right-click menu; an empty Title draws a separator.
type MenuItem struct {
	Title   string
	OnClick func()
}

// Tray is the running icon. Create it with Run; SetTooltip is safe from any goroutine.
type Tray struct {
	hwnd        uintptr
	nid         notifyIconData
	menu        []MenuItem
	mu          sync.Mutex
	pendingTip  string
	taskbarMsg  uint32 // "TaskbarCreated": Explorer restarted, re-add the icon
	onLeftClick func()
}

var current *Tray

// Run creates the icon and runs the Win32 message loop on the calling goroutine until Quit.
// ready is called once the icon exists.
func Run(tip string, menu []MenuItem, onLeftClick func(), ready func(*Tray)) error {
	runtime.LockOSThread()
	t := &Tray{menu: menu, onLeftClick: onLeftClick}
	current = t

	inst, _ := getModuleHandle()
	cls, _ := windows.UTF16PtrFromString("GoBatteryMonitorTray")
	wc := wndClassEx{WndProc: syscall.NewCallback(wndProc), Instance: inst, ClassName: cls}
	wc.Size = uint32(unsafe.Sizeof(wc))
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, err := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(cls)),
		0, 0, 0, 0, 0, hwndMessage, 0, inst, 0)
	if hwnd == 0 {
		return err
	}
	t.hwnd = hwnd
	tc, _ := windows.UTF16PtrFromString("TaskbarCreated")
	r, _, _ := procRegisterWindowMsg.Call(uintptr(unsafe.Pointer(tc)))
	t.taskbarMsg = uint32(r)

	icon, _, _ := procLoadIcon.Call(0, idiApp)
	t.nid = notifyIconData{Wnd: hwnd, ID: 1, Flags: nifMessage | nifIcon | nifTip, CallbackMessage: wmTrayCB, Icon: icon}
	t.nid.Size = uint32(unsafe.Sizeof(t.nid))
	setTip(&t.nid, tip)
	procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&t.nid)))
	defer procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&t.nid)))

	if ready != nil {
		ready(t)
	}
	var m msg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return nil
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// SetTooltip replaces the hover text (Windows keeps at most 127 characters).
func (t *Tray) SetTooltip(s string) {
	t.mu.Lock()
	t.pendingTip = s
	t.mu.Unlock()
	procPostMessage.Call(t.hwnd, wmSetTip, 0, 0)
}

// Quit removes the icon and ends Run.
func (t *Tray) Quit() { procPostMessage.Call(t.hwnd, wmClose, 0, 0) }

func setTip(n *notifyIconData, s string) {
	u := windows.StringToUTF16(s)
	if len(u) > len(n.Tip) {
		u = append(u[:len(n.Tip)-1], 0)
	}
	n.Tip = [128]uint16{}
	copy(n.Tip[:], u)
}

func (t *Tray) showMenu() {
	m, _, _ := procCreatePopupMenu.Call()
	for i, it := range t.menu {
		if it.Title == "" {
			procAppendMenu.Call(m, mfSeparator, 0, 0)
			continue
		}
		s, _ := windows.UTF16PtrFromString(it.Title)
		procAppendMenu.Call(m, mfString, uintptr(i+1), uintptr(unsafe.Pointer(s)))
	}
	var pt struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(t.hwnd) // otherwise the menu does not close when clicking away
	cmd, _, _ := procTrackPopupMenu.Call(m, tpmReturnCmd|tpmRightAlign, uintptr(pt.X), uintptr(pt.Y), 0, t.hwnd, 0)
	procDestroyMenu.Call(m)
	if cmd > 0 && int(cmd) <= len(t.menu) && t.menu[cmd-1].OnClick != nil {
		t.menu[cmd-1].OnClick()
	}
}

func wndProc(hwnd uintptr, m uint32, w, l uintptr) uintptr {
	t := current
	switch {
	case m == wmTrayCB:
		switch uint32(l) {
		case wmRButtonUp:
			t.showMenu()
		case wmLButtonUp:
			if t.onLeftClick != nil {
				t.onLeftClick()
			}
		}
		return 0
	case m == wmSetTip:
		t.mu.Lock()
		setTip(&t.nid, t.pendingTip)
		t.mu.Unlock()
		procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&t.nid)))
		return 0
	case t != nil && m == t.taskbarMsg:
		procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&t.nid)))
		return 0
	case m == wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(m), w, l)
	return r
}

func getModuleHandle() (uintptr, error) {
	var h windows.Handle
	err := windows.GetModuleHandleEx(0, nil, &h)
	return uintptr(h), err
}
