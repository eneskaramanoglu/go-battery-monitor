// Package device defines what a battery reading looks like and how drivers plug in.
package device

import (
	"fmt"
	"sort"

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
	Percent    int // 0..100, -1 when unknown
	Charging   bool
	Asleep     bool // receiver answered, device sleeping: keep showing the last value
	Millivolts int  // 0 when the device does not report a voltage
}

func (s Status) String() string {
	level := "?"
	if s.Percent >= 0 {
		level = fmt.Sprintf("%d%%", s.Percent)
	}
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

// PollAll enumerates HID once and runs every driver over it. Blocking: call it off the UI thread.
func PollAll() ([]Status, error) {
	cols, err := hid.Enumerate()
	if err != nil {
		return nil, err
	}
	ctx := &PollContext{Collections: cols, claimed: map[vidpid]bool{}}
	for _, d := range drivers {
		func() {
			defer func() { recover() }() // one broken driver must not take the app down
			d.Poll(ctx)
		}()
	}
	sort.SliceStable(ctx.Results, func(i, j int) bool { return ctx.Results[i].Kind < ctx.Results[j].Kind })
	return ctx.Results, nil
}
