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

## Desteklenen cihazlar

| Sürücü | Cihazlar | Kaynak |
|---|---|---|
| `compx` | ATK / VXE ve diğer Compx alıcılı fareler (`373B`, `3554`), ör. ATK 8K dongle `373B:1278` (gerçek cihazda test edildi). Fare uyurken son değer gri gösterilir | Kendi analizimiz: [docs/protocols/compx.md](docs/protocols/compx.md) |
| `playstation` | DualSense, DualSense Edge: USB kabloyla; Bluetooth'ta yalnızca başka bir uygulama (ör. Steam) tam rapor modunu açtıysa. *Gerçek cihazda henüz test edilmedi.* | Giriş raporundaki pil baytı (pasif okuma) |
| `xinput` | Xbox kumandaları ve onlara takılı kulaklıklar (kaba seviye: Empty / Low / Medium / Full) | Microsoft XInput API |
| `bluetooth` | Windows'un pil seviyesini bildiği her Bluetooth cihaz (BLE fare/klavye, eller serbest kulaklıklar...) | Windows cihaz özelliği `DEVPKEY_Bluetooth_Battery` |

## Görünüm

**Görev çubuğunda (varsayılan):** bildirim alanının hemen solunda `🎧 40%  🖱 85%` biçiminde, Windows'un kendi ikon fontuyla (Segoe Fluent Icons / MDL2). Şarj olurken yeşil şimşek, düşük pilde kırmızı, uyuyan cihaz gri. Görev çubuğuna gömülü şeffaf bir pencere (`UpdateLayeredWindow`) kullanılır; tıklayınca menü açılır. Görev çubuğu dikeyse otomatik olarak tepsi ikonlarına geçer.

**Tepsi ikonları:** Her cihaz kendi tepsi ikonunda yüzdesini gösterir: normal (temaya göre beyaz/siyah), %15 ve altı kırmızı, şarj olurken yeşil, uyurken gri. Kaba seviye bildiren cihazlarda baş harf (E/L/M/F). İkonlar GDI ile çizilir; tema ve DPI değişince yeniden çizilir. Önizleme: `go run ./tools/iconpreview out.png`.

Seviyeler 60 saniyede bir okunur; bir HID cihaz takılınca/çıkarılınca (2 sn sonra) ve bilgisayar uykudan uyanınca (8 sn sonra) hemen yenilenir. Sağ tık menüsündeki **Start with Windows** yönetici izni gerektirmeden `HKCU\...\Run` kaydını açar/kapatır.

## Ayarlar ve bildirimler

Sağ tık menüsü: **Devices** (cihaz ikonlarını gizle/göster), **Low battery level** (eşik: %5–30), **Low battery notifications**, **Start with Windows**. Bir cihaz eşiğe indiğinde bir kez Windows bildirimi gösterilir; şarja takılınca veya eşiğin 5 puan üstüne çıkınca yeniden kurulur. Ayarlar `%APPDATA%\go-battery-monitor\settings.json` dosyasında saklanır.

## Kullanım

```
batterymon.exe           # tepside çalışır (sağ tık: Refresh now / Exit)
batterymon.exe --list    # bulunan cihazları ve pil seviyelerini yazar
batterymon.exe --dump    # tüm HID koleksiyonlarını yazar (yeni cihaz eklemek için)
batterymon.exe --demo    # örnek cihazlarla tepsi arayüzünü gösterir (geliştirme)
```

## Derleme

Go 1.27 veya üstü (<https://go.dev/dl>, ya da `winget install GoLang.Go`).

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

## Sürüm yayınlama

`v0.1.0` gibi bir etiket push'landığında GitHub Actions (`.github/workflows/release.yml`) testleri
çalıştırır, amd64 ve arm64 exe'lerini sürüm bilgisiyle derler, zip + SHA256SUMS üretir ve GitHub
Release yayınlar:

```
git tag v0.1.0
git push origin v0.1.0
```

## Yol haritası

- [x] HID katmanı, tepsi, sürücü arayüzü, CI
- [x] İkon, manifest (PerMonitorV2 DPI), sürüm bilgisi
- [x] Bluetooth (Windows pil özelliği), XInput, DualSense sürücüleri
- [x] Yüzde çizen tepsi ikonları, görev çubuğu metni, tema/DPI uyumu
- [x] Cihaz takma / uykudan uyanma yenilemesi, Windows ile başlat
- [x] Düşük pil bildirimi, ayarlar (eşik, gizli cihazlar, görünüm)
- [x] Otomatik sürüm yayınlama (GitHub Actions)
- [x] ATK / Compx fare alıcıları ([notlar](docs/protocols/compx.md))
- [ ] ATK N9 Pro kulaklık (seri port protokolü)
- [ ] DualShock 4, `Windows.Gaming.Input` ile Xbox kumandalarında gerçek yüzde
- [ ] Ayarlar penceresi, çoklu dil (TR / EN)
