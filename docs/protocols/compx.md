# Compx mouse receivers (ATK / VXE, vendor ids 373B and 3554)

Source: our own analysis of ATK's public web configurator (ATK V HUB, hub.atk.pro, version
3.2.33), plus tests on a `373B:1278` "Wireless mouse 8k dongle-L" receiver. No code from other
projects was used.

## Transport

- ATK's device table maps `373B:1278` to its "COMPX" mouse controller and opens the interface
  with the vendor usage page `FF04`. On Windows each collection is a separate handle; the
  command reports live in the collection **`FF02:0002`** (17-byte input and output reports).
- Requests are **output report 8** carrying a 16-byte frame; answers are **input report 8**
  whose first byte repeats the command. The vendor app waits up to 3 seconds for an answer.

## Frame

| Byte | Meaning |
|---|---|
| 0 | command |
| 1 | status (answer): 0 success, 1 fail, 2 EEPROM error, 0xFF not supported |
| 2–3 | EEPROM address, big endian |
| 4 | number of valid data bytes |
| 5–14 | data |
| 15 | checksum = `0x55 - (0x08 + sum(bytes 0..14))` (mod 256) |

Verified on hardware: the receiver answers with a frame whose checksum follows this rule.

## Commands

| Id | Name | Notes |
|---|---|---|
| 1 | DownLoadData | sent by the vendor app before reading mouse info; 8 data bytes, the first 4 random; the answer carries cid, mid and device type |
| 3 | GetWirelessMouseOnline | |
| 4 | GetBatteryLevel | answer data: level %, charging flag, voltage mV (big endian, 2 bytes) |
| 7 | **SetEEPROM** | **writes**: never send |
| 8 | GetEEPROM | |
| 9 | **RestoreFactory** | **never send** |
| 16 | GetMouseCIDMID | |
| 18 | GetMouseVersion | |
| 29 | GetDongleVersion | |

## Status

- `GetBatteryLevel` alone got **no answer** within 3.5 s from the `373B:1278` receiver while the
  mouse was awake. The vendor app always sends `DownLoadData` first in the same session, which
  suggests an identification/unlock step. Not verified yet.
- The driver (`internal/drivers/compx.go`) is therefore **not registered**.

## Incident log

- 2026-11-18: while analysing, command 7 was mistakenly sent 4 times with address 0 and
  length 0 (read from the wrong command table). In this family 7 is SetEEPROM. The receiver
  answered "fail" each time and the mouse kept working normally (battery still readable by
  other software afterwards). Lesson: always confirm the command table that the frame class
  actually uses before sending anything.
