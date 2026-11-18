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
| ATK 8K fare alıcısı | `373B:1278` | MI 1: `FF02:0002` (17/17), `FF06:0002` (49/49), `FF04:0002` (feature 8) | Destekleniyor: `compx` sürücüsü, donanımda doğrulandı ([compx.md](protocols/compx.md)) |
| ATK N9 Pro kulaklık alıcısı | `373B:11CB` | MI 3: `FF02:0002` (65/65); ayrıca sanal COM portu (COM3) | Beklemede: kulaklık kendiliğinden veri göndermiyor (HID + COM3 pasif dinlendi). Protokol yalnızca ATK HUB masaüstü uygulamasında; indirme sunucusu `dl-global.atk.io` bu ağdan erişilemiyor (bkz. aşağı) |
| DualSense | `054C:0CE6` | Bluetooth; bağlıyken HID görünür | Giriş raporu herkese açık belgelenmiş |
| NuPhy Kick75 | `19F5:1026` / `2620` | MI 3: `0001:0000` (65/65), NuPhy IO yapılandırma kanalı | Aşağıya bakın |
| Bilinmeyen | `1044:7A42` | `FF01:0001`, `FF00:0001` (65/65, feature 9) | Ne olduğu belirlenecek |

### NuPhy Kick75

NuPhy'nin web yapılandırıcısı (drive.nuphy.io) incelendi: Kick75, NuPhy'nin "V3" protokolünü
kullanıyor (64 baytlık çerçeve, oturum anahtarıyla XOR). Bu protokolün web istemcisinde pil okuma
yok; NuPhy'nin pil göstergesi yalnızca başka bir protokol ailesindeki WH80 için açık. Yani bilinen,
belgelenmiş bir pil komutu yok. Belgelenmemiş komut denemek klavye ayarlarını bozabileceği için
şimdilik desteklenmiyor.

**Güncelleme (2026-11-18):**

- **NuPhy IO tekrar incelendi** (drive.nuphy.io, drive.nuphyio.com ve önizleme ortamı io-pre.nuphyio.com aynı sürüm): V3 komut tablosu (`GetBase 0xA0` … `SetIapMode 0xEF`) değişmemiş, pil komutu yok. Yapılandırıcının Kick75 özellik listesinde de pil yok. NuPhy'nin kendi pil göstergesi yalnızca "GCome" protokol ailesindeki WH80 / WH80 dongle için açık.
- **Pasif dinleme:** `go run ./tools/listen -vid 19F5 -pid 2620` (alıcı) ve `-pid 1026` (kablo) ile 15'er saniye dinlendi; klavye hiçbir koleksiyondan kendiliğinden rapor göndermiyor. Yani "WH80'deki gibi kendiliğinden gelen pil raporu" yolu da Kick75'te yok.
- **Kalan tek yol Bluetooth:** Kick75 Bluetooth ile eşlenirse ve standart BLE pil servisini (Battery Service 0x180F) sunuyorsa, Windows pil yüzdesini Ayarlar > Bluetooth'ta gösterir ve `bluetooth` sürücümüz bunu kendiliğinden okur. Bu henüz denenmedi (klavye şu an alıcı/kablo ile bağlı).
- Not: Kick75'in bir de QMK/VIA sürümü var (farklı bellenim). Bu notlar NuPhy IO sürümü (`19F5:1026`, ürün adı "NuPhy Kick75 IO") içindir.

### ATK N9 Pro (beklemede, 2026-11-18)

- `go run ./tools/listen -vid 373B -pid 11CB -com COM3 -baud 9600` ile 20 sn dinlendi: ne HID `FF02:0002`
  ne de COM3 kendiliğinden veri gönderiyor; pil ancak sorgu ile okunabilir.
- ATK'nın web sürücüsü N9 Pro'yu yalnızca masaüstü (Electron) sürümünde destekliyor (`isElectron`,
  `platforms: win32`, 9600 baud 8N1). Seri protokol web paketinde yok.
- Kaynak: ATK HUB masaüstü uygulaması. Sürüm API'si `https://api.vxe.com/v1/device/update/win32/version`
  şu adresleri veriyor: yükleyici `https://dl-global.atk.io/ATK-hub/atk-hub-3.2.33.exe` ve yalnızca uygulama
  kodu (asar) `https://dl-global.atk.io/ATK-hub/asar/83/app`. Bu sunucu (Cloudflare 188.114.96/97.x)
  geliştirme makinesinin ağından erişilemiyor; dosya elle indirilip analiz edilecek.
- GPL lisanslı projelerin N9 Pro koduna bakılmadı (temiz oda).
