# Temiz oda (clean room) kuralları

Bu proje MIT lisanslı kalmak zorunda. Benzer işi yapan açık kaynak projelerin çoğu GPL lisanslı;
onlardan **kod kopyalamak veya satır satır başka dile çevirmek** bu projeyi GPL türevi yapar.

## Serbest olanlar

- Kendi cihazlarımızda yaptığımız **USB/Bluetooth yakalamaları** (Wireshark + USBPcap) ve
  `batterymon --dump` çıktıları.
- Üreticinin kendi masaüstü uygulamasını veya **web sürücüsünü** (WebHID) analiz etmek, ne
  gönderip ne aldığını gözlemlemek.
- Telif hakkı olmayan **olgular**: VID/PID değerleri, rapor kimlikleri, bayt konumları,
  "pil yüzdesi 5. baytta" gibi bilgiler. Bunları kendi kelimelerimizle belgeleriz.
- MIT / BSD / Apache / kamu malı lisanslı protokol notları ve kodları (lisans şartlarına uyarak,
  `THIRD_PARTY_NOTICES` dosyasında atıfla).
- Microsoft'un resmi Windows API belgeleri.

## Yasak olanlar

- GPL / LGPL / AGPL lisanslı projelerin kaynak kodunu kopyalamak, çevirmek, yapısını taklit etmek.
- Böyle bir projenin kodunu açıp, ekranda dururken sürücü yazmak.

## Önerilen iş akışı

1. `--dump` ile cihazın HID koleksiyonlarını bul (hangi usage page vendor tanımlı, rapor boyları).
2. Üretici yazılımı çalışırken Wireshark + USBPcap ile trafiği yakala; pil değişince değişen
   baytı bul (şarja tak/çıkar, farklı pil seviyelerinde tekrar yakala).
3. Bulguları `docs/protocols/<marka>.md` içine kendi cümlelerinle yaz (komut, cevap, bayt anlamları).
4. Sürücüyü yalnızca bu nota bakarak yaz; mümkünse sadece **okuma** komutları kullan, cihazın
   ayarlarını değiştirebilecek yazma komutlarını gönderme.
5. Yakalanmış örnek cevaplarla birim testi ekle.

## Not

Bu repo, GPL lisanslı bir C++ projesi incelendikten sonra başlatıldı. İskelet kodu (HID katmanı,
tepsi, sürücü arayüzü) o projeden bağımsız olarak Windows API belgelerine göre yazıldı. Sürücüler
yukarıdaki kurallarla, kendi analizlerimizden yazılacak.
