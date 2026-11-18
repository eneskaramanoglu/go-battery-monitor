package drivers

import "testing"

func TestParseBlackShark(t *testing.T) {
	// Example battery reply from the protocol notes: 0x4B = 75 %.
	reply := []byte{0x02, 0x0E, 0x50, 0x49, 0x08, 0xCE, 0x2D, 0x83, 0x03, 0x00, 0x04, 0x00, 0x21, 0x01, 0x01, 0x4B}
	if v, ok := parseBlackShark(reply, bsCmdBattery); !ok || v != 75 {
		t.Fatalf("battery = %d, %v", v, ok)
	}
	if _, ok := parseBlackShark(reply, bsCmdCharging); ok {
		t.Fatal("battery reply accepted as charging reply")
	}
	status := []byte{0x02, 0x0D, 0x50, 0x49, 0x01, 0xC0, 0, 0, 0, 0, 0, 0, 0x21, 0x01, 0x01, 0x4B}
	if _, ok := parseBlackShark(status, bsCmdBattery); ok {
		t.Fatal("unsolicited status frame accepted")
	}
	nack := append([]byte(nil), reply...)
	nack[13] = 0x00
	if _, ok := parseBlackShark(nack, bsCmdBattery); ok {
		t.Fatal("reply without ack accepted")
	}
	q := bsQuery(bsCmdCharging)
	if len(q) != 12 || q[0] != 0x02 || q[10] != 0x2A {
		t.Fatalf("query % X", q)
	}
}
