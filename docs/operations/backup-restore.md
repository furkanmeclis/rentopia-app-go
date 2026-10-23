# Yedekleme ve geri yükleme

Prod stack'i (`compose.prod.yml`) için yedekleme, geri yükleme ve staging provası (TEC-741, kabul: RT-A32). Araçlar: [`scripts/backup.sh`](../../scripts/backup.sh), [`scripts/restore.sh`](../../scripts/restore.sh) (ortak yardımcılar [`scripts/backup-common.sh`](../../scripts/backup-common.sh)), opt-in `backup` compose servisi ([`deploy/backup/Dockerfile`](../../deploy/backup/Dockerfile)).

Bu belgede "doğrulanmadı" yazan her madde kodla ya da gerçek sağlayıcıyla teyit edilmemiştir; prova sırasında kontrol edin.

## Ne yedekleniyor

| Bileşen | Nasıl | İçerik |
|---------|-------|--------|
| Postgres (`<NAME_PREFIX>-postgres`, `DB_NAME`) | `pg_dump -Fc`, container içinden (sunucuyla aynı sürüm) | Tüm iş verisi; `integration_jobs`, `outbox_events`, `notification_dispatches` dahil |
| Redis (`<NAME_PREFIX>-redis`) | `redis-cli --rdb` (replikasyon anlık görüntüsü) | Asynq kuyrukları: pending / active / scheduled / retry / archived görevler |
| Nesne deposu (SeaweedFS S3, `S3_BUCKET`) | `rclone sync` (S3 API, `S3_ACCESS_KEY` / `S3_SECRET_KEY`) → `objects.tar` | Sözleşme ve onay PDF'leri, muayene fotoğrafları, araç belgeleri, logolar, site medyası, dışa aktarımlar |

Yedeklenmeyenler:

| Bileşen | Neden / ne yapılır |
|---------|--------------------|
| Meilisearch (`meili_data`) | Türetilmiş veri. `restore.sh` boşaltır; worker açılışta boş her indeksi yeniden kurar (`searchengine.Indexer.Bootstrap`). `--keep-search` ile atlanır |
| wuzapi (`wuzapi_data`, WhatsApp oturumları) | Felakette organizasyonlar numarayı panelden QR ile yeniden bağlar. Staging'e asla kopyalanmaz (gerçek numaralardan mesaj gider) |
| `site_domains` (yalnız `SITE_DOMAIN_PROVISIONER=traefik_file`) | Worker'ın bu dosyaları kayıtlardan yeniden üretip üretmediği doğrulanmadı |
| Centrifugo | Durumsuz |
| `.env.server` | Sette yok. `APP_ENCRYPTION_KEY` olmadan geri yüklenen veritabanındaki şifreli alanlar (entegrasyon kimlik bilgileri; customers, contracts, invoices, sms, whatsapp, kabis, channels, publicapi modüllerinin şifreli alanları) okunamaz. `.env.server` kopyasını age gizli anahtarının yanında, parola yöneticisinde tutun |

## Set biçimi

Her çalıştırma bir set yazar: `$BACKUP_DIR/<NAME_PREFIX>-<YYYYmmddTHHMMSSZ>.tar.age`, yanında `.tar.age.sha256` ve şifresiz `.manifest.txt` (yalnız sayılar, gizli bilgi yok).

Şifreli tar'ın içi:

| Dosya | |
|-------|-|
| `manifest.txt` | Set adı, kaynak `NAME_PREFIX`, UTC zaman, bileşen sha256'ları, şema sürümü, ana tabloların satır sayıları, `integration_jobs` / `notifications` / `notification_dispatches` / PDF durum dağılımı, Asynq kuyruk boyları, nesne sayısı |
| `postgres.dump` | `pg_dump -Fc` |
| `redis.rdb` | Redis anlık görüntüsü |
| `objects.tar` | Bucket'ın tamamı (anahtar yolu = dosya yolu) |

Sıra **Postgres → Redis → nesneler** ve set atomik değil (üç ayrı an, aralarında saniyeler–dakikalar):

- Nesneler en son: uygulama önce nesneyi yükler, sonra satırı yazar (ör. `contracts/usecase/pdf.go`: `Upload` → `SetDocumentVersionPDF`). Dump'taki her anahtar kopyadadır; sonradan yüklenenler sahipsiz kalır, zararsızdır.
- Redis, dump'tan sonra: arada kuyruğa giren görevler dump'ta olmayan satırları gösterebilir; işleyiciler "bulunamadı" ile biter (ör. integration runner `ErrNotFound` → görev başarılı sayılır). Görevi Redis'te olmayan satırlar için aşağıdaki "(b)" tablosuna bakın.
- `pg_dump` kendi içinde tutarlıdır (tek snapshot).

**RPO = yedek aralığı (varsayılan 24 saat).** WAL arşivleme / PITR kurulmadı. Dokploy'un yerleşik veritabanı yedeği kullanılmıyor; Dokploy'un compose içindeki servisleri/volume'ları yedekleyip yedeklemediği doğrulanmadı. Daha düşük RPO için `scripts/backup.sh` host cron'undan sık çalıştırılabilir (saklama her günün en yeni setini tutar).

