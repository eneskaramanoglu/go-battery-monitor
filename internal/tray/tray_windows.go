// Package tray is the notification-area UI written directly against Win32: a message-only
// window, one icon per device with its battery level drawn in it (or the app icon when there
// is no device), tooltips, and a right-click menu.
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
	procLoadImage           = user32.NewProc("LoadImageW")
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
	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmSettingChange = 0x001A
	wmDpiChanged    = 0x02E0
	wmLButtonUp     = 0x0202
	wmRButtonUp     = 0x0205
	wmApp           = 0x8000
	wmTrayCB        = wmApp + 1
	wmApply         = wmApp + 2
	nimAdd          = 0
	nimModify       = 1
	nimDelete       = 2
	nifMessage      = 0x1
	nifIcon         = 0x2
	nifTip          = 0x4
	mfString        = 0x0
	mfGrayed        = 0x1
	mfChecked       = 0x8
	mfSeparator     = 0x800
	mfPopup         = 0x10
	nifInfo         = 0x10
	niifWarning     = 0x2
	tpmReturnCmd    = 0x100
	tpmRightAlign   = 0x8
	idiApp          = 32512
	appIconID       = 1
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

// Style picks the colour of a device icon.
type Style int

const (
	Normal   Style = iota
	Low            // red
	Charging       // green
	Dim            // grey: asleep / last known value
)

// Icon is one device icon: Text is drawn into it ("85"), Tooltip shows on hover.
type Icon struct {
	Key     string
	Text    string
	Tooltip string
	Style   Style
}

// MenuItem is one entry of the right-click menu; an empty Title draws a separator and
// Children turn the item into a submenu.
type MenuItem struct {
	Title    string
	OnClick  func()
	Checked  bool
	Disabled bool
	Children []MenuItem
}

// Config describes the tray. Callbacks run on the UI thread: keep them short (start a
// goroutine for anything slow).
type Config struct {
	Tooltip        string                         // app icon tooltip until SetTooltip is called
	Menu           func(target string) []MenuItem // builds the right-click menu; target is the key of the device clicked, or ""
	OnLeftClick    func()                         // left click on any icon
	OnDeviceChange func()                         // a HID device arrived or was removed
	OnResume       func()                         // the PC woke up from sleep or hibernation
	Ready          func(*Tray)                    // called once the tray exists, before the message loop
}

// Tray is the running UI. SetIcons, SetTooltip and Quit are safe from any goroutine.
type Tray struct {
	hwnd       uintptr
	taskbarMsg uint32
	cfg        Config

	mu      sync.Mutex
	want    []Icon // latest requested icons, applied on the UI thread
	appTip  string
	shown   map[string]*shownIcon
	nextID  uint32
	appIcon *notifyIconData // nil while device icons are shown
	notes   []note          // notifications waiting for the UI thread
	wantBar []BarItem       // taskbar text requested by SetTaskbar
	barSet  bool
	barAll  bool             // also on secondary monitors' taskbars
	bars    map[uintptr]*bar // taskbar text windows, by host taskbar
}

type shownIcon struct {
	nid  notifyIconData
	icon Icon
}

var current *Tray

// Run creates the tray and runs the Win32 message loop on the calling goroutine until Quit.
func Run(cfg Config) error {
	runtime.LockOSThread()
	t := &Tray{cfg: cfg, appTip: cfg.Tooltip, shown: map[string]*shownIcon{}, nextID: 100}
	current = t

	inst, _ := getModuleHandle()
	cls, _ := windows.UTF16PtrFromString("GoBatteryMonitorTray")
	wc := wndClassEx{WndProc: syscall.NewCallback(wndProc), Instance: inst, ClassName: cls}
	wc.Size = uint32(unsafe.Sizeof(wc))
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	// A hidden top-level window rather than a message-only one: only top-level windows get the
	// broadcasts we need (TaskbarCreated after an Explorer restart, WM_SETTINGCHANGE on theme change).
	hwnd, _, err := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(cls)),
		0, 0, 0, 0, 0, 0, 0, inst, 0)
	if hwnd == 0 {
		return err
	}
	t.hwnd = hwnd
	tc, _ := windows.UTF16PtrFromString("TaskbarCreated")
	r, _, _ := procRegisterWindowMsg.Call(uintptr(unsafe.Pointer(tc)))
	t.taskbarMsg = uint32(r)

	registerHIDNotifications(hwnd)
	procSetTimer.Call(hwnd, barTimerID, barTimerInterval, 0) // follows taskbar / tray width changes
	t.apply()
	defer t.removeAll()
	if cfg.Ready != nil {
		cfg.Ready(t)
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

// SetIcons replaces the device icons. With no icons the app icon is shown instead.
func (t *Tray) SetIcons(icons []Icon) {
	t.mu.Lock()
	t.want = append([]Icon(nil), icons...)
	t.mu.Unlock()
	procPostMessage.Call(t.hwnd, wmApply, 0, 0)
}

// SetTooltip sets the hover text of the app icon (shown when there is no device).
func (t *Tray) SetTooltip(s string) {
	t.mu.Lock()
	t.appTip = s
	t.mu.Unlock()
	procPostMessage.Call(t.hwnd, wmApply, 0, 0)
}

// Quit removes the icons and ends Run.
func (t *Tray) Quit() { procPostMessage.Call(t.hwnd, wmClose, 0, 0) }

// apply reconciles the shown icons with t.want. UI thread only.
func (t *Tray) apply() {
	t.mu.Lock()
	want, tip := t.want, t.appTip
	t.mu.Unlock()
	light := taskbarIsLight()

	keep := map[string]bool{}
	for _, ic := range want {
		keep[ic.Key] = true
		s := t.shown[ic.Key]
		if s == nil {
			s = &shownIcon{nid: t.newNID(t.nextID)}
			t.nextID++
			t.shown[ic.Key] = s
			s.icon = ic
			t.setIconImage(s, light)
			setTip(&s.nid, ic.Tooltip)
			procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&s.nid)))
			continue
		}
		redraw := s.icon.Text != ic.Text || s.icon.Style != ic.Style
		s.icon = ic
		if redraw {
			t.setIconImage(s, light)
		}
		setTip(&s.nid, ic.Tooltip)
		procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&s.nid)))
	}
	for k, s := range t.shown {
		if !keep[k] {
			procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&s.nid)))
			procDestroyIcon.Call(s.nid.Icon)
			delete(t.shown, k)
		}
	}

	switch {
	case len(t.shown) == 0 && t.appIcon == nil:
		n := t.newNID(appIconID)
		n.Icon = AppIcon()
		setTip(&n, tip)
		t.appIcon = &n
		procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(t.appIcon)))
	case len(t.shown) == 0:
		setTip(t.appIcon, tip)
		procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(t.appIcon)))
	case t.appIcon != nil:
		procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(t.appIcon)))
		t.appIcon = nil
	}
}

