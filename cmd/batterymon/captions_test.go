package main

import (
	"reflect"
	"testing"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
)

func TestCaptions(t *testing.T) {
	st := []device.Status{
		{Name: "MX Master 3S", Kind: device.Mouse},
		{Name: "G502 HERO", Kind: device.Mouse},
		{Name: "AirPods Pro", Kind: device.Headset}, // alone in its kind
		{Name: "Xbox controller", Kind: device.Gamepad},
		{Name: "Xbox controller", Kind: device.Gamepad}, // same name: numbered
	}
	want := []string{"MX", "G50", "", "1", "2"}
	if got := captions(st); !reflect.DeepEqual(got, want) {
		t.Fatalf("captions = %q, want %q", got, want)
	}
}

func TestShortLabel(t *testing.T) {
	for name, want := range map[string]string{
		"ATK wireless mouse":         "ATK",
		"Logitech PRO X SUPERLIGHT":  "PRO",
		"Wireless mouse 8k dongle-L": "8K",
		"Demo keyboard":              "DK",
		"Türkçe Fare":                "TF",
	} {
		if got := shortLabel(name); got != want {
			t.Errorf("shortLabel(%q) = %q, want %q", name, got, want)
		}
	}
}
