package drivers

// Apple AirPods and Beats: battery levels from the Bluetooth LE "Proximity Pairing"
// broadcasts Apple audio devices send (Continuity message type 0x07). Layout from Celosia &
// Cunche, "Discontinued Privacy", PETS 2020, Fig. 5:
//
//	0x07 | length | 0x01 | device model (2) | UTP (1) | battery 1 (1) | battery 2 (1) |
//	lid open count (1) | color (1) | 0x00 | encrypted payload (16)
//
//	battery 1: high nibble and low nibble = the two earbuds' levels (10 steps)
//	battery 2: bits 4/5/6 = left / right / case charging, low nibble = case level
//
// The paper does not say which nibble is the left or the right earbud, so the buds are shown
// without a side ("Buds 80% / 70%") and the lower one is used as the device level.
//
// Privacy: everybody's AirPods broadcast these messages. Only broadcasts whose model matches
// an Apple audio device paired with this PC are used, and only the strongest one per model.

import (
	"strings"
	"sync"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	winrtgo "github.com/saltosystems/winrt-go"
	"github.com/saltosystems/winrt-go/windows/devices/bluetooth/advertisement"
	"github.com/saltosystems/winrt-go/windows/foundation"
	"github.com/saltosystems/winrt-go/windows/storage/streams"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/i18n"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/pnp"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/winrt"
)

const (
	appleCompanyID        = 0x004C
	continuityProximity   = 0x07
	proximityMinLen       = 11 // up to and including the 0x00 byte before the encrypted part
	airpodsMinRSSI        = -75
	airpodsFreshFor       = 2 * time.Minute
	airpodsFirstScanDelay = 3 * time.Second
)

// proximity is one decoded Proximity Pairing broadcast.
type proximity struct {
	model        uint16
	buds         [2]int // percent, -1 unknown
	caseLevel    int    // percent, -1 unknown
	budsCharging bool
	caseCharging bool
}

// level converts a 10-step nibble to percent; values above 10 mean "unknown".
func level(n byte) int {
	if n > 10 {
		return -1
	}
	return int(n) * 10
}

// parseProximity finds a Proximity Pairing message in Apple manufacturer data (company id
// already stripped). Continuity data is a list of type/length/value messages.
func parseProximity(data []byte) (proximity, bool) {
	for i := 0; i+2 <= len(data); {
		typ, n := data[i], int(data[i+1])
		body := data[i+2:]
		if n > len(body) {
			return proximity{}, false
		}
		body = body[:n]
		if typ == continuityProximity && len(body) >= proximityMinLen-2 && body[0] == 0x01 {
			b1, b2 := body[4], body[5]
			return proximity{
				model:        uint16(body[1]) | uint16(body[2])<<8,
				buds:         [2]int{level(b1 >> 4), level(b1 & 0x0F)},
				caseLevel:    level(b2 & 0x0F),
				budsCharging: b2&0x30 != 0,
				caseCharging: b2&0x40 != 0,
			}, true
		}
		i += 2 + n
	}
	return proximity{}, false
}

// status turns a broadcast into a device status. ok is false when no level is known.
func (p proximity) status(key, name string) (device.Status, bool) {
	lowest := -1
	var parts []string
	for _, b := range p.buds {
		if b < 0 {
			continue
		}
		if lowest < 0 || b < lowest {
			lowest = b
		}
		parts = append(parts, itoa(b)+"%")
	}
	if lowest < 0 {
		return device.Status{}, false
	}
	detail := i18n.T("Buds") + " " + strings.Join(parts, " / ")
	if len(parts) == 1 {
		detail = "" // headphones (AirPods Max, Beats): one battery
	}
	if p.caseLevel >= 0 {
		c := i18n.T("case") + " " + itoa(p.caseLevel) + "%"
		if p.caseCharging {
			c += " (" + i18n.T("charging") + ")"
		}
		if detail == "" {
			detail = c
		} else {
			detail += ", " + c
		}
	}
	return device.Status{
		Key: key, Name: name, Via: "Bluetooth", Kind: device.Headset,
		Percent: lowest, Charging: p.budsCharging, Detail: detail,
	}, true
}

// --- background scanner ---

type sighting struct {
	p    proximity
	rssi int16
	at   time.Time
}

var airpodsScan struct {
	once    sync.Once
	started time.Time
	mu      sync.Mutex
	best    map[uint16]sighting // strongest recent broadcast per model
	watcher *advertisement.BluetoothLEAdvertisementWatcher
	handler *foundation.TypedEventHandler
	err     error
}

// startScanner starts a passive BLE watcher for the life of the process.
func startScanner() {
	airpodsScan.once.Do(func() {
		airpodsScan.best = map[uint16]sighting{}
		airpodsScan.started = time.Now()
		done := make(chan struct{})
		go func() {
			defer close(done)
			if err := winrt.InitMTA(); err != nil {
				airpodsScan.err = err
				return
			}
			w, err := advertisement.NewBluetoothLEAdvertisementWatcher()
			if err != nil {
				airpodsScan.err = err
				return
			}
			w.SetScanningMode(advertisement.BluetoothLEScanningModePassive)
			iid := winrtgo.ParameterizedInstanceGUID(foundation.GUIDTypedEventHandler,
				advertisement.SignatureBluetoothLEAdvertisementWatcher,
				advertisement.SignatureBluetoothLEAdvertisementReceivedEventArgs)
			h := foundation.NewTypedEventHandler(ole.NewGUID(iid), onAdvertisement)
			if _, err := w.AddReceived(h); err != nil {
				airpodsScan.err = err
				return
			}
			if err := w.Start(); err != nil {
				airpodsScan.err = err
				return
			}
			airpodsScan.watcher, airpodsScan.handler = w, h // keep alive
		}()
		<-done
	})
}

