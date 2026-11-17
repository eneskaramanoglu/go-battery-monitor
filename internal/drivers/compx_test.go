package drivers

import "testing"

func TestCompxChecksumMatchesHardware(t *testing.T) {
	// Real answer captured from a 373B:1278 receiver (status 1 = fail, checksum 0x45).
	answer := []byte{0x08, 0x07, 0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x45}
	if got := compxChecksum(answer[1:16]); got != 0x45 {
		t.Fatalf("checksum = %#x, want 0x45", got)
	}
	req := compxRequest(compxCmdGetBattery)
	if len(req) != 17 || req[0] != 0x08 || req[1] != 0x04 || req[16] != 0x49 {
		t.Fatalf("request = % X", req)
	}
}

func TestParseCompxBatteryHardwareAnswer(t *testing.T) {
	// Real answers captured from a 373B:1278 receiver (ATK mouse at 80 %, 4217 mV).
	answer := []byte{0x08, 0x04, 0x00, 0x00, 0x00, 0x02, 0x50, 0x00, 0x10, 0x79, 0, 0, 0, 0, 0, 0, 0x6E}
	b, ok := parseCompxBattery(answer)
	if !ok || !b.online || b.percent != 80 || b.charging || b.millivolts != 4217 {
		t.Fatalf("hardware answer = %+v, %v", b, ok)
	}
	hs := compxHandshakeRequest([4]byte{0x12, 0x34, 0x56, 0x78})
	want := []byte{0x08, 0x01, 0, 0, 0, 0x08, 0x12, 0x34, 0x56, 0x78, 0, 0, 0, 0, 0, 0, 0x30}
	if string(hs) != string(want) {
		t.Fatalf("handshake = % X, want % X", hs, want)
	}
	hsAnswer := []byte{0x08, 0x01, 0, 0, 0, 0x08, 0x47, 0xF2, 0x7A, 0xF2, 0x01, 0x34, 0x05, 0, 0, 0, 0x65}
	if compxChecksum(hsAnswer[1:16]) != hsAnswer[16] {
		t.Fatal("handshake answer checksum")
	}
}

func TestParseCompxBattery(t *testing.T) {
	frame := func(status, level, charging byte, mv uint16) []byte {
		r := make([]byte, 17)
		r[0], r[1], r[2] = 0x08, compxCmdGetBattery, status
		r[6], r[7], r[8], r[9] = level, charging, byte(mv>>8), byte(mv)
		r[16] = compxChecksum(r[1:16])
		return r
	}
	b, ok := parseCompxBattery(frame(0, 80, 0, 4217))
	if !ok || !b.online || b.percent != 80 || b.charging || b.millivolts != 4217 {
		t.Errorf("discharging = %+v, %v", b, ok)
	}
	b, ok = parseCompxBattery(frame(0, 120, 1, 4300))
	if !ok || b.percent != 100 || !b.charging {
		t.Errorf("charging/clamp = %+v, %v", b, ok)
	}
	b, ok = parseCompxBattery(frame(1, 0, 0, 0))
	if !ok || b.online {
		t.Errorf("failed status = %+v, %v", b, ok)
	}
	bad := frame(0, 50, 0, 4000)
	bad[16]++
	if _, ok := parseCompxBattery(bad); ok {
		t.Error("bad checksum accepted")
	}
	other := frame(0, 50, 0, 4000)
	other[1] = 0x12
	if _, ok := parseCompxBattery(other); ok {
		t.Error("answer to another command accepted")
	}
}
