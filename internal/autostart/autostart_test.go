package autostart

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// Uses a scratch key instead of the real Run key, and deletes it afterwards.
func TestSetAndEnabled(t *testing.T) {
	keyPath = `Software\go-battery-monitor-test\Run`
	t.Cleanup(func() {
		registry.DeleteKey(registry.CURRENT_USER, keyPath)
		registry.DeleteKey(registry.CURRENT_USER, `Software\go-battery-monitor-test`)
	})

	if Enabled() {
		t.Fatal("enabled before Set")
	}
	if err := Set(true); err != nil {
		t.Fatal(err)
	}
	if !Enabled() {
		t.Fatal("not enabled after Set(true)")
	}
	if err := Set(false); err != nil {
		t.Fatal(err)
	}
	if Enabled() {
		t.Fatal("still enabled after Set(false)")
	}
	if err := Set(false); err != nil {
		t.Fatalf("Set(false) twice: %v", err)
	}
}
