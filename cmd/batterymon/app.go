package main

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/autostart"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/settings"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/tray"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/version"
)

// app is the tray-mode state: the latest readings, settings and the poll scheduling.
type app struct {
	t   *tray.Tray
	low *lowTracker

	mu      sync.Mutex
	cfg     settings.Settings
	last    []device.Status
	lastErr error
	polling bool
	timer   *time.Timer
}

func runTray() {
	a := &app{low: newLowTracker(), cfg: settings.Load()}
	err := tray.Run(tray.Config{
		Tooltip:        "Battery monitor: reading devices...",
		Menu:           a.menu,
		OnLeftClick:    a.refresh,
		OnDeviceChange: func() { a.refreshSoon(2 * time.Second) },
		// Wireless devices take a few seconds to reconnect after the PC wakes up.
		OnResume: func() { a.refreshSoon(8 * time.Second) },
		Ready: func(tr *tray.Tray) {
			a.t = tr
			a.refresh()
			go func() {
				for range time.Tick(pollInterval) {
					a.refresh()
				}
			}()
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "tray:", err)
		os.Exit(1)
	}
}

// refresh polls in the background; a poll already running makes it a no-op.
func (a *app) refresh() {
	a.mu.Lock()
	if a.polling {
		a.mu.Unlock()
		return
	}
	a.polling = true
	a.mu.Unlock()
	go func() {
		st, err := poll()
		a.mu.Lock()
		a.polling = false
		a.last, a.lastErr = st, err
		cfg := a.cfg
		a.mu.Unlock()

		// Always run the tracker so turning notifications on later does not fire stale warnings.
		lows := a.low.check(st, cfg.LowThreshold)
		if cfg.Notify {
			for _, s := range lows {
				a.t.Notify(s.Key, "Low battery: "+s.Name, s.Name+" is at "+s.LevelText()+".")
			}
		}
		a.render()
	}()
}

// refreshSoon coalesces bursts of events (a dongle exposes several HID interfaces, each
// raising its own arrival) into a single poll once things settle.
func (a *app) refreshSoon(delay time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.timer != nil {
		a.timer.Stop()
	}
	a.timer = time.AfterFunc(delay, a.refresh)
}

// render pushes the latest readings to the tray, honouring hidden devices.
func (a *app) render() {
	a.mu.Lock()
	st, err, cfg := a.last, a.lastErr, a.cfg
	a.mu.Unlock()
	icons := make([]tray.Icon, 0, len(st))
	for _, s := range st {
		if !cfg.IsHidden(s.Key) {
			icons = append(icons, iconFor(s, cfg.LowThreshold))
		}
	}
	tip := "Battery monitor"
	switch {
	case err != nil:
		tip += ": " + err.Error()
	case len(st) == 0:
		tip += ": no supported device"
	case len(icons) == 0:
		tip += ": all devices hidden"
	}
	a.t.SetTooltip(tip)
	a.t.SetIcons(icons)
}

// update changes the settings, saves them and redraws.
func (a *app) update(change func(*settings.Settings)) {
	a.mu.Lock()
	change(&a.cfg)
	cfg := a.cfg
	a.mu.Unlock()
	settings.Save(cfg)
	a.render()
}

func (a *app) menu() []tray.MenuItem {
	a.mu.Lock()
	st, cfg := a.last, a.cfg
	a.mu.Unlock()

	var devices []tray.MenuItem
	for _, s := range st {
		key, hidden := s.Key, cfg.IsHidden(s.Key)
		devices = append(devices, tray.MenuItem{
			Title:   s.Name + "  (" + s.LevelText() + ")",
			Checked: !hidden,
			OnClick: func() { a.update(func(c *settings.Settings) { c.SetHidden(key, !hidden) }) },
		})
	}
	if len(devices) == 0 {
		devices = []tray.MenuItem{{Title: "No device found", Disabled: true}}
	}

	var thresholds []tray.MenuItem
	for _, v := range settings.Thresholds {
		thresholds = append(thresholds, tray.MenuItem{
			Title:   strconv.Itoa(v) + "%",
			Checked: cfg.LowThreshold == v,
			OnClick: func() { a.update(func(c *settings.Settings) { c.LowThreshold = v }) },
		})
	}

	return []tray.MenuItem{
		{Title: "Battery monitor " + version.String(), Disabled: true},
		{},
		{Title: "Devices", Children: devices},
		{Title: "Low battery level", Children: thresholds},
		{Title: "Low battery notifications", Checked: cfg.Notify, OnClick: func() {
			a.update(func(c *settings.Settings) { c.Notify = !c.Notify })
		}},
		{Title: "Start with Windows", Checked: autostart.Enabled(), OnClick: func() {
			autostart.Set(!autostart.Enabled())
		}},
		{},
		{Title: "Refresh now", OnClick: a.refresh},
		{Title: "Exit", OnClick: func() { a.t.Quit() }},
	}
}
