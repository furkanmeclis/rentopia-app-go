# Veri modeli ve şema migration'ları

Kaynak: `backend/migrations` (000001–000135, 127 migration; numaralarda boşluk var: 000034–000039, 000076, 000085), firma tabloları listesi `backend/internal/database/queries/tenant_tables.txt`, karma kapsamlılar `mixed_scope_tables.txt`. Kurallar: [backend/DATABASE_RULES.md](../../backend/DATABASE_RULES.md); alan tasarımları: [../design/](../design).

## Alanlar ve ana tablolar

Firma (organization) tabloları, `tenant_tables.txt`'ten:

| Alan | Ana tablolar | Tasarım |
|------|--------------|---------|
| Firma çekirdeği | `branches`, `service_points`, `organization_members`, `organization_member_roles`, `organization_member_branches`, `organization_invitation_roles` / `_branches`, `organization_settings_versions`, `organization_plan`, `organization_features`, `support_grants` | [tenant-core](../design/tenant-core.md) |
| Filo | `vehicle_segments`, `rental_groups` (+ `_catalog_models`, `_catalog_types`), `rental_products`, `rental_product_versions`, `rental_product_images`, `vehicles`, `vehicle_events`, `vehicle_rental_groups`, `vehicle_documents`, `vehicle_maintenance`, `vehicle_reminder_deliveries`, `vehicle_blocks`, `vehicle_transfers`, `one_way_rules`, `vehicle_busy_intervals` | [fleet-availability-inputs](../design/fleet-availability-inputs.md) |
| Müsaitlik | `vehicle_allocations`, `channel_capacity_rules` | [availability-engine](../design/availability-engine.md) |
| Fiyat | `rate_plans`, `rate_plan_versions`, `rate_bands`, `date_rules`, `date_rule_versions`, `date_rule_version_targets`, `extras`, `extra_prices`, `extra_branch_stock`, `equipment_allocations`, `campaigns`, `campaign_targets`, `campaign_redemptions` | [pricing](../design/pricing.md), [extras](../design/extras.md), [campaigns](../design/campaigns.md) |
| Müşteri | `customers`, `customer_contacts`, `customer_marketing_consents`, `drivers`, `customer_documents`, `corporate_agreements`, `agreement_contacts`, `agreement_drivers` | [customers](../design/customers.md), [corporate-agreements](../design/corporate-agreements.md) |
| Rezervasyon | `quotes`, `quote_lines`, `reservation_sequences`, `reservations`, `reservation_drivers`, `reservation_events`, `reservation_vehicle_assignments`, `resolution_tasks` | [reservations](../design/reservations.md) |
| Kiralama ve operasyon | `rentals`, `rental_vehicles`, `rental_drivers`, `rental_events`, `rental_charges`, `rental_billing_periods`, `operation_tasks`, `operation_task_events` (saha görevleri bu tabloyu genişletir) | [rentals](../design/rentals.md), [field-tasks](../design/field-tasks.md) |
| Belgeler | `contract_templates`, `documents`, `document_versions`, `document_approvals`, `document_approval_links` | [contracts](../design/contracts.md) |
| Teslim / iade | `inspection_templates`, `inspections`, `inspection_damages`, `inspection_photos`, `inspection_signatures` | [inspections](../design/inspections.md) |
| Ödeme | `integration_accounts`, `payment_links`, `payments`, `payment_refunds` | [payments](../design/payments.md) |
| Finans | `accounts`, `ledger_entries`, `ledger_entry_links`, `collections`, `collection_allocations`, `security_deposits`, `deposit_deductions`, `cash_accounts`, `cash_movements`, `expense_categories`, `expenses`, `suppliers`, `supplier_invoices`, `supplier_invoice_lines`, `supplier_payments`, `supplier_payment_allocations` | [ledger](../design/ledger.md), [collections](../design/collections.md), [deposits](../design/deposits.md), [cash-accounts](../design/cash-accounts.md), [suppliers](../design/suppliers.md) |
| Fatura | `invoice_settings`, `invoice_tax_mappings`, `invoice_sequences`, `einvoice_taxpayers`, `invoices`, `invoice_lines` | [einvoice](../design/einvoice.md) |
| Resmî / dış | `kabis_notifications`, `vehicle_toll_tags`, `toll_passes`, `toll_pass_revisions`, `toll_pass_events`, `toll_settings` | [kabis](../design/kabis.md), [tolls](../design/tolls.md) |
| Mesajlaşma | `sms_messages`, `sms_otp_challenges`, `whatsapp_sessions`, `whatsapp_messages`, `notification_rules`, `notification_message_templates`, `notification_dispatches`, `notification_deliveries`, `customer_push_subscriptions` | [sms](../design/sms.md), [whatsapp](../design/whatsapp.md), [notification-engine](../design/notification-engine.md) |
| Kanal yöneticisi | `channel_accounts`, `channel_mappings`, `channel_offers`, `channel_reservations`, `channel_messages`, `channel_sync_runs` | [channel-manager](../design/channel-manager.md) |
| Müşteri yüzü | `customer_portal_sessions`, `customer_requests`, `public_api_keys`, `public_api_idempotency`, `public_quotes`, `public_reservations`, `public_webhook_endpoints`, `public_webhook_deliveries`, `site_settings`, `site_setting_versions`, `site_domains`, `site_media`, `site_leads`, `site_articles` | [customer-portal](../design/customer-portal.md), [public-api](../design/public-api.md), [branded-site](../design/branded-site.md), [site-leads](../design/site-leads.md) |

