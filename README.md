# go-battery-monitor

Kablosuz fare, klavye, kulaklık ve kumandaların pil seviyesini Windows bildirim alanında gösteren,
**saf Go** ile yazılmış küçük bir uygulama. cgo yok, hidapi yok, ek DLL yok: tek bir statik `.exe`.

> Durum: **erken iskelet.** HID taraması, tepsi ikonu, `--list` / `--dump` çalışıyor; henüz cihaz
> sürücüsü yok. Yol haritası aşağıda.

## Neden Go, RAM ne kadar?

Aynı makinede ölçüldü (Windows 11, Görev Yöneticisi'nin "Bellek" sütunu = özel çalışma kümesi):

| Uygulama | Özel çalışma kümesi | Toplam çalışma kümesi | Ayrılan (commit) | Exe |
|---|---|---|---|---|
| Bu iskelet (tepsi + 60 sn'de bir HID taraması) | **~2 MB** | ~11 MB | ~13 MB | ~1,6 MB |
| C++/Win32 referans uygulama (tam özellikli) | ~3,7 MB | ~27 MB | ~5 MB | ~0,35 MB |

Özet: Go'nun çalışma zamanı birkaç MB adres alanı ayırır ama gerçekten kullanılan RAM C++ ile aynı
seviyededir. Sürücüler eklendikçe hedef **< 8 MB** özel bellek. Bunu korumak için:

- **GUI çatısı yok.** Fyne, Wails (WebView2) gibi çatılar 50–150 MB tüketir. Tepsi ve görev çubuğu
  çizimi doğrudan Win32 (`user32`, `shell32`, `gdi32`) ile yapılır.
- **cgo yok.** HID erişimi `hid.dll` + `cfgmgr32` sistem çağrılarıyla (`golang.org/x/sys/windows`).
- Tepsi modunda `debug.SetGCPercent(50)` ve `debug.SetMemoryLimit(16 MiB)`.
- Yoklama (poll) tek bir goroutine'de, 60 saniyede bir; cihazlar takılınca/uyanınca ek yoklama.

## Kullanım

```
batterymon.exe           # tepside çalışır (sağ tık: Refresh now / Exit)
batterymon.exe --list    # bulunan cihazları ve pil seviyelerini yazar
batterymon.exe --dump    # tüm HID koleksiyonlarını yazar (yeni cihaz eklemek için)
```

## Derleme

Go 1.21 veya üstü (önerilen: en güncel kararlı sürüm, <https://go.dev/dl>).

```
powershell -File build.ps1
```

Çıktı: `bin\batterymon.exe` (`-H windowsgui`: konsol penceresi açılmaz).

## Mimari

```
cmd/batterymon/       giriş noktası: bayraklar, tepsi döngüsü, tooltip
internal/hid/         HID taraması + Open/Read/Write/GetFeature/SetFeature (overlapped I/O, zaman aşımı)
internal/device/      Status tipi, Driver arayüzü, PollContext (claim mantığı), kayıt sırası
internal/drivers/     her protokol ailesi için bir dosya; init() içinde device.Register
internal/tray/        Win32 mesaj penceresi, Shell_NotifyIcon, sağ tık menüsü
```

Bir sürücü şuna benzer:

```go
type myDriver struct{}

func (myDriver) Name() string { return "example" }

func (myDriver) Poll(ctx *device.PollContext) {
	for _, c := range ctx.Find(0x1234, func(c hid.Collection) bool { return c.UsagePage == 0xFF00 }) {
		d, err := hid.Open(c)
		if err != nil {
			continue
		}
		// ... komutu gönder, cevabı oku, yüzdeyi çöz ...
		d.Close()
		ctx.Add(device.Status{Key: "example:1234", Name: "Example mouse", Via: "2.4 GHz receiver",
			Kind: device.Mouse, Percent: 80}, c.VendorID, c.ProductID)
	}
}

func init() { device.Register(myDriver{}) }
```

## Lisans ve temiz oda (clean room)

Bu proje **MIT** lisanslıdır. GPL lisanslı projelerden kod kopyalanmaz veya çevrilmez; cihaz
protokolleri kendi USB yakalamalarımız ve analizlerimizle çıkarılır. Kurallar:
[docs/CLEAN_ROOM.md](docs/CLEAN_ROOM.md). Araştırma notları: [docs/RESEARCH.md](docs/RESEARCH.md).

## Yol haritası

1. **Görev çubuğu metni** (`🎧 40%  🖱 85%`), pil yüzdesini çizen tepsi ikonları (GDI), koyu/açık tema, DPI.
2. **Manifest + ikon** (`go-winres`): DPI farkındalığı, Common Controls v6.
3. **Sürücüler** (önce elimizdeki donanım):
   - Windows'un zaten bildiği Bluetooth pil seviyeleri (`DEVPKEY_Bluetooth_DeviceBattery`)
   - DualSense / DualShock 4 (Bluetooth ve USB giriş raporu)
   - ATK 8K fare alıcısı `373B:1278` (USB yakalamasıyla)
   - ATK N9 Pro kulaklık `373B:11CB` (sanal COM portu)
   - Xbox kumandaları (`Windows.Gaming.Input` / XInput)
4. Cihaz takılınca/uyanınca yenileme (`WM_DEVICECHANGE`, `WM_POWERBROADCAST`), Windows ile başlat,
   düşük pil bildirimi.
5. Kurulum paketi ve GitHub Actions ile otomatik sürüm.
