package device

import (
	"testing"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
)

func TestClaimHidesProductFromLaterDrivers(t *testing.T) {
	ctx := &PollContext{
		Collections: []hid.Collection{
			{VendorID: 0x373B, ProductID: 0x1278, UsagePage: 0xFF02},
			{VendorID: 0x373B, ProductID: 0x1278, UsagePage: 0x0001},
			{VendorID: 0x373B, ProductID: 0x11CB, UsagePage: 0xFF02},
		},
		claimed: map[vidpid]bool{},
	}
	if got := len(ctx.Find(0x373B, nil)); got != 3 {
		t.Fatalf("Find before claim = %d collections, want 3", got)
	}
	ctx.Add(Status{Key: "mouse", Percent: 85}, 0x373B, 0x1278)
	got := ctx.Find(0x373B, nil)
	if len(got) != 1 || got[0].ProductID != 0x11CB {
		t.Fatalf("Find after claim = %+v, want only 11CB", got)
	}
	vendorPage := ctx.Find(0x373B, func(c hid.Collection) bool { return c.UsagePage == 0xFF02 })
	if len(vendorPage) != 1 {
		t.Fatalf("Find with predicate = %d, want 1", len(vendorPage))
	}
	if len(ctx.Results) != 1 || ctx.Results[0].Percent != 85 {
		t.Fatalf("Results = %+v", ctx.Results)
	}
}

func TestFilterCollections(t *testing.T) {
	cols := []hid.Collection{
		{VendorID: 0x373B, ProductID: 0x1278},
		{VendorID: 0x1234, ProductID: 0xABCD},
		{VendorID: 0x056A, ProductID: 0x0001},                  // Wacom tablet
		{VendorID: 0x9999, ProductID: 0x0001, UsagePage: 0x0D}, // any digitizer
	}
	got := filterCollections(cols, []string{" 1234:abcd ", "garbage"})
	if len(got) != 1 || got[0].ProductID != 0x1278 {
		t.Fatalf("filtered = %+v", got)
	}
	if _, _, ok := ParseVIDPID("12345:1"); ok {
		t.Fatal("out of range VID accepted")
	}
}

func TestStatusString(t *testing.T) {
	s := Status{Name: "Mouse", Via: "USB", Percent: -1}
	if got := s.String(); got == "" {
		t.Fatal("empty String()")
	}
}
