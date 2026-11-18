package drivers

// Logitech mice, keyboards and headsets over HID++ 2.0: receivers (Unifying, Bolt,
// Lightspeed, Nano), corded devices and Bluetooth devices. Protocol notes and sources:
// docs/protocols/logitech-hidpp.md. Only read requests are ever sent.
//
// EXPERIMENTAL: written from documentation and tested against a fake device only.

import (
	"errors"
	"strings"
	"time"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
)

const (
	logiVID           = 0x046D
	hidppShort        = 0x10
	hidppLong         = 0x11
	hidppLongLen      = 20
	hidppSoftwareID   = 0x0A
	hidppErr20        = 0xFF
	hidppErr10        = 0x8F
	hidppIndexDirect  = 0xFF
	hidppReplyTimeout = 600 * time.Millisecond

	featRoot           = 0x0000
	featDeviceName     = 0x0005
	featBatteryStatus  = 0x1000
	featBatteryVoltage = 0x1001
	featUnifiedBattery = 0x1004

	errInvalidSubID  = 0x01 // HID++ 1.0 only device
	errUnknownDevice = 0x08 // nothing paired at this index
	errResource      = 0x09 // device asleep / out of range
)

// hidppIO is the transport: write one long report, read the next input report from any of
// the device's HID++ collections. A fake implements it in tests.
type hidppIO interface {
	write(report []byte) error
	read(timeout time.Duration) ([]byte, error)
}

// hidppError is an error answer from the device.
type hidppError struct{ code byte }

func (e hidppError) Error() string { return "hid++ error " + hex4(uint16(e.code)) }

var errNoReply = errors.New("hid++: no reply")

func errCode(err error) byte {
	var he hidppError
	if errors.As(err, &he) {
		return he.code
	}
	return 0
}

// hidppRequest builds a long request report.
func hidppRequest(index, feature, function byte, params ...byte) []byte {
	r := make([]byte, hidppLongLen)
	r[0], r[1], r[2], r[3] = hidppLong, index, feature, function<<4|hidppSoftwareID
	copy(r[4:], params)
	return r
}

// matchReply checks whether an input report answers req: it returns the parameters of a
// reply, an error answer, or ok=false for an unrelated report.
func matchReply(req, r []byte) (params []byte, err error, ok bool) {
	if len(r) < 7 || (r[0] != hidppShort && r[0] != hidppLong) || r[1] != req[1] {
		return nil, nil, false
	}
	if (r[2] == hidppErr20 || r[2] == hidppErr10) && r[3] == req[2] && r[4] == req[3] {
		return nil, hidppError{r[5]}, true
	}
	if r[2] == req[2] && r[3] == req[3] {
		p := make([]byte, 16)
		copy(p, r[4:])
		return p, nil, true
	}
	return nil, nil, false
}

// hidppCall sends one request and waits for its answer.
func hidppCall(io hidppIO, index, feature, function byte, params ...byte) ([]byte, error) {
	req := hidppRequest(index, feature, function, params...)
	if err := io.write(req); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(hidppReplyTimeout)
	for time.Now().Before(deadline) {
		r, err := io.read(time.Until(deadline))
		if err != nil {
			if errors.Is(err, hid.ErrTimeout) {
				break
			}
			return nil, err
		}
		if p, err, ok := matchReply(req, r); ok {
			return p, err
		}
	}
	return nil, errNoReply
}

// hidppDevice is one device behind an index, with its feature table resolved lazily.
type hidppDevice struct {
	io       hidppIO
	index    byte
	features map[uint16]byte
}

func (d *hidppDevice) feature(id uint16) (byte, bool) {
	if idx, ok := d.features[id]; ok {
		return idx, idx != 0
	}
	p, err := hidppCall(d.io, d.index, 0, 0, byte(id>>8), byte(id))
	idx := byte(0)
	if err == nil {
		idx = p[0]
	}
	d.features[id] = idx
	return idx, idx != 0
}

// ping reports whether a HID++ 2.0 device answers at this index; asleep is set when the
// receiver knows the device but it does not answer right now.
func (d *hidppDevice) ping() (present, asleep bool) {
	p, err := hidppCall(d.io, d.index, 0, 1, 0, 0, 0x5A)
	switch {
	case err == nil:
		return p[0] >= 2, false // major 1 would be HID++ 1.0, not handled
	case errCode(err) == errResource:
		return false, true
	}
	return false, false
}

