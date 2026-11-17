// Package settings stores user preferences as JSON in %APPDATA%\go-battery-monitor.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// Settings are the user's preferences. The zero value is not valid; use Defaults or Load.
type Settings struct {
	LowThreshold int      `json:"lowThreshold"` // percent at or below which a device counts as low
	Notify       bool     `json:"notify"`       // show a notification when a device gets low
	Hidden       []string `json:"hidden"`       // device keys whose icon is hidden
	Display      string   `json:"display"`      // DisplayTaskbar or DisplayTray
}

// Display modes.
const (
	DisplayTaskbar = "taskbar" // text on the taskbar next to the notification area
	DisplayTray    = "tray"    // one notification-area icon per device
)

// Thresholds offered in the menu.
var Thresholds = []int{5, 10, 15, 20, 25, 30}

// Defaults returns the settings used before the user changes anything.
func Defaults() Settings { return Settings{LowThreshold: 15, Notify: true, Display: DisplayTaskbar} }

// dir is a variable so tests can redirect it.
var dir = func() string {
	base, err := os.UserConfigDir() // %APPDATA%
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "go-battery-monitor")
}

var mu sync.Mutex

// Path is the settings file location.
func Path() string { return filepath.Join(dir(), "settings.json") }

// Load reads the settings, falling back to defaults for a missing or broken file.
func Load() Settings {
	mu.Lock()
	defer mu.Unlock()
	s := Defaults()
	b, err := os.ReadFile(Path())
	if err != nil {
		return s
	}
	if json.Unmarshal(b, &s) != nil {
		return Defaults()
	}
	if s.LowThreshold < 1 || s.LowThreshold > 99 {
		s.LowThreshold = Defaults().LowThreshold
	}
	if s.Display != DisplayTaskbar && s.Display != DisplayTray {
		s.Display = DisplayTaskbar
	}
	return s
}

// Save writes the settings atomically (temp file + rename), so a crash never leaves half a file.
func Save(s Settings) error {
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(dir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, Path())
}

// IsHidden reports whether a device's icon is hidden.
func (s Settings) IsHidden(key string) bool { return slices.Contains(s.Hidden, key) }

// SetHidden hides or shows a device's icon.
func (s *Settings) SetHidden(key string, hidden bool) {
	s.Hidden = slices.DeleteFunc(s.Hidden, func(k string) bool { return k == key })
	if hidden {
		s.Hidden = append(s.Hidden, key)
	}
}
