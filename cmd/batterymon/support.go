package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/sys/windows"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/settings"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/version"
)

// writeSupportFile saves everything needed to add or debug a device into a dated text file
// on the Desktop (Documents if there is no Desktop) and returns its path. It contains device
// names and ids only: no settings paths, user names or other personal data.
func writeSupportFile(devices []device.Status, cfg settings.Settings) (string, error) {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	if err != nil || dir == "" {
		if dir, err = windows.KnownFolderPath(windows.FOLDERID_Documents, 0); err != nil {
			dir = os.TempDir()
		}
	}
	path := filepath.Join(dir, "batterymon-support-"+time.Now().Format("2006-01-02-1504")+".txt")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := writeSupport(f, devices, cfg); err != nil {
		return "", err
	}
	return path, nil
}

func writeSupport(w io.Writer, devices []device.Status, cfg settings.Settings) error {
	v := windows.RtlGetVersion()
	fmt.Fprintf(w, "batterymon %s support file, %s\n", version.String(), time.Now().Format(time.RFC3339))
	fmt.Fprintf(w, "Windows %d.%d build %d, %s\n", v.MajorVersion, v.MinorVersion, v.BuildNumber, runtime.GOARCH)
	fmt.Fprintf(w, "Settings: display=%s experimental=%v language=%q ignore=%v\n\n", cfg.Display, cfg.Experimental, cfg.Language, cfg.Ignore)

	fmt.Fprintln(w, "== Devices found")
	if len(devices) == 0 {
		fmt.Fprintln(w, "(none)")
	}
	for _, s := range devices {
		fmt.Fprintf(w, "%-24s %s kind=%v brand=%q mV=%d\n", s.Key, s, s.Kind, s.Brand, s.Millivolts)
	}
	fmt.Fprintln(w)
	return writeDump(w)
}

// showInExplorer opens Explorer with the file selected.
func showInExplorer(path string) {
	exec.Command("explorer.exe", "/select,", path).Start()
}
