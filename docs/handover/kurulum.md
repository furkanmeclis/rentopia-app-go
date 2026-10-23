# Kurulum ve çalıştırma

Ayrıntılı komut başvurusu: [../DEVELOPMENT.md](../DEVELOPMENT.md). Bu belge sırayı ve teslimde bilinmesi gerekenleri toplar. Make hedefleri kök `Makefile` ve `backend/Makefile` ile karşılaştırıldı.

## 1. Ön koşullar

| Araç | Sürüm / kaynak |
|------|----------------|
| Go | `backend/go.mod` (`go 1.26.4`) |
| Node + pnpm | `frontend/.nvmrc`, `site/.nvmrc` (`22.22.0`); CI pnpm `11.22.0` |
| Docker + Compose | Altyapı ve prod stack |
| golang-migrate (`migrate` CLI), sqlc, air, golangci-lint, gofumpt | [backend/TOOLCHAIN.md](../../backend/TOOLCHAIN.md); `make tools` sürümleri yazar |
| Prod host | bash 4+, `openssl`, `awk` (`scripts/prod-env.sh`); yedek için `age` |

## 2. Ortam dosyası

Tek kanonik dosya repo kökündeki `.env` (`backend/.env` yalnız yedek; `frontend/.env.local` tutulmaz).

```sh
cp .env.example .env
```

| Anahtar | Not |
|---------|-----|
| `NAME_PREFIX`, `COMPOSE_PROJECT_NAME`, `LOCAL_NETWORK_NAME`, `APP_NETWORK_NAME`, `*_VOLUME_NAME` | Aynı Docker host'unda başka stack varsa çakışmayı önler |
| `API_URL` | `/v1` ile bitmeli (varsayılan `http://127.0.0.1:8080/v1`) |
| `TENANCY_ENABLED=true` | Firma rotaları (`/v1/tenant/*`) yalnız bununla bağlanır; `.env.example` ve prod şeması `true` |
| `APP_ENV` | `development` (yerel varsayılan) / `staging` / `production`; bkz. §9 |
| `SITE_BASE_URL` | Markalı site tabanı; yerelde `http://localhost:3100` |

## 3. Yerel çalıştırma

```sh
make local-dev            # infra (compose.local.yml) → Postgres bekle → migrate-up → :8080/:3000 boşalt → air + pnpm dev
make create-super-admin   # varsayılan admin@example.com / Password1; SA_EMAIL SA_PASSWORD SA_NAME SA_SURNAME ile değişir
make site-dev             # markalı site :3100 → http://{subdomain}.localhost:3100
```

