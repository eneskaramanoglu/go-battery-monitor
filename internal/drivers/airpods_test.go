package drivers

import (
	"os"
	"sync"
	"testing"
	"time"
)

// Listens to real BLE traffic for a few seconds and counts Apple Continuity message types
// (no addresses are printed). Set BATTERYMON_BLE_TEST=1 to run; CI has no Bluetooth radio.
func TestScannerHearsApple(t *testing.T) {
	if os.Getenv("BATTERYMON_BLE_TEST") == "" {
		t.Skip("set BATTERYMON_BLE_TEST=1 to listen to real Bluetooth advertisements")
	}
	var mu sync.Mutex
	types := map[byte]int{}
	total := 0
	appleSeen = func(d []byte) {
		mu.Lock()
		defer mu.Unlock()
		total++
		for i := 0; i+2 <= len(d); i += 2 + int(d[i+1]) {
			types[d[i]]++
		}
		if p, ok := parseProximity(d); ok {
			// Unencrypted header bytes only (model, UTP, battery, lid, color): no address.
			hdr := d
			for i := 0; i+2 <= len(d); i += 2 + int(d[i+1]) {
				if d[i] == continuityProximity && i+13 <= len(d) {
					hdr = d[i : i+13]
				}
			}
			t.Logf("proximity: model=%04X buds=%v case=%d budsCharging=%v caseCharging=%v header=% X",
				p.model, p.buds, p.caseLevel, p.budsCharging, p.caseCharging, hdr)
		}
	}
	defer func() { appleSeen = nil }()
	startScanner()
	if airpodsScan.err != nil {
		t.Fatal(airpodsScan.err)
	}
	time.Sleep(8 * time.Second)
	mu.Lock()
	defer mu.Unlock()
	t.Logf("Apple advertisements: %d, Continuity types: %v", total, types)
}

// proximityMsg builds a Proximity Pairing message following the PETS 2020 layout.
func proximityMsg(model uint16, b1, b2 byte) []byte {
	m := []byte{0x07, 0x19, 0x01, byte(model), byte(model >> 8), 0x2B, b1, b2, 0x05, 0x01, 0x00}
	return append(m, make([]byte, 16)...) // encrypted payload
}

func TestParseProximity(t *testing.T) {
	// Another Continuity message (Nearby Info 0x10) first, then proximity pairing.
	data := append([]byte{0x10, 0x05, 0x01, 0x18, 0x44, 0x11, 0x22}, proximityMsg(0x200E, 0x87, 0x54)...)
	p, ok := parseProximity(data)
	if !ok {
		t.Fatal("not found")
	}
	if p.model != 0x200E || p.buds != [2]int{80, 70} || p.caseLevel != 40 || !p.budsCharging || !p.caseCharging {
		t.Fatalf("parsed %+v", p)
	}
	s, ok := p.status("k", "My AirPods Pro")
	if !ok || s.Percent != 70 || !s.Charging || s.Detail != "Buds 80% / 70%, case 40% (charging)" {
		t.Fatalf("status %+v", s)
	}
}

func TestProximityUnknownLevels(t *testing.T) {
	// Headphones: one battery, other nibble 0xF; case nibble 0xF (no case).
	p, ok := parseProximity(proximityMsg(0x200A, 0x6F, 0x0F))
	if !ok || p.buds != [2]int{60, -1} || p.caseLevel != -1 {
		t.Fatalf("parsed %+v", p)
	}
	s, ok := p.status("k", "AirPods Max")
	if !ok || s.Percent != 60 || s.Detail != "" {
		t.Fatalf("status %+v", s)
	}
	if _, ok := (proximity{buds: [2]int{-1, -1}, caseLevel: 50}).status("k", "x"); ok {
		t.Fatal("status without bud levels accepted")
	}
}

func TestParseProximityRejectsBadData(t *testing.T) {
	for _, d := range [][]byte{
		nil,
		{0x07},
		{0x07, 0x19, 0x01, 0x0E}, // length beyond data
		{0x10, 0x02, 0x00, 0x00}, // only other messages
		proximityMsg(0x200E, 0x88, 0x00)[:8],
	} {
		if _, ok := parseProximity(d); ok {
			t.Errorf("accepted % X", d)
		}
	}
}

func TestItoa(t *testing.T) {
	for n, want := range map[int]string{0: "0", 7: "7", 40: "40", 100: "100"} {
		if got := itoa(n); got != want {
			t.Errorf("itoa(%d) = %q", n, got)
		}
	}
}
