package main

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
)

// captions gives each device a tiny label of up to 3 characters when another visible device
// has the same kind (two mice would otherwise show the same glyph). A device alone in its
// kind gets "". Labels come from the name ("MX Master 3S" -> "MX", "G502 HERO" -> "G50");
// when that does not tell them apart, devices of that kind are numbered 1, 2, ...
func captions(st []device.Status) []string {
	out := make([]string, len(st))
	byKind := map[device.Kind][]int{}
	for i, s := range st {
		byKind[s.Kind] = append(byKind[s.Kind], i)
	}
	for _, idx := range byKind {
		if len(idx) < 2 {
			continue
		}
		seen := map[string]bool{}
		unique := true
		labels := make([]string, len(idx))
		for j, i := range idx {
			labels[j] = shortLabel(st[i].Name)
			if labels[j] == "" || seen[labels[j]] {
				unique = false
			}
			seen[labels[j]] = true
		}
		for j, i := range idx {
			if unique {
				out[i] = labels[j]
			} else {
				out[i] = strconv.Itoa(j + 1)
			}
		}
	}
	return out
}

// shortLabel picks a short, recognisable token from a device name: the first word that has a
// digit or is all capitals (model names such as "MX", "PRO", "G502"), else the initials.
func shortLabel(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, w := range words {
		if strings.EqualFold(w, "Logitech") || strings.EqualFold(w, "wireless") {
			continue
		}
		hasDigit := strings.IndexFunc(w, unicode.IsDigit) >= 0
		allCaps := len([]rune(w)) >= 2 && strings.ToUpper(w) == w && strings.IndexFunc(w, unicode.IsLetter) >= 0
		if hasDigit || allCaps {
			return clip(strings.ToUpper(w), 3)
		}
	}
	var initials []rune
	for _, w := range words {
		initials = append(initials, unicode.ToUpper([]rune(w)[0]))
	}
	return clip(string(initials), 3)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
