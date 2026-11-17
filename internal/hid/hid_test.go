package hid

import "testing"

func TestInterfaceNumber(t *testing.T) {
	cases := map[string]int{
		`\\?\hid#vid_373b&pid_1278&mi_01&col03#8&2c1f&0&0002#{4d1e55b2-f16f-11cf-88cb-001111000030}`: 1,
		`\\?\HID#VID_19F5&PID_1026&MI_0A#9&1&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}`:          10,
		`\\?\hid#vid_1462&pid_7c37#6&1b2&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}`:              -1,
		`\\?\hid#{00001124-0000-1000-8000-00805f9b34fb}_vid&0002054c_pid&0ce6#a&1&0&0000`:            -1,
	}
	for path, want := range cases {
		if got := interfaceNumber(path); got != want {
			t.Errorf("interfaceNumber(%q) = %d, want %d", path, got, want)
		}
	}
}
