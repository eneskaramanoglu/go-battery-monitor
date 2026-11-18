// batterymon shows the battery level of wireless mice, keyboards, headsets and controllers
// on the Windows taskbar.
//
//	batterymon            run on the taskbar / in the tray
//	batterymon --list     print every device found and its battery level, then exit
//	batterymon --dump     print HID collections, unopenable HID devices and Bluetooth devices
//	                      (also saved to %TEMP%\batterymon-dump.txt), then exit
//	batterymon --quit     close the running instance
//	batterymon --demo     sample devices instead of real ones (UI testing)
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	_ "github.com/eneskaramanoglu/go-battery-monitor/internal/drivers" // registers every driver
	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/pnp"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/settings"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/version"
)

const pollInterval = 60 * time.Second

var demo bool

// pollOptions turns the user's settings into poll options.
func pollOptions(cfg settings.Settings) device.Options {
	return device.Options{Ignore: cfg.Ignore, Experimental: cfg.Experimental}
}

// poll reads every device, or returns sample devices in --demo mode.
func poll(cfg settings.Settings) ([]device.Status, error) {
	if !demo {
		return device.PollAll(pollOptions(cfg))
	}
	return []device.Status{
		{Key: "demo:mouse", Name: "Demo mouse", Via: "2.4 GHz receiver", Kind: device.Mouse, Percent: 85},
		{Key: "demo:headset", Name: "Demo headset", Via: "2.4 GHz receiver", Kind: device.Headset, Percent: 12},
		{Key: "demo:pad", Name: "Demo controller", Via: "USB cable", Kind: device.Gamepad, Percent: 40, Charging: true},
		{Key: "demo:kbd", Name: "Demo keyboard", Via: "Bluetooth", Kind: device.Keyboard, Percent: 100, Asleep: true},
	}, nil
}

func main() {
	list := flag.Bool("list", false, "print devices and battery levels, then exit")
	dump := flag.Bool("dump", false, "print HID and Bluetooth devices, then exit")
	quit := flag.Bool("quit", false, "close the running instance")
	ver := flag.Bool("version", false, "print the version, then exit")
	flag.BoolVar(&demo, "demo", false, "show sample devices instead of real ones (UI testing)")
	flag.Parse()

	switch {
	case *ver:
		attachConsole()
		fmt.Println("batterymon", version.String())
		return
	case *quit:
		attachConsole()
		if !quitRunning() {
			fmt.Println("batterymon is not running")
		}
		return
	case *list || *dump:
		attachConsole()
		var err error
		if *dump {
			err = runDump()
		} else {
			err = runList()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	// A tray app idles almost all the time: return memory to Windows eagerly.
	debug.SetGCPercent(50)
	debug.SetMemoryLimit(16 << 20)

	if !singleInstance() {
		return
	}
	runTray()
}

func runList() error {
	st, err := device.PollAll(pollOptions(settings.Load()))
	if err != nil {
		return err
	}
	if len(st) == 0 {
		fmt.Printf("No supported device found (%d drivers registered). Use --dump to see HID collections.\n", len(device.Drivers()))
		return nil
	}
	for _, s := range st {
		fmt.Println(s)
	}
	return nil
}

// runDump prints everything useful for adding a new device and saves a copy to %TEMP%.
func runDump() error {
	path := filepath.Join(os.TempDir(), "batterymon-dump.txt")
	var w io.Writer = os.Stdout
	if f, err := os.Create(path); err == nil {
		defer f.Close()
		w = io.MultiWriter(os.Stdout, f)
	}
	fmt.Fprintf(w, "batterymon %s dump, %s\n\n", version.String(), time.Now().Format(time.RFC3339))

	cols, err := hid.Enumerate()
	if err != nil {
		return err
	}
	sort.SliceStable(cols, func(i, j int) bool {
		a, b := cols[i], cols[j]
		if a.VendorID != b.VendorID {
			return a.VendorID < b.VendorID
		}
		if a.ProductID != b.ProductID {
			return a.ProductID < b.ProductID
		}
		return a.Interface < b.Interface
	})
	fmt.Fprintln(w, "== HID collections")
	fmt.Fprintln(w, "VID:PID    MI  PAGE:USAGE  IN  OUT FEAT  PRODUCT")
	for _, c := range cols {
		fmt.Fprintf(w, "%04X:%04X %3d  %04X:%04X %4d %4d %4d  %s\n", c.VendorID, c.ProductID, c.Interface,
			c.UsagePage, c.Usage, c.InputLen, c.OutputLen, c.FeatureLen, c.Product)
	}

	if bad := hid.Unopenable(); len(bad) > 0 {
		fmt.Fprintln(w, "\n== HID interfaces that could not be opened")
		for _, p := range bad {
			fmt.Fprintln(w, p)
		}
	}

	fmt.Fprintln(w, "\n== Bluetooth devices")
	for _, enum := range []string{"BTHENUM", "BTHLE"} {
		ids, _ := pnp.DeviceIDs(enum)
		for _, id := range ids {
			if !strings.Contains(strings.ToUpper(id), `\DEV_`) {
				continue
			}
			n, err := pnp.Locate(id)
			if err != nil {
				continue
			}
			vid, _ := n.Uint(pnp.KeyBluetoothVID)
			pid, _ := n.Uint(pnp.KeyBluetoothPID)
			conn := "?"
			if c, ok := n.Bool(pnp.KeyIsConnected); ok {
				conn = fmt.Sprint(c)
			}
			fmt.Fprintf(w, "%-7s %04X:%04X connected=%-5s %-40s %v\n", enum, vid, pid, conn, n.Name(), n.Strings(pnp.KeyContainerCategory))
		}
	}
	fmt.Fprintf(w, "\nSaved to %s\n", path)
	return nil
}

// quitRunning asks a running instance to close; false when none is running.
func quitRunning() bool {
	user32 := windows.NewLazySystemDLL("user32.dll")
	cls, _ := windows.UTF16PtrFromString("GoBatteryMonitorTray")
	h, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(cls)), 0)
	if h == 0 {
		return false
	}
	const wmClose = 0x0010
	user32.NewProc("PostMessageW").Call(h, wmClose, 0, 0)
	return true
}

// singleInstance returns false when another copy is already running.
func singleInstance() bool {
	name, _ := windows.UTF16PtrFromString(`Local\go-battery-monitor`)
	_, err := windows.CreateMutex(nil, false, name)
	return err != windows.ERROR_ALREADY_EXISTS
}

// attachConsole makes --list / --dump print into the terminal they were started from, since
// the exe is built as a GUI program (-H windowsgui) and has no console of its own.
func attachConsole() {
	if _, err := os.Stdout.Stat(); err == nil {
		return // output is redirected to a file or pipe already
	}
	const attachParentProcess = ^uint32(0)
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")
	if r, _, _ := proc.Call(uintptr(attachParentProcess)); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = f, f
		fmt.Println()
	}
}
