package install

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// Installs the test binary into a temp folder with a scratch registry key, checks every
// artifact, then uninstalls. Nothing touches the real Start menu or Settings > Apps.
func TestInstallUninstall(t *testing.T) {
	root := t.TempDir()
	programsRoot = func() string { return filepath.Join(root, "Programs") }
	startMenuDir = func() string { return filepath.Join(root, "StartMenu") }
	uninstallKey = `Software\go-battery-monitor-test\Uninstall`
	t.Cleanup(func() {
		registry.DeleteKey(registry.CURRENT_USER, uninstallKey)
		registry.DeleteKey(registry.CURRENT_USER, `Software\go-battery-monitor-test`)
	})

	exe, err := Install("v9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(exe); err != nil || fi.Size() == 0 {
		t.Fatalf("installed exe missing: %v", err)
	}
	if _, err := os.Stat(shortcutPath()); err != nil {
		t.Fatalf("shortcut missing: %v", err)
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	ver, _, _ := k.GetStringValue("DisplayVersion")
	un, _, _ := k.GetStringValue("UninstallString")
	k.Close()
	if ver != "9.9.9" || un != `"`+exe+`" --uninstall` {
		t.Fatalf("uninstall entry: version %q, command %q", ver, un)
	}

	if err := Uninstall(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(shortcutPath()); !os.IsNotExist(err) {
		t.Fatal("shortcut still there")
	}
	if _, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE); err == nil {
		t.Fatal("uninstall entry still there")
	}
	if err := Uninstall(false); err != nil {
		t.Fatalf("second uninstall: %v", err)
	}
}
