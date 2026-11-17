// batterymon shows the battery level of wireless mice, keyboards, headsets and controllers
// in the Windows notification area.
//
//	batterymon          run in the tray
//	batterymon --list   print every device found and its battery level, then exit
//	batterymon --dump   print every HID collection (for reverse engineering new devices)
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"time"

	"golang.org/x/sys/windows"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	_ "github.com/eneskaramanoglu/go-battery-monitor/internal/drivers" // registers every driver
	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/version"
)

const pollInterval = 60 * time.Second

var demo bool

// poll reads every device, or returns sample devices in --demo mode.
func poll() ([]device.Status, error) {
	if !demo {
		return device.PollAll()
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
	dump := flag.Bool("dump", false, "print every HID collection, then exit")
	ver := flag.Bool("version", false, "print the version, then exit")
	flag.BoolVar(&demo, "demo", false, "show sample devices instead of real ones (UI testing)")
	flag.Parse()

	if *ver {
		attachConsole()
		fmt.Println("batterymon", version.String())
		return
	}
	if *list || *dump {
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
	st, err := device.PollAll()
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

func runDump() error {
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
	fmt.Println("VID:PID    MI  PAGE:USAGE  IN  OUT FEAT  PRODUCT")
	for _, c := range cols {
		fmt.Printf("%04X:%04X %3d  %04X:%04X %4d %4d %4d  %s\n", c.VendorID, c.ProductID, c.Interface,
			c.UsagePage, c.Usage, c.InputLen, c.OutputLen, c.FeatureLen, c.Product)
	}
	return nil
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
