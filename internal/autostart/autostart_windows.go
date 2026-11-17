// Package autostart adds or removes the app from the current user's Run key, so it starts
// with Windows without needing admin rights.
package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// keyPath and valueName are variables so tests can point them elsewhere.
var (
	keyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName = "go-battery-monitor"
)

// Enabled reports whether the Run entry exists and points at this executable.
func Enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(valueName)
	if err != nil {
		return false
	}
	exe, err := executable()
	return err == nil && strings.EqualFold(strings.Trim(v, `"`), exe)
}

// Set adds (on) or removes (off) the Run entry for this executable.
func Set(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err := k.DeleteValue(valueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(valueName, `"`+exe+`"`)
}

func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe, nil
}
