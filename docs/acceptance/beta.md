# Beta kabul raporu: RT-A01…A32

TEC-740. Her RT senaryosu için otomatik test, sonuç ve kanıt. Kaynak:
Linear "Kabul testi" etiketli işler (Beta gereksinimleri §18). Gereksinim
metni yalnızca "RT-Axx testi" diyen senaryolarda kabul ölçütü işin kapsamından
ve modül kodundan türetildi; türetilen ölçüt "(türetilmiş)" ile işaretli.

Son koşu: 2026-10-23, yerel (Postgres 18, Redis 8, SeaweedFS, Chromium 141).

- Go: `make acceptance-backend` → 76 üst düzey test, 76 geçti, 0 atlandı
  (`go test ./... -count=1 -v -run "$ACCEPTANCE_GO_RUN"`).
- Playwright: `make acceptance-e2e` → 6 senaryo, 6 geçti.
- Tam `go test ./...` (CI ile aynı veritabanı kurulumu): 129 paket geçti.

Sonuç: **geçti** · **başarısız** · **bağımlılık bekliyor** (gerçek sağlayıcı
uç noktası / resmî yöntem yok; in-process sahte ile test edildi) · **manuel**.
Yeni yazılan testler **yeni** ile işaretli; diğerleri özellikle birlikte
yazılmış mevcut testlerdir (yeniden yazılmadı, referans verildi).

## Senaryolar

