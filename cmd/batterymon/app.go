package main

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/autostart"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/i18n"
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
	i18n.Set(a.cfg.Language)
	err := tray.Run(tray.Config{
		Tooltip:        i18n.T("Battery monitor: reading devices..."),
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
		a.mu.Lock()
		cfg := a.cfg
		a.mu.Unlock()
		st, err := poll(cfg)
		a.mu.Lock()
		a.polling = false
		a.last, a.lastErr = st, err
		cfg = a.cfg
		a.mu.Unlock()

		// Always run the tracker so turning notifications on later does not fire stale warnings.
		lows := a.low.check(st, cfg.LowThreshold)
		if cfg.Notify {
			for _, s := range lows {
				a.t.Notify(s.Key, i18n.F("Low battery: %s", name(s)), i18n.F("%s is at %s.", name(s), levelText(s)))
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
	var visible []device.Status
	for _, s := range st {
		if !cfg.IsHidden(s.Key) {
			visible = append(visible, s)
		}
	}
	tip := i18n.T("Battery monitor")
	switch {
	case err != nil:
		tip += ": " + err.Error()
	case len(st) == 0:
		tip += ": " + i18n.T("no supported device")
	case len(visible) == 0:
		tip += ": " + i18n.T("all devices hidden")
	}

	// Taskbar mode: text on the taskbar, a single app icon in the tray for the menu.
	// Falls back to per-device tray icons when the taskbar is vertical or missing.
	if cfg.Display == settings.DisplayTaskbar && tray.TaskbarAvailable() {
		items := make([]tray.BarItem, 0, len(visible))
		for _, s := range visible {
			items = append(items, barItemFor(s, cfg.LowThreshold))
		}
		if len(visible) > 0 {
			tip = summary(visible)
		}
		a.t.SetTooltip(tip)
		a.t.SetIcons(nil)
		a.t.SetTaskbar(items, !cfg.MainMonitorOnly)
		return
	}
	icons := make([]tray.Icon, 0, len(visible))
	for _, s := range visible {
		icons = append(icons, iconFor(s, cfg.LowThreshold))
	}
	a.t.SetTooltip(tip)
	a.t.SetTaskbar(nil, false)
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

func (a *app) menu(target string) []tray.MenuItem {
	a.mu.Lock()
	st, cfg := a.last, a.cfg
	a.mu.Unlock()

	// Right-clicked on a device: offer to hide it first. It comes back from Devices.
	var head []tray.MenuItem
	for _, s := range st {
		if s.Key == target && target != "" {
			key := s.Key
			head = []tray.MenuItem{
				{Title: name(s) + "  " + levelText(s), Disabled: true},
				{Title: i18n.T("Hide this device"), OnClick: func() {
					a.update(func(c *settings.Settings) { c.SetHidden(key, true) })
				}},
				{},
			}
		}
	}

	var devices []tray.MenuItem
	for _, s := range st {
		key, hidden := s.Key, cfg.IsHidden(s.Key)
		devices = append(devices, tray.MenuItem{
			Title:   name(s) + "  (" + levelText(s) + ")",
			Checked: !hidden,
			OnClick: func() { a.update(func(c *settings.Settings) { c.SetHidden(key, !hidden) }) },
		})
	}
	if len(devices) == 0 {
		devices = []tray.MenuItem{{Title: i18n.T("No device found"), Disabled: true}}
	}

	var thresholds []tray.MenuItem
	for _, v := range settings.Thresholds {
		thresholds = append(thresholds, tray.MenuItem{
			Title:   strconv.Itoa(v) + "%",
			Checked: cfg.LowThreshold == v,
			OnClick: func() { a.update(func(c *settings.Settings) { c.LowThreshold = v }) },
		})
	}

	return append(head, []tray.MenuItem{
		{Title: i18n.T("Battery monitor") + " " + version.String(), Disabled: true},
		{},
		// How the levels are shown: directly in the menu so it is one right click away.
		{Title: i18n.T("Show on the taskbar"), Checked: cfg.Display == settings.DisplayTaskbar, OnClick: func() {
			a.update(func(c *settings.Settings) { c.Display = settings.DisplayTaskbar })
		}},
		{Title: i18n.T("Show as tray icons"), Checked: cfg.Display == settings.DisplayTray, OnClick: func() {
			a.update(func(c *settings.Settings) { c.Display = settings.DisplayTray })
		}},
		{Title: i18n.T("Show on"), Children: []tray.MenuItem{
			{Title: i18n.T("All monitors"), Checked: !cfg.MainMonitorOnly, OnClick: func() {
				a.update(func(c *settings.Settings) { c.MainMonitorOnly = false })
			}},
			{Title: i18n.T("Main monitor only"), Checked: cfg.MainMonitorOnly, OnClick: func() {
				a.update(func(c *settings.Settings) { c.MainMonitorOnly = true })
			}},
		}},
		{},
		{Title: i18n.T("Devices"), Children: devices},
		{Title: i18n.T("Low battery level"), Children: thresholds},
		{Title: i18n.T("Low battery notifications"), Checked: cfg.Notify, OnClick: func() {
			a.update(func(c *settings.Settings) { c.Notify = !c.Notify })
		}},
		{Title: i18n.T("Experimental drivers (untested devices)"), Checked: cfg.Experimental, OnClick: func() {
			a.update(func(c *settings.Settings) { c.Experimental = !c.Experimental })
			a.refresh()
		}},
		{Title: i18n.T("Start with Windows"), Checked: autostart.Enabled(), OnClick: func() {
			autostart.Set(!autostart.Enabled())
		}},
		{Title: i18n.T("Language"), Children: a.languageMenu(cfg)},
		{},
		{Title: i18n.T("Refresh now"), OnClick: a.refresh},
		{Title: i18n.T("Exit"), OnClick: func() { a.t.Quit() }},
	}...)
}

// languageMenu lists Automatic plus every language; choosing one applies it at once.
func (a *app) languageMenu(cfg settings.Settings) []tray.MenuItem {
	set := func(code string) func() {
		return func() {
			i18n.Set(code)
			a.update(func(c *settings.Settings) { c.Language = code })
			a.refresh() // device names and levels are rebuilt in the new language
		}
	}
	items := []tray.MenuItem{{Title: i18n.T("Automatic"), Checked: cfg.Language == "", OnClick: set("")}}
	for _, l := range i18n.Languages {
		items = append(items, tray.MenuItem{Title: l.Name, Checked: cfg.Language == l.Code, OnClick: set(l.Code)})
	}
	return items
}