// appleSeen, when set (tests only), sees every Apple manufacturer payload.
var appleSeen func([]byte)

// onAdvertisement runs on a Windows thread-pool thread for every BLE advertisement.
func onAdvertisement(_ *foundation.TypedEventHandler, _ unsafe.Pointer, argsPtr unsafe.Pointer) {
	args := (*advertisement.BluetoothLEAdvertisementReceivedEventArgs)(argsPtr)
	rssi, _ := args.GetRawSignalStrengthInDBm()
	ad, err := args.GetAdvertisement()
	if err != nil || ad == nil {
		return
	}
	defer ad.Release()
	list, err := ad.GetManufacturerData()
	if err != nil || list == nil {
		return
	}
	defer list.Release()
	n, _ := list.GetSize()
	for i := uint32(0); i < n; i++ {
		ptr, err := list.GetAt(i)
		if err != nil || ptr == nil {
			continue
		}
		md := (*advertisement.BluetoothLEManufacturerData)(ptr)
		if id, _ := md.GetCompanyId(); id == appleCompanyID {
			if data := manufacturerBytes(md); len(data) > 0 {
				if appleSeen != nil {
					appleSeen(data)
				}
				if p, ok := parseProximity(data); ok {
					record(p, rssi)
				}
			}
		}
		md.Release()
	}
}

func manufacturerBytes(md *advertisement.BluetoothLEManufacturerData) []byte {
	buf, err := md.GetData()
	if err != nil || buf == nil {
		return nil
	}
	defer buf.Release()
	n, err := buf.GetLength()
	if err != nil || n == 0 {
		return nil
	}
	r, err := streams.DataReaderFromBuffer(buf)
	if err != nil {
		return nil
	}
	defer r.Release()
	b, _ := r.ReadBytes(n)
	return b
}

func record(p proximity, rssi int16) {
	airpodsScan.mu.Lock()
	defer airpodsScan.mu.Unlock()
	now := time.Now()
	cur, ok := airpodsScan.best[p.model]
	// Replace when stronger, or when the stored one is getting old.
	if !ok || rssi >= cur.rssi || now.Sub(cur.at) > 15*time.Second {
		airpodsScan.best[p.model] = sighting{p, rssi, now}
	}
}

func latest(model uint16) (sighting, bool) {
	airpodsScan.mu.Lock()
	defer airpodsScan.mu.Unlock()
	s, ok := airpodsScan.best[model]
	if !ok || time.Since(s.at) > airpodsFreshFor || s.rssi < airpodsMinRSSI {
		return sighting{}, false
	}
	return s, true
}

// --- driver ---

type pairedApple struct {
	pid  uint16
	name string
}

// pairedAppleAudio lists Apple devices paired with this PC (Bluetooth vendor id 0x004C).
// An Apple audio device's Bluetooth product id is its Proximity Pairing model number.
func pairedAppleAudio() []pairedApple {
	var out []pairedApple
	seen := map[uint16]bool{}
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
			pid, ok := n.Uint(pnp.KeyBluetoothPID)
			if vid != appleCompanyID || !ok || seen[uint16(pid)] {
				continue
			}
			seen[uint16(pid)] = true
			out = append(out, pairedApple{uint16(pid), n.Name()})
		}
	}
	return out
}

type airpodsDriver struct{}

func init() { device.Register(airpodsDriver{}) }

func (airpodsDriver) Name() string { return "airpods" }

var airpodsLast lastKnown

func (airpodsDriver) Poll(ctx *device.PollContext) {
	paired := pairedAppleAudio()
	if len(paired) == 0 {
		return // never scan for other people's AirPods when none are paired here
	}
	startScanner()
	if airpodsScan.err != nil {
		return
	}
	if wait := airpodsFirstScanDelay - time.Since(airpodsScan.started); wait > 0 {
		time.Sleep(wait) // first poll: give the watcher a moment to hear something
	}
	for _, d := range paired {
		key := "airpods:" + hex4(d.pid)
		name := d.name
		if name == "" {
			name = "AirPods"
		}
		if s, ok := latest(d.pid); ok {
			if st, ok := s.p.status(key, name); ok {
				airpodsLast.set(key, st)
				ctx.Add(st, appleCompanyID, d.pid)
				continue
			}
		}
		// In the case with the lid closed, or out of range: last reading, dimmed.
		if last, ok := airpodsLast.get(key); ok {
			last.Asleep, last.Charging = true, false
			ctx.Add(last, appleCompanyID, d.pid)
		}
	}
}

func itoa(n int) string {
	if n == 100 {
		return "100"
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return string([]rune{rune('0' + n/10), rune('0' + n%10)})
}
