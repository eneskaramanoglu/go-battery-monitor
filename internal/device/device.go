// Package device defines what a battery reading looks like and how drivers plug in.
package device

import (
	"fmt"
	"sort"
	"strings"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/hid"
)

// Kind is the device category; it picks the icon shown next to the number.
type Kind int

const (
	Other Kind = iota
	Mouse
	Keyboard
	Headset
	Gamepad
)

func (k Kind) String() string {
	return [...]string{"other", "mouse", "keyboard", "headset", "gamepad"}[k]
}

// Status is one device's battery state at the moment of a poll.
type Status struct {
	Key        string // stable id across polls, e.g. "atk:373B:1278"
	Name       string // shown to the user
	Via        string // "2.4 GHz receiver", "Bluetooth", "USB cable", ...
	Kind       Kind
	Percent    int    // 0..100, -1 when unknown
	Level      string // coarse level ("Low", "Full"...) for devices that report no percentage
	Charging   bool
	Asleep     bool // receiver answered, device sleeping: keep showing the last value
	Millivolts int  // 0 when the device does not report a voltage
}

// LevelText is the battery level as shown to the user: "85%", "Low", or "?".
func (s Status) LevelText() string {
	switch {
	case s.Percent >= 0:
		return fmt.Sprintf("%d%%", s.Percent)
	case s.Level != "":
		return s.Level
	}
	return "?"
}

func (s Status) String() string {
	level := s.LevelText()
	if s.Charging {
		level += " (charging)"
	}
	if s.Asleep {
		level += " (asleep)"
	}
	return fmt.Sprintf("%-32s %-18s %s", s.Name, "["+s.Via+"]", level)
}

// Driver reads batteries for one protocol family. Poll inspects ctx.Collections, talks to
// the devices it recognises, reports them with ctx.Add and claims them so drivers that run
// later (generic fallbacks) do not report the same device twice.
type Driver interface {
	Name() string
	Poll(ctx *PollContext)
}

type vidpid struct{ vid, pid uint16 }

// PollContext is shared by every driver during one poll.
type PollContext struct {
	Collections []hid.Collection
	Results     []Status
	claimed     map[vidpid]bool
}

// Claimed reports whether an earlier driver already handled this product.
func (c *PollContext) Claimed(vid, pid uint16) bool { return c.claimed[vidpid{vid, pid}] }

// Claim marks a product as handled without reporting a result (e.g. device switched off).
func (c *PollContext) Claim(vid, pid uint16) { c.claimed[vidpid{vid, pid}] = true }

// Add reports a reading and claims its product.
func (c *PollContext) Add(s Status, vid, pid uint16) {
	c.Results = append(c.Results, s)
	c.Claim(vid, pid)
}

// Find returns the unclaimed collections of one vendor that match pred (nil = all).
func (c *PollContext) Find(vid uint16, pred func(hid.Collection) bool) []hid.Collection {
	var out []hid.Collection
	for _, col := range c.Collections {
		if col.VendorID == vid && !c.Claimed(col.VendorID, col.ProductID) && (pred == nil || pred(col)) {
			out = append(out, col)
		}
	}
	return out
}

var drivers []Driver

// Register adds a driver; call it from the driver's init(). Drivers run in registration
// order, so specific drivers must register before generic ones.
func Register(d Driver) { drivers = append(drivers, d) }

// Drivers lists the registered drivers in run order.
func Drivers() []Driver { return drivers }

// Experimental is implemented by drivers that were written from documentation but never
// tested on real hardware; they only run when Options.Experimental is set.
type Experimental interface {
	Experimental() bool
}

// IsExperimental reports whether a driver is marked experimental.
func IsExperimental(d Driver) bool {
	e, ok := d.(Experimental)
	return ok && e.Experimental()
}

// Options tune one poll.
type Options struct {
	// Ignore lists products the app must never open, as "VID:PID" in hex ("28BD:0947").
	Ignore []string
	// Experimental enables drivers that are not tested on real hardware.
	Experimental bool
}

// tabletVendors make drawing tablets, which some drivers misbehave with when their HID
// interfaces are opened by another program; they never carry a battery we could read.
var tabletVendors = map[uint16]bool{
	0x056A: true, // Wacom
	0x256C: true, // Huion / Gaomon
	0x28BD: true, // XP-Pen
	0x5543: true, // UC-Logic
}

// ParseVIDPID parses "VID:PID" in hex.
func ParseVIDPID(s string) (vid, pid uint16, ok bool) {
	var v, p uint32
	if n, err := fmt.Sscanf(strings.TrimSpace(s), "%x:%x", &v, &p); n != 2 || err != nil || v > 0xFFFF || p > 0xFFFF {
		return 0, 0, false
	}
	return uint16(v), uint16(p), true
}

// filterCollections drops ignored products and drawing tablets.
func filterCollections(cols []hid.Collection, ignore []string) []hid.Collection {
	skip := map[vidpid]bool{}
	for _, s := range ignore {
		if v, p, ok := ParseVIDPID(s); ok {
			skip[vidpid{v, p}] = true
		}
	}
	out := cols[:0:0]
	for _, c := range cols {
		if skip[vidpid{c.VendorID, c.ProductID}] || tabletVendors[c.VendorID] || c.UsagePage == 0x000D {
			continue
		}
		out = append(out, c)
	}
	return out
}

// PollAll enumerates HID once and runs every driver over it. Blocking: call it off the UI thread.
func PollAll(opts Options) ([]Status, error) {
	cols, err := hid.Enumerate()
	if err != nil {
		return nil, err
	}
	ctx := &PollContext{Collections: filterCollections(cols, opts.Ignore), claimed: map[vidpid]bool{}}
	for _, d := range drivers {
		if IsExperimental(d) && !opts.Experimental {
			continue
		}
		func() {
			defer func() { recover() }() // one broken driver must not take the app down
			d.Poll(ctx)
		}()
	}
	sort.SliceStable(ctx.Results, func(i, j int) bool { return ctx.Results[i].Kind < ctx.Results[j].Kind })
	return ctx.Results, nil
}