## Kurulum

### 1. Şifreleme anahtarı (age)

Sunucuda değil, yönetici makinesinde:

```sh
age-keygen -o rentopia-backup.agekey      # gizli anahtar: parola yöneticisi + çevrimdışı kopya
age-keygen -y rentopia-backup.agekey      # age1... → BACKUP_AGE_RECIPIENT
```

- Sunucu yalnız açık anahtarı bilir: setleri yazabilir, açamaz. Geri yükleme `--identity` ile gizli anahtar dosyasını ister.
- Birden çok alıcı virgülle: `BACKUP_AGE_RECIPIENT=age1aaa...,age1bbb...` (iki kişi ayrı anahtar tutar).
- Değiştirme: yeni açık anahtarı ekleyin, eski gizli anahtarı eski setlerin saklaması bitene kadar (en az `BACKUP_KEEP_WEEKLY` hafta + off-site saklama) saklayın.

### 2. Env (şema v9'dan beri; güncel v10)

```sh
scripts/prod-env.sh upgrade .env.server    # yeni anahtarları ekler, mevcut değerleri korur
```

| Anahtar | Varsayılan | Anlam |
|---------|-----------|-------|
| `COMPOSE_PROFILES` | boş | `backup` → zamanlanmış `backup` servisi başlar |
| `BACKUP_AGE_RECIPIENT` | boş | age açık anahtar(lar)ı; boşsa yedek çalışmaz |
| `BACKUP_DIR` | `/var/backups/<NAME_PREFIX>` | Host'ta setlerin dizini (servis aynı yola bağlar) |
| `BACKUP_TIME` | `03:15` | Günlük çalışma saati (servis: Europe/Istanbul) |
| `BACKUP_KEEP_DAILY` | `7` | Son N günün her birinin en yeni seti |
| `BACKUP_KEEP_WEEKLY` | `4` | Son M haftanın (Pazartesi başlangıç, UTC) her birinin en yeni seti |
| `BACKUP_OFFSITE_ENDPOINT` | boş | Off-site S3 uç noktası (ör. `https://s3.eu-central-1.amazonaws.com`) |
| `BACKUP_OFFSITE_REGION` | boş (`us-east-1`) | Off-site bölge |
| `BACKUP_OFFSITE_BUCKET` | boş | Doluysa her set `<bucket>/<NAME_PREFIX>/` altına yüklenir |
| `BACKUP_OFFSITE_ACCESS_KEY` / `BACKUP_OFFSITE_SECRET_KEY` | boş | Off-site kimlik bilgileri |

Script'e özel (env dosyasına yazılmaz): `BACKUP_RCLONE_IMAGE` (varsayılan `rclone/rclone:1.68.2`), `BACKUP_S3_ENDPOINT` (varsayılan `http://seaweedfs.<APP_NETWORK_NAME>:9000`), `BACKUP_AGE_IDENTITY_FILE` (`--identity` yerine).

### 3. Zamanlama

İkisinden biri:

