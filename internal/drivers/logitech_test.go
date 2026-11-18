package drivers

import (
	"testing"
	"time"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
)

// fakeLogi simulates HID++ devices behind indices. Each device has a feature table and
// answers per (feature id, function).
type fakeLogi struct {
	devices map[byte]*fakeLogiDevice
	asleep  map[byte]bool
	queue   [][]byte
	writes  [][]byte
}

type fakeLogiDevice struct {
	features []uint16 // index -> feature id; index 0 is IRoot
	answers  map[[2]uint16][]byte
}

func (f *fakeLogi) write(r []byte) error {
	f.writes = append(f.writes, append([]byte(nil), r...))
	idx, feat, fnSw := r[1], r[2], r[3]
	errShort := func(code byte) []byte { return []byte{hidppShort, idx, hidppErr10, feat, fnSw, code, 0} }
	if f.asleep[idx] {
		f.queue = append(f.queue, errShort(errResource))
		return nil
	}
	d := f.devices[idx]
	if d == nil {
		f.queue = append(f.queue, errShort(errUnknownDevice))
		return nil
	}
	reply := func(p []byte) {
		out := make([]byte, hidppLongLen)
		out[0], out[1], out[2], out[3] = hidppLong, idx, feat, fnSw
		copy(out[4:], p)
		// unrelated traffic first, as a real receiver interleaves mouse events
		f.queue = append(f.queue, []byte{0x02, 0, 0, 0, 0, 0, 0, 0}, out)
	}
	fn := fnSw >> 4
	if feat == 0 {
		switch fn {
		case 0: // GetFeature
			id := uint16(r[4])<<8 | uint16(r[5])
			for i, fid := range d.features {
				if fid == id && i > 0 {
					reply([]byte{byte(i), 0, 0})
					return nil
				}
			}
			reply([]byte{0, 0, 0})
		case 1: // ping
			reply([]byte{4, 2, r[6]})
		}
		return nil
	}
	if int(feat) >= len(d.features) {
		f.queue = append(f.queue, []byte{hidppLong, idx, hidppErr20, feat, fnSw, 0x06, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
		return nil
	}
	key := [2]uint16{d.features[feat], uint16(fn)}
	if fid := d.features[feat]; fid == featDeviceName && fn == 1 {
		name := d.answers[[2]uint16{featDeviceName, 99}] // full name stored under fn 99
		start := int(r[4])
		chunk := name[start:min(len(name), start+16)]
		reply(chunk)
		return nil
	}
	if a, ok := d.answers[key]; ok {
		reply(a)
		return nil
	}
	f.queue = append(f.queue, []byte{hidppLong, idx, hidppErr20, feat, fnSw, 0x07, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	return nil
}

func (f *fakeLogi) read(time.Duration) ([]byte, error) {
	if len(f.queue) == 0 {
		return nil, hid.ErrTimeout
	}
	r := f.queue[0]
	f.queue = f.queue[1:]
	return r, nil
}

func TestPollHidppReceiver(t *testing.T) {
	mouseName := []byte("MX Master 3S for Business")
	f := &fakeLogi{
		devices: map[byte]*fakeLogiDevice{
			1: {
				features: []uint16{featRoot, 0x0001, featDeviceName, featUnifiedBattery},
				answers: map[[2]uint16][]byte{
					{featDeviceName, 0}:     {byte(len(mouseName))},
					{featDeviceName, 2}:     {3}, // mouse
					{featDeviceName, 99}:    mouseName,
					{featUnifiedBattery, 0}: {0x0F, 0x03}, // all levels, rechargeable, percentage
					{featUnifiedBattery, 1}: {62, 0x04, 0x01},
				},
			},
		},
		asleep: map[byte]bool{2: true},
	}
	logiLast.set("logi:C52B:0002", device.Status{Key: "logi:C52B:0002", Name: "K850", Percent: 40, Kind: device.Keyboard})
	got := pollHidpp(f, hid.Collection{VendorID: logiVID, ProductID: 0xC52B, Product: "USB Receiver"}, false)
	if len(got) != 2 {
		t.Fatalf("got %d devices: %+v", len(got), got)
	}
	m := got[0]
	if m.Name != string(mouseName) || m.Percent != 62 || !m.Charging || m.Kind != device.Mouse || m.Via != "Logitech receiver" {
		t.Errorf("mouse = %+v", m)
	}
	if k := got[1]; !k.Asleep || k.Percent != 40 {
		t.Errorf("sleeping keyboard = %+v", k)
	}
	for _, w := range f.writes {
		if len(w) != hidppLongLen || w[0] != hidppLong || w[3]&0x0F != hidppSoftwareID {
			t.Fatalf("bad request % X", w)
		}
		// Read-only check: only IRoot, name and battery features are ever addressed.
		if w[2] != 0 {
			d := f.devices[w[1]]
			if fid := d.features[w[2]]; fid != featDeviceName && fid != featUnifiedBattery {
				t.Fatalf("request to unexpected feature %#04x", fid)
			}
		}
	}
}

func TestPollHidppCordedVoltageOnly(t *testing.T) {
	f := &fakeLogi{devices: map[byte]*fakeLogiDevice{
		hidppIndexDirect: {
			features: []uint16{featRoot, featBatteryVoltage},
			answers:  map[[2]uint16][]byte{{featBatteryVoltage, 0}: {0x0F, 0x46, 0x80}}, // 3910 mV, charging
		},
	}}
	got := pollHidpp(f, hid.Collection{VendorID: logiVID, ProductID: 0xC08B, Product: "G502 HERO"}, false)
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	s := got[0]
	if s.Name != "G502 HERO" || s.Millivolts != 3910 || !s.Charging || s.Percent < 70 || s.Percent > 72 || s.Via != "USB cable" {
		t.Errorf("corded = %+v", s)
	}
}

func TestDecodeUnifiedBatteryLevelOnly(t *testing.T) {
	b, ok := decodeUnifiedBattery([]byte{0x0F, 0x01}, []byte{0, 0x02, 0x00}) // no percentage support
	if !ok || b.percent != -1 || b.level != "Low" || b.charging {
		t.Fatalf("level only = %+v", b)
	}
	if _, ok := decodeUnifiedBattery([]byte{0x0F, 0x03}, []byte{50, 0x04, 0x06}); ok {
		t.Fatal("thermal error accepted")
	}
}

func TestVoltageToPercent(t *testing.T) {
	for mv, want := range map[int]int{3400: 0, 3500: 0, 3810: 50, 3875: 65, 4150: 100, 4300: 100} {
		if got := voltageToPercent(mv); got != want {
			t.Errorf("voltageToPercent(%d) = %d, want %d", mv, got, want)
		}
	}
}

func TestMatchReplyIgnoresOtherTraffic(t *testing.T) {
	req := hidppRequest(1, 3, 1)
	if _, _, ok := matchReply(req, []byte{hidppLong, 2, 3, req[3], 0, 0, 0}); ok {
		t.Error("reply for another index matched")
	}
	if _, _, ok := matchReply(req, []byte{hidppLong, 1, 3, req[3] ^ 0x01, 0, 0, 0}); ok {
		t.Error("reply with another software id matched")
	}
	_, err, ok := matchReply(req, []byte{hidppShort, 1, hidppErr10, 3, req[3], errUnknownDevice, 0})
	if !ok || errCode(err) != errUnknownDevice {
		t.Errorf("HID++ 1.0 error not decoded: %v %v", err, ok)
	}
}