| RT | Senaryo (kabul ölçütü) | Test (dosya: fonksiyon) | Sonuç | Kanıt |
|----|------------------------|-------------------------|-------|-------|
| A01 | A firması kullanıcısı B'nin rezervasyon / dosya / rapor / export / arama sonucuna hiçbir yoldan erişemez | `backend/internal/httpserver/isolation_test.go`: `TestTenantIsolationRT_A01` (her `/v1/tenant` rotası: 403 / 404 / sızıntı taraması, export, depolama öneki, arama); `customerportal/portal_test.go`: `TestPortalScoping` | geçti | `--- PASS: TestTenantIsolationRT_A01 (15.98s)` |
| A02 | Tek şubeli personel başka şubenin finansını / teslimini API'den ve ekrandan açamaz (403/404), export'ta görmez | Go: `branches/branch_scope_test.go`: `TestBranchScopeRTA02`; **yeni** `rentals/rt_a02_test.go`: `TestRT_A02_RentalHandoverBranchScope` (detay, teslim, iade, liste, export satırları, operasyon görevleri); `finance/integration_test.go`, `invoices/integration_test.go`, `reservations/usecase/changes_test.go`: `TestBranchScope`. E2E **yeni** `frontend/e2e/specs/rt-a02-branch-scope.spec.ts` (BFF üzerinden 404, CSV export'ta HVL yok, ekranda "Kiralama bulunamadı", iade formu açılmıyor) | geçti | `--- PASS: TestRT_A02_RentalHandoverBranchScope (0.95s)`; `frontend/e2e/evidence/rt-a02-*.png` |
| A03 | 10 Clio + 10 Corsa + 10 i20 tek grup garantili ürün → sitede tek kart, kapasite 30; 20 Egea EXACT_VARIANT tek ürün, 21. araç kart sayısını değiştirmez; model garantili üründe başka model kaynak sayılmaz | Go: `availability/usecase/acceptance_test.go`: `TestRT_A03_ThirtyVehiclesOneProduct`, `TestModelGuaranteeNotSoldOnOtherModel`; **yeni** `availability/usecase/rt_a03_test.go`: `TestRT_A03_MixedModelsOneCardAndExactVariant`. E2E **yeni** `rt-a03-a27-storefront.spec.ts` › RT-A03 (panel arama kapasitesi 30, sitede tek kart; 2 HVL aracı MRK'de sayılmaz) | geçti | `--- PASS: TestRT_A03_MixedModelsOneCardAndExactVariant (0.55s)`; `rt-a03-site-one-card.png`. Not: açık API `units_left` değerini tasarım gereği 10 ile sınırlar (openapi `public/availability`), kapasite 30 panel aramasında doğrulanır |
| A04 | Son kapasiteye eşzamanlı 2 istek → yalnız 1 başarılı | `availability/usecase/acceptance_test.go`: `TestRT_A04_LastUnitConcurrency` (2 ve 6 goroutine, kilitli ve yalnız EXCLUDE; kanal bariyeri, uyku yok) | geçti | `--- PASS: TestRT_A04_LastUnitConcurrency (2.18s)` |
| A05 | A'dan alınıp B'ye iade edilen araç hazırlık sonrası B'de satılabilir, A'da değil | `availability/usecase/acceptance_test.go`: `TestRT_A05_OneWayReturnMovesCapacity`; `availability/engine/engine_test.go`: `TestOneWayReturnMovesTheVehicle` | geçti | `--- PASS: TestRT_A05_OneWayReturnMovesCapacity (0.68s)` |
| A06 | Kiralama aralığının ortasında engeli olan araç satılmaz | `TestRT_A06_BlockInTheMiddle`; engine `TestBlockInTheMiddle` | geçti | `--- PASS: TestRT_A06_BlockInTheMiddle (0.54s)` |
| A07 | 8 gün × 1.300 = 10.400 ₺ | `platform/pricing/pricing_test.go`: `TestAcceptanceRTA07EightDaysBand`; API: `modules/pricing/integration_test.go`: `TestPricingEndToEnd` | geçti | `--- PASS: TestAcceptanceRTA07EightDaysBand`, `TestPricingEndToEnd (0.74s)` |
| A08 | YAZ +%20 → 1.560 × 8 = 12.480 ₺ | `TestAcceptanceRTA08SummerPercent`; `TestPricingEndToEnd` | geçti | `--- PASS: TestAcceptanceRTA08SummerPercent` |
| A09 | 2 × 1.300 + 6 × 1.560 = 11.960 ₺ | `TestAcceptanceRTA09RuleStartsMidRental`; `TestPricingEndToEnd` | geçti | `--- PASS: TestAcceptanceRTA09RuleStartsMidRental` |
| A10 | +100 ₺ kuralı → 1.400 ₺/gün | `TestAcceptanceRTA10AmountRule`; `TestPricingEndToEnd` | geçti | `--- PASS: TestAcceptanceRTA10AmountRule` |
| A11 | Öncelik şube+kanal → şube → kanal → ortak; eşit öncelik aktivasyonda reddedilir | `TestAcceptanceRTA11Precedence`; `TestPricingEndToEnd` (aktivasyon reddi API'den) | geçti | `--- PASS: TestAcceptanceRTA11Precedence` |
| A12 | Son hakka iki eşzamanlı kullanım → limit aşılmaz | `campaigns/usecase/redeem_test.go`: `TestRT_A12_LastRedemptionConcurrency`; `modules/pricing/extras_coupons_test.go`: `TestPreviewExtrasAndCoupons` | geçti | `--- PASS: TestRT_A12_LastRedemptionConcurrency (1.02s)` |
| A13 | Doğru alt grup fiyatı; son çocuk koltuğuna eşzamanlı ikinci tahsis reddedilir | `extras/usecase/equipment_test.go`: `TestRT_A13_PerProductPrice`, `TestRT_A13_LastChildSeatConcurrency`, `TestRT_A13_WithoutLockOversells` (kilitsiz fazla satışın kanıtı) | geçti | `--- PASS: TestRT_A13_LastChildSeatConcurrency (0.79s)` |
| A14 | (türetilmiş) Otomatik onay: müsaitlik doğrulanır → süreli hold → ödeme başarılı → kesinleşir; süresi dolan hold kapasiteyi bırakır | **yeni** `reservations/usecase/rt_acceptance_test.go`: `TestRT_A14_AutomaticApprovalFlow`; personel onaylı akış: `payments_flow_test.go`: `TestRTA14StaffApprovedFlow` | geçti | `--- PASS: TestRT_A14_AutomaticApprovalFlow (0.91s)` |
| A15 | (türetilmiş) Personel onaylı talep stok ayırmaz; kabul → hold + ödeme süresi; süre sonrası ödeme → müsaitlik yeniden doğrulanır, karşılanamazsa çözüm / iade işi | `payments_flow_test.go`: `TestRTA14StaffApprovedFlow`, `TestRTA15LatePaymentCapacityGone`, `TestLatePaymentReinstatesWhenCapacityFree` | geçti | `--- PASS: TestRTA15LatePaymentCapacityGone (0.94s)` |
| A16 | Webhook: imza, sipariş referansı, tutar, para birimi, firma doğrulanır; aynı bildirim 3 kez → tek tahsilat + tek cari mahsup | `payments_flow_test.go`: `TestRTA16WebhookReplayedThreeTimes`, `TestVerificationMismatchIsRefused` (tutar); **yeni** `rt_acceptance_test.go`: `TestRT_A16_WebhookVerifiesEveryField` (para birimi, sipariş referansı, başka firmanın anahtarıyla imza, bilinmeyen sipariş); `finance/integration_test.go`: `TestWebhookPostingIsIdempotent` (tek cari kayıt) — iyzico sahte sunucusu (`iyzicofake`) ile | geçti (sahte) · gerçek iyzico sandbox: **bağımlılık bekliyor** | `--- PASS: TestRT_A16_WebhookVerifiesEveryField (1.84s)` |
| A17 | Yüklenmemiş fotoğraf varken teslim formu nihai olmaz | Go: `inspections/integration_test.go`: `TestRT_A17_UnuploadedPhotoBlocksFinalize`. E2E **yeni** `rt-a17-pending-photo.spec.ts` (6. fotoğrafın yüklemesi ağda bekletilir: "Tamamla" pasif, kontrol listesi "1 fotoğraf yüklenmeyi / doğrulanmayı bekliyor", API 409; yükleme bırakılınca form tamamlanır) | geçti | `rt-a17-complete-disabled-while-uploading.png`, `rt-a17-finalized-after-upload.png` |
| A18 | Onaylı sözleşmede araç / bedel değişince yeni sürüm + yeniden onay | `rentals/integration_test.go`: `TestRT_A18_ContractReapproval` | geçti | `--- PASS: TestRT_A18_ContractReapproval (1.40s)` |
| A19 | (türetilmiş) İmzasız / itirazlı iade yine kaydedilir, itiraz işaretli, yetkili incelemesine düşer; ihtilaflı hasar otomatik borca dönüşmez | Go: `TestRT_A19_DisputedReturnNeverBecomesDebt`, `TestRT_A19_SignedReturnCompletes`. E2E **yeni** `rt-a19-disputed-return.spec.ts` (sihirbazda imzasız tamamlama → "İade yetkili incelemesinde", inceleme kuyruğunda satır, kiralama `return_check`, tüm ücretler `proposed` + itirazlı) | geçti | `rt-a19-return-recorded-in-review.png`, `rt-a19-review-queue.png` |
| A20 | (türetilmiş) Aynı dış işlem kimliği ikinci masraf üretmez; tutar değişimi revizyon; plaka + teslim–iade aralığı → doğru kiralama; belirsizse inceleme kuyruğu | `tolls/integration_test.go`: `TestRT_A20_*` (8 test) | geçti | `--- PASS: TestRT_A20_DuplicateImport (0.93s)` |
| A21 | (türetilmiş) Bağlantı kesilince iş bekler ve yeniden denenir; belirsiz sonuçta önce Status; yerel "bekliyor" resmî bildirim gibi gösterilmez | `kabis/integration_test.go`: `TestRT_A21_*` (9 test); `integrations/jobs/usecase/runner_test.go`: `TestRunnerUncertainChecksStatusWithoutDoubleSubmit`, `TestRunnerResubmitsOnlyWhenConfirmedNotDone` | geçti (sahte adaptör) · KABİS resmî yöntem: **bağımlılık bekliyor** | `--- PASS: TestRT_A21_UncertainChecksStatusFirst (1.50s)` |
| A22 | (türetilmiş) Taslak → UBL-TR XML → XSD / Schematron; tekrar istekte çift fatura yok; gönderim hatası finans kaydını silmez | `invoices/integration_test.go`: `TestRT_A22_InvoiceLinksAccrualsWithoutNewDebt`, `TestBuildAndValidateTemelAndEArsiv`, `TestIdempotentSendCreatesOneInvoice`, `TestDoubleInvoicingRefused`, `TestUncertainSendChecksStatusFirst`, `TestSendFailureKeepsFinance` | geçti (sahte entegratör) · e-fatura entegratörü: **bağımlılık bekliyor** | `--- PASS: TestBuildAndValidateTemelAndEArsiv (2.85s)` |
| A23 | Fatura + tahsilat + depozito aynı kiralamada → kira tek kez borç / gelir | `finance/integration_test.go`: `TestRT_A23_RentDebitedOnce`; `TestRT_A22_InvoiceLinksAccrualsWithoutNewDebt` | geçti | `--- PASS: TestRT_A23_RentDebitedOnce (1.61s)` |
| A24 | Aylık tahakkuk / fatura tekrar borç üretmez; kısmi dönem izlenir | `agreements/integration_test.go`: `TestRT_A24_MonthlyAccrualNeverTwice` | geçti | `--- PASS: TestRT_A24_MonthlyAccrualNeverTwice (1.82s)` |
| A25 | Rezervasyon + değişiklik + iptal tekrarlı mesajlarla → tek dış referans, kapasite / fiyat farkı / cari tutarlı | `channels/integration_test.go`: `TestRT_A25_BookingModificationCancellationWithRepeats` | geçti (sahte) · gerçek Yolcu360: **bağımlılık bekliyor** | `--- PASS: TestRT_A25_… (1.31s)` |
| A26 | Obilet adaptörü aynı çekirdeğe bağlanır | `channels/obilet_test.go`: `TestRT_A26_ObiletAvailabilityPriceBookingCancellation` | geçti (sahte) · gerçek Obilet (TEC-681): **bağımlılık bekliyor** | `--- PASS: TestRT_A26_… (1.67s)` |
| A27 | Aynı arama site, widget ve API'de aynı fiyat / kapasite | Go: `publicapi/publicapi_test.go`: `TestCatalogAndSameAnswerEverywhere` (API, site ve widget oturumları + panel araması). E2E **yeni** `rt-a03-a27-storefront.spec.ts` › RT-A27 (açık API gizli anahtar, markalı site, partner origin'e gömülü widget: aynı alt grup, aynı toplam ₺4.500,00, `units_left` aynı) | geçti | `rt-a27-site-search.png`, `rt-a27-widget-on-partner.png` |
| A28 | Yönetim paketindeki firma açık API / widget açmayı dener → 403 FEATURE_NOT_ENABLED; müşteri belge bağlantıları çalışır | `organizations/plans_routes_test.go`: `TestPlatformPlanRoutes`; `publicapi/features_test.go`: `TestPlanFeatureGate`; `middleware/features_test.go`: `TestRequireFeature`; `platform/features/links_test.go`: `TestCustomerLinksAreNeverGated`; `site/features_test.go`: `TestPublicSiteOutsidePlanIsNotFound` | geçti | `--- PASS: TestPlatformPlanRoutes (0.24s)` |
| A29 | (türetilmiş) Adrese / havalimanına teslim → uygunluk + ücret; görev atanır, personel iş listesinde görür, görevden teslim formuna geçilir | Go: `fieldtasks/integration_test.go`: `TestRT_A29_AirportDelivery`. E2E **yeni** `rt-a29-delivery-task.spec.ts` (rezervasyonda 750 ₺ teslim satırı, saha personelinin "Görevlerim" listesinde görev, "Yola çık" → "Teslim formuna geç" → kiralamanın teslim sihirbazı, görev `en_route`) | geçti | `rt-a29-my-tasks.png`, `rt-a29-task-en-route.png`, `rt-a29-handover-form-from-task.png` |
| A30 | Bir kanalda hata → kayıtlı; yapılandırılan alternatif kanal çalışır | `notifyengine/integration_test.go`: `TestFallbackAfterUndeliveredReport`, `TestFallbackAfterRejectionAndDelivered` | geçti (log / sahte SMS) | `--- PASS: TestFallbackAfterUndeliveredReport (0.39s)` |
| A31 | Çakışan uzatma reddedilir, alternatif + fark teklifi | `reservations/usecase/changes_test.go`: `TestRT_A31_ConflictingExtensionOffersAlternative` | geçti | `--- PASS: TestRT_A31_… (0.93s)` |
| A32 | Yedekten geri dönüş provası | [operations/backup-restore.md](../operations/backup-restore.md) (staging provası) | manuel | runbook'taki prova kaydı |

Playwright senaryoları gerçek akışı sürer: API (Go, süreç içi worker),
panel (Next.js, BFF `/api/v1`), markalı site ve widget; organizasyon,
şubeler, 32 araç, ürün, tarife, kullanıcılar ve kiralamalar
`frontend/e2e/global-setup.ts` → `support/seed.ts` ile panelin kullandığı
API'den kurulur, kullanıcılar gerçek giriş formundan oturum açar.

## Uçtan uca kapsamın sınırları

- **A17 / A19 / A29:** fotoğrafın kendisi mobil kamera yerine dosya girişiyle
  verilir (`setInputFiles`); diğer slotların fotoğrafları, okuma / doğrulama
  alanları ve A19'daki hasar + itiraz BFF üzerinden API ile girilir.
  Sürülen ve doğrulanan kısım: yükleme kuyruğu, "Tamamla" kapısı, imzasız
  tamamlama, inceleme kuyruğu, görevden forma geçiş. İmza tuvali e2e'de
  çizilmez (Go: `TestRT_A19_SignedReturnCompletes`).
- **A27 widget:** partner sayfası test içinde loopback'te açılan küçük bir
  HTTP sunucusudur (`http://partner.localhost:13200`, sitenin
  `widget.allowed_domains` listesinde). `page.route` ile sahte sayfa
  kullanılmaz: Chromium'un yerel ağ erişimi denetimi adresi olmayan sayfadan
  yerel siteye çerçeve isteğini bekletir.
- **A03 / A27 kapasite:** açık API `units_left` en fazla 10 gösterir;
  site ve widget yalnız ≤ 3 için "son N araç" yazar. Kapasite 30 panel
  aramasında, eşitlik `units_left` / fiyat üzerinden doğrulanır.
- **A02 finans:** BFF'te HVL tahsilatı (ödeme) 404, ödeme listesinde yok;
  finans hesap / hareket / kasa ayrıntıları Go `TestBranchScope` testlerinde.
- **A29 ataması** seed'de yönetici adına API ile yapılır; atama ekranı
  e2e'de sürülmez (Go testi atamayı, bildirimi ve formla tamamlamayı kapsar).

## Yerelde çalıştırma

```bash
# Go senaryoları (CI ile aynı: postgres:18-alpine + migrate/migrate:v4.19.1)
docker run -d --name rt-pg -e POSTGRES_USER=app -e POSTGRES_PASSWORD=app \
  -e POSTGRES_DB=app_test -p 55432:5432 postgres:18-alpine
docker run --rm --network host -v "$PWD/backend/migrations:/migrations:ro" \
  migrate/migrate:v4.19.1 -path /migrations \
  -database "postgres://app:app@localhost:55432/app_test?sslmode=disable" up
export TEST_DATABASE_URL=postgres://app:app@localhost:55432/app_test?sslmode=disable
export CATALOG_TEST_DATABASE_URL=$TEST_DATABASE_URL
make acceptance-backend          # = go test ./... -run "$ACCEPTANCE_GO_RUN" -v

# Playwright: yığını kur, çalıştır, kaldır (hepsi: make acceptance)
make acceptance-stack-up         # frontend/e2e/stack/up.sh
make acceptance-e2e              # cd frontend && pnpm e2e
make acceptance-stack-down       # frontend/e2e/stack/down.sh
```

- Yığın (`frontend/e2e/stack/env.sh`): Postgres :55433, Redis :56380,
  SeaweedFS :59000 (Docker), API :18080, panel :13000 (`next dev`), site
  :13100 (`next build && next start`). `make local-dev` portlarına dokunmaz;
  kök `.env` okunmaz (API durum klasöründen çalışır).
- Panel `next dev` ile çalışır: üretim derlemesi oturum çerezini yalnız
  `__Secure-` adıyla okur (üretimde HTTPS), düz http test yığınında oturum
  açılamaz. İlk sayfa derlemeleri bu yüzden yavaştır.
- Chromium: `E2E_CHROMIUM_PATH`, yoksa `/opt/pw-browsers/chromium`, yoksa
  Playwright'ın kendi tarayıcısı (`pnpm exec playwright install chromium`).
  Sabitlenen sürüm `@playwright/test` 1.64.1.
- Her koşu yeni bir organizasyon (`kabul-<zaman>`) kurar; çalışma durumu
  (fixture, oturumlar, iz, HTML rapor) `$E2E_STATE/playwright` altındadır,
  repoda değil.
- Giriş hız sınırı: aynı e-posta + IP için 15 dakikada 10 deneme. Yönetici
  her koşuda 2 kez girer; art arda 5'ten fazla koşu için Redis'i sıfırlayın
  (`make acceptance-stack-down`).
- Kanıt ekran görüntüleri `frontend/e2e/evidence/*.png` (her biri < 200 KB,
  repoda tutulur; koşu üzerine yazar). Go ve Playwright koşu logları CI'da
  `acceptance-evidence` artefaktıdır.

## CI

`.github/workflows/ci.yml` → `acceptance` işi (`needs: [backend, frontend]`,
backend ile aynı Postgres servisi ve migrate adımı):

- Her push'ta Go senaryoları (`make acceptance-backend`).
- Playwright senaryoları yalnız elle tetiklemede (`workflow_dispatch`,
  `e2e: true`): tam yığın (üç konteyner, API derlemesi, panel dev, site
  derlemesi) 45 dakika sınırlı bir koşuda kurulur; her push'ta koşturmak
  için yeterince hızlı / kararlı değildir.
- `auto-pr` bu işe bağlı değildir (`needs` değişmedi): kabul koşusu
  raporlar, birleştirmeyi engellemez.

## Bağımlılık bekleyenler

| Sağlayıcı | Şimdi | Bekleyen |
|-----------|-------|----------|
| iyzico | `iyzicofake` (imzalı webhook, retrieve, iade) ve `cmd/iyzico-fake` | Gerçek sandbox hesabı ve webhook tekrar kaydı (A16) |
| Netgsm SMS | log sağlayıcı / `cmd/netgsm-fake` | Gerçek hesapla teslim raporu (A30) |
| KABİS | sahte adaptör, iş kuyruğu kuralları | Resmî entegrasyon yöntemi ve test ortamı; kurgusal üretim bildirimi yapılmaz (A21) |
| e-Fatura entegratörü | sahte entegratör, go-ubltr XSD / Schematron | Entegratör seçimi ve test hesabı (A22) |
| Yolcu360 | sahte kanal | Gerçek uç nokta ve kimlik bilgileri (A25) |
| Obilet | sahte adaptör | TEC-681 (A26) |

## Bilinen hatalar

Bu koşuda başarısız senaryo yok; ürün hatası bulunmadı.
