// listen passively prints what a device sends by itself: input reports from every HID
// collection of a VID/PID and, optionally, bytes arriving on a serial port. It never writes
// anything to the device, so it is safe to use on unknown hardware.
//
//	go run ./tools/listen -vid 373B -pid 11CB -com COM3 -baud 9600 -for 20s
package main

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
)

func main() {
	vid := flag.String("vid", "", "vendor id (hex)")
	pid := flag.String("pid", "", "product id (hex)")
	com := flag.String("com", "", "serial port to listen on, e.g. COM3 (optional)")
	baud := flag.Int("baud", 9600, "serial baud rate")
	dur := flag.Duration("for", 15*time.Second, "how long to listen")
	flag.Parse()

	h := func(s string) uint16 { v, _ := strconv.ParseUint(s, 16, 16); return uint16(v) }
	var mu sync.Mutex
	start := time.Now()
	out := func(src, s string) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Printf("%6.2fs %-16s %s\n", time.Since(start).Seconds(), src, s)
	}
	deadline := start.Add(*dur)
	var wg sync.WaitGroup

	if *vid != "" {
		cols, err := hid.Enumerate()
		if err != nil {
			log.Fatal(err)
		}
		for _, c := range cols {
			if c.VendorID != h(*vid) || c.ProductID != h(*pid) || c.InputLen == 0 {
				continue
			}
			d, err := hid.Open(c)
			if err != nil {
				out(fmt.Sprintf("%04X:%04X", c.UsagePage, c.Usage), "cannot open: "+err.Error())
				continue
			}
			src := fmt.Sprintf("hid %04X:%04X", c.UsagePage, c.Usage)
			out(src, fmt.Sprintf("listening (in %d bytes)", c.InputLen))
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer d.Close()
				buf := make([]byte, 256)
				for time.Now().Before(deadline) {
					if n, err := d.Read(buf, 200*time.Millisecond); err == nil {
						out(src, fmt.Sprintf("% X", buf[:n]))
					}
				}
			}()
		}
	}

	if *com != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			listenSerial(*com, *baud, deadline, out)
		}()
	}
	wg.Wait()
}

// listenSerial opens the port read-only in spirit: it configures 8N1 at baud and only reads.
func listenSerial(port string, baud int, deadline time.Time, out func(string, string)) {
	p, _ := windows.UTF16PtrFromString(`\\.\` + port)
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		out(port, "cannot open: "+err.Error())
		return
	}
	defer windows.CloseHandle(h)
	var dcb windows.DCB
	dcb.DCBlength = uint32(unsafe.Sizeof(dcb))
	if err := windows.GetCommState(h, &dcb); err != nil {
		out(port, "GetCommState: "+err.Error())
		return
	}
	dcb.BaudRate = uint32(baud)
	dcb.ByteSize = 8
	dcb.Parity = windows.NOPARITY
	dcb.StopBits = windows.ONESTOPBIT
	if err := windows.SetCommState(h, &dcb); err != nil {
		out(port, "SetCommState: "+err.Error())
		return
	}
	// Return from ReadFile after 200 ms without data.
	to := windows.CommTimeouts{ReadIntervalTimeout: 50, ReadTotalTimeoutConstant: 200}
	windows.SetCommTimeouts(h, &to)
	out(port, fmt.Sprintf("listening at %d baud", baud))
	buf := make([]byte, 256)
	for time.Now().Before(deadline) {
		var n uint32
		if err := windows.ReadFile(h, buf, &n, nil); err != nil {
			out(port, "read: "+err.Error())
			return
		}
		if n > 0 {
			out(port, fmt.Sprintf("% X", buf[:n]))
		}
	}
}
