// Package i18n translates the user interface. English text is the key, so code stays
// readable and a missing translation falls back to English.
package i18n

import (
	"fmt"
	"sync/atomic"

	"golang.org/x/sys/windows"
)

// Languages offered in the menu, by code; "" means "follow Windows".
var Languages = []struct{ Code, Name string }{
	{"en", "English"},
	{"tr", "Türkçe"},
}

var catalogs = map[string]map[string]string{
	"tr": tr,
}

var current atomic.Value // string

func init() { current.Store("en") }

// Set selects a language by code; "" or an unknown code follows the Windows display language.
func Set(code string) {
	if _, ok := catalogs[code]; ok || code == "en" {
		current.Store(code)
		return
	}
	current.Store(System())
}

// Current returns the active language code.
func Current() string { return current.Load().(string) }

// System returns the code matching the Windows display language, or "en".
func System() string {
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")
	id, _, _ := proc.Call()
	switch id & 0x3FF { // primary language id
	case 0x1F:
		return "tr"
	}
	return "en"
}

// T translates s into the current language.
func T(s string) string {
	if c := catalogs[Current()]; c != nil {
		if t, ok := c[s]; ok {
			return t
		}
	}
	return s
}

// F translates a format string, then formats it.
func F(format string, args ...any) string { return fmt.Sprintf(T(format), args...) }
