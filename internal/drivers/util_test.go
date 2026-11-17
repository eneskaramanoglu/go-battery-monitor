package drivers

import (
	"testing"
	"time"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
)

func TestLastKnown(t *testing.T) {
	var l lastKnown
	if _, ok := l.get("m"); ok {
		t.Fatal("empty cache returned a value")
	}
	l.set("m", device.Status{Key: "m", Percent: 80})
	if s, ok := l.get("m"); !ok || s.Percent != 80 {
		t.Fatalf("get = %+v, %v", s, ok)
	}
	l.m["m"] = lastEntry{l.m["m"].s, time.Now().Add(-staleAfter - time.Minute)}
	if _, ok := l.get("m"); ok {
		t.Fatal("stale reading returned")
	}
}
