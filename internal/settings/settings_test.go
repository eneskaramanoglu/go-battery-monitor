package settings

import (
	"os"
	"testing"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	d := t.TempDir()
	dir = func() string { return d }

	if got := Load(); got.LowThreshold != 15 || !got.Notify || len(got.Hidden) != 0 {
		t.Fatalf("defaults = %+v", got)
	}
	s := Load()
	s.LowThreshold = 25
	s.Notify = false
	s.SetHidden("ps:0CE6", true)
	s.SetHidden("ps:0CE6", true) // no duplicates
	s.SetHidden("xinput:0", true)
	s.SetHidden("xinput:0", false)
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	got := Load()
	if got.LowThreshold != 25 || got.Notify || len(got.Hidden) != 1 || !got.IsHidden("ps:0CE6") || got.IsHidden("xinput:0") {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestLoadBrokenFileFallsBack(t *testing.T) {
	d := t.TempDir()
	dir = func() string { return d }
	if err := os.WriteFile(Path(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(); got.LowThreshold != 15 {
		t.Fatalf("broken file = %+v", got)
	}
	if err := os.WriteFile(Path(), []byte(`{"lowThreshold": 500, "notify": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(); got.LowThreshold != 15 {
		t.Fatalf("out of range threshold kept: %+v", got)
	}
}
