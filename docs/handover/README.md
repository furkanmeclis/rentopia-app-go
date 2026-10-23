# Teslim belgeleri

Rentopya yazılımcı teslim seti (TEC-743, §17). Her belge repodaki kod, yapılandırma ve mevcut belgelerle doğrulandı; doğrulanamayan her madde "doğrulanmadı" diye işaretli. Ekip içi belgedir: Linear kimlikleri (TEC-…) burada geçer, ürün arayüzünde geçmez.

## Belgeler

| Belge | İçerik |
|-------|--------|
| [kurulum.md](./kurulum.md) | Ön koşullar, `.env`, yerel çalıştırma, prod (`compose.prod.yml` + Dokploy), `scripts/prod-env.sh`, migration, ilk yönetici ve firma, site alan adı + HTTPS, yedek, gözlemleme, ortam ayrımı |
| [veri-modeli.md](./veri-modeli.md) | Alanlar ve ana tablolar, kiracılık (organization) kuralı, sürümlü kayıtlar, para / zaman kuralları, migration ekleme, seed verisi, testlerin firma kurması |
| [public-api.md](./public-api.md) | `/v1/public/*`: kimlik doğrulama, uç noktalar, plan kapısı, webhook, hız sınırı, test erişimi, Yolcu360 / Obilet eşlemesi |
| [ayarlar.md](./ayarlar.md) | Firma ayarları, üyeler / roller / şube kapsamı, paket ve özellikler, sağlayıcı bağlantıları, bildirimler, destek erişimi, antet, site; her biri için ekran, API, yetki |
| [kayit-takibi.md](./kayit-takibi.md) | İmzalı belgeler, finans kayıtları, dış işlem kayıtları (`integration_jobs`): nerede, nasıl incelenir (SQL + panel) |
| [test-ve-bilinenler.md](./test-ve-bilinenler.md) | Kabul sonuçlarının yeri, testlerin çalıştırılması, geri yükleme provası, açık kalan maddeler |
| [../guides/firma-yoneticisi.md](../guides/firma-yoneticisi.md) | Firma yöneticisi kullanım rehberi: ilk kurulum sırası, günlük işler |
| [../guides/teslim-iade-personeli.md](../guides/teslim-iade-personeli.md) | Teslim / iade personeli (mobil) rehberi |

İlgili mevcut belgeler: [../DEVELOPMENT.md](../DEVELOPMENT.md), [../ARCHITECTURE.md](../ARCHITECTURE.md), [../operations/backup-restore.md](../operations/backup-restore.md), [../operations/observability.md](../operations/observability.md), [../design/](../design), [../../backend/docs/README.md](../../backend/docs/README.md), kabul sonuçları [../acceptance/beta.md](../acceptance/beta.md) (paralel hazırlanıyor).

## Teslimdeki durum

Kod tarafı: aşağıdaki alanlar repoda uygulanmış ve test edilmiş durumda (Go testleri CI'da gerçek Postgres ile koşar; RT-A kabul testleri için [test-ve-bilinenler.md](./test-ve-bilinenler.md)).

| Alan | Durum |
|------|-------|
| Platform kabuğu (`/platform`), firma paneli (`/t/{slug}`), müşteri portalı (`/r/{orgSlug}`), markalı site (`site/`) | Uygulandı |
| Filo, fiyatlandırma, müsaitlik, rezervasyon, kiralama, teslim / iade formları, saha görevleri, sözleşmeler | Uygulandı |
| Finans (cari, tahsilat, depozito, kasa / banka, gider, tedarikçi), raporlar, araç kârlılığı | Uygulandı |
| Public API, widget, markalı site, özel alan adı | Uygulandı |
| Paket / özellik kapısı (TEC-674), destek erişimi (TEC-678), gözlemleme (TEC-744), yedek / geri yükleme (TEC-741) | Uygulandı |
| iyzico, Netgsm, WhatsApp (wuzapi) adaptörleri | Kodda var; iyzico ve Netgsm HTTP fake'leriyle (`cmd/iyzico-fake`, `cmd/netgsm-fake`), WhatsApp `whatsapp_fake` ile test edildi; firma hesaplarıyla canlı çalışma doğrulanmadı (hesaplar TEC-684 / TEC-686) |
| e-Fatura entegratörü, KABİS (resmî), HGS sağlayıcısı, Yolcu360, Obilet | Adaptör iskeleti / varsayımlı sözleşme + test (fake) sağlayıcıları; resmî bağlantı bağımlılık bekliyor (aşağıda) |

Bağımlılık bekleyenler (Linear M0 epiği). Hepsi [test-ve-bilinenler.md](./test-ve-bilinenler.md) "Açık maddeler" tablosunda da var.

| Ticket | Bekleyen | Etkilenen alan |
|--------|----------|----------------|
| TEC-679 | İdes pilot veri toplama | Filo / ürün / fiyat ilk yüklemesi, pilot kabul |
| TEC-680 | Yolcu360 teknik sözleşme ve test erişimi | `channel.yolcu360` (adaptör belgelenmiş kısma göre yazıldı, `yolcu360_fake` test yolu) |
| TEC-681 | Obilet iş bağlantısı ve test erişimi | `channel.obilet` (yol / alan adları varsayımlı, `obilet_fake`) |
| TEC-682 | KABİS resmî bağlantı yöntemi ve İdes yetkileri | `kabis` (resmî slot `waiting_dependency`) |
| TEC-683 | HGS sağlayıcı seçimi | `hgs` (yalnız `hgs_fake`) |
| TEC-684 | İdes için iyzico üye işyeri hesabı ve sandbox | Online ödeme, ödeme linkleri |
| TEC-685 | e-Belge entegratörü seçimi ve İdes mükellef hesabı | `einvoice` (resmî slot `waiting_dependency`) |
| TEC-686 | Bildirim sağlayıcı hesapları: SMS, e-posta, wuzapi, push | `sms`, `whatsapp`, SMTP, VAPID |
| TEC-687 | İdes'ten sözleşme / politika / KVKK metinleri | Sözleşme şablonları, firma politikaları, site metinleri |
| TEC-742 | Canlı kurulumun kendisi | Prod stack, alan adları, yedek zamanlaması |

Canlı (prod) kurulum TEC-742'ye bağlı ve açık; bu sette hiçbir bileşen için "canlıda çalışıyor" denmez (doğrulanmadı).
