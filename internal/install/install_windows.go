// Package install makes the exe its own installer, per user and without admin rights: it
// copies itself to %LOCALAPPDATA%\Programs\go-battery-monitor, adds a Start menu shortcut
// and an entry in Settings > Apps (HKCU uninstall key), and can remove all of that again.
package install

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows/registry"
)

const (
	appDir      = "go-battery-monitor"
	exeName     = "batterymon.exe"
	displayName = "Battery Monitor"
	publisher   = "eneskaramanoglu"
)

// Variables so tests can redirect everything into a scratch area.
var (
	programsRoot = func() string { return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs") }
	startMenuDir = func() string {
		return filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`)
	}
	uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\go-battery-monitor`
)

// Dir is the install folder.
func Dir() string { return filepath.Join(programsRoot(), appDir) }

// Exe is the installed executable's path.
func Exe() string { return filepath.Join(Dir(), exeName) }

func shortcutPath() string { return filepath.Join(startMenuDir(), displayName+".lnk") }

// RunningInstalled reports whether the current process is the installed copy.
func RunningInstalled() bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(self), filepath.Clean(Exe()))
}

// Portable reports whether a "portable.txt" next to the exe asks never to install.
func Portable() bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(filepath.Dir(self), "portable.txt"))
	return err == nil
}

// Install copies the running exe into the install folder, creates the Start menu shortcut
// and the uninstall entry, and returns the installed exe's path.
func Install(version string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return "", err
	}
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(Exe())) {
		if err := copyFile(self, Exe()); err != nil {
			return "", fmt.Errorf("copy: %w", err)
		}
	}
	if err := createShortcut(shortcutPath(), Exe()); err != nil {
		return "", fmt.Errorf("shortcut: %w", err)
	}
	if err := writeUninstallEntry(version); err != nil {
		return "", fmt.Errorf("uninstall entry: %w", err)
	}
	return Exe(), nil
}

// Uninstall removes the shortcut and the uninstall entry. When removeFiles is set it also
// deletes the install folder once this process has exited (a running exe cannot delete
// itself), using a detached cmd.exe that waits two seconds.
func Uninstall(removeFiles bool) error {
	var errs []error
	if err := os.Remove(shortcutPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, uninstallKey); err != nil && !errors.Is(err, registry.ErrNotExist) {
		errs = append(errs, err)
	}
	if removeFiles {
		cmd := exec.Command("cmd.exe", "/c", "timeout /t 2 /nobreak >nul & rmdir /s /q \""+Dir()+"\"")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008 /* DETACHED_PROCESS */}
		if err := cmd.Start(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func writeUninstallEntry(version string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	var size uint32
	if fi, err := os.Stat(Exe()); err == nil {
		size = uint32(fi.Size() / 1024)
	}
	for name, v := range map[string]string{
		"DisplayName":     displayName,
		"DisplayVersion":  strings.TrimPrefix(version, "v"),
		"Publisher":       publisher,
		"InstallLocation": Dir(),
		"DisplayIcon":     Exe(),
		"UninstallString": `"` + Exe() + `" --uninstall`,
		"URLInfoAbout":    "https://github.com/eneskaramanoglu/go-battery-monitor",
	} {
		if err := k.SetStringValue(name, v); err != nil {
			return err
		}
	}
	for name, v := range map[string]uint32{"NoModify": 1, "NoRepair": 1, "EstimatedSize": size} {
		if err := k.SetDWordValue(name, v); err != nil {
			return err
		}
	}
	return nil
}

// createShortcut writes a .lnk with the Windows Script Host shell object.
func createShortcut(lnk, target string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		var oe *ole.OleError
		if !errors.As(err, &oe) || oe.Code() != 1 { // S_FALSE: already initialised
			return err
		}
	}
	defer ole.CoUninitialize()
	if err := os.MkdirAll(filepath.Dir(lnk), 0o755); err != nil {
		return err
	}
	unk, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return err
	}
	defer unk.Release()
	shell, err := unk.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer shell.Release()
	v, err := oleutil.CallMethod(shell, "CreateShortcut", lnk)
	if err != nil {
		return err
	}
	sc := v.ToIDispatch()
	defer sc.Release()
	for prop, val := range map[string]string{
		"TargetPath":       target,
		"WorkingDirectory": filepath.Dir(target),
		"Description":      "Battery levels of wireless devices on the taskbar",
		"IconLocation":     target + ",0",
	} {
		if _, err := oleutil.PutProperty(sc, prop, val); err != nil {
			return err
		}
	}
	_, err = oleutil.CallMethod(sc, "Save")
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
