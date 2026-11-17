# Araştırma notları (Kasım 2026)

## Go ile Windows tepsi uygulaması: kütüphane seçimi

| İhtiyaç | Seçim | Neden / alternatifler |
|---|---|---|
| Sistem çağrıları | `golang.org/x/sys/windows` | Resmi; `CM_Get_Device_Interface_List`, overlapped I/O hazır |
| HID | `hid.dll` doğrudan (bu repo, `internal/hid`) | `sstallion/go-hid`, `karalabe/hid` cgo + hidapi ister: tek statik exe olmaz |
| Tepsi | Win32 doğrudan (`internal/tray`) | `fyne.io/systray` kullanılabilir ama görev çubuğu çizimi için zaten Win32 gerekiyor |
| Arayüz | GDI ile kendi çizimimiz | Fyne / Wails / WebView2: 50–150 MB RAM, hedefe aykırı |
| Seri port (ATK kulaklık) | `CreateFile("\\.\COMx")` + `SetCommState` | Ek kütüphane gerekmez |
| Bluetooth pil | `cfgmgr32` cihaz özelliği (`DEVPKEY_Bluetooth_DeviceBattery`) | WinRT (`go-ole`) yalnızca gerekirse |
| Xbox kumanda | `xinput1_4.dll`, sonra `Windows.Gaming.Input` (WinRT) | |
| Manifest/ikon | `github.com/tc-hib/go-winres` | DPI farkındalığı ve Common Controls v6 için |

## Ölçümler

Bkz. README, "Neden Go, RAM ne kadar?". Prototip: saf Go, `HWND_MESSAGE` pencere + tepsi ikonu +
2 sn'de bir tüm HID arayüzlerini açıp `HidD_GetAttributes` okuma. Özel çalışma kümesi ~2,3 MB.
GC ayarı (`SetGCPercent(25)`, `SetMemoryLimit(8 MiB)`) bu yükte fark yaratmadı; büyük tamponlar
eklenince önemli hale gelir.

## Bu makinedeki cihazlar (`--dump`)

| Cihaz | VID:PID | Dikkate değer koleksiyonlar | Durum |
|---|---|---|---|
| ATK 8K fare alıcısı | `373B:1278` | MI 1: `FF02:0002` (17/17), `FF06:0002` (49/49), `FF04:0002` (feature 8) | Çerçeve formatı çözüldü ve donanımda doğrulandı; pil okuma doğrulanmadı ([compx.md](protocols/compx.md)) |
| ATK N9 Pro kulaklık alıcısı | `373B:11CB` | MI 3: `FF02:0002` (65/65); ayrıca sanal COM portu (COM3) | ATK yalnızca masaüstü uygulamasında seri port üzerinden (9600 baud) destekliyor; protokol web paketinde yok |
| DualSense | `054C:0CE6` | Bluetooth; bağlıyken HID görünür | Giriş raporu herkese açık belgelenmiş |
| NuPhy Kick75 | `19F5:1026` / `2620` | MI 3: `0001:0000` (65/65), NuPhy IO yapılandırma kanalı | Aşağıya bakın |
| Bilinmeyen | `1044:7A42` | `FF01:0001`, `FF00:0001` (65/65, feature 9) | Ne olduğu belirlenecek |

### NuPhy Kick75

NuPhy'nin web yapılandırıcısı (drive.nuphy.io) incelendi: Kick75, NuPhy'nin "V3" protokolünü
kullanıyor (64 baytlık çerçeve, oturum anahtarıyla XOR). Bu protokolün web istemcisinde pil okuma
yok; NuPhy'nin pil göstergesi yalnızca başka bir protokol ailesindeki WH80 için açık. Yani bilinen,
belgelenmiş bir pil komutu yok. Belgelenmemiş komut denemek klavye ayarlarını bozabileceği için
şimdilik desteklenmiyor.
