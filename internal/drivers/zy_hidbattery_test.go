package drivers

import (
	"testing"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
)

func TestPickBatteryCap(t *testing.T) {
	caps := []hid.ValueCap{
		{UsagePage: 0x01, Usage: 0x30, LogicalMin: -127, LogicalMax: 127}, // X axis
		{UsagePage: 0x85, Usage: 0x66, LogicalMin: 0, LogicalMax: 4000},   // mAh, not a percentage
		{UsagePage: 0x85, Usage: 0x64, LogicalMin: 0, LogicalMax: 100},    // relative SoC
		{UsagePage: 0x06, Usage: 0x20, LogicalMin: 0, LogicalMax: 255},    // battery strength
	}
	v, ok := pickBatteryCap(caps)
	if !ok || v.UsagePage != 0x06 || v.Usage != 0x20 {
		t.Fatalf("picked %+v, %v", v, ok)
	}
	if _, ok := pickBatteryCap(caps[:2]); ok {
		t.Fatal("non-battery fields picked")
	}
}

func TestScalePercent(t *testing.T) {
	cases := []struct {
		raw  uint32
		cap  hid.ValueCap
		want int
		ok   bool
	}{
		{80, hid.ValueCap{LogicalMin: 0, LogicalMax: 100, BitSize: 8}, 80, true},
		{255, hid.ValueCap{LogicalMin: 0, LogicalMax: 255, BitSize: 8}, 100, true},
		{128, hid.ValueCap{LogicalMin: 0, LogicalMax: 255, BitSize: 8}, 50, true},
		{120, hid.ValueCap{LogicalMin: 0, LogicalMax: 100, BitSize: 8}, 0, false},
		{0xFF, hid.ValueCap{LogicalMin: -1, LogicalMax: 100, BitSize: 8}, 0, true}, // -1 sign-extended
	}
	for _, c := range cases {
		got, ok := scalePercent(c.raw, c.cap)
		if got != c.want || ok != c.ok {
			t.Errorf("scalePercent(%d, %+v) = %d, %v; want %d, %v", c.raw, c.cap, got, ok, c.want, c.ok)
		}
	}
}

func TestKindOfProduct(t *testing.T) {
	cols := []hid.Collection{
		{VendorID: 1, ProductID: 1, UsagePage: 0x01, Usage: 0x06},
		{VendorID: 1, ProductID: 1, UsagePage: 0x01, Usage: 0x02},
		{VendorID: 2, ProductID: 2, UsagePage: 0x01, Usage: 0x05},
	}
	if k := kindOfProduct(cols, 1, 1); k != device.Mouse {
		t.Errorf("keyboard+mouse product = %v", k)
	}
	if k := kindOfProduct(cols, 2, 2); k != device.Gamepad {
		t.Errorf("gamepad = %v", k)
	}
}
