# Logitech HID++ (battery reading)

Sources (no GPL code was read):

- Logitech's own public documentation: [Logitech/cpg-docs](https://github.com/Logitech/cpg-docs) (HID++ 2.0 packet structure, IRoot `0x0000`).
- [libratbag](https://github.com/libratbag/libratbag) (MIT): request/response matching, error codes, features `0x1000` and `0x1001`.
- [OpenLogi `openlogi-hidpp`](https://github.com/AprilNEA/OpenLogi) (0BSD): features `0x1004`, `0x1001` flags and `0x0005`.

## Transport

- Vendor collections: `FF00:0001` (short report `0x10`, 7 bytes) and `FF00:0002` (long report `0x11`, 20 bytes) on USB receivers and corded devices; `FF43:0202` (long only) on Bluetooth devices.
- Packet: `report id | device index | feature index | function << 4 | software id | parameters`.
- Device index: `0xFF` for the receiver itself or a corded / Bluetooth device; `1..6` for devices paired to a receiver.
- We send long reports only (every HID++ 2.0 device accepts them) and read answers from both collections, because HID++ 1.0 errors come back as short reports.
- A reply matches when report id is `0x10`/`0x11`, device index, feature index and function/software-id byte are equal to the request.
- Errors:
  - HID++ 2.0: byte 2 = `0xFF`, byte 3 = feature index, byte 4 = function/sw id, byte 5 = error code.
  - HID++ 1.0: byte 2 = `0x8F`, same layout. Code `0x01` (invalid sub id) means the device only speaks HID++ 1.0; `0x08` (unknown device) means nothing is paired at that index; `0x09` (resource error) means the device is asleep or out of range.

## Requests (all read-only)

| Feature | Function | Parameters | Answer |
|---|---|---|---|
| IRoot `0x0000` | 0 `GetFeature` | feature id (big endian) | [0] feature index (0 = not supported) |
| IRoot `0x0000` | 1 `GetProtocolVersion` | `0, 0, ping` | [0] major, [1] minor, [2] ping echo; used to find which receiver slots have a device |
| Device name and type `0x0005` | 0 count, 1 name chunk (char index), 2 type | | name bytes; type 0 keyboard, 3 mouse, 4 trackpad, 5 trackball, 8 headset, 11 joystick, 12 gamepad |
| Unified battery `0x1004` | 0 capabilities | | [1] bit 1 = state of charge is a real percentage |
| Unified battery `0x1004` | 1 status | | [0] state of charge %, [1] level bit (1 critical, 2 low, 4 good, 8 full), [2] status: 0 discharging, 1 charging, 2 charging nearly full, 3 full, 4 slow charging, 5-7 errors |
| Battery level status `0x1000` | 0 | | [0] level %, [1] next level, [2] status: 0 discharging, 1 recharging, 2 final stage, 3 complete, 4 below optimal speed, 5+ errors |
| Battery voltage `0x1001` | 0 | | [0..1] millivolts (big endian), [2] flags: bit 7 external power; with external power, low 3 bits 0 charging, 1 full, 2 not charging |

`0x1001` gives no percentage; ours is an approximate single-cell Li-ion curve (`voltageToPercent`).

## Status

Not tested on real hardware (no Logitech device available); the driver is **experimental** (off unless "Experimental drivers" is enabled). Tested against a scripted fake device in `logitech_test.go`.
