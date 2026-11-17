package drivers

// ATK / VXE and other mice on the Compx receiver family (ATK vendor id 373B, Compx 3554).
// Protocol notes: docs/protocols/compx.md (our own analysis of ATK's public web configurator).
//
// Verified on a 373B:1278 receiver: GetBatteryLevel answers 80 % / 4217 mV, matching other
// software. When the radio link is idle the receiver can stay silent; then the vendor's
// identification exchange (DownLoadData) is sent first and the battery asked again.
//
// Frame (16 bytes, output report 8 on vendor collection FF02:0002, answer = input report 8):
//
//	byte 0     command
//	byte 1     status in the answer: 0 success, 1 fail, 2 EEPROM error, 0xFF not supported
//	bytes 2-3  EEPROM address (big endian), byte 4 valid data length, bytes 5..14 data
//	byte 15    checksum = 0x55 - (0x08 + sum of bytes 0..14)
//
// Commands used here: 1 = DownLoadData (identification, changes nothing) and
// 4 = GetBatteryLevel -> data: level %, charging flag, voltage mV (big endian).
// WARNING: command 7 in this family is SetEEPROM and 9 RestoreFactory; never send them.

import (
	"strings"
	"time"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
)

const (
	compxReportID      = 0x08
	compxFrameLen      = 16
	compxCmdHandshake  = 0x01 // DownLoadData
	compxCmdGetBattery = 0x04
)

var compxVendors = []uint16{0x373B, 0x3554}

type compxDriver struct{}

// Registered before the generic drivers; the file name sorts before "zz_bluetooth.go".
func init() { device.Register(compxDriver{}) }

func (compxDriver) Name() string { return "compx" }

func (compxDriver) Poll(ctx *device.PollContext) {
	for _, vid := range compxVendors {
		for _, c := range ctx.Find(vid, isCompxCollection) {
			if ctx.Claimed(c.VendorID, c.ProductID) {
				continue // two collections of one product
			}
			key := "compx:" + hex4(c.VendorID) + ":" + hex4(c.ProductID)
			r, ok := readCompx(c)
			if !ok || !r.online {
				// Mouse asleep or off: show the last reading, dimmed, if we have one.
				if last, seen := compxLast.get(key); seen {
					last.Asleep, last.Charging = true, false
					ctx.Add(last, c.VendorID, c.ProductID)
				} else {
					ctx.Claim(c.VendorID, c.ProductID)
				}
				continue
			}
			s := device.Status{
				Key:        key,
				Name:       compxName(c),
				Via:        compxVia(c),
				Kind:       device.Mouse,
				Percent:    r.percent,
				Charging:   r.charging,
				Millivolts: r.millivolts,
			}
			compxLast.set(key, s)
			ctx.Add(s, c.VendorID, c.ProductID)
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

// readCompx asks for the battery. A sleeping mouse leaves the request unanswered (the
// receiver does not answer for it), which is reported as ok=false.
func readCompx(c hid.Collection) (compxBattery, bool) {
	d, err := hid.Open(c)
	if err != nil {
		return compxBattery{}, false
	}
	defer d.Close()
	d.Flush()
	return compxAskBattery(d, 2*time.Second)
}

func compxAskBattery(d *hid.Device, wait time.Duration) (compxBattery, bool) {
	if err := d.Write(compxRequest(compxCmdGetBattery), 300*time.Millisecond); err != nil {
		return compxBattery{}, false
	}
	var b compxBattery
	ok := compxAwait(d, wait, func(r []byte) bool {
		var parsed bool
		b, parsed = parseCompxBattery(r)
		return parsed
	})
	return b, ok
}

// compxHandshakeRequest builds DownLoadData (command 1). Not sent by the driver: the receiver
// answers it by itself and it does not wake a sleeping mouse. Kept for protocol tests.
func compxHandshakeRequest(challenge [4]byte) []byte {
	r := make([]byte, 1+compxFrameLen)
	r[0], r[1], r[5] = compxReportID, compxCmdHandshake, 8
	copy(r[6:10], challenge[:])
	r[16] = compxChecksum(r[1:16])
	return r
}

// compxAwait reads input reports until match accepts one or wait expires; other reports
// (DPI change notifications...) are skipped.
func compxAwait(d *hid.Device, wait time.Duration, match func([]byte) bool) bool {
	buf := make([]byte, 64)
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		n, err := d.Read(buf, 200*time.Millisecond)
		if err == hid.ErrTimeout {
			continue
		}
		if err != nil {
			return false
		}
		if match(buf[:n]) {
			return true
		}
	}
	return false
}