type logiBattery struct {
	percent  int    // -1 when only a coarse level is known
	level    string // "Critical" / "Low" / "Good" / "Full"
	charging bool
	mv       int
}

// battery tries the battery features from newest to oldest.
func (d *hidppDevice) battery() (logiBattery, bool) {
	if idx, ok := d.feature(featUnifiedBattery); ok {
		caps, err1 := hidppCall(d.io, d.index, idx, 0)
		st, err2 := hidppCall(d.io, d.index, idx, 1)
		if err1 == nil && err2 == nil {
			return decodeUnifiedBattery(caps, st)
		}
	}
	if idx, ok := d.feature(featBatteryStatus); ok {
		if st, err := hidppCall(d.io, d.index, idx, 0); err == nil {
			return decodeBatteryStatus(st)
		}
	}
	if idx, ok := d.feature(featBatteryVoltage); ok {
		if st, err := hidppCall(d.io, d.index, idx, 0); err == nil {
			return decodeBatteryVoltage(st)
		}
	}
	return logiBattery{}, false
}

func decodeUnifiedBattery(caps, st []byte) (logiBattery, bool) {
	if st[2] >= 5 {
		return logiBattery{}, false // battery / thermal / charging error
	}
	b := logiBattery{percent: -1, charging: st[2] == 1 || st[2] == 2 || st[2] == 4}
	switch {
	case st[1]&8 != 0:
		b.level = "Full"
	case st[1]&4 != 0:
		b.level = "Good"
	case st[1]&2 != 0:
		b.level = "Low"
	case st[1]&1 != 0:
		b.level = "Critical"
	}
	if caps[1]&0x02 != 0 && st[0] <= 100 {
		b.percent = int(st[0])
	}
	if b.percent < 0 && b.level == "" {
		return logiBattery{}, false
	}
	return b, true
}

func decodeBatteryStatus(st []byte) (logiBattery, bool) {
	if st[0] > 100 || st[2] >= 5 {
		return logiBattery{}, false
	}
	return logiBattery{percent: int(st[0]), charging: st[2] == 1 || st[2] == 2 || st[2] == 4}, true
}

func decodeBatteryVoltage(st []byte) (logiBattery, bool) {
	mv := int(st[0])<<8 | int(st[1])
	if mv == 0 {
		return logiBattery{}, false
	}
	flags := st[2]
	charging := flags&0x80 != 0 && flags&0x07 == 0
	return logiBattery{percent: voltageToPercent(mv), charging: charging, mv: mv}, true
}

// voltageCurve is an approximate resting-voltage curve of a single Li-ion cell
// (millivolts -> percent), our own coarse table; linear between points.
var voltageCurve = [][2]int{
	{3500, 0}, {3680, 10}, {3720, 20}, {3750, 30}, {3780, 40}, {3810, 50},
	{3850, 60}, {3900, 70}, {3970, 80}, {4050, 90}, {4150, 100},
}

func voltageToPercent(mv int) int {
	if mv <= voltageCurve[0][0] {
		return 0
	}
	for i := 1; i < len(voltageCurve); i++ {
		lo, hi := voltageCurve[i-1], voltageCurve[i]
		if mv <= hi[0] {
			return lo[1] + (mv-lo[0])*(hi[1]-lo[1])/(hi[0]-lo[0])
		}
	}
	return 100
}

// nameAndKind reads feature 0x0005 (device name and type).
func (d *hidppDevice) nameAndKind() (string, device.Kind) {
	idx, ok := d.feature(featDeviceName)
	if !ok {
		return "", device.Other
	}
	kind := device.Other
	if t, err := hidppCall(d.io, d.index, idx, 2); err == nil {
		kind = logiKind(t[0])
	}
	cnt, err := hidppCall(d.io, d.index, idx, 0)
	if err != nil || cnt[0] == 0 {
		return "", kind
	}
	var name []byte
	for len(name) < int(cnt[0]) {
		chunk, err := hidppCall(d.io, d.index, idx, 1, byte(len(name)))
		if err != nil {
			break
		}
		n := min(len(chunk), int(cnt[0])-len(name))
		name = append(name, chunk[:n]...)
	}
	return strings.TrimRight(string(name), "\x00 "), kind
}

