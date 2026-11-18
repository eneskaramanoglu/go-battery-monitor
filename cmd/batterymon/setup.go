package main

import (
	"os/exec"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/autostart"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/i18n"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/install"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/settings"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/version"
)

const (
	mbOK           = 0x00
	mbYesNo        = 0x04
	mbIconInfo     = 0x40
	mbIconQuestion = 0x20
	mbIconError    = 0x10
	idYes          = 6
)

func messageBox(text string, flags uintptr) int {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(i18n.T("Battery monitor"))
	r, _, _ := windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0,
		uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags)
	return int(r)
}

// offerInstall asks once, for release builds started from outside the install folder,
// whether to install. It returns true when the installed copy was started instead.
func offerInstall() bool {
	cfg := settings.Load()
	if version.Version == "dev" || demo || install.RunningInstalled() || install.Portable() || cfg.InstallDeclined {
		return false
	}
	i18n.Set(cfg.Language)
	if messageBox(i18n.T("Install Battery Monitor for this user?\n\nIt is copied to your programs folder and gets a Start menu entry and an uninstaller in Settings > Apps. No administrator rights are needed.\n\nChoose No to run it from here without installing."),
		mbYesNo|mbIconQuestion) != idYes {
		cfg.InstallDeclined = true
		settings.Save(cfg)
		return false
	}
	return installAndRestart()
}

// installAndRestart installs this exe and starts the installed copy.
func installAndRestart() bool {
	exe, err := install.Install(version.Version)
	if err != nil {
		messageBox(i18n.F("Installation failed: %v", err), mbOK|mbIconError)
		return false
	}
	releaseInstanceLock() // let the installed copy become the running instance
	if err := exec.Command(exe).Start(); err != nil {
		messageBox(i18n.F("Installation failed: %v", err), mbOK|mbIconError)
		return false
	}
	return true
}

// uninstall stops a running copy, removes autostart, the shortcut, the Settings > Apps entry
// and the install folder. Settings in %APPDATA% are kept.
func uninstall() {
	quitRunning()
	time.Sleep(500 * time.Millisecond)
	autostart.Set(false)
	if err := install.Uninstall(true); err != nil {
		messageBox(i18n.F("Uninstall failed: %v", err), mbOK|mbIconError)
		return
	}
	messageBox(i18n.F("Battery Monitor has been removed. Your settings are kept in %s.", filepath.Dir(settings.Path())), mbOK|mbIconInfo)
}