func (t *Tray) newNID(id uint32) notifyIconData {
	n := notifyIconData{Wnd: t.hwnd, ID: id, Flags: nifMessage | nifIcon | nifTip, CallbackMessage: wmTrayCB}
	n.Size = uint32(unsafe.Sizeof(n))
	return n
}

func (t *Tray) setIconImage(s *shownIcon, light bool) {
	old := s.nid.Icon
	s.nid.Icon = renderTextIcon(s.icon.Text, palette(s.icon.Style, light))
	if old != 0 {
		procDestroyIcon.Call(old)
	}
}

// redrawAll re-renders every icon (theme or DPI changed) and re-adds them after an Explorer
// restart when readd is true.
func (t *Tray) redrawAll(readd bool) {
	light := taskbarIsLight()
	for _, s := range t.shown {
		t.setIconImage(s, light)
		op := uintptr(nimModify)
		if readd {
			op = nimAdd
		}
		procShellNotifyIcon.Call(op, uintptr(unsafe.Pointer(&s.nid)))
	}
	if readd && t.appIcon != nil {
		procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(t.appIcon)))
	}
	for _, b := range t.bars {
		if readd {
			b.hwnd = 0 // the old taskbar took our child window down with it
		}
		b.lastTB = rect{} // force a redraw (theme / DPI may have changed)
	}
	t.applyBar()
}

// repositionBar redraws the taskbar text only when the taskbar or the notification area
// moved or resized (cheap check every couple of seconds).
func (t *Tray) repositionBar() {
	t.mu.Lock()
	want := len(t.wantBar) > 0
	t.mu.Unlock()
	if !want {
		return
	}
	t.mu.Lock()
	all := t.barAll
	t.mu.Unlock()
	hosts := taskbarHosts(all)
	if len(hosts) != len(t.bars) {
		t.applyBar() // a monitor (or its taskbar) came or went
		return
	}
	for _, h := range hosts {
		b := t.bars[h.hwnd]
		tb, notify, ok := h.geometry()
		if b == nil || !ok {
			t.applyBar()
			return
		}
		alive := false
		if b.hwnd != 0 {
			r, _, _ := procIsWindow.Call(b.hwnd)
			alive = r != 0
		}
		if !alive || tb != b.lastTB || notify != b.lastNotify || !b.onTop() {
			// first draw, Explorer restart, taskbar resized, tray icons added/removed, or the
			// taskbar raised its own XAML layer above us again
			t.applyBar()
			return
		}
	}
}

func (t *Tray) removeAll() {
	for _, b := range t.bars {
		b.destroy()
	}
	for _, s := range t.shown {
		procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&s.nid)))
		procDestroyIcon.Call(s.nid.Icon)
	}
	if t.appIcon != nil {
		procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(t.appIcon)))
	}
}

// keyForIconID maps a tray icon id (wParam of the callback) back to its device key.
func (t *Tray) keyForIconID(id uint32) string {
	for k, s := range t.shown {
		if s.nid.ID == id {
			return k
		}
	}
	return ""
}

func setTip(n *notifyIconData, s string) {
	u := windows.StringToUTF16(s)
	if len(u) > len(n.Tip) {
		u = append(u[:len(n.Tip)-1], 0)
	}
	n.Tip = [128]uint16{}
	copy(n.Tip[:], u)
}