Platform (organizasyonsuz) ve karma tablolar:

| Grup | Tablolar |
|------|----------|
| Kimlik / RBAC | `users`, `roles` (platform ya da firma kapsamlı: `organization_id` + `scope`), `permissions`, `role_permissions`, `user_roles`, `refresh_tokens`, `otp_codes`, `webauthn_credentials`, `push_subscriptions` |
| Firma kaydı | `organizations`, `organization_invitations` (karma) |
| Paket | `features`, `plans`, `plan_features` (seed: 000134) |
| Katalog (TSB) | `catalog_brands`, `catalog_models`, `catalog_types`, `catalog_valuations`, `catalog_model_rules`, `catalog_imports` |
| Motorlar | `export_jobs`, `import_jobs`, `import_changes`, `bulk_jobs`, `activity_events`, `notifications`, `outbox_events`, `app_settings` (antet), `app_logs`, `log_purge_rules`, `platform_health_alert_cursors` |
| Entegrasyon işleri | `integration_jobs`, `integration_attempts` (karma: firma işi `organization_id` dolu) |

## Kiracılık kuralı

- Firmaya ait her tabloda `organization_id` var; her sorgu onunla filtreler. `go test ./...` içindeki sorgu koruması (`internal/database/queries_test.go`) `tenant_tables.txt`'teki tablolara dokunan her sorguda `organization_id` arar.
- Organizasyon istemciden alınmaz: `X-Organization` başlığı + üyelik → `RequireOrganization` → `orgctx.Scope` (`internal/platform/orgctx`). Gövde ya da URL'de organizasyon id'si kabul edilmez.
- Başka firmanın kaydı 404 döner. Şube kapsamı: `orgctx.BranchScope` / `CheckBranch` (kapsam dışı kayıt 404), `RequireAllBranches` (403).
- Kuyruk görevleri organizasyonla damgalanır, worker `WithJobScope` ile yeniden kapsar.
- RT-A01 (`internal/httpserver/isolation_test.go`, `TestTenantIsolationRT_A01`) iki firmayla her `/v1/tenant` rotasını gezer ve `openapi.yaml` ile karşılaştırır; `TEST_DATABASE_URL` ister.
- Yeni firma tablosu: `tenant_tables.txt`'e eklenir; kontrol listesi `.agents/skills/add-tenant-resource`.

## Sürümlü kayıtlar

| Kayıt | Tablolar | Kural |
|-------|----------|-------|
| Ürün | `rental_products` → `rental_product_versions` | Baş satır + sürümler; rezervasyon satılan sürümü (`sold_product_version`), güncel sürümü (`product_version`) ve satıldığı ayar (politika) sürümünü saklar |
| Tarife | `rate_plans` → `rate_plan_versions` (+ `rate_bands`) | En çok bir `draft` + bir `active`; düzenleme taslağa yazar, aktivasyon tek transaction'da `active → superseded`, `draft → active`; taslak olmayan sürüm ve çocukları tetikleyiciyle değiştirilemez |
| Tarih kuralı | `date_rules` → `date_rule_versions` (+ `_targets`) | Tarife ile aynı desen |
| Firma ayarları | `organization_settings_versions` | Her kayıt yeni sürüm (Ayarlar → Genel → Sürüm geçmişi) |
| Sözleşme şablonu | `contract_templates` (satır = sürüm) | draft → active → archived; tür başına bir aktif |
| Belge | `documents` → `document_versions` | Sürüm değiştirilemez (`document_versions_immutable`); içerik değişince yeni sürüm, eski onaylar geçersiz |
| Teslim / iade formu | `inspections` | Tamamlanan form değiştirilemez; düzeltme `revise` ile yeni sürüm, eski `superseded` |
| Site | `site_settings` → `site_setting_versions` | Taslak / yayın / geri yükleme |

