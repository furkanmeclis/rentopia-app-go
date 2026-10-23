# Gözlemleme: hata izleme, başarısız iş uyarıları, ortam ayrımı

TEC-744 (§16). Kod: [`backend/internal/observability`](../../backend/internal/observability), [`backend/internal/logging`](../../backend/internal/logging), [`backend/internal/platform/envguard`](../../backend/internal/platform/envguard), [`modules/platformhealth`](../../backend/internal/modules/platformhealth).

## Ortam (`APP_ENV`)

| Değer | Kullanım |
|-------|----------|
| `development` (varsayılan) | Yerel geliştirme; örnek gizli bilgiler kabul edilir |
| `staging` | Prova stack'i (ayrı `NAME_PREFIX`, genelde geri yüklenmiş prod verisi) |
| `production` | Canlı |

- Tek anahtar: backend, worker, frontend ve site aynı `APP_ENV`'i okur (`compose.prod.yml` hepsine geçirir; şema varsayılanı `production`). Bilinmeyen değerde API açılmaz.
- `development` dışında örnek gizli bilgiler (JWT, `APP_ENCRYPTION_KEY`, …) reddedilir.
- Hata izleme ortamı = `APP_ENV` (`SENTRY_ENVIRONMENT` doluysa o). Sürüm: `SENTRY_RELEASE` (boşsa backend build revizyonu).
- Site: `production` dışında `robots.txt` her şeyi kapatır (staging indekslenmez).

### Sağlayıcılar: sandbox / prod seçimi

| Sağlayıcı | Nasıl seçilir | `production` dışında |
|-----------|---------------|----------------------|
| KABİS | Firma entegrasyon hesabı: `kabis` (resmî) / `kabis_fake` | Resmî adaptör **reddedilir** (`Ready`/`Check` hata döner, işler `waiting_dependency`'de kalır, hiçbir şey gönderilmez). Açmak: `KABIS_ALLOW_PRODUCTION_IN_NON_PROD=true` |
| e-Fatura entegratörü | Hesap: `einvoice` (resmî) / `einvoice_fake` | Aynı kural; `EINVOICE_ALLOW_PRODUCTION_IN_NON_PROD=true` |
| Netgsm SMS | Hesap modu: `live` → `NETGSM_LIVE_BASE_URL`, `sandbox` → `NETGSM_SANDBOX_BASE_URL` | `live` hesap, URL Netgsm'in resmî host'u (`*.netgsm.com.tr`) iken **reddedilir** (kesin red, kod `environment`). Live URL `netgsm-fake`'e yönlendirilmişse çalışır. Açmak: `NETGSM_ALLOW_PRODUCTION_IN_NON_PROD=true` |
| iyzico | Hesap modu → `IYZICO_SANDBOX_BASE_URL` / `IYZICO_LIVE_BASE_URL` | Koruma yok: staging'de `IYZICO_LIVE_BASE_URL` → `cmd/iyzico-fake` ([backup-restore](./backup-restore.md#staging-yalıtımı-zorunlu)) |
| Fake / log sağlayıcılar (`*_fake`, `SMS_LOG_PROVIDER`) | — | Yalnız `production` dışında kayıtlı |

`*_ALLOW_PRODUCTION_IN_NON_PROD` anahtarları `production`'da yok sayılır; staging'de yalnız gözetimli kabul testi için, iş bitince `false`.

## Hata izleme (Sentry protokolü)

DSN boşsa her şey kapalı (no-op). Anahtarlar: `BACKEND_SENTRY_DSN` (API + worker), `FRONTEND_SENTRY_DSN` (panel: sunucu, edge, tarayıcı), `SITE_SENTRY_DSN` (site: sunucu + edge).

| Olay | Etiketler |
|------|-----------|
| HTTP panic, 5xx | `request_id`, `http.route`, `organization_uuid`, kullanıcı yalnız iç `id` |
| Asynq görev hatası (son deneme = error, öncekiler warning) | `task_type`, `task_id`, `queue`, `retry`, yük içindeki `*_id`'ler (`organization_id`, `job_id`, …) |
| Entegrasyon işi `failed` / `needs_verification` | `job_kind`, `job_uuid`, `job_status`, `provider`, `organization_id` |
| ERROR log satırı | mesaj başına dakikada bir |

İstek gövdesi, çerez, sorgu dizesi, yetki başlıkları gönderilmez; e-posta, telefon, token metinden temizlenir. Kapanışta (API, worker) bekleyen olaylar gönderilir (`Flush`).

## Loglar

- stdout: JSON (`time`, `level`, `msg`, öznitelikler); bağlamlı loglarda `request_id` ve `organization_uuid` otomatik eklenir.
- Maskeleme (stdout + log görüntüleyici `app_logs`): anahtarı `password`, `token`, `secret`, `authorization`, `api_key`, `cookie`, `credential`, `iban`, `card`, `cvv`, `otp`, `pin` … içeren değerler `[redacted]`; metin içinde Bearer/Basic, JWT, `token=…`, TR IBAN, Luhn geçen kart numarası maskelenir (`logging/redact.go`, test: `redact_test.go`).

## Başarısız iş uyarıları

| Tetik | Kaynak |
|-------|--------|
| Asynq görevi yeniden denemeleri tükendi (arşiv / "ölü") | worker `ErrorHandler` → `AlertDeadTask` |
| `integration_jobs` → `failed` veya `needs_verification` | runner dinleyicisi (API ve worker) |
| Kaynakların yeni kritik sorunları (5 dk tarama, TEC-739) | `RunAlerts` (değişmedi) |

- Alıcılar: `platform.health.read` yetkili aktif platform kullanıcıları, uygulama içi bildirim (organizasyonsuz, platform kabuğunda); `ALERT_EMAIL` doluysa aynı uyarı e-postayla (Türkçe).
- Tekilleştirme: kaynak + iş türü başına 15 dakikada bir uyarı (Redis `SET NX`, anahtar `alerts:failure:<kaynak>:<tür>`; Redis yoksa süreç içi).
- Platform sağlık panosu → Bağlantı sağlığı → "Kuyruklar ve işler": kuyruk başına bekleyen / işleniyor / zamanlanmış / yeniden denenecek / ölü sayıları (Asynq inspector) ve durum başına entegrasyon işi sayısı (`GET /v1/platform/health` → `queues`, `integration_jobs`).
- Bilinen sınır: API içi worker (`QUEUE_WORKER_INPROCESS=true`, yalnız yerel) ölü görev uyarısı göndermez; Sentry'ye raporlar.

## Performans hedefi: müsaitlik araması

Test: `TestSearchPilotLoad` ([`availability/usecase/perf_test.go`](../../backend/internal/modules/availability/usecase/perf_test.go)), `TEST_DATABASE_URL` yoksa atlanır.

- Veri: 3 şube, 150 araç (5 model), 5 GROUP ürün, 12 aya yayılmış 2.000 rezervasyon.
- Ölçüm: tüm ürünler için tam arama, rastgele 1–7 günlük 200 istek (10 ısınma hariç).
- Bütçe: p95 ≤ 1000 ms (`AVAILABILITY_P95_BUDGET_MS` ile değişir); aşarsa test kırılır.

| Tarih | Ortam | p50 | p95 | max |
|-------|-------|-----|-----|-----|
| 2026-10-23 | Docker Postgres 18.6, 4 vCPU geliştirme konteyneri, istekler sıralı | 285 ms | 383 ms | 602 ms |

```bash
cd backend && TEST_DATABASE_URL=postgres://… go test ./internal/modules/availability/usecase -run SearchPilotLoad -v -count=1
```
