# Kayıt takibi: imzalı belgeler, finans, dış işlemler

Her kayıt için: nerede tutulur, panelde nereden görülür, SQL ile nasıl incelenir. SQL'ler salt okunurdur; her sorguda `organization_id` filtresi kullanın (`SELECT id FROM organizations WHERE slug = '<slug>'`). Prod'da veritabanına erişim: `docker compose --env-file .env.server -f compose.prod.yml exec postgres psql -U <DB_USER> <DB_NAME>` (Adminer yalnız yerel compose'da). Kayıtlarda değişiklik panelden ya da API'den yapılır; doğrudan `UPDATE` yalnız [backup-restore.md](../operations/backup-restore.md)'de tanımlı kurtarma durumları içindir.

## 1. İmzalı belgeler

### Sözleşme ve formlar (`documents`, `document_versions`, `document_approvals`)

| Kayıt | Alanlar | Kural |
|-------|---------|-------|
| `documents` | `kind` (`rental_contract`, `handover_form`, `return_form`, `terms`), `reservation_id`, `rental_id`, `status` (draft / approved / void), `current_version_id` | Rezervasyon ve tür başına bir canlı belge |
| `document_versions` | `version`, `template_version`, `html`, `content_hash` (HTML'in SHA-256'sı), `data_snapshot`, `changed_keys`, `status` (draft → issued → approved / superseded), `pdf_status`, `pdf_object_key`, `pdf_sha256`, `pdf_size_bytes`, `pdf_error` | Değiştirilemez (tetikleyici); içerik değişince yeni sürüm |
| `document_approvals` | `content_hash`, `party` (customer / staff), `method` (`in_person`, `click`, `signature`, `remote_sms`), `approver_name`, `approved_at`, `invalidated_at`, `invalidated_reason`, onay PDF'i (`pdf_object_key`, `pdf_sha256`) | `(organization_id, version_id, content_hash)` bileşik FK: onay yalnız gördüğü içeriğe bağlanır; yeni sürüm eski onayı geçersiz kılar |
| `document_approval_links` | `token_hash`, `signer_role` (renter / driver), `signer_name`, durum | Uzaktan onay bağlantısı `/d/{token}` + SMS kodu |

PDF anahtarı: `organizations/<org>/documents/<doküman>/<sürüm>.pdf` (`storage.DocumentPDFObjectKey`); dosya API üzerinden akar (`GET /v1/tenant/documents/{uuid}/versions/{v}/pdf`, onay PDF'i `…/approvals/{a}/pdf`, yetki `tenant.document.download`). Tarayıcıya bucket adresi verilmez.

Panel: rezervasyon ve kiralama detayında **Belgeler** sekmesi (sürümler, onaylar, onay bağlantıları, PDF).

```sql
-- Bir kiralamanın belgeleri, güncel sürüm ve geçerli onaylar
SELECT d.kind, d.status, v.version, v.status AS version_status, v.content_hash, v.pdf_status, v.pdf_object_key,
       a.party, a.method, a.approver_name, a.approved_at, a.invalidated_at, a.invalidated_reason
FROM documents d
JOIN document_versions v ON v.organization_id = d.organization_id AND v.document_id = d.id
LEFT JOIN document_approvals a ON a.organization_id = v.organization_id AND a.document_version_id = v.id
WHERE d.organization_id = :org AND d.rental_id = (SELECT id FROM rentals WHERE organization_id = :org AND uuid = :rental)
ORDER BY d.kind, v.version;

-- Basılmayı bekleyen / başarısız PDF'ler
SELECT uuid, pdf_status, pdf_error, updated_at FROM document_versions
WHERE organization_id = :org AND pdf_status IN ('pending', 'failed');
```

### Teslim / iade formları (`inspections`)

| Tablo | Kanıt |
|-------|-------|
| `inspections` | Tür (handover / return), km, yakıt, ekipman, doğrulama, müşteri incelemesi, `signature_status` (pending / signed / refused / waived), `review_status` (none / pending / resolved), `status` (draft / finalized / superseded; düzeltme = yeni sürüm) |
| `inspection_photos` | Slot, `client_ref`, sunucu `sha256`, sunucu `uploaded_at`, cihaz zamanı ve EXIF ayrı kolonlarda, `scan_status` (skipped / clean / infected) |
| `inspection_damages` | Bölge, tür, derece, `is_new`, itiraz |
| `inspection_signatures` | `signer_name`, `signer_role`, imza PNG `object_key`, `strokes`, `point_count`, `signed_at`, `device_signed_at` |

Panel: **Teslim / iade formları** (`/t/{slug}/inspections`, `tenant.rentals.read`); inceleme kuyruğu `?review_status=pending`. Fotoğraflar 10 dakikalık imzalı bağlantıyla açılır (`/v1/public/media/inspections/{token}`).

Müşteri tarafı: müşteri portalı `/r/{orgSlug}` (rezervasyon referansı + tek kullanımlık kod) ve uzaktan onay `/d/{token}`.

## 2. Finans kayıtları

| Kayıt | Tablo ve durumlar | Panel |
|-------|-------------------|-------|
| Cari (borç / alacak) | `accounts` (active / archived), `ledger_entries`: `kind` accrual / collection / payment / refund / offset / adjustment / reversal, `direction` debit / credit, `purpose` rent / charge / prepayment / deposit / general, `source_type` + kaynak id'leri; düzeltme yeni kayıtla (`reverses_entry_id`, `adjusts_entry_id`), silme yok | Finans → Cari hesaplar; kiralama detayı → Cari |
| Tahsilat | `collections`: recorded / partially_refunded / refunded / void; `settlement_status` none / pending / settled; `collection_allocations` | Finans → Tahsilatlar |
| Online ödeme | `payments`: pending / succeeded / failed / cancelled / partially_refunded / refunded; `payment_links`: active / paid / expired / cancelled; `payment_refunds` | Rezervasyon → Ödemeler |
| Depozito | `security_deposits`: pending (Provizyon bekliyor) / held (Tutuluyor) / partially_captured (Kesintili kapandı) / released (Serbest bırakıldı) / refunded (İade edildi) / cancelled; `deposit_deductions` | Finans → Depozitolar; kiralama → Depozito |
| Kasa / banka | `cash_accounts`, `cash_movements` | Finans → Kasa ve banka, Kasa / banka hareketleri |
| Fatura | `invoices.status`: draft (Taslak) / queued (Kuyrukta) / sent (Gönderildi) / awaiting_result (Sonuç bekleniyor) / succeeded (Kesildi) / failed (Başarısız) / cancelled (İptal edildi); `uuid` = ETTN (taslakta üretilir, her denemede aynı); `integration_job_id`, `last_error`, `pdf_key`; `invoice_lines` | Finans → Faturalar (XML / PDF / önizleme) |

Mükerrer kayıt güvenceleri (benzersiz indeksler) ve kontrol SQL'leri: [backup-restore.md → (c)](../operations/backup-restore.md#c-tamamlanmış-tahsilat--fatura--bildirim-tekrar-oluşmuyor).

```sql
-- Bir kiralamanın cari hareketleri
SELECT entry_date, kind, direction, purpose, amount, currency, source_type, description
FROM ledger_entries WHERE organization_id = :org
  AND rental_id = (SELECT id FROM rentals WHERE organization_id = :org AND uuid = :rental)
ORDER BY entry_date, id;

-- Takılan faturalar
SELECT uuid, number, status, last_error, updated_at FROM invoices
WHERE organization_id = :org AND status IN ('queued', 'sent', 'awaiting_result', 'failed') ORDER BY updated_at;
```

## 3. Dış işlem kayıtları (`integration_jobs`)

SMS, e-Fatura, KABİS, HGS, kanal yöneticisi, WhatsApp ve ödeme webhook / iade işleri tek tabloda; her sağlayıcı çağrısı `integration_attempts`'te bir satır (istek / yanıt özeti; kimlik bilgisi, SMS metni, OTP, tam telefon, kart verisi yok). Tasarım: [../design/integration-jobs.md](../design/integration-jobs.md).

| `status` | Panel etiketi | Anlam | Kim ilerletir |
|----------|---------------|-------|---------------|
| `pending` | Gönderilecek | Gönderilmedi ya da sağlayıcı "yapılmadı" dedi | Runner (sweep her dakika) |
| `processing` | İşleniyor | Ödeme akışı satır içi aldı | Ödeme modülü |
| `submitted` | Sağlayıcıya iletildi | Kabul edildi, sonuç sorgulanıyor | Runner (`CheckStatus`) |
| `needs_verification` | Doğrulama gerekiyor | Sonuç bilinmiyor (zaman aşımı, 5xx); yeniden gönderilmez | Yalnız `CheckStatus` ya da personel |
| `succeeded` | Başarılı | Tamam | — |
| `failed` | Başarısız | Sağlayıcı yapılmadığını teyit etti | Personel "Yeniden dene" |
| `blocked` | Eksik veri — gönderilmedi | Gerekli veri yok | Sahibi modül (ör. KABİS yeniden doğrulama) |
| `waiting_dependency` | Bağımlılık bekliyor | Çalışan adaptör yok (resmî KABİS / e-Fatura) | Adaptör gelince |
| `reported_manually` | Manuel bildirildi | Resmî manuel kanaldan bildirildi | Personel |
| `reconciled` | Uzlaştırıldı | Sağlayıcıyla teyit edildi | Personel |
| `ignored` | Yok sayıldı | İşlem gerekmeyen bildirim | — |

Yeniden deneme: `POST /v1/tenant/integration-jobs/{uuid}/retry` (`tenant.integrations.manage`); `failed` / `needs_verification` / `submitted` işlerde önce durum sorgulanır, yanıt `already_done` / `in_progress` / `resubmitted`; bilinmiyorsa hiçbir şey gönderilmez (`409 INTEGRATION_JOB_UNVERIFIED`). Otomatik deneme sayısı `max_attempts` (varsayılan 5, SMS 8); geri çekilme 15 sn … 30 dk. KABİS'e özel elle kapatma: KABİS bildirimleri sayfası (`tenant.kabis.retry`, `tenant.kabis.reconcile`). Panelde genel "uzlaştırıldı" işlemi yok; geri yükleme sonrası SQL yolu backup-restore.md'de.

| Nerede | Ne |
|--------|----|
| Ayarlar → Entegrasyonlar → hesap → işler | Hesabın iş listesi, iş detayı (denemeler), "Yeniden dene" |
| KABİS bildirimleri (`/t/{slug}/kabis`) | Kiralama başına KABİS kayıtları |
| HGS/OGS geçişleri, Faturalar, Kanal yöneticisi | Modülün kendi durumları |
| Platform → Kullanım ve sağlık (`/platform/health`, `platform.health.read`) | Bağlantı sağlığı → "Kuyruklar ve işler": Asynq kuyruk sayıları, durum başına iş sayısı (`GET /v1/platform/health`) |
| Uyarılar | İş `failed` / `needs_verification` olunca ve Asynq görevi denemeleri tükenince platform kullanıcılarına uygulama içi bildirim, `ALERT_EMAIL` doluysa e-posta; kaynak + tür başına 15 dk'da bir ([observability.md](../operations/observability.md#başarısız-iş-uyarıları)) |
| Sentry | `failed` / `needs_verification` işler `job_kind`, `job_uuid`, `provider` etiketleriyle |

```sql
-- Takılan dış işler
SELECT uuid, provider, kind, status, attempts, max_attempts, next_attempt_at, provider_ref, updated_at
FROM integration_jobs WHERE organization_id = :org
  AND status IN ('pending', 'submitted', 'needs_verification', 'failed', 'blocked', 'waiting_dependency')
ORDER BY updated_at;

-- Bir işin denemeleri
SELECT attempt_no, operation, trigger, outcome, error, started_at, duration_ms, response_summary
FROM integration_attempts WHERE integration_job_id = (SELECT id FROM integration_jobs WHERE uuid = :job) ORDER BY attempt_no;
```