Ayrıntı: [pricing.md → Versioned records](../design/pricing.md#versioned-records).

## Para ve zaman

| Konu | Kural | Kaynak |
|------|-------|--------|
| Tutar | `NUMERIC(14,2)`; Go'da `int64` kuruş (`backend/pkg/money`); asla float | pricing.md |
| Para birimi | `TRY` varsayılan (`currency CHAR(3)`) | migration'lar |
| Yuvarlama | Tek kural: kuruşa, sıfırdan uzağa yarım | `pkg/money` |
| JSON | Tutar her zaman string (`"1300.00"`); oran baz puan (`Percent(20)` = 2000) | pricing.md |
| Saklama | Anlar UTC (`timestamptz`) | `pkg/rentaltime` |
| Takvim | Europe/Istanbul (`time/tzdata` gömülü) | `pkg/rentaltime` |
| Aralık | Yarı açık `[start, end)` (tarih kuralı "1 Haziran – 1 Eylül" = 1 Haziran – 31 Ağustos) | pricing.md |
| Kiralama günü | `ceil(süre / 24h)`, en az 1 (24h = 1 gün, 24h + 1 dk = 2 gün) | pricing.md |
| Doluluk aralığı | `tstzrange [starts_at, ends_at + prep)`; prep = iade şubesinin `prep_minutes` (varsayılan 60) | availability-engine.md |

## Migration ekleme

Skill: `.agents/skills/db-migration`. Sıra:

1. `make -C backend migrate-create NAME=<açıklama>` (sıralı `00000N_…`, zaman damgası yok).
2. `up` + `down` SQL; firma tablosuysa `organization_id`, FK'ler, `uq_` / `idx_` adları, `set_updated_at()` tetikleyicisi.
3. `make migrate-up`.
4. Sorgular `internal/database/queries/*.sql` → `make -C backend sqlc` (üretilen `internal/database/db` elle düzenlenmez).
5. Firma tablosu → `tenant_tables.txt`.
6. Yeni yetki slug'ı → migration + `internal/platform/rbac` + `frontend/src/config/permissions.ts`.
7. Yeni `/v1` rotası → `backend/docs/openapi.yaml`, ardından `cd frontend && pnpm api:generate`.

## Seed verisi

| Ne | Nasıl |
|----|-------|
| Yetkiler ve şablon roller | `*_seed_*_rbac` migration'ları (000005, 000030, 000033, 000046, 000049, 000055, 000062, 000065, 000067, 000071, 000075, 000080, 000084, 000094, 000098, 000100, 000102, 000104, 000106, 000109, 000111, 000114, 000116, 000119, 000123) |
| Paketler ve özellikler | 000134 (`management` = Yönetim, `complete` = Komple; mevcut firmalar Komple) |
| Katalog görünen markaları | 000040 (`catalog_display_brands`), 000056 (KGM / SsangYong) |
| Platform süper yöneticisi | `backend/cmd/create-super-admin` (`make create-super-admin`, prod `make prod-super-admin`); kullanıcıyı upsert eder, `super_admin` rolünü atar |
| TSB kasko kataloğu | `backend/cmd/catalog-import -file <liste.xlsx> [-period YYYY-MM] [-apply] [-diff]`, `-rebuild-auto`; ya da Platform → Araç kataloğu → Aktarımlar |
| Yeni firmanın "Merkez" şubesi (kod `MRK`) | Firma oluşturma kancası (`modules/branches/usecase/hooks.go`) |

Demo kullanıcı / firma seed'i yok (DATABASE_RULES: migration'da yalnız referans verisi).

## Testlerin firma kurması

| Test | Nasıl kurar |
|------|-------------|
| RT-A01 (`internal/httpserver/isolation_test.go`) | `TEST_DATABASE_URL` (migrate edilmiş DB) üzerinde iki kullanıcı ve `orgusecase.CreateOrganization` ile iki firma ("Iso A", "Iso B"); her kaynağa işaretli kayıt ekler, sunucuyu `httpserver.New` ile süreç içinde kurar. `TEST_MEILI_HOST` varsa arama da test edilir |
| Modül entegrasyon testleri (`*integration_test.go`, `rt_*_test.go`) | Aynı `TEST_DATABASE_URL`; paylaşılan tabloları silen testler `internal/database/dbtest.FreshDatabase` ile özel, migrate edilmiş geçici DB açar (rol `CREATEDB` ister) |
| Playwright kabul (`frontend/e2e/support/seed.ts`) | Ürünün kendi API rotalarıyla: MRK / HVL şubeleri, havalimanı hizmet noktası, segment, "KOMPAKT" grubu, aktif tarifeli ürün, 32 araç, müşteri + sürücü, MRK'ya kısıtlı şube müdürü ve saha personeli, senaryo kiralamaları, markalı site + widget, gizli Public API anahtarı |

`internal/acceptance` paketi yok.