- `make local-dev` migration için host'taki `migrate` CLI'ını kullanır (`backend/Makefile` `migrate-up`, önce `db-create`). Migration hatası akışı durdurmaz (`|| true`); çıktıyı okuyun.
- Worker varsayılan olarak API süreci içinde çalışır (`QUEUE_WORKER_INPROCESS=true`, `.env.example`). Ayrı süreç: `QUEUE_WORKER_INPROCESS=false` + `make -C backend worker`.
- Diğer hedefler: `make infra` / `infra-down` / `infra-ps` / `infra-logs`, `make backend-dev`, `make frontend-dev`, `make migrate-up`, `make search-reindex`, `make check-i18n`, `make free-dev-ports`, `make tools`. `backend/`: `make -C backend migrate-down` (1 adım geri), `migrate-create NAME=…`, `sqlc`, `test`, `lint`, `openapi-lint`, `health`.
- Kök `Makefile`'da `make migrate` hedefi yok; doğru hedef `make migrate-up`.
- Yerel URL'ler (panel :3000, API :8080, Scalar `/docs/`, Adminer :8081, MailHog :8025, …): [DEVELOPMENT.md → Local URLs](../DEVELOPMENT.md#local-urls).
- Sağlayıcı fake'leri ve iyzico / Netgsm HTTP fake'leri: [DEVELOPMENT.md → Provider fakes](../DEVELOPMENT.md#provider-fakes).

## 4. Prod: `compose.prod.yml` + Dokploy

| Adım | Komut / yer |
|------|-------------|
| 1. Env üret | `make prod-env` = `scripts/prod-env.sh init -o .env.server` (yalnız uygulama URL'ini sorar; `--app-url URL` ile sorusuz). Dosya mod 600 |
| 2. Kontrol | `make prod-config` (`docker compose … config`) |
| 3. Başlat | `make prod-up` (`up -d --build`); Dokploy'da `.env.server` içeriği compose uygulamasının env'ine yapıştırılır |
| 4. Domain'ler (Dokploy) | panel → `frontend` :3000; `realtime.<app host>` → `centrifugo` :8000; `*.<app host>` (wildcard sertifika, DNS challenge) → `site` :3000 |
| 5. İlk yönetici | `make prod-super-admin SA_EMAIL=… SA_PASSWORD=…` (backend container'ında `/app/create-super-admin`) |
| 6. TSB kataloğu | `docker compose --env-file .env.server -f compose.prod.yml exec backend /app/catalog-import -file <container içi yol>.xlsx -apply` (önce `-apply` olmadan kuru çalıştırma) ya da Platform → Araç kataloğu → Aktarımlar |
| Düz Docker host | `docker network create dokploy-network` bir kez; otomatik güncelleme `scripts/server-sync.sh` / `make prod-sync` (cron örneği DEVELOPMENT.md'de) |

Ağ: yalnız `frontend`, `site`, `centrifugo` `dokploy-network`'e katılır; backend, worker, Postgres, Redis, SeaweedFS, Meilisearch, wuzapi özel ağda kalır, hiçbir servis host portu açmaz. Tarayıcı API'ye yalnız panelin `/api/v1` proxy'si üzerinden ulaşır.

`APP_ENCRYPTION_KEY` veri olan kurulumda asla değişmez (entegrasyon kimlik bilgileri, şifreli kişisel veri, kör indeksler ondan türetilir); `.env.server` yedeğini ayrı saklayın ([backup-restore](../operations/backup-restore.md)).

### `scripts/prod-env.sh`

Şema: `deploy/env/schema` (`@version 10`). Değişiklik kuralı: yeni anahtar şemaya eklenir ve `@version` aynı değişiklikte artar.

| Komut | Make | Ne yapar |
|-------|------|----------|
| `init [-o FILE] [--app-url URL]` | `make prod-env` | Tüm URL'leri türetir, tüm gizli bilgileri üretir |
| `upgrade FILE [-o OUT] [--app-url URL]` | `make prod-env-upgrade` | Mevcut değerleri korur, yeniden adlandırmaları uygular, yalnız eksik anahtarları üretir, kaldırılanları yorum yapar; eski dosya `<file>.bak.<zaman>` |
| `check FILE` | `make prod-env-check` | Eksik anahtar / şema farkı varsa exit 1 |

### Migration

| Ortam | Nasıl |
|-------|-------|
| Yerel | `make migrate-up` (host `migrate` CLI, `backend/migrations`) |
| Prod | `migrate` servisi: backend imajı `APP_ROLE=migrate` ile `migrate -path /app/migrations … up` çalıştırır; `backend` ve `worker` bunun başarıyla bitmesini bekler (`service_completed_successfully`, `AUTO_MIGRATE=false`). `migrate` ikilisi imaja `migrate/migrate:v4.19.1`'den kopyalanır (CI test veritabanı da aynı sürüm) |
| Geri alma | Prod'da otomatik geri alma yok (`server-sync.sh --force` migration'ları geri almaz). Elle: `migrate … down 1`; `down` dosyaları veri kaybını belirtir |

Migration yazma: [veri-modeli.md](./veri-modeli.md#migration-ekleme).

## 5. İlk yönetici ve ilk firma

1. `make create-super-admin` (yerel) / `make prod-super-admin` (prod). Parola en az 8 karakter, büyük + küçük harf + rakam.
2. `/platform/login` → Platform → **Firmalar** → **Firma oluştur**: Firma adı, Kısa ad (`/t/{kısa-ad}`), Sahip (Var olan kullanıcı ya da E-posta ile davet; davet 7 gün geçerli), paket (Komple / Yönetim). Firma oluşunca varsayılan "Merkez" şubesi açılır.
3. Firma paneli `/t/{slug}`; sonraki adımlar: [../guides/firma-yoneticisi.md](../guides/firma-yoneticisi.md).

## 6. Markalı site, alan adı, HTTPS

| Konu | Ayrıntı |
|------|---------|
| Alt alan adı | Site `{subdomain}.<SITE_BASE_URL host>` adresinde (prod varsayılanı: app host). Firma: Ayarlar → Markalı site (`/t/{slug}/settings/site`, API `/v1/tenant/site`, `tenant.site.read` / `.manage`, özellik `site`) |
| Wildcard HTTPS | Dokploy'da `*.<app host>` → `site` :3000, wildcard sertifika (DNS challenge) |
| Özel alan adı | Ayarlar → Markalı site → Alan adları (`/v1/tenant/site/domains`). Firma `_rentopia-verify.{host}` TXT kaydını ve `{host}` CNAME → `SITE_DOMAIN_TARGET_HOST` (ya da A/AAAA → `SITE_DOMAIN_TARGET_IPS`) yayınlar. Worker her dakika kontrol eder; 72 saatte doğrulanmayan kayıt başarısız olur. Firma başına en çok 3 alan adı |
| TLS sağlayıcı | `SITE_DOMAIN_PROVISIONER`: `manual` (varsayılan: platform yöneticisi Dokploy'da host'u `site` :3000'e ekler, Let's Encrypt açar, sonra Platform → Alan adları → "HTTPS aktif"; API `POST /v1/platform/site-domains/{uuid}/mark-tls-active`), `dokploy` (`DOKPLOY_URL`, `DOKPLOY_API_KEY`, `DOKPLOY_COMPOSE_ID`), `traefik_file` (`SITE_DOMAIN_TRAEFIK_DIR`) |

Ayrıntı: [../design/branded-site.md](../design/branded-site.md) §2.

## 7. Yedek ve geri yükleme

[../operations/backup-restore.md](../operations/backup-restore.md): `make prod-backup`, `make prod-backup-status`, `scripts/restore.sh`, `COMPOSE_PROFILES=backup` zamanlaması, staging provası (RT-A32). Prova sonucu: [test-ve-bilinenler.md](./test-ve-bilinenler.md#geri-yükleme-provası).

## 8. Gözlemleme

[../operations/observability.md](../operations/observability.md): Sentry DSN'leri (`BACKEND_SENTRY_DSN`, `FRONTEND_SENTRY_DSN`, `SITE_SENTRY_DSN`), log maskeleme, başarısız iş uyarıları (`ALERT_EMAIL`), platform sağlık panosu (`/platform/health`, Kullanım ve bağlantı sağlığı).

## 9. Ortam ayrımı

| Konu | Kural |
|------|-------|
| `APP_ENV` | Backend, worker, frontend, site aynı değeri okur; prod şeması `production`. Bilinmeyen değerde API açılmaz; `development` dışında örnek gizli bilgiler reddedilir |
| KABİS resmî adaptör | `production` dışında reddedilir; açmak için `KABIS_ALLOW_PRODUCTION_IN_NON_PROD=true` |
| e-Fatura resmî entegratör | Aynı; `EINVOICE_ALLOW_PRODUCTION_IN_NON_PROD=true` |
| Netgsm `live` hesap | URL resmî `*.netgsm.com.tr` iken `production` dışında reddedilir; `NETGSM_ALLOW_PRODUCTION_IN_NON_PROD=true` |
| iyzico | Koruma yok; staging'de `IYZICO_LIVE_BASE_URL` → `cmd/iyzico-fake` (açık madde) |
| Fake sağlayıcılar | `*_fake`, `SMS_LOG_PROVIDER` yalnız `production` dışında kayıtlı |
| Site | `production` dışında `robots.txt` her şeyi kapatır |
| Staging | Ayrı `NAME_PREFIX`; dış ağ çıkışı kapalı; [backup-restore → Staging yalıtımı](../operations/backup-restore.md#staging-yalıtımı-zorunlu) |

`*_ALLOW_PRODUCTION_IN_NON_PROD` anahtarları `production`'da yok sayılır.
