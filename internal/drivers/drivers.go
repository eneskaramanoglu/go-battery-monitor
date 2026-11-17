// Package drivers holds one file per protocol family. Each file registers its driver in
// init() with device.Register; registration order is run order, so put vendor-specific
// drivers in files that sort before generic fallbacks (e.g. "zz_bluetooth.go").
//
// Clean-room rule: write drivers only from your own USB captures (Wireshark + USBPcap),
// --dump output, vendor apps/web drivers you analysed yourself, or permissively licensed /
// public-domain protocol notes. Never copy or translate code from GPL projects. See
// docs/CLEAN_ROOM.md.
package drivers
