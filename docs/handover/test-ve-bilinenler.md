# Testler, kabul sonuçları ve bilinen açıklar

## Kabul senaryoları

Senaryo → test → sonuç → kanıt tablosu (RT-A01…RT-A32): [../acceptance/beta.md](../acceptance/beta.md). Bu belge sonuçları tekrar etmez; çalıştırma yollarını ve açık maddeleri toplar. Kanıt ekran görüntüleri: `frontend/e2e/evidence/`.

## Testleri çalıştırma

### Tek komut

| Hedef | Ne yapar |
|-------|----------|
| `make acceptance` | `acceptance-backend`, sonra Playwright stack'i kurar, senaryoları koşar, stack'i kapatır (hata olsa da) |
| `make acceptance-backend` | `cd backend && go test ./... -count=1 -v -run '$(ACCEPTANCE_GO_RUN)'`; `TEST_DATABASE_URL` yoksa durur |
| `make acceptance-stack-up` / `acceptance-stack-down` | `frontend/e2e/stack/up.sh` / `down.sh` |
| `make acceptance-e2e` | `cd frontend && pnpm e2e` |

Bu hedefler ve CI `acceptance` işi kabul paketiyle (TEC-740) birlikte gelir; bu belge yazılırken çalışma ağacında henüz commit edilmemişti. Senaryo ↔ test eşlemesi `ACCEPTANCE_GO_RUN` ve [beta.md](../acceptance/beta.md)'de.

### Backend (Go)

```sh
cd backend
go test ./...                                     # veritabanı testleri TEST_DATABASE_URL yoksa atlanır
TEST_DATABASE_URL=postgres://app:change_me@127.0.0.1:5432/app_test?sslmode=disable \
CATALOG_TEST_DATABASE_URL=postgres://app:change_me@127.0.0.1:5432/postgres?sslmode=disable \
  go test ./... -run 'RT_?A' -count=1             # yalnız RT adlı testler
```

