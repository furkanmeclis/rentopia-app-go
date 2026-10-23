# Public rezervasyon API'si

Sözleşme: `backend/docs/openapi.yaml`, etiket **Public Booking** (ve **Branded Site**, **Payments**, **Documents**). Tasarım: [../design/public-api.md](../design/public-api.md). Kod: `backend/internal/modules/publicapi`, panel `frontend/src/features/public-api`.

## Taban adres

| Kim | Adres |
|-----|-------|
| Partner / firmanın kendi sunucusu | `https://<app host>/api/v1/public/…` (panelin BFF'i geçirir). Panelde Ayarlar → Public API özeti aynı tabanı gösterir (`PUBLIC_FRONTEND_URL` + `/api`) |
| Markalı site | Sunucu tarafı, iç ağdan `API_URL` (`…/v1/public/…`) |
| Yerel | `http://127.0.0.1:8080/v1/public/…` |

BFF yalnız şu kökleri dışarıya geçirir: `sessions`, `locations`, `vehicle-subgroups`, `extras`, `availability`, `quotes`, `reservations` (`frontend/src/lib/server/public-booking.ts`). `campaigns` ve `site/leads` markalı sitenin sunucu tarafından çağrılır.

## Kimlik doğrulama

Organizasyon her zaman kimlik bilgisinden çıkar; URL ya da gövdeden alınmaz.

| Model | Kullanım | Ayrıntı |
|-------|----------|---------|
| Gizli anahtar `rtp_sk_…` | Sunucudan sunucuya | `Authorization: Bearer rtp_sk_…`. Tarayıcı origin'i taşıyan istek reddedilir (`SECRET_KEY_IN_BROWSER`) |
| Yayımlanabilir anahtar `rtp_pk_…` + oturum `rtp_ps_…` | Markalı site, widget | `POST /v1/public/sessions {publishable_key}` anahtarın `allowed_origins` listesindeki origin'den → 30 dk'lık oturum token'ı; yalnız o origin'den geçerli. Anahtar bearer olarak kullanılmaz |
| `X-Reservation-Token` | Rezervasyon okuma | `GET /v1/public/reservations/{reference}` için; gizli anahtar kendi yaptığı rezervasyonda gerekmez. Uyuşmazlık 404 |
| `Idempotency-Key` | Her yazma | Zorunlu (`IDEMPOTENCY_KEY_REQUIRED`); 24 saat saklanır (`public_api_idempotency`). Aynı anahtar + farklı gövde `IDEMPOTENCY_KEY_REUSED`, işlemdeyken `IDEMPOTENCY_IN_PROGRESS` |

Her çağrıda: firma aktif ve erişim penceresinde (`ORGANIZATION_UNAVAILABLE`), anahtarın satış kanalı firma ayarlarında açık (`CHANNEL_DISABLED`), rota kapsamı (`INSUFFICIENT_SCOPE`). Kapsamlar: `catalog.read`, `quotes.write`, `reservations.write`, `reservations.read` (yeni anahtar hepsini alır). Anahtar döndürme: eski anahtar varsayılan 24 saat (0–168) çalışır; iptal anında.

## Uç noktalar

| Yöntem ve yol | Kapsam | Not |
|---------------|--------|-----|
| `POST /v1/public/sessions` | — | Tarayıcı oturumu (yayımlanabilir anahtar) |
| `GET /v1/public/locations` | `catalog.read` | Şubeler, online seçilebilir hizmet noktaları, çalışma saatleri |
| `GET /v1/public/vehicle-subgroups` | `catalog.read` | Yayında ürünler ("alt grup"), kiralama koşulları, görsel yolları |
| `GET /v1/public/extras` | `catalog.read` | Kanalda satılan ek hizmetler |
| `GET /v1/public/availability` | `catalog.read` | Alt gruplar fiyatlı; `pickup_branch_uuid`, `return_branch_uuid`, tarih, `subgroup_uuids` |
| `POST /v1/public/quotes` | `quotes.write` | `Idempotency-Key`; teklifi fiyatlar ve saklar |
| `GET /v1/public/quotes/{uuid}` | `quotes.write` | Aynı kanalın teklifi |
| `POST /v1/public/reservations` | `reservations.write` | `Idempotency-Key`; teklif yeniden doğrulanır; kiracı her zaman yeni müşteri kartı olur |
| `GET /v1/public/reservations/{reference}` | `reservations.read` | `X-Reservation-Token` |
| `POST /v1/public/reservations/{reference}/change-requests` | `reservations.write` | `Idempotency-Key`; rezervasyon başına bir açık talep (`REQUEST_EXISTS`); personel panelde yanıtlar (Müşteri talepleri) |
| `POST /v1/public/reservations/{reference}/cancellation-requests` | `reservations.write` | `Idempotency-Key`; politika izin veriyorsa hemen iptal (`outcome: self_cancelled`) |
| `POST /v1/public/reservations/{reference}/payment-links` | `reservations.write` | `Idempotency-Key`; ön ödeme ya da kalan tutar için `/pay/{token}`; `NOTHING_DUE`, `PAYMENT_UNAVAILABLE` |
| `GET /v1/public/campaigns`, `GET /v1/public/campaigns/{uuid}` | `catalog.read` (web vitrin oturumu) | Görünür, geçerli kampanyalar |
| `POST /v1/public/site/leads` | `catalog.read` (canlı site anahtarı) | Site formları; KVKK onayı zorunlu; `Idempotency-Key` gerekmez |

Diğer `/v1/public/*` aileleri (Public Booking dışı): `site/*` (host çözümleme, görseller, makaleler, sitemap), `pay/{token}` + `payments/iyzico/webhook`, `documents/{token}` (uzaktan sözleşme onayı), `media/*`, `whatsapp/webhook/{token}`.

Hata kodları (zarf standardı): `UNAUTHENTICATED`, `ORIGIN_NOT_ALLOWED`, `SECRET_KEY_IN_BROWSER`, `INSUFFICIENT_SCOPE`, `CHANNEL_DISABLED`, `ORGANIZATION_UNAVAILABLE`, `IDEMPOTENCY_*`, `RATE_LIMITED`, `VALIDATION_ERROR`, `CAPACITY_UNAVAILABLE`, `QUOTE_EXPIRED`, `QUOTE_USED`, `QUOTE_PRICE_CHANGED`, `PRICING_UNAVAILABLE`, `DRIVER_NOT_ELIGIBLE`, `EQUIPMENT_UNAVAILABLE`, `REQUEST_EXISTS`, `NOTHING_DUE`, `PAYMENT_UNAVAILABLE`, `INVALID_STATE`, `NOT_FOUND`. Katalog: [backend/docs/api/errors.md](../../backend/docs/api/errors.md).

## Paket kapısı

| Anahtar kanalı | Özellik | Kapalıyken |
|----------------|---------|------------|
| `web` | `site` | Oturum / çağrı `403 FEATURE_NOT_ENABLED`; site host'u `404 SITE_NOT_FOUND` (planı açığa vurmaz) |
| `widget` | `widget` | `403 FEATURE_NOT_ENABLED` |
| `api` | `public_api` | `403 FEATURE_NOT_ENABLED`; panelde yeni anahtar / webhook oluşturma da kapalı |

Mevcut anahtarlar silinmez; özellik yeniden açılınca çalışır (test: `publicapi/features_test.go`, RT-A28). Ayrıca kanalın firma ayarlarında (Ayarlar → Genel → Satış kanalları) açık olması gerekir.

## Webhook'lar

| Konu | Kural |
|------|-------|
| Olaylar | `reservation.created`, `reservation.confirmed`, `reservation.changed`, `reservation.cancelled` (iptal, süre dolumu, no-show), `reservation.plate_fixed`; yalnız Public API ile yapılan rezervasyonlar |
| Yük | Referans, durum, ödeme durumu, tarihler, şubeler, alt grup, kanal, tutarlar, `external_reference`; kişisel veri yok |
| Başlıklar | `Rentopia-Webhook-Id`, `Rentopia-Webhook-Event`, `Rentopia-Webhook-Timestamp`, `Rentopia-Webhook-Signature: v1=<hex HMAC-SHA256(secret, "<timestamp>.<body>")>` |
| Başarı | Herhangi bir 2xx |
| Yeniden deneme | 1 dk, 5 dk, 30 dk, 2 sa, 6 sa, 12 sa, 24 sa; 8 deneme sonra `failed`. Panelde elle yeniden dene, test ping, sırrı döndür (bir kez gösterilir) |
| Hedef | Yalnız public `https`; özel / loopback / link-local adresler kayıtta ve bağlantıda reddedilir (development ve testler hariç) |

## Hız sınırları

`internal/platform/ratelimit` (Redis INCR + TTL; Redis yoksa açık geçer).

| Sınır | Değer |
|-------|-------|
| IP başına | 600 / dk |
| Kimlik başına | Anahtarın `rate_limit_per_minute` değeri (gizli anahtar: anahtar başına; yayımlanabilir: tarayıcı oturumu başına) |
| Rota | Teklif 120 / dk, rezervasyon 20 / 10 dk, değişiklik / iptal talebi 10 / sa, ödeme linki 20 / 10 dk |
| Oturum açma | 30 / dk / IP |

Aşımda `429 RATE_LIMITED` + `Retry-After`.

## Belgeler (Scalar)

API `GET /docs/` (Scalar) ve `GET /docs/openapi.yaml` sunar (`internal/httpserver/docs.go`). Yerelde `http://127.0.0.1:8080/docs/`. Prod'da backend dışarı açık değil ve BFF `/docs`'u geçirmez: partnerlere `backend/docs/openapi.yaml` dosyası verilir ya da `/docs/` iç ağdan açılır.

## Test erişimi

1. Test firmasında Ayarlar → **Public API** (`/t/{slug}/settings/public-api`, okuma `tenant.public_api.read`, yazma `tenant.public_api.manage`, özellik `public_api`) → **API anahtarları** sekmesinde anahtar oluştur (kanal, kapsamlar, origin'ler, dakika sınırı). Gizli anahtar bir kez gösterilir.
2. Ayarlar → Genel → Satış kanalları'nda ilgili kanal (Web sitesi / Rezervasyon widget'ı / API) açık olmalı.
3. Ödeme denemek için test firmasına sandbox iyzico hesabı bağlanır (yerelde `cmd/iyzico-fake`, [DEVELOPMENT.md](../DEVELOPMENT.md#provider-fakes)).
4. Webhook'lar: **Webhook'lar** sekmesi; teslimatlar **Teslimatlar** sekmesinde.

## Yolcu360 / Obilet eşlemesi

Pazar yerleri Public API'yi kullanmaz; kanal yöneticisi (`modules/channels`, panel Kanal yöneticisi `/t/{slug}/channels`) ile bağlanır. Ayrıntı: [../design/channel-manager.md](../design/channel-manager.md).

| Konu | Public API | Kanal yöneticisi |
|------|-----------|------------------|
| Yön | Partner bizi çağırır | Biz envanter iteriz (`PushInventory`), mesaj çekeriz (`PullMessages`) + onaylarız (`Acknowledge`), mutabakat (`Statement`) |
| Kimlik | `rtp_sk_` anahtarı | Firmanın entegrasyon hesabı (kind `channel`): Yolcu360 `supplier_code`, `api_key`, `api_secret`, isteğe bağlı `base_url`; Obilet `partner_code`, `api_token`, isteğe bağlı `base_url` |
| Ürün eşleme | `vehicle-subgroups` uuid | `channel_mappings` (ürün kodu, lokasyon kodu) |
| Rezervasyon | `POST /v1/public/reservations` | Kanal mesajı → `channel_reservations` → rezervasyon (`external_reference`, `collecting_party`) |
| Kapasite | Kanal satışı | `channel_capacity_rules` (stop-sale, kota) |
| Özellik | `public_api` | `channel.yolcu360`, `channel.obilet` |

Açık: Yolcu360 bağlantı yönü, ürün kodlarını kimin ürettiği, müsaitlik biçimi, kesinleşme noktası, yeniden deneme / iptal kuralları yazılı teyit bekliyor (TEC-680). Obilet'in araç kiralama tedarikçi yolları ve alan adları varsayımlı (`platform/channels/obilet/obilet.go`; TEC-681). İkisinde de test yolu `yolcu360_fake` / `obilet_fake` (`scenario`: ok / outage / reject).
