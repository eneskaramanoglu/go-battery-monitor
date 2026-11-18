# Razer BlackShark V2 Pro (2023), 2.4 GHz dongle `1532:0555`

Source: [matej-basic/blackshark-battery](https://github.com/matej-basic/blackshark-battery)
(MIT), README section "Protocol". That project credits HaloBattery (MIT) and OpenRazer PR #2862,
whose frame layout comes from Synapse USB captures. We used the documented facts only; no code
was copied.

## Transport

- Vendor collection `FF00:0001`, report id `2`, 63-byte input and output (64 with the id). No
  feature reports.
- Output frames are 64 bytes, zero padded.

| Frame | Bytes |
|---|---|
| Remote mode on | `02 80 07 00 00 50 41 0e 00 02 e1 01` |
| Remote mode off | `02 80 07 00 00 50 41 0e 00 02 e1 00` |
| Battery query | `02 80 08 00 00 50 41 08 00 03 21 00` |
| Charging query | `02 80 08 00 00 50 41 08 00 03 2a 00` |

A query is wrapped like Synapse does it: one extra "remote on" to wake the radio link, then
"remote on", the query, and finally "remote off". "Remote mode" is a link state, not a
setting stored in the headset; the driver always switches it off again.

Replies start `02 0e 50 49`; byte 12 echoes the command, byte 13 is `01` (ack), byte 15 is the
value: battery 0-100, charging `00` on battery / non-zero while charging. Unsolicited status
frames start `02 0d 50 49 01 c0` and are ignored.

## Our driver

`internal/drivers/razer_blackshark.go`: experimental, queries at most every 5 minutes (2 while
charging), shows the last value dimmed when the headset is off. Not tested on hardware by us.