- `TEST_DATABASE_URL` migrate edilmiş bir veritabanı ister (`DB_NAME=app_test make migrate-up` ya da CI'daki `migrate/migrate:v4.19.1` komutu). Rol `CREATEDB` yetkisi taşımalı (`dbtest.FreshDatabase` geçici DB açar).
- Desen: test adlarının çoğu `TestRT_A…` (ör. `TestRT_A04_LastUnitConcurrency`), bir kısmı `TestRTA14…` / `TestRTA15…` / `TestRTA16…`; yalnız `-run RT_A` bu üçünü kaçırır. Bazı senaryolar başka adlı testlerle kanıtlanır (liste `Makefile` `ACCEPTANCE_GO_RUN`). RT-A01: `TestTenantIsolationRT_A01` (`internal/httpserver`).
- Arama kısmı için `TEST_MEILI_HOST`, `TEST_MEILI_KEY`. Müsaitlik performansı: `-run SearchPilotLoad` ([observability.md](../operations/observability.md#performans-hedefi-müsaitlik-araması)).
- Diğer: `make -C backend lint`, `make -C backend openapi-lint`, `make -C backend sqlc` (fark çıkmamalı).

### Uçtan uca (Playwright)

| Adım | Komut |
|------|-------|
| Stack | `frontend/e2e/stack/up.sh` (Docker'da Postgres :55433, Redis :56380, S3 :59000; API :18080 süreç içi worker ile, panel :13000, site :13100; `make local-dev`'e dokunmaz). Durdurma `frontend/e2e/stack/down.sh` |
| Çalıştırma | `cd frontend && pnpm e2e` (`playwright test -c e2e/playwright.config.ts`), senaryolar `frontend/e2e/specs/` |
| Tarayıcı | `E2E_CHROMIUM_PATH`, yoksa `/opt/pw-browsers/chromium`, yoksa Playwright'ın kendi Chromium'u |
| Veri | `frontend/e2e/support/seed.ts` firmayı ürünün API'siyle kurar ([veri-modeli.md](./veri-modeli.md#testlerin-firma-kurması)); gerçek sağlayıcı bağlanmaz |
| Çıktı | Rapor `frontend/e2e/.state/report`, kanıt `frontend/e2e/evidence/` |

### CI (`.github/workflows/ci.yml`)

| İş | Adımlar |
|----|---------|
| `commit-policy` | Commit mesajında araç atfı reddi |
| `prod-env` | Şemadan env üret, prod compose doğrula |
| `backend` | `sqlc generate`, golangci-lint, test DB migrate (`migrate/migrate:v4.19.1`), `go test ./...` (`TEST_DATABASE_URL` dolu: RT testleri koşar), OpenAPI lint |
| `frontend` | `pnpm typecheck`, `pnpm lint`, `node scripts/check-i18n.mjs` |
| `site` | typecheck, lint, format, unit test, build |
| `acceptance` | Go kabul senaryoları, Playwright senaryoları, kanıt artefaktı `acceptance-evidence` (TEC-740 ile gelir; yukarıdaki not) |
| `auto-pr` | Testler geçince PR açar / birleştirir (`[no-automerge]` hariç) |

## Geri yükleme provası

Yöntem ve RT-A32 kontrolleri: [../operations/backup-restore.md](../operations/backup-restore.md#staging-provası). Prova sonucu (tarih, set, `rows.*` farkı, eksik PDF sayısı, tekrar gönderilen SMS'ler) repoda kayıtlı değil: doğrulanmadı. Staging stack'i canlı kurulumla birlikte gelir (TEC-742); prova notu beta.md'ye ya da bu bölüme eklenir.

## Açık maddeler

Hepsi **açık**. Sahip alan = düzeltmenin yapılacağı modül / ekip.

### Bağımlılık bekleyenler (Linear M0)

| # | Madde | Sahip alan | Durum |
|---|-------|------------|-------|
| 1 | İdes pilot veri toplama (TEC-679) | Ürün / İdes; filo, fiyat ilk yüklemesi | açık |
| 2 | Yolcu360 teknik sözleşme ve test erişimi (TEC-680) | `channels` (Yolcu360 adaptörü) | açık |
| 3 | Obilet iş bağlantısı ve test erişimi (TEC-681) | `channels` (Obilet adaptörü; yollar varsayımlı) | açık |
| 4 | KABİS resmî bağlantı yöntemi ve İdes yetkileri (TEC-682) | `kabis`, `platform/kabis` | açık |
| 5 | HGS sağlayıcı seçimi (TEC-683) | `tolls`, `platform/tolls` | açık |
| 6 | İdes için iyzico üye işyeri hesabı ve sandbox (TEC-684) | `payments` | açık |
| 7 | e-Belge entegratörü seçimi ve İdes mükellef hesabı (TEC-685) | `invoices`, `platform/einvoice` | açık |
| 8 | Bildirim sağlayıcı hesapları: SMS, e-posta, wuzapi, push (TEC-686) | `sms`, `whatsapp`, `notifyengine`, SMTP / VAPID env | açık |
| 9 | İdes sözleşme / politika / KVKK metinleri (TEC-687) | Firma ayarları, sözleşme şablonları, site | açık |
| 10 | Canlı kurulum (TEC-742) | Operasyon | açık |

### Bilinen eksikler

| # | Madde | Kaynak / doğrulama | Sahip alan | Durum |
|---|-------|--------------------|------------|-------|
| 11 | Geri yükleme sonrası tekrar önleme boşlukları: setten sonra gönderilmiş SMS / e-posta yeniden gider (Netgsm'e idempotency anahtarı gitmez, SMTP'de yok); Redis'siz geri yüklemede `queued` platform bildirimleri ve `pending` PDF'ler takılı kalır; WhatsApp mesaj kimliği tekrarı ve outbox yeniden yayınının etkisi doğrulanmadı | [backup-restore.md (b), (c)](../operations/backup-restore.md#c-tamamlanmış-tahsilat--fatura--bildirim-tekrar-oluşmuyor) | Operasyon, `sms`, `notifications`, `contracts` | açık |
| 12 | Paket dönemi uygulanmıyor: `organization_plan.period_starts_at` / `period_ends_at` saklanır, özellik çözümü dönemi okumaz | Kod: dönem alanları yalnız taşınır ve gösterilir (`features/store.go`, `orgcache/features.go`); `features.Effective` dönemi şimdiyle karşılaştırmaz | `platform/features`, `organizations` | açık |
| 13 | Paket düşürülünce kuyruktaki işler devam ediyor: kapı yeni iş oluşturan rotalarda ve bazı arka plan yollarında; entegrasyon runner'ı işi işlerken özelliği sormuyor | Kod: `integrations/jobs/usecase/runner.go`'da özellik kontrolü yok | `integrations` (runner), `platform/features` | açık |
| 14 | iyzico için production koruması yok (KABİS / e-Fatura / Netgsm'deki gibi) | [observability.md](../operations/observability.md#sağlayıcılar-sandbox--prod-seçimi) | `payments`, `platform/envguard` | açık |
| 15 | Süreç içi worker (`QUEUE_WORKER_INPROCESS=true`, yerel) ölü görev uyarısı göndermez, yalnız Sentry'ye raporlar | observability.md | `queue`, `observability` | açık |
| 16 | Markalı sitenin tarayıcı hataları Sentry'ye gitmiyor: site yalnız sunucu + edge (`site/src/instrumentation.ts`; `instrumentation-client` yok) | DEVELOPMENT.md, kod | `site` | açık |
| 17 | Araç detayı → Hasar kayıtları sekmesi gerçek liste değil: teslim / iade formları listesini plakayla arayarak açar; formlar listesinde araç filtresi gerekiyor | Kod: `vehicle-detail-page.tsx` `VehicleDamageTab` | `inspections` (API filtresi), `vehicles` (panel) | açık |
| 18 | Platform → Firmalar → firma → Alan adları / Bağlantılar sekmeleri yalnız bilgi kartı ve genel listeye bağlantı; firma kapsamlı platform uç noktaları gerekiyor | Kod: `organization-detail-page.tsx` `ManagedInPanelTab` | `site`, `integrations`, `organizations` | açık |
| 19 | Kampanya "tamamlanan kiralama" sayacı yalnız `returned` durumlu kiralamaları sayar (`return_check`'tekiler sayılmaz) | Kod: `queries/rentals.sql` `CountCustomerCompletedRentals` | `campaigns`, `rentals` | açık |

Ek gözlem (bu teslim kapsamında düzeltilmedi): [../design/inspections.md](../design/inspections.md) §3 "Saha görevleri (`/t/{slug}/field`) açık teslim / iade görevlerinin DataTable'ı; sihirbaz URL'inin QR kodu tarayıcıda üretilir" diyor. Kodda `/t/{slug}/field` artık "Görevlerim" (`MyFieldTasksPage`) ve inspections / field-tasks / rentals özelliklerinde QR üretimi yok; teslim / iade görevleri Operasyon görevleri ve Görev panosunda listeleniyor.
