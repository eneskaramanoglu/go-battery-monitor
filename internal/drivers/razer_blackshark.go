package drivers

// Razer BlackShark V2 Pro (2023) on its 2.4 GHz dongle 1532:0555. Protocol facts from the MIT
// licensed matej-basic/blackshark-battery (README "Protocol"), which follows HaloBattery (MIT)
// and OpenRazer PR #2862's Synapse captures; no code was taken. Notes:
// docs/protocols/razer-blackshark-v2-pro.md.
//
// 64-byte output reports with report id 2 on the vendor collection FF00:0001. Like Synapse,
// a query is wrapped in "remote mode": remote on (twice: the first wakes the radio link),
// the query, then remote off. Replies start 02 0E 50 49; byte 12 echoes the command, byte 13
// is 01 (ack), byte 15 the value (battery 0..100; charging 00 = on battery).
//
// EXPERIMENTAL: not tested on hardware by us.

import (
	"sync"
	"time"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
)

const (
	razerVID          = 0x1532
	blacksharkV2Pro23 = 0x0555
	bsCmdBattery      = 0x21
	bsCmdCharging     = 0x2A
	bsInterval        = 5 * time.Minute // the headset lasts ~70 h; Synapse-like polling is plenty
	bsIntervalCharge  = 2 * time.Minute
)

var (
	bsRemoteOn  = []byte{0x02, 0x80, 0x07, 0x00, 0x00, 0x50, 0x41, 0x0E, 0x00, 0x02, 0xE1, 0x01}
	bsRemoteOff = []byte{0x02, 0x80, 0x07, 0x00, 0x00, 0x50, 0x41, 0x0E, 0x00, 0x02, 0xE1, 0x00}
)

func bsQuery(cmd byte) []byte {
	return []byte{0x02, 0x80, 0x08, 0x00, 0x00, 0x50, 0x41, 0x08, 0x00, 0x03, cmd, 0x00}
}

// parseBlackShark returns the value of a reply to cmd.
func parseBlackShark(r []byte, cmd byte) (byte, bool) {
	if len(r) < 16 || r[0] != 0x02 || r[1] != 0x0E || r[2] != 0x50 || r[3] != 0x49 {
		return 0, false
	}
	if r[12] != cmd || r[13] != 0x01 {
		return 0, false
	}
	return r[15], true
}

type blacksharkDriver struct{}

func init() { device.Register(blacksharkDriver{}) }

func (blacksharkDriver) Name() string       { return "razer-blackshark" }
func (blacksharkDriver) Experimental() bool { return true }

// Results are reused between polls (bsInterval) so the headset is not queried every minute.
var bsCache struct {
	sync.Mutex
	at     time.Time
	status device.Status
	ok     bool
}

var bsLast lastKnown

func (blacksharkDriver) Poll(ctx *device.PollContext) {
	cols := ctx.Find(razerVID, func(c hid.Collection) bool {
		return c.ProductID == blacksharkV2Pro23 && c.UsagePage == 0xFF00 && c.Usage == 0x0001 && c.OutputLen >= 64
	})
	if len(cols) == 0 {
		return
	}
	c := cols[0]
	key := "razer:" + hex4(c.ProductID)

	bsCache.Lock()
	cached, fresh := bsCache.status, bsCache.ok
	interval := bsInterval
	if cached.Charging {
		interval = bsIntervalCharge
	}
	fresh = fresh && time.Since(bsCache.at) < interval
	bsCache.Unlock()
	if fresh {
		ctx.Add(cached, c.VendorID, c.ProductID)
		return
	}

	pct, charging, ok := readBlackShark(c)
	if !ok {
		// Headset off or out of range: last reading, dimmed.
		if last, seen := bsLast.get(key); seen {
			last.Asleep, last.Charging = true, false
			ctx.Add(last, c.VendorID, c.ProductID)
		} else {
			ctx.Claim(c.VendorID, c.ProductID)
		}
		bsCache.Lock()
		bsCache.ok = false
		bsCache.Unlock()
		return
	}
	s := device.Status{
		Key: key, Name: "Razer BlackShark V2 Pro", Via: "2.4 GHz receiver", Kind: device.Headset,
		Percent: pct, Charging: charging, Brand: "razer",
	}
	bsLast.set(key, s)
	bsCache.Lock()
	bsCache.status, bsCache.at, bsCache.ok = s, time.Now(), true
	bsCache.Unlock()
	ctx.Add(s, c.VendorID, c.ProductID)
}

func readBlackShark(c hid.Collection) (pct int, charging, ok bool) {
	d, err := hid.Open(c)
	if err != nil {
		return 0, false, false
	}
	defer d.Close()
	d.Flush()
	write := func(f []byte) bool { return d.Write(f, 300*time.Millisecond) == nil }
	defer write(bsRemoteOff) // always leave remote mode

	ask := func(cmd byte) (byte, bool) {
		if !write(bsRemoteOn) || !write(bsQuery(cmd)) {
			return 0, false
		}
		buf := make([]byte, 128)
		deadline := time.Now().Add(700 * time.Millisecond)
		for time.Now().Before(deadline) {
			n, err := d.Read(buf, 150*time.Millisecond)
			if err != nil {
				continue
			}
			if v, ok := parseBlackShark(buf[:n], cmd); ok {
				return v, true
			}
		}
		return 0, false
	}

	write(bsRemoteOn) // wakes the radio link
	time.Sleep(100 * time.Millisecond)
	var level byte
	for range 3 {
		if v, got := ask(bsCmdBattery); got {
			level, ok = v, true
			break
		}
	}
	if !ok || level > 100 {
		return 0, false, false
	}
	if v, got := ask(bsCmdCharging); got {
		charging = v != 0
	}
	return int(level), charging, true
}