// showMenuFor opens the right-click menu; target is the key of the device under the cursor.
func (t *Tray) showMenuFor(target string) {
	if t.cfg.Menu == nil {
		return
	}
	var actions []func() // command id i+1 runs actions[i]
	m := buildMenu(t.cfg.Menu(target), &actions)
	var pt struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(t.hwnd) // otherwise the menu does not close when clicking away
	cmd, _, _ := procTrackPopupMenu.Call(m, tpmReturnCmd|tpmRightAlign, uintptr(pt.X), uintptr(pt.Y), 0, t.hwnd, 0)
	procDestroyMenu.Call(m) // also destroys the submenus
	if cmd > 0 && int(cmd) <= len(actions) && actions[cmd-1] != nil {
		actions[cmd-1]()
	}
}

func buildMenu(items []MenuItem, actions *[]func()) uintptr {
	m, _, _ := procCreatePopupMenu.Call()
	for _, it := range items {
		if it.Title == "" {
			procAppendMenu.Call(m, mfSeparator, 0, 0)
			continue
		}
		flags := uintptr(mfString)
		if it.Checked {
			flags |= mfChecked
		}
		if it.Disabled {
			flags |= mfGrayed
		}
		s, _ := windows.UTF16PtrFromString(it.Title)
		if len(it.Children) > 0 {
			sub := buildMenu(it.Children, actions)
			procAppendMenu.Call(m, flags|mfPopup, sub, uintptr(unsafe.Pointer(s)))
			continue
		}
		*actions = append(*actions, it.OnClick)
		procAppendMenu.Call(m, flags, uintptr(len(*actions)), uintptr(unsafe.Pointer(s)))
	}
	return m
}

// Notify shows a Windows notification attached to a device icon (or the app icon).
func (t *Tray) Notify(key, title, text string) {
	t.mu.Lock()
	t.notes = append(t.notes, note{key, title, text})
	t.mu.Unlock()
	procPostMessage.Call(t.hwnd, wmApply, 0, 0)
}

type note struct{ key, title, text string }

// flushNotes shows pending notifications. UI thread only.
func (t *Tray) flushNotes() {
	t.mu.Lock()
	notes := t.notes
	t.notes = nil
	t.mu.Unlock()
	for _, n := range notes {
		var nid *notifyIconData
		if s := t.shown[n.key]; s != nil {
			nid = &s.nid
		} else if t.appIcon != nil {
			nid = t.appIcon
		} else {
			for _, s := range t.shown {
				nid = &s.nid
				break
			}
		}
		if nid == nil {
			continue
		}
		info := *nid
		info.Flags = nifInfo
		copyUTF16(info.InfoTitle[:], n.title)
		copyUTF16(info.Info[:], n.text)
		info.InfoFlags = niifWarning
		procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&info)))
	}
}

func copyUTF16(dst []uint16, s string) {
	u := windows.StringToUTF16(s)
	if len(u) > len(dst) {
		u = append(u[:len(dst)-1], 0)
	}
	clear(dst)
	copy(dst, u)
}

func wndProc(hwnd uintptr, m uint32, w, l uintptr) uintptr {
	t := current
	switch {
	case m == wmTrayCB:
		switch uint32(l) {
		case wmRButtonUp:
			t.showMenuFor(t.keyForIconID(uint32(w)))
		case wmLButtonUp:
			call(t.cfg.OnLeftClick)
		}
		return 0
	case m == wmDeviceChange:
		if w == dbtDeviceArrival || w == dbtDeviceRemoveComplete {
			call(t.cfg.OnDeviceChange)
		}
		return 1
	case m == wmPowerBroadcast:
		if w == pbtAPMResumeAutomatic {
			call(t.cfg.OnResume)
		}
		return 1
	case m == wmApply:
		t.apply()
		t.applyBar()
		t.flushNotes()
		return 0
	case m == wmTimer && w == barTimerID:
		t.repositionBar()
		return 0
	case m == wmSettingChange, m == wmDpiChanged:
		t.redrawAll(false) // light/dark switch or scaling change
	case t != nil && m == t.taskbarMsg:
		t.redrawAll(true)
		return 0
	case m == wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(m), w, l)
	return r
}

// AppIcon returns the "APP" icon embedded by go-winres at the small (tray) size for the
// current DPI, or the stock application icon when the exe has no resources.
func AppIcon() uintptr {
	inst, _ := getModuleHandle()
	name, _ := windows.UTF16PtrFromString("APP")
	const imageIcon, lrDefaultSize, lrShared = 1, 0x40, 0x8000
	h, _, _ := procLoadImage.Call(inst, uintptr(unsafe.Pointer(name)), imageIcon, 0, 0, lrDefaultSize|lrShared)
	if h == 0 {
		h, _, _ = procLoadIcon.Call(0, idiApp)
	}
	return h
}

func getModuleHandle() (uintptr, error) {
	var h windows.Handle
	err := windows.GetModuleHandleEx(0, nil, &h)
	return uintptr(h), err
}
