package drivers

// Bluetooth devices whose battery level Windows already knows (Settings > Bluetooth shows it):
// BLE mice and keyboards (HID over GATT battery service), hands-free headsets (HFP AT+IPHONEACCEV /
// AT+BIEV) and others. Windows stores it as DEVPKEY_Bluetooth_Battery on one of the device's
// nodes; we group nodes by container id so each physical device is reported once.
//
// The file name starts with "zz_" so this generic fallback registers after the vendor drivers:
// a device a vendor driver already reported (same VID/PID) is skipped here.

import (
	"strings"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
	"github.com/eneskaramanoglu/go-battery-monitor/internal/pnp"
)

type bluetoothDriver struct{}

func init() { device.Register(bluetoothDriver{}) }

func (bluetoothDriver) Name() string { return "bluetooth" }

// btNode is what we read from one Bluetooth device node.
type btNode struct {
	ID        string
	Container string
	Name      string
	Battery   int   // -1 when the node has no battery property
	Connected *bool // nil when the node does not say
	VID, PID  uint16
	CoD       uint32   // class of device (classic Bluetooth only)
	Category  []string // container categories, e.g. "{Input.Mouse}"
}

// isDeviceNode reports whether the node is the device itself (BTHENUM\DEV_..., BTHLE\DEV_...)
// rather than one of its services; that node carries the name, ids and connection state.
func (n btNode) isDeviceNode() bool { return strings.Contains(strings.ToUpper(n.ID), `\DEV_`) }

func (bluetoothDriver) Poll(ctx *device.PollContext) {
	var nodes []btNode
	for _, enum := range []string{"BTHENUM", "BTHLE", "BTHLEDEVICE", "BTHHFENUM"} {
		ids, err := pnp.DeviceIDs(enum)
		if err != nil {
			continue
		}
		for _, id := range ids {
			if n, ok := readBTNode(id); ok {
				nodes = append(nodes, n)
			}
		}
	}
	for _, r := range collectBluetooth(nodes, ctx.Claimed) {
		ctx.Add(r.status, r.vid, r.pid)
	}
}

func readBTNode(id string) (btNode, bool) {
	dn, err := pnp.Locate(id)
	if err != nil {
		return btNode{}, false
	}
	n := btNode{ID: id, Battery: -1, Name: dn.Name()}
	n.Container, _ = dn.GUID(pnp.KeyContainerID)
	if n.Container == "" {
		return n, false
	}
	if b, ok := dn.Uint(pnp.KeyBluetoothBattery); ok && b <= 100 {
		n.Battery = int(b)
	}
	if c, ok := dn.Bool(pnp.KeyIsConnected); ok {
		n.Connected = &c
	}
	if v, ok := dn.Uint(pnp.KeyBluetoothVID); ok {
		n.VID = uint16(v)
	}
	if p, ok := dn.Uint(pnp.KeyBluetoothPID); ok {
		n.PID = uint16(p)
	}
	n.CoD, _ = dn.Uint(pnp.KeyBluetoothClassOfDevice)
	n.Category = dn.Strings(pnp.KeyContainerCategory)
	return n, true
}

type btResult struct {
	status   device.Status
	vid, pid uint16
}

// collectBluetooth groups nodes by container and turns each container that has a battery
// level and is connected into one Status. Pure function, unit tested.
func collectBluetooth(nodes []btNode, claimed func(vid, pid uint16) bool) []btResult {
	type group struct {
		dev     *btNode
		battery int
		names   []string
	}
	groups := map[string]*group{}
	var order []string
	for i := range nodes {
		n := &nodes[i]
		g := groups[n.Container]
		if g == nil {
			g = &group{battery: -1}
			groups[n.Container] = g
			order = append(order, n.Container)
		}
		if n.Battery > g.battery {
			g.battery = n.Battery
		}
		if n.isDeviceNode() && g.dev == nil {
			g.dev = n
		}
		if n.Name != "" {
			g.names = append(g.names, n.Name)
		}
	}

	var out []btResult
	for _, c := range order {
		g := groups[c]
		if g.battery < 0 {
			continue
		}
		var dev btNode
		if g.dev != nil {
			dev = *g.dev
		}
		if dev.Connected != nil && !*dev.Connected {
			continue // Windows keeps the last level of a switched-off device; do not show stale data
		}
		if dev.VID != 0 && claimed(dev.VID, dev.PID) {
			continue
		}
		name := dev.Name
		if name == "" && len(g.names) > 0 {
			name = g.names[0]
		}
		if name == "" {
			name = "Bluetooth device"
		}
		out = append(out, btResult{
			status: device.Status{
				Key:     "bt:" + strings.Trim(c, "{}"),
				Name:    name,
				Via:     "Bluetooth",
				Kind:    btKind(dev.Category, dev.CoD, name),
				Percent: g.battery,
			},
			vid: dev.VID, pid: dev.PID,
		})
	}
	return out
}

// btKind picks the category from Windows' container category, then the Bluetooth class of
// device, then the name.
func btKind(category []string, cod uint32, name string) device.Kind {
	for _, c := range category {
		c = strings.ToLower(strings.Trim(c, "{}"))
		switch {
		case strings.HasPrefix(c, "input.mouse") || strings.HasPrefix(c, "input.trackball"):
			return device.Mouse
		case strings.HasPrefix(c, "input.keyboard"):
			return device.Keyboard
		case strings.HasPrefix(c, "input.gaming") || strings.HasPrefix(c, "input.gamepad") || strings.HasPrefix(c, "input.joystick"):
			return device.Gamepad
		case strings.HasPrefix(c, "audio.headset") || strings.HasPrefix(c, "audio.headphone"):
			return device.Headset
		}
	}
	if cod != 0 {
		major := (cod >> 8) & 0x1F
		minor := (cod >> 2) & 0x3F
		switch major {
		case 0x04: // audio / video
			return device.Headset
		case 0x05: // peripheral
			switch {
			case minor&0x0F == 0x01 || minor&0x0F == 0x02: // joystick, gamepad
				return device.Gamepad
			case minor&0x30 == 0x20:
				return device.Mouse
			case minor&0x30 != 0: // keyboard, or keyboard + pointing combo
				return device.Keyboard
			}
		}
	}
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "mouse") || strings.Contains(n, "trackball"):
		return device.Mouse
	case strings.Contains(n, "keyboard") || strings.Contains(n, "keys"):
		return device.Keyboard
	case strings.Contains(n, "controller") || strings.Contains(n, "gamepad"):
		return device.Gamepad
	case strings.Contains(n, "buds") || strings.Contains(n, "head") || strings.Contains(n, "pods") ||
		strings.Contains(n, "earphone") || strings.Contains(n, "audio"):
		return device.Headset
	}
	return device.Other
}
