package drivers

// ATK / VXE and other mice on the Compx receiver family (ATK vendor id 373B, Compx 3554).
// Protocol notes: docs/protocols/compx.md (our own analysis of ATK's public web configurator).
//
// STATUS: NOT REGISTERED. The frame format below is confirmed on hardware (the 373B:1278
// receiver answers with a valid checksum), but the battery read itself is not verified yet:
// the receiver did not answer GetBatteryLevel on its own, and the vendor app sends an
// identification exchange (DownLoadData) first. Until that sequence is verified on a real
// mouse, this driver is not registered, so the app never sends these commands.
//
// Frame (16 bytes, output report 8 on vendor collection FF02:0002, answer = input report 8):
//
//	byte 0     command
//	byte 1     status in the answer: 0 success, 1 fail, 2 EEPROM error, 0xFF not supported
//	bytes 2-3  EEPROM address (big endian), byte 4 valid data length, bytes 5..14 data
//	byte 15    checksum = 0x55 - (0x08 + sum of bytes 0..14)
//
// Commands used here (read-only): 4 = GetBatteryLevel -> data: level %, charging flag,
// voltage mV (big endian). WARNING: command 7 in this family is SetEEPROM; never send it.

import (
	"strings"
	"time"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
)

const (
	compxReportID      = 0x08
	compxFrameLen      = 16
	compxCmdGetBattery = 0x04
)

var compxVendors = []uint16{0x373B, 0x3554}

// compxDriver is intentionally not registered yet (see STATUS above).
type compxDriver struct{}

func (compxDriver) Name() string { return "compx" }

func (compxDriver) Poll(ctx *device.PollContext) {
	for _, vid := range compxVendors {
		for _, c := range ctx.Find(vid, isCompxCollection) {
			if ctx.Claimed(c.VendorID, c.ProductID) {
				continue // two collections of one product
			}
			r, ok := readCompx(c)
			if !ok {
				continue
			}
			if !r.online {
				ctx.Claim(c.VendorID, c.ProductID) // receiver present, mouse off or asleep
				continue
			}
			ctx.Add(device.Status{
				Key:        "compx:" + hex4(c.VendorID) + ":" + hex4(c.ProductID),
				Name:       compxName(c),
				Via:        compxVia(c),
				Kind:       device.Mouse,
				Percent:    r.percent,
				Charging:   r.charging,
				Millivolts: r.millivolts,
			}, c.VendorID, c.ProductID)
		}
	}
}

// isCompxCollection matches the vendor collection that carries the 16-byte command frames.
func isCompxCollection(c hid.Collection) bool {
	return c.UsagePage == 0xFF02 && c.Usage == 0x0002 && c.InputLen == 17 && c.OutputLen == 17
}

func compxName(c hid.Collection) string {
	p := strings.TrimSpace(c.Product)
	lp := strings.ToLower(p)
	if p == "" || strings.Contains(lp, "dongle") || strings.Contains(lp, "receiver") {
		if c.VendorID == 0x373B {
			return "ATK wireless mouse"
		}
		return "Wireless mouse"
	}
	return p
}

func compxVia(c hid.Collection) string {
	p := strings.ToLower(c.Product)
	if strings.Contains(p, "dongle") || strings.Contains(p, "receiver") {
		return "2.4 GHz receiver"
	}
	return "USB"
}

// compxRequest builds the 17-byte output report (report id + frame) for a data-less command.
func compxRequest(cmd byte) []byte {
	r := make([]byte, 1+compxFrameLen)
	r[0] = compxReportID
	r[1] = cmd
	r[16] = compxChecksum(r[1:16])
	return r
}

func compxChecksum(frame15 []byte) byte {
	sum := byte(compxReportID)
	for _, b := range frame15 {
		sum += b
	}
	return 0x55 - sum
}

type compxBattery struct {
	percent    int
	charging   bool
	millivolts int
	online     bool
}

// parseCompxBattery decodes the answer to GetBatteryLevel (report id included).
func parseCompxBattery(r []byte) (compxBattery, bool) {
	if len(r) < 1+compxFrameLen || r[0] != compxReportID || r[1] != compxCmdGetBattery {
		return compxBattery{}, false
	}
	if compxChecksum(r[1:16]) != r[16] {
		return compxBattery{}, false
	}
	if r[2] != 0 {
		return compxBattery{online: false}, true // receiver answered without a mouse
	}
	return compxBattery{
		percent:    min(int(r[6]), 100),
		charging:   r[7] != 0,
		millivolts: int(r[8])<<8 | int(r[9]),
		online:     true,
	}, true
}

func readCompx(c hid.Collection) (compxBattery, bool) {
	d, err := hid.Open(c)
	if err != nil {
		return compxBattery{}, false
	}
	defer d.Close()
	d.Flush()
	if err := d.Write(compxRequest(compxCmdGetBattery), 300*time.Millisecond); err != nil {
		return compxBattery{}, false
	}
	buf := make([]byte, 64)
	deadline := time.Now().Add(3 * time.Second) // the receiver asks the mouse over the air
	for time.Now().Before(deadline) {
		n, err := d.Read(buf, 200*time.Millisecond)
		if err != nil {
			if err == hid.ErrTimeout {
				continue
			}
			return compxBattery{}, false
		}
		if b, ok := parseCompxBattery(buf[:n]); ok {
			return b, true
		}
	}
	return compxBattery{}, false
}
