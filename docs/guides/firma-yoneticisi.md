# Firma yöneticisi rehberi

Firma paneli: `/t/{kısa-ad}`. Menü adları panelde göründüğü gibidir. Bir öğe görünmüyorsa rolünüzde yetkisi yoktur; "paket dışı" etiketliyse firmanızın paketinde kapalıdır (okunur, değiştirilemez). Ayar ayrıntıları: [../handover/ayarlar.md](../handover/ayarlar.md).

Ekran görüntüleri: kabul çalıştırmasının kanıt klasöründen (`frontend/e2e/evidence/`) eklenecek.

## İlk kurulum sırası

| # | Menü | Yapılacak |
|---|------|-----------|
| 1 | Operasyon → **Şubeler** | "Merkez" (MRK) hazır gelir. Diğer şubeleri, çalışma saatlerini, hazırlık süresini, hizmet noktalarını (havalimanı, otel, adres bölgesi; teslimat ücreti) ekleyin |
| 2 | Filo → **Segmentler**, **Kiralama grupları**, **Ürünler** | Segment → grup (katalog model / tip ölçütleri) → satılan ürün ("alt grup"): garanti türü, kiralama koşulları (depozito, sürücü yaşı, ehliyet yılı, günlük km, km aşım ücreti), görseller |
| 3 | Filo → **Araçlar** → **İçe aktar** | Araç listesini dosyadan yükleyin (önce örnek dosya, ardından önizleme / fark), ya da tek tek ekleyin. Araçları gruplara bağlayın |
| 4 | Fiyatlandırma → **Tarifeler** | Ürün başına tarife ve gün bantları; **Taslak olarak kaydet**, sonra **Aktifleştir**. Toplu görünüm: Fiyat matrisi (`/pricing/rate-plans/matrix`) |
| 5 | Fiyatlandırma → **Tarih kuralları** | Sezon / dönem kuralları (bitiş günü dahil değil: "1 Haziran – 1 Eylül" 31 Ağustos'ta biter); **Aktifleştir** |
| 6 | Fiyatlandırma → **Ek hizmetler** (ve **Kampanyalar**) | Ek hizmet fiyatları, şube stoğu; kampanya / kupon |
| 7 | Fiyatlandırma → **Fiyat önizleme** | Bir tarih aralığı için fiyatın dökümünü kontrol edin |
| 8 | Ayarlar → **Genel** | Firma (ticari unvan, vergi no, fatura adresi), Rezervasyon & ödeme (Rezervasyon onayı, süreler, ön ödeme, geç iade ücretleri), **Politikalar** (iptal, no-show, erken iade, uzatma, yakıt, kilometre), **Satış kanalları**, **Vergi**. Her kayıt yeni sürümdür |
| 9 | Ayarlar → **Sözleşme şablonları**, **Teslim / iade formları** | Kiralama sözleşmesi, Teslim formu, İade formu, Kiralama koşulları şablonları: **Yeni şablon** → **Alan ekle** → **Aktifleştir**. Form şablonu: zorunlu fotoğraf slotları, ekipman listesi |
| 10 | Ayarlar → **Üyeler**, **Roller** | Davet gönderin; rol(ler) ve şube kapsamı (tüm şubeler / seçili şubeler) verin. Hazır roller: Sahip, Yönetici, Üye, Merkez Yöneticisi, Şube Yöneticisi, Rezervasyon / Satış, Muhasebe, Teslim / İade |
| 11 | Ayarlar → **Markalı site** | Tema, içerik, görseller, katalog görünürlüğü, SEO, widget; **Yayın**. **Alan adları** sekmesinde kendi alan adınızı ekleyin, TXT ve CNAME kayıtlarını DNS'e girin (doğrulama birkaç dakika – 72 saat) |
| 12 | Ayarlar → **Entegrasyonlar** | iyzico ödeme, Netgsm SMS, WhatsApp, e-Fatura, KABİS, HGS, Yolcu360, obilet hesaplarını bağlayın; **Test (sandbox) modu** ile deneyin. WhatsApp için ayrıca Ayarlar → **WhatsApp**'ta QR okutun. e-Fatura için Ayarlar → **Fatura ayarları** |
| 13 | Ayarlar → **Bildirimler** | Olay kuralları (kanallar, yedek kanal, hatırlatmalar), şablonlar; test gönderimi |
| 14 | Ayarlar → **Public API** (gerekirse) | Partner için anahtar ve webhook ([../handover/public-api.md](../handover/public-api.md)) |

Destek ekibinin panelinize girmesi gerekirse: Ayarlar → **Destek erişimi** → süre ve kapsam (Salt okunur / Tam yetki); işiniz bitince iptal edin.

## Günlük işler

| İş | Menü | Not |
|----|------|-----|
| Durum özeti | Firma → **Genel bakış** | Operasyon özeti: bugün ilgilenmeniz gerekenler |
| Onay bekleyen talepler | Rezervasyonlar → **Onay kuyruğu** | SLA süresine göre sıralı; **Kabul et** ya da **Reddet** (ret gerekçesi; ayrılan araç serbest kalır). Kural: Ayarlar → Genel → Rezervasyon onayı |
| Rezervasyon | Rezervasyonlar → **Rezervasyonlar**, **Yeni rezervasyon**, **Teklifler** | Detayda: Özet, Fiyat, Araç planı, Ödemeler, Belgeler, Müşteri talepleri, Çözüm görevleri, Zaman çizelgesi. İşlemler: Kesinleştir, Plaka değiştir / ata, Değiştir / Yükselt, Uzat, İptal et, Gelmedi (no-show) |
| Planlama sorunları | Rezervasyonlar → **Çözüm görevleri** | Araç planı bozulan rezervasyonlar |
| Müşteri talepleri | Rezervasyonlar → **Müşteri talepleri** | Portal / API'den gelen değişiklik ve iptal talepleri |
| Kiralama | Kiralamalar → **Kiralamalar** | Durumlar: Hazırlık, Kirada, İade kontrolü, İade edildi, İptal. Detayda Mobil teslim / Mobil iade, Uzat, Araç değiştir, Ücret öner, İade kontrolünü tamamla |
| Saha | Kiralamalar → **Görev panosu**, **Operasyon görevleri** | Teslimat / toplama / transfer / temizlik görevlerini atayın |
| İtirazlı iadeler | Kiralamalar → **Teslim / iade formları** (inceleme kuyruğu) | İmzasız ya da itirazlı iadeyi inceleyin; itirazlı ücreti onaylamak `tenant.damage.approve` + not ister, kendiliğinden borca dönüşmez |
| Resmî kayıtlar | **KABİS bildirimleri**, **HGS/OGS geçişleri**, **HGS inceleme kuyruğu** | "Bağımlılık bekliyor" / "Doğrulama gerekli" durumlarını izleyin |
| Tahsilat ve cari | Finans → **Tahsilatlar**, **Cari hesaplar**, **Depozitolar**, **Kasa ve banka** | Kayıtlar silinmez; düzeltme ters kayıtla |
| Fatura | Finans → **Faturalar** | Taslak → gönderim; durum "Sonuç bekleniyor" / "Başarısız" olanları takip edin |
| Gider | Finans → **Giderler**, **Tedarikçiler**, **Gider faturaları**, **Tedarikçi ödemeleri**, **Vadesi yaklaşan ödemeler** | |
| Raporlar | Raporlar → **Rapor merkezi**, **Doluluk**, **Gelir / gider**, **Kanal performansı**, **Cari / vade**, **Şube karşılaştırma**; Finans → **Araç kârlılığı** | |
| Dış işler | Ayarlar → **Entegrasyonlar** → hesap → işler | "Doğrulama gerekiyor" ya da "Başarısız" işte **Yeniden dene** önce sağlayıcıdan durumu sorar; çift gönderim olmaz |
