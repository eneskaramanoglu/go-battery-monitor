package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/settings"
)

func TestWriteSupport(t *testing.T) {
	var b bytes.Buffer
	devs := []device.Status{{Key: "compx:373B:1278", Name: "ATK wireless mouse", Via: "2.4 GHz receiver", Percent: 80}}
	if err := writeSupport(&b, devs, settings.Defaults()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"support file", "Windows ", "== Devices found", "compx:373B:1278", "== HID collections", "== Bluetooth devices"} {
		if !strings.Contains(out, want) {
			t.Errorf("support file lacks %q", want)
		}
	}
}