**A. Compose servisi** (Dokploy'da da): env'e `COMPOSE_PROFILES=backup`, sonra `make prod-up` (Dokploy: redeploy). Servis `deploy/backup/Dockerfile` imajını kurar (alpine + bash, age, docker-cli) ve `backup.sh schedule` çalıştırır. Docker soketini bağlar: host'ta root yetkisine eşdeğerdir; servisi `dokploy-network`'e eklemeyin, port açmayın. Dokploy'un `COMPOSE_PROFILES`'ı env'den okuduğu doğrulanmadı; okumazsa B'yi kullanın.

**B. Host cron** (`server-sync.sh` ile aynı düzen; host'ta bash 4+, docker, `age` gerekir: `apt install age`):

```cron
15 3 * * * /opt/rentopia/scripts/backup.sh --env-file /opt/rentopia/.env.server >> /var/log/rentopia-backup.log 2>&1
```

rclone host'a kurulmaz: `rclone/rclone:1.68.2` imajı app ağına bağlanarak çalışır (SeaweedFS dışarı açık değil).

Elle: `make prod-backup`, deneme: `scripts/backup.sh --env-file .env.server --dry-run`.

### 4. Off-site

- Başka sağlayıcı ya da bölgede, S3 uyumlu, önceden oluşturulmuş bucket (script bucket oluşturmaz).
- Anahtara yalnız yazma (`PutObject`) yetkisi verin; script off-site'ta hiçbir şey silmez. Sunucu ele geçirilse bile eski setler silinemesin diye bucket'ta versioning / Object Lock açın.
- Off-site saklamayı bucket yaşam döngüsü kuralı belirler (ör. 35 gün sonra sil).
- Setler zaten şifreli; sağlayıcı tarafı şifreleme ek katmandır.

### 5. İzleme

```sh
make prod-backup-status            # en yeni set 26 saatten eskiyse / yoksa exit 1
docker logs <NAME_PREFIX>-backup   # "BACKUP FAILED" satırları
```

## Saklama

Her çalıştırma sonunda yerel dizin budanır: son `BACKUP_KEEP_DAILY` günün her birinin en yeni seti + son `BACKUP_KEEP_WEEKLY` haftanın her birinin en yeni seti; en yeni set her zaman kalır, aynı günün eski setleri silinir. Setler tamdır (bucket her sette baştan); disk ihtiyacı ≈ (dump + bucket) × (günlük + haftalık).

## Geri yükleme

Gerekenler: set dosyası (+ `.sha256`), age gizli anahtar dosyası, **hedef** stack'in env dosyası, hedef stack'in `postgres`, `redis`, `seaweedfs` container'ları ayakta.

Off-site'tan indirme:

```sh
export RCLONE_CONFIG_OFFSITE_TYPE=s3 RCLONE_CONFIG_OFFSITE_PROVIDER=Other \
  RCLONE_CONFIG_OFFSITE_ENDPOINT=... RCLONE_CONFIG_OFFSITE_ACCESS_KEY_ID=... RCLONE_CONFIG_OFFSITE_SECRET_ACCESS_KEY=...
docker run --rm -v "$PWD:/data" -e RCLONE_CONFIG_OFFSITE_TYPE -e RCLONE_CONFIG_OFFSITE_PROVIDER \
  -e RCLONE_CONFIG_OFFSITE_ENDPOINT -e RCLONE_CONFIG_OFFSITE_ACCESS_KEY_ID -e RCLONE_CONFIG_OFFSITE_SECRET_ACCESS_KEY \
  rclone/rclone:1.68.2 copy "offsite:<bucket>/<NAME_PREFIX>/" /data --include "<set>*"
```

Çalıştırma:

```sh
scripts/restore.sh <set>.tar.age --identity rentopia-backup.agekey --env-file .env.staging --dry-run   # aç + kontrol et
scripts/restore.sh <set>.tar.age --identity rentopia-backup.agekey --env-file .env.staging
```

Sıra:

1. Arşiv sha256'sı, şifre çözme, bileşen sha256'ları (manifest).
2. Hedef kontrolü: `--target` varsayılanı `staging`. Setin geldiği stack'e (aynı `NAME_PREFIX`) dönüş prod sayılır; prod `--yes-i-mean-prod` olmadan reddedilir. Onay için hedef `NAME_PREFIX` yazdırılır (`--yes` atlar).
3. `backend`, `worker` (ve çalışıyorsa `backup`) durdurulur.
4. Postgres: veritabanı silinip yeniden oluşturulur (yeni migration'ların tabloları kalmasın), `pg_restore --clean --if-exists --no-owner --no-acl --exit-on-error`.
5. Bucket: `rclone sync` (setteki nesneler; fazlası silinir). `--skip-objects` atlar.
6. Redis: durdurulur, `dump.rdb` volume'a yazılır, AOF geçici bir redis-server ile bu dump'tan yeniden kurulur (sunucu `--appendonly yes` ile çalıştığından aksi halde eski AOF yüklenirdi), başlatılır. `--skip-redis` atlar.
7. Meilisearch boşaltılır (`--keep-search` atlar).
8. Migration: `<NAME_PREFIX>-migrate` container'ı yeniden çalıştırılır (yoksa `docker compose run --rm migrate`). İmaj setten yeniyse şema ilerler.
9. Doğrulama (henüz trafik yokken): satır sayıları manifest ile karşılaştırılır, PDF anahtarları bucket'ta aranır, kuyruk durumu yazılır.
10. `worker` ve `backend` başlatılır (`--no-start` durdurulmuş bırakır). Sonra tekrar: `scripts/restore.sh verify --env-file ... --manifest /tmp/<set>.manifest.txt`.

**Prod'a dönüşte** `--no-start` kullanın ve worker'ı başlatmadan önce aşağıdaki "(c)" bölümündeki gözden geçirmeyi yapın: setten sonra gerçekten gönderilmiş işler, worker açılınca yeniden gönderilir. SMS / e-posta / bildirim motoru mesajları için bu adım [`messaging-reconcile`](#mesaj-mutabakatı-messaging-reconcile)'dır (9. ile 10. adım arası); entegrasyon işleri (e-Fatura, HGS, kanal yöneticisi, WhatsApp, runner SMS işleri) için `integration-jobs-reconcile` (aşağıda "(c)"). Ödemeler ve iadeler worker'ı beklemez: eşleşmeyen bildirimler ve iade koruması trafik başlayınca çalışır.

## Staging provası

### Staging stack'i

- Ayrı `NAME_PREFIX` (ör. `rentopia-staging`): ayrı container, volume, ağ. Aynı `NAME_PREFIX`'e geri yükleme prod sayılır.
- `scripts/prod-env.sh init -o .env.staging --app-url https://staging.example.com`, ardından `NAME_PREFIX`, imaj adları düzenlenir.
- `APP_ENCRYPTION_KEY` prod'dan kopyalanır (yoksa şifreli alanlar okunamaz). Bu yüzden staging de prod gizli bilgisi taşır; erişimi prod gibi kısıtlayın.
- DB / Redis / S3 parolaları staging'in kendisinin olabilir (`pg_restore --no-owner`, bucket S3 API ile yazılır).

### Staging yalıtımı (zorunlu)

Geri yüklenen veritabanında organizasyonların canlı entegrasyon hesapları (iyzico, Netgsm, …) ve müşterilerin gerçek telefon / e-postaları var; staging worker'ı bunları kullanarak gönderim yapar.

| Kanal | Önlem |
|-------|-------|
| E-posta | `SMTP_*` → MailHog ya da boş |
| Ortam | `APP_ENV=staging`: resmî KABİS / e-Fatura adaptörleri ve Netgsm'in resmî live host'u reddedilir ([observability](./observability.md#ortam-app_env)) |
| Netgsm SMS | `NETGSM_LIVE_BASE_URL` → `cmd/netgsm-fake` (gelenleri `/` sayfasında listeler) |
| iyzico | `IYZICO_LIVE_BASE_URL` → `cmd/iyzico-fake` |
| WhatsApp | Staging'in kendi boş `wuzapi_data` volume'u (oturum yok → gönderim başarısız) |
| Site alan adları | `DOKPLOY_*` boş, `SITE_DOMAIN_PROVISIONER=manual` |
| e-Fatura, KABİS | Prod'da çalışan adaptör yok (resmi slotlar `waiting_dependency`, TEC-685 / TEC-682); fake'ler production dışında kayıtlıdır; `APP_ENV=staging` resmî slotu ayrıca reddeder |
| HGS, kanal yöneticileri, Public API webhook'ları, push | Uç noktaları env ile yönlendirilemiyor (doğrulanmadı) → **staging host'unda dış ağ çıkışını kapatın** (yalnız imaj çekme vb. izinli) |

### Prova adımları

Amaç: (a) kayıt tutarlılığı, (b) bekleyen işlerin kaldığı yerden devamı, (c) tekrar oluşturma olmaması; ayrıca "set ile felaket arası pencere"de olanların bilinen davranışını görmek.

1. Prod'un son setini staging'e yükleyin (yukarıdaki komut). Çıktıdaki doğrulama: `rows.*` farkı 0, eksik PDF anahtarı 0.
2. Staging'de bekleyen iş üretin: netgsm-fake kapalıyken birkaç SMS (işler `pending` / backoff), bir sözleşme PDF'i, bir tahsilat, bir fatura taslağı.
3. Staging'in setini alın: `BACKUP_AGE_RECIPIENT` dolu bir `.env.staging` ile `scripts/backup.sh --env-file .env.staging`. Manifestteki `integration_jobs.*`, `asynq.*` değerlerini not edin.
4. "Kayıp pencere"yi oynatın: netgsm-fake'i açın, işlerin bir kısmı gönderilsin; fake'in listesini kaydedin. Bir tahsilat daha girin.
5. 3. adımdaki seti staging'e geri yükleyin.
6. RT-A32 kontrolleri (aşağıda). 4. adımdaki tahsilatın kaybolduğunu (RPO) görmeniz beklenir. 5. adımı `--no-start` ile yaptıysanız worker'ı başlatmadan [`messaging-reconcile`](#mesaj-mutabakatı-messaging-reconcile) `list --before <3. adımın set zamanı>` 4. adımda gönderilen SMS'leri listeler; netgsm-fake listesiyle karşılaştırıp `mark --yes` edin, sonra worker'ı başlatın: işaretlenenler ikinci kez gitmez, işaretlenmeyenler gider. Sonuçları prova notuna yazın.

## RT-A32 doğrulama

`restore.sh` sonunda ve `scripts/restore.sh verify --env-file <env> --manifest <manifest>` ile otomatik kısım çalışır; kalanı elle.

### (a) Belgeler / kayıtlar tutarlı

- `rows.*` sayıları manifest ile aynı (trafik başlamadan önce). Tablolar: organizations, users, customers, vehicles, reservations, rentals, documents, document_versions, document_approvals, inspections, payments, payment_refunds, collections, ledger_entries, invoices, security_deposits, notifications, notification_dispatches, notification_deliveries, sms_messages, whatsapp_messages, kabis_notifications, toll_passes, integration_jobs, integration_attempts, outbox_events; ayrıca `schema_migrations` sürümü.
- PDF anahtarları: `document_versions.pdf_object_key` (`pdf_status = 'ready'`) ve `document_approvals.pdf_object_key` bucket'ta var (`missing: 0`). Anahtar biçimi `organizations/<org>/documents/<doküman>/<sürüm>.pdf` (`storage.DocumentPDFObjectKey`).
- Elle: panelden bir sözleşme PDF'i ve bir muayene fotoğrafı açın.
- Eksik sürüm PDF'i varsa: sürümün HTML'i veritabanında olduğundan yeniden basılabilir; `pdf_status`'u `failed` yapın, ilk indirmede yeniden istenir (`OpenPDF` → `requestPDF`: `none` / `failed` ya da 15 dk'dan eski `pending` durumundan). Yeni PDF'in baytları (ve `pdf_sha256`) farklı olur. Onay PDF'leri için aynı yol doğrulanmadı.

```sql
UPDATE document_versions SET pdf_status = 'failed', pdf_error = 'restore: object missing'
WHERE pdf_object_key IN (/* restore.sh'in listelediği eksik anahtarlar */);
```

### (b) Bekleyen işler kaldığı yerden devam ediyor

| İş | Durum nerede | Geri yükleme sonrası | Kod |
|----|--------------|----------------------|-----|
| Entegrasyon işleri (SMS, e-Fatura, KABİS, HGS, kanal yöneticisi, WhatsApp) | `integration_jobs` (Postgres) | `app:integrations:sweep` her dakika `RunDue`: `pending` / `submitted` / `needs_verification`, `next_attempt_at <= now`, `locked_until` geçmiş, `attempts < max_attempts`. Dump anında çağrı ortasındaki iş 2 dk'lık kilit (lease) dolunca yeniden alınır. Redis'teki `app:integrations:run` görevi yalnız hızlandırır; aynı işe ikinci görev `LockRunnerJob` → `ErrBusy` ile çağrı yapmaz | `modules/integrations/jobs/usecase/runner.go`, `queries/integrations.sql`, `queue/integrations.go` |
| Bildirim motoru (müşteri / personel SMS, e-posta, WhatsApp, push) | `notification_dispatches` | Her dakika sweep `pending` ve zamanı gelmişleri gönderir. 10 dk'dan uzun `sending` kalan dispatch'ler worker açılışında ve her sweep'te `ReclaimStuck` ile ele alınır: denemeye hiçbir şey kaydedilmemişse (sağlayıcı ref'i / kanal kaydı yok) `pending`'e döner ve aynı deneme satırında sürer; kanalın gönderim defterinde (SMS: teslimatın `sms_messages` satırı) mesaj varsa gönderildi sayılır, tekrar gönderilmez; rapor bekleyen SMS kesinleşmişse (rapor kaybolmuş, mutabık) dispatch kapanır | `modules/notifyengine/usecase/reclaim.go`, `queue/notify_engine.go` |
| Platform bildirimleri (uygulama içi, e-posta) | `notifications` + Asynq `app:notification:deliver` | Görev Redis'te varsa onu teslim eder. 5 dk'dan eski `queued` satırlar her 2 dk'da (`app:notification:sweep`) ve worker açılışında satırın uuid'sinden türeyen görev kimliğiyle yeniden kuyruğa girer (görev hâlâ Redis'teyse kuyruk reddeder, iş yapılmaz); 2 dk'dan eski `processing` satırları tek bir sweep devralıp yeniden teslim eder (`ReclaimStuck`). Aynı satırın ikinci görevi hiçbir şey yapmaz | `modules/notifications/usecase/service.go`, `queue/notification_sweep.go` |
| Domain olayları | `outbox_events` | Publisher döngüsü `pending` satırları alır (en az bir kez) | `platform/outbox/outbox.go` |
| Bekleyen ödemeler | `payments` | `ReconcilePending` her dakika 2 dk–2 gün arası bekleyen checkout'ları sağlayıcıdan sorgular | `modules/payments/usecase/service.go` |
| Sözleşme PDF'leri | `document_versions.pdf_status` + `pdf_requested_at` + Asynq görevi | Görev, sürüm uuid'sinden türeyen görev kimliğiyle (`documents:render-pdf:<uuid>`) kuyruğa girer. `pending` + görev Redis'te → basılır. 15 dk'dan eski `pending` (görev kaybolmuş) worker açılışında ve her dakika (`SweepPendingPDFs`) ya da ilk indirmede yeniden istenir: görev hâlâ Redis'teyse kuyruk reddeder (ikinci basım yok), yoksa bir kez kuyruğa girer. Bu sütundan önce istenmiş `pending`'ler (`pdf_requested_at` boş) eski sayılır | `modules/contracts/usecase/pdf.go` |
| Diğer Asynq görevleri | Redis | `active` görevler kilit süresi dolunca Asynq recoverer ile retry'a alınır ve yeniden çalışır (en az bir kez; Asynq v0.26 davranışı). Periyodik görevler worker açılışında yeniden kaydedilir. Export / import / toplu işlem görevleri önce iş satırını `queued`'dan alır; ikinci çalışma (retry, geri yükleme sonrası recoverer) hiçbir şey yapmadan biter (`Process*` → `ErrNoRows` → nil). Dump anında `processing` / `applying` olan iş ise kendiliğinden bitmez (açık; panelden yeniden başlatılır) | `queue/*.go` |
| Arama | Meilisearch | Boş indeksler worker açılışında yeniden kurulur | `platform/searchengine/indexer.go` |

Kontrol: `restore.sh` çıktısındaki "(b) work to resume" bölümü + birkaç dakika sonra tekrar `verify`; `integration_jobs.pending` / `submitted` ve `asynq.*.pending` / `retry` azalmalı, `document_versions.pdf_pending` 0'a inmeli. Panel: Entegrasyon işleri ekranı.

Çalışan güvence testleri (`backend/internal/modules/integrations/jobs/usecase/runner_test.go`): `TestRunnerUncertainChecksStatusWithoutDoubleSubmit`, `TestRunnerResubmitsOnlyWhenConfirmedNotDone`, `TestRunnerUnsupportedCheckNeverResends`, `TestRunnerRetryAfterDefinitiveFailure`.

### (c) Tamamlanmış tahsilat / fatura / bildirim tekrar oluşmuyor

Veritabanı içindeki güvenceler (geri yüklenen zaman çizgisinde ikinci kayıt oluşamaz):

| Kayıt | Mekanizma | Kaynak |
|-------|-----------|--------|
| Tahsilat | `uq_collections_organization_payment` (ödeme başına bir), `uq_collections_organization_idempotency` (istemci anahtarı), `uq_collections_organization_entry` | migration 000101 |
| Muhasebe kaydı | `uq_ledger_entries_payment`, `_payment_refund`, `_accrual`, `_reverses`; `PostPending` idempotent | 000096 |
| Fatura | `uq_invoices_organization_idempotency`, `uq_invoices_organization_job`, `uq_invoices_organization_number`; e-Fatura ETTN ilk gönderimden önce üretilir ve her denemede aynıdır, belirsiz sonuçta önce durum sorgulanır | 000108, `platform/einvoice/einvoice.go` |
| Ödeme | `uq_payments_organization_provider_payment`, `uq_payments_organization_conversation`; durum `pending → succeeded` koşullu | 000078 |
| Entegrasyon işi | `uq_integration_jobs_provider_key` (sağlayıcı + idempotency key); belirsiz sonuç yeniden gönderilmeden önce `CheckStatus` ile doğrulanır | 000077, `runner.go` |
| Bildirim motoru | `uq_notification_dispatches_idempotency` (olay + referans + ayırıcı; alıcı referanstan ya da ayırıcıdan gelir), `uq_notification_deliveries_dispatch_channel` (dispatch + kanal başına bir deneme). Sağlayıcının kabulü (ref, `sent_at`) ve dispatch durumu tek transaction'da yazılır; gönderim defteri olan kanal (SMS) her gönderimden önce sorulur | 000113, `notifyengine/usecase/engine.go` |
| SMS mesajı | `uq_sms_messages_organization_idempotency`: bildirim motorunun SMS'i `notify:<teslimat uuid>` anahtarını taşır; aynı teslimat için ikinci gönderim mevcut mesajı döner, Netgsm'e ikinci istek gitmez. `reconciled` mesajın gönderim işi sağlayıcıyı çağırmadan biter | 000137, `modules/sms/usecase/service.go` |
| Platform bildirimi | `uq_notifications_idempotency`: olaydan doğan bildirim `event:<olay id>\|<tür>\|user:<id>\|<kanal>`, bildirim motorunun push'u `notify:<teslimat uuid>\|inapp`; ikinci ekleme satır oluşturmaz. Teslim görevi satır uuid'sinden türeyen Asynq görev kimliğiyle kuyruğa girer | 000137, `modules/notifications/usecase/service.go` |
| Public API yazmaları | `public_api_idempotency` (24 saat) | 000115 |

Kontrol SQL'i (hepsi 0 satır döner):

```sql
SELECT organization_id, payment_id, count(*) FROM collections WHERE payment_id IS NOT NULL GROUP BY 1, 2 HAVING count(*) > 1;
SELECT organization_id, payment_id, count(*) FROM ledger_entries WHERE payment_id IS NOT NULL GROUP BY 1, 2 HAVING count(*) > 1;
SELECT organization_id, integration_job_id, count(*) FROM invoices WHERE integration_job_id IS NOT NULL GROUP BY 1, 2 HAVING count(*) > 1;
SELECT provider, idempotency_key, count(*) FROM integration_jobs GROUP BY 1, 2 HAVING count(*) > 1;
SELECT organization_id, dispatch_id, channel, count(*) FROM notification_deliveries GROUP BY 1, 2, 3 HAVING count(*) > 1;
```

**Garanti olmayanlar** (set zamanı ile felaket arasındaki pencere; veritabanı bu pencereyi bilmez):

- Pencerede girilen tahsilat, fatura, iade, rezervasyon kaybolur (RPO). Sistem bunları çoğaltmaz; personel yeniden girer ve mutabakat yapar.
- Pencerede ödeme yapan müşteri: ödeme satırı setten sonra oluştuysa gelen webhook kaydı bulamaz. Bildirim `payment_webhook_orphans`'a yazılır: imzası bir firmanın iyzico hesabıyla doğrulanırsa o firmaya bağlanır (cevap 200, iyzico tekrar göndermez), doğrulanamazsa platform düzeyinde kalır (cevap 404 olarak sürer). Platform sağlığı sayfası eşleşmeyenleri sayar (kaynak `payment_webhook_orphans`). Firma Finans → Eşleşmeyen ödemeler ekranında bildirimi rezervasyonla eşleştirir (`POST /v1/tenant/payments/orphans/{uuid}/match`): checkout sağlayıcıdan okunur, ödenmiş ve bizim sipariş referansımızı taşıyorsa ödeme kendi uuid'siyle yeniden oluşturulup bir kez onaylanır. Rezervasyonun kendisi de pencerede oluştuysa önce yeniden girilir. Satır sette `pending` ise `ReconcilePending` sağlayıcıdan doğrular, tek tahsilat oluşur. Webhook hiç gelmediyse (iyzico'da bildirim kapalı) ödeme yalnız iyzico panelinden görülür.
- Pencerede yapılan iade: `refunded_amount` geride kalır. İade koruması: sağlayıcı iadesinden önce sağlayıcının o ödemedeki iadeleri okunur (iyzico raporlama `payment/details`; sandbox'ta doğrulanmadı, TEC-684); sağlayıcı kayıtlarımızdan fazla iade gösteriyorsa iade yapılmaz, `409 REFUND_STATE_MISMATCH` döner ve mutabakat kaydı açılır; sağlayıcının iadeleri okunamazsa iade engellenmez (korumadan önceki gibi yapılır), uyarı loglanır, Sentry'ye gider ve ödeme başına bir `unverified` mutabakat kaydı açılır (personel sonradan kontrol eder). Kayıt `POST /v1/tenant/payments/reconciliation-items/{uuid}/resolve` ile kapatılır (`record_refund`: sağlayıcıdaki eksik iade, sağlayıcı çağrılmadan kayda geçer; `dismiss`). Geri yükleme sonrası özet: `GET /v1/tenant/payments/reconciliation?since=<set zamanı>` (ödeme başına bir retrieve + bir iade sorgusu; iyzico adaptöründe listeleme yok, kaydımız olmayan ödeme yalnız eşleşmeyen bildirim olarak görünür).
- Dış gönderimler setteki durumdan yeniden yapılır:
  - **SMS (Netgsm)**: sette `queued` olan mesajın işi Submit edilir; Netgsm'e bizim tarafımızdan tekrar önleme anahtarı gitmez (`jobid` kabulde sağlayıcı verir, `platform/sms/netgsm/netgsm.go`) ve Netgsm raporu yalnız `jobid` ile sorgulanabildiğinden sette `jobid`'si olmayan mesajın gönderilip gönderilmediği otomatik anlaşılamaz. Pencerede gönderilmiş SMS, [`messaging-reconcile`](#mesaj-mutabakatı-messaging-reconcile) ile işaretlenmezse ikinci kez gider.
  - **E-posta**: SMTP'de tekrar önleme yok; sette `pending` dispatch'ler ve `queued` / `processing` platform bildirimleri, `messaging-reconcile` ile işaretlenmezse yeniden gönderilir. Çökme / dump anında yarıda kalan e-posta denemesi (kayıt yok) en az bir kez kuralıyla yeniden denenir.
  - **WhatsApp**: mesaj kimliğini biz seçip gönderiyoruz (`wuzapi.go`, `Id`); WhatsApp'ın aynı kimliği ikinci kez düşürdüğü doğrulanmadı.
  - **e-Fatura**: aynı ETTN tekrar gider; entegratörün ikinciyi reddetmesi beklenir (entegratör henüz seçilmedi, TEC-685; doğrulanmadı).
  - **Outbox**: setten sonra yayımlanmış olaylar yeniden yayımlanır. Yan etkili tek tüketici bildirim merkezidir (`notifications/bus.go`) ve olay kimliğiyle idempotenttir (ikinci yayın satır oluşturmaz, `TestOutboxReplayCreatesOneNotification`). Bildirim motoru outbox tüketicisi değildir (rezervasyon geçişinin transaction'ında ve sweep'te yazılır; anahtarı olay + referans + ayırıcı). Outbox'tan entegrasyon işi oluşturan tüketici yok; entegrasyon işleri modüllerin kendi transaction'ında `uq_integration_jobs_provider_key` ile oluşur.
- Prod'a dönüşte azaltma (`--no-start` ile, worker kapalıyken):

```sh
# Gönderilmeyi bekleyen dış işler (tüm firmalar; kuru çalışma, yalnız listeler). --before: setin UTC zamanı (RFC 3339).
make prod-integration-jobs-reconcile ARGS="--before 2026-10-20T03:15:00Z"
# Daraltma: --kinds einvoice.send,sms.send  --orgs <org uuid>,...  --statuses pending,submitted,needs_verification,failed
# Sağlayıcı panelleri / gönderim kayıtlarıyla set zamanından sonrası karşılaştırılır; yapıldığı teyit edilenler işaretlenir
# (runner bir daha çağırmaz; kim / ne zaman / neden işte ve etkinlik kaydında kalır, SMS kaydı ve e-Fatura durumu izler):
make prod-integration-jobs-reconcile ARGS='--jobs <uuid>,<uuid> --apply --note "geri yükleme: Netgsm panelinde gönderilmiş"'
# Bir türün tamamı teyit edildiyse filtreyle: --before ... --kinds einvoice.send --apply --note "..."
# SMS mesajları, bildirim motoru dispatch'leri, platform bildirimleri: aşağıdaki messaging-reconcile.
```

Yerelde / staging'de repo kökünden `make integration-jobs-reconcile ARGS="..."`.

Araç hiçbir şey göndermez; ödeme iadeleri (`payment.refund`, aşağıdaki iade koruması) ve KABİS bildirimleri (KABİS ekranındaki mutabakat, kanıtla) dışarıda kalır; worker'ın tuttuğu iş `busy` olarak raporlanır. Firma içinde aynı işlem panelde: Entegrasyonlar → hesabın işleri → "Mutabık işaretle" (`POST /v1/tenant/integration-jobs/{uuid}/reconcile`, toplu: `POST /v1/tenant/integration-jobs/reconcile`).

### Mesaj mutabakatı (`messaging-reconcile`)

Sette gönderilmeyi bekleyen ama set zamanından sonra (kayıp pencerede) gerçekten gönderilmiş mesajlar, worker açılınca ikinci kez gider: Netgsm ve SMTP tekrarı düşürecek bir anahtar almaz, geri yüklenen veritabanı da pencereyi bilmez. Karar operatöründür; araç listeler ve işaretler (`cmd/messaging-reconcile`, `internal/platform/msgreconcile`).

Ne zaman: geri yükleme `--no-start` ile, 9. adımdan (doğrulama) sonra, worker'ı başlatmadan (10. adım) önce. Staging provasında da aynı yer.

```sh
# Set zamanı: set adındaki UTC zaman (rentopia-20261020T031500Z.tar.age → 20261020T031500Z) ya da RFC 3339.
make prod-messaging-reconcile ARGS="list --before 20261020T031500Z"
# Yalnız bir organizasyon / tür: --org <slug>, --kind dispatch,sms,notification; ileride planlı hatırlatmalar
# --due-until (varsayılan: şimdi) sonrasıysa listelenmez (pencerede gönderilmiş olamazlar).

# Sağlayıcı kayıtlarıyla karşılaştırın (Netgsm panelindeki gönderilenler, mail sunucusu logu, netgsm-fake listesi):
make prod-messaging-reconcile ARGS="mark --before 20261020T031500Z --uuids <uuid,uuid,...>"        # kuru çalışma
make prod-messaging-reconcile ARGS="mark --before 20261020T031500Z --uuids <uuid,uuid,...> --yes"  # işaretle
# Listedeki hepsi gönderildiyse --uuids'siz mark aynı filtrenin tamamını işaretler.
```

Yerelde / staging'de repo kökünden `make messaging-reconcile ARGS="..."` (kök `.env`). Prod hedefi backend imajını `docker compose run --rm --no-deps` ile çalıştırır (backend durdurulmuşken de çalışır).

| Tür | Liste | İşaretleme | Sonra |
|-----|-------|------------|-------|
| `dispatch` | `notification_dispatches` `pending` / `sending`, set zamanından önce oluşmuş, `scheduled_at <= --due-until` | `reconciled` (+ not) | Sweep göndermez; panelde "Geri yükleme sonrası mutabık" |
| `sms` | `sms_messages` `queued`, sağlayıcı mesaj kimliği yok, set zamanından önce | `reconciled`, mühürlü metin silinir | Gönderim işi Netgsm'i çağırmadan `succeeded` biter (deneme yanıtı `reconciled: true`); motorun dispatch'i "ulaştı" olur |
| `notification` | `notifications` `queued` / `processing` (platform ve organizasyon; e-posta, uygulama içi, push) | `cancelled` + `last_error` notu + `reconciled` geçmiş kaydı | Teslim görevi hiçbir şey yapmaz |

İşaretlenmeyenler normal akışla gönderilir. Sağlayıcı kimliği olan SMS (`submitted`) listede yoktur: rapor sorgulanır, yeniden gönderilmez. `unknown` (sonucu belirsiz) SMS'ler entegrasyon işi olarak `needs_verification`'da kalır, entegrasyon işleri ekranından çözülür. WhatsApp mesajları ve diğer entegrasyon işleri bu aracın kapsamında değil (yukarıdaki `integration-jobs-reconcile`).

## Sorun giderme

| Belirti | Neden / çözüm |
|---------|---------------|
| `missing env: BACKUP_AGE_RECIPIENT` | Açık anahtar girilmemiş |
| `another backup is running` | Yarım kalmış çalıştırma; süreç yoksa `$BACKUP_DIR/.lock` dizinini silin |
| `pg_restore: error: ...` (`--exit-on-error`) | Hedef veritabanı yarım kalır; sorunu giderip restore'u tekrarlayın (veritabanı her seferinde yeniden oluşturulur) |
| Redis'te anahtar sayısı 0 | `redis.rdb` boş ya da AOF yeniden kurulamadı; `restore.sh` çıktısındaki `keys:` satırına bakın |
| `container not running: <NAME_PREFIX>-...` | `NAME_PREFIX` / env dosyası yanlış stack'i gösteriyor ya da altyapı kapalı |
