// hidprobe sends raw output reports to a HID collection and prints the input reports that
// come back. Development tool for protocol analysis.
//
// WARNING: this sends whatever bytes you give it. Many vendor protocols put write commands
// (EEPROM writes, factory reset, firmware update mode) right next to the read commands. Only
// send commands you have confirmed to be read-only, from the exact command table the device's
// frame format uses.
//
//	go run ./tools/hidprobe -vid 373B -pid 1278 -page FF02 -usage 2 -hex "08 07 00 ... 46"
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
)

func main() {
	vid := flag.String("vid", "", "vendor id (hex)")
	pid := flag.String("pid", "", "product id (hex)")
	page := flag.String("page", "", "usage page (hex)")
	usage := flag.String("usage", "", "usage (hex)")
	data := flag.String("hex", "", "output report bytes, report id first")
	wait := flag.Duration("wait", 700*time.Millisecond, "how long to collect input reports")
	flag.Parse()

	h := func(s string) uint16 { v, _ := strconv.ParseUint(s, 16, 16); return uint16(v) }
	var reports [][]byte // several reports separated by ";" are sent in order on one handle
	for _, part := range strings.Split(*data, ";") {
		r, err := hex.DecodeString(strings.ReplaceAll(strings.TrimSpace(part), " ", ""))
		if err != nil || len(r) == 0 {
			log.Fatal("bad -hex")
		}
		reports = append(reports, r)
	}
	cols, err := hid.Enumerate()
	if err != nil {
		log.Fatal(err)
	}
	for _, c := range cols {
		if c.VendorID != h(*vid) || c.ProductID != h(*pid) || c.UsagePage != h(*page) || c.Usage != h(*usage) {
			continue
		}
		d, err := hid.Open(c)
		if err != nil {
			log.Fatalf("open: %v", err)
		}
		d.Flush()
		buf := make([]byte, 128)
		for _, report := range reports {
			fmt.Printf("-> % X\n", report)
			if err := d.Write(report, 300*time.Millisecond); err != nil {
				log.Fatalf("write: %v", err)
			}
			end := time.Now().Add(*wait)
			for time.Now().Before(end) {
				n, err := d.Read(buf, 100*time.Millisecond)
				if err == nil {
					fmt.Printf("<- % X\n", buf[:n])
				}
			}
		}
		d.Close()
		return
	}
	log.Fatal("collection not found")
}
