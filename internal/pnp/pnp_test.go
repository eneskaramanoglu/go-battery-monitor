package pnp

import (
	"strings"
	"testing"
)

// Every Windows machine (including CI runners) has software devices under ROOT.
func TestRootDevicesHaveNames(t *testing.T) {
	ids, err := DeviceIDs("ROOT")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("no ROOT devices")
	}
	named := 0
	for _, id := range ids {
		if !strings.HasPrefix(strings.ToUpper(id), `ROOT\`) {
			t.Errorf("unexpected id %q", id)
		}
		n, err := Locate(id)
		if err != nil {
			t.Fatalf("Locate(%q): %v", id, err)
		}
		if n.Name() != "" {
			named++
		}
	}
	if named == 0 {
		t.Fatal("no ROOT device has a name: property reads are broken")
	}
}

// Logs the Bluetooth devices of the machine running the test (informational).
func TestBluetoothNodesReadable(t *testing.T) {
	ids, _ := DeviceIDs("BTHENUM")
	for _, id := range ids {
		if !strings.Contains(strings.ToUpper(id), `\DEV_`) {
			continue
		}
		n, err := Locate(id)
		if err != nil {
			t.Fatal(err)
		}
		vid, _ := n.Uint(KeyBluetoothVID)
		pid, _ := n.Uint(KeyBluetoothPID)
		cod, _ := n.Uint(KeyBluetoothClassOfDevice)
		conn, hasConn := n.Bool(KeyIsConnected)
		cont, _ := n.GUID(KeyContainerID)
		t.Logf("%s vid=%04X pid=%04X cod=%#x connected=%v(%v) container=%s category=%v",
			n.Name(), vid, pid, cod, conn, hasConn, cont, n.Strings(KeyContainerCategory))
	}
}