func logiKind(t byte) device.Kind {
	switch t {
	case 0, 2:
		return device.Keyboard
	case 3, 4, 5:
		return device.Mouse
	case 8:
		return device.Headset
	case 11, 12:
		return device.Gamepad
	}
	return device.Other
}

// --- Windows transport ---

// hidppChannel writes on the long collection and reads from long and short collections.
type hidppChannel struct {
	long  *hid.Device
	short *hid.Device // may be nil (Bluetooth devices have long reports only)
	turn  int
}

func (c *hidppChannel) write(r []byte) error { return c.long.Write(r, 300*time.Millisecond) }

func (c *hidppChannel) read(timeout time.Duration) ([]byte, error) {
	buf := make([]byte, 64)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		devs := []*hid.Device{c.long, c.short}
		d := devs[c.turn%2]
		c.turn++
		if d == nil {
			continue
		}
		n, err := d.Read(buf, 40*time.Millisecond)
		if err == nil {
			return append([]byte(nil), buf[:n]...), nil
		}
		if !errors.Is(err, hid.ErrTimeout) {
			return nil, err
		}
	}
	return nil, hid.ErrTimeout
}

func (c *hidppChannel) close() {
	c.long.Close()
	if c.short != nil {
		c.short.Close()
	}
}

func isHidppLong(c hid.Collection) bool {
	return (c.UsagePage == 0xFF00 && c.Usage == 0x0002 || c.UsagePage == 0xFF43 && c.Usage == 0x0202) && c.InputLen >= hidppLongLen
}

func isHidppShort(c hid.Collection) bool {
	return c.UsagePage == 0xFF00 && c.Usage == 0x0001 && c.InputLen >= 7
}

// isLogiReceiver: Logitech's receivers use product ids 0xC5xx.
func isLogiReceiver(pid uint16) bool { return pid&0xFF00 == 0xC500 }

type logitechDriver struct{}

func init() { device.Register(logitechDriver{}) }

func (logitechDriver) Name() string       { return "logitech" }
func (logitechDriver) Experimental() bool { return true }

var logiLast lastKnown

func (logitechDriver) Poll(ctx *device.PollContext) {
	for _, lc := range ctx.Find(logiVID, isHidppLong) {
		if ctx.Claimed(lc.VendorID, lc.ProductID) {
			continue
		}
		long, err := hid.Open(lc)
		if err != nil {
			continue
		}
		ch := &hidppChannel{long: long}
		for _, sc := range ctx.Find(logiVID, isHidppShort) {
			if sc.ProductID == lc.ProductID && sc.Interface == lc.Interface {
				if s, err := hid.Open(sc); err == nil {
					ch.short = s
				}
				break
			}
		}
		ch.long.Flush()
		results := pollHidpp(ch, lc, isBluetoothPath(lc.Path))
		ch.close()
		for _, s := range results {
			ctx.Results = append(ctx.Results, s)
		}
		ctx.Claim(lc.VendorID, lc.ProductID)
	}
}

// pollHidpp reads every device behind one HID++ endpoint. Transport-independent, tested
// with a fake.
func pollHidpp(io hidppIO, c hid.Collection, bluetooth bool) []device.Status {
	indices := []byte{hidppIndexDirect}
	via := "USB cable"
	switch {
	case bluetooth:
		via = "Bluetooth"
	case isLogiReceiver(c.ProductID):
		indices = []byte{1, 2, 3, 4, 5, 6}
		via = "Logitech receiver"
	}
	var out []device.Status
	for _, idx := range indices {
		d := &hidppDevice{io: io, index: idx, features: map[uint16]byte{}}
		key := "logi:" + hex4(c.ProductID) + ":" + hex4(uint16(idx))
		present, asleep := d.ping()
		if !present {
			if asleep {
				if last, ok := logiLast.get(key); ok {
					last.Asleep, last.Charging = true, false
					out = append(out, last)
				}
			}
			continue
		}
		b, ok := d.battery()
		if !ok {
			continue
		}
		name, kind := d.nameAndKind()
		if name == "" {
			name = strings.TrimSpace(c.Product)
		}
		if name == "" {
			name = "Logitech device"
		}
		s := device.Status{
			Key: key, Name: name, Via: via, Kind: kind,
			Percent: b.percent, Charging: b.charging, Millivolts: b.mv,
		}
		if b.percent < 0 {
			s.Level = b.level
		}
		logiLast.set(key, s)
		out = append(out, s)
	}
	return out
}
