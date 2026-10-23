# Ayarlar

Firma paneli ayarları sol menüde **Ayarlar** grubunda (`/t/{slug}/settings/*`); platform ayarları `/platform/*`. Her satırda: ekran, API, yetki ve (varsa) paket özelliği. Yetki slug'ları `backend/internal/platform/rbac`, menü `frontend/src/config/nav.ts`.

## Özet

| Menü (tr) | Yol | API | Okuma / yazma yetkisi | Özellik |
|-----------|-----|-----|------------------------|---------|
| Ayarlar → Genel | `/t/{slug}/settings` | `GET/PUT /v1/tenant/settings`, `…/settings/versions*`; ad / logo `GET/PATCH /v1/tenant/organization`, `PUT/DELETE …/organization/logo` | `tenant.settings.read` / `.write` | — |
| Ayarlar → Üyeler | `/t/{slug}/settings/members` | `/v1/tenant/members*`, `/v1/tenant/invitations*` | `tenant.members.read` / `.write` | — |
| Ayarlar → Roller | `/t/{slug}/settings/roles` | `/v1/tenant/roles*` | `tenant.roles.read` / `.write` | — |
| Ayarlar → Destek erişimi | `/t/{slug}/settings/support-access` | `GET/POST /v1/tenant/support-access`, `POST …/{uuid}/revoke` | `tenant.support_access.manage` | — |
| Ayarlar → Entegrasyonlar | `/t/{slug}/settings/integrations` | `/v1/tenant/integrations*`, `/v1/tenant/integration-jobs*` | `tenant.integrations.manage` | Sağlayıcıya göre (aşağıda) |
| Ayarlar → Public API | `/t/{slug}/settings/public-api` | `/v1/tenant/public-api/*` | `tenant.public_api.read` / `.manage` | `public_api` |
| Ayarlar → Markalı site | `/t/{slug}/settings/site` | `/v1/tenant/site*` | `tenant.site.read` / `.manage` | `site` |
| Ayarlar → Site rehberi | `/t/{slug}/settings/site-articles` | `/v1/tenant/site/articles*` | `tenant.site.read` / `.manage` | `site` |
| Ayarlar → Site başvuruları | `/t/{slug}/settings/site-leads` | `/v1/tenant/site/leads*` | `tenant.site_leads.read` / `.manage` | `site` |
| Ayarlar → Fatura ayarları | `/t/{slug}/settings/invoices` | `GET/PUT /v1/tenant/invoices/settings` | `tenant.invoices.read` / `tenant.invoices.settings` | `einvoice` (yazma) |
| Ayarlar → Bildirimler | `/t/{slug}/settings/notifications` | `/v1/tenant/notification-*` | `tenant.notification_settings.read` / `.write` | SMS / WhatsApp kanalları `sms` / `whatsapp` |
| Ayarlar → WhatsApp | `/t/{slug}/settings/whatsapp` | `/v1/tenant/whatsapp/*` | `tenant.integrations.manage` | `whatsapp` |
| Ayarlar → Teslim / iade formları | `/t/{slug}/settings/inspection-templates` | `/v1/tenant/inspection-templates*` | `tenant.rentals.read` / kaydetme `tenant.rentals.close` | — |
| Ayarlar → Sözleşme şablonları | `/t/{slug}/settings/contract-templates` | `/v1/tenant/contract-templates*` | `tenant.contracts.read` / `.write`, aktifleştirme `.activate` | — |

Özellik kapalıyken menü öğesi "paket dışı" etiketiyle kalır ve sayfa okunur; değişiklik `403 FEATURE_NOT_ENABLED` alır (Fatura ayarları menüden gizlenir).

## Firma ayarları (Ayarlar → Genel)

Modül `backend/internal/modules/orgsettings`, panel `frontend/src/features/tenant-settings`. Her kayıt yeni sürüm (`organization_settings_versions`); rezervasyon satıldığı sürümü saklar. Sekmeler ve alanlar (tr etiketleri):

| Sekme | Alanlar |
|-------|---------|
| Firma | Ticari unvan, Vergi / T.C. kimlik no, Vergi dairesi, Fatura adresi, Telefon, E-posta; firma adı ve logo |
| Rezervasyon & ödeme | Rezervasyon onayı (Otomatik (ödeme ile onaylanır) / Personel onayı), Personel onay süresi (dk), Ödeme bekleme süresi (dk), Teklif geçerlilik süresi (dk), En uzun online kiralama (gün), Rezervasyon referans öneki; Ön ödeme kuralı (Ön ödeme yok / Sabit tutar / Yüzde), Tam ödemeye izin ver; Geç iade toleransı (dk), Ek saat ücreti (₺), Eksik yakıt ücreti (% başına, ₺) |
| Politikalar | İptal, Gelmeme (no-show), Erken iade, Uzatma, Yakıt, Kilometre politikası (metinler sözleşmeye ve siteye gider) |
| Satış kanalları | Web sitesi, Rezervasyon widget'ı, API, Şube, Yolcu360, obilet; kanal başına Onay protokolü (Firma onay modu / Otomatik / Personel onayı / Kanal onaylar) |
| Vergi | Varsayılan KDV oranı (%); Fatura kalemi eşlemeleri (muhasebe kodu, KDV) |
| Sürüm geçmişi | Sürümler ve karşılaştırma |

Kiralama koşulları (depozito, en düşük sürücü yaşı, ehliyet yılı, günlük km, km aşım ücreti) ürün sürümündedir: Filo → Ürünler (`tenant.fleet.read` / `.write`).

## Üyeler, roller, şube kapsamı

- Üye bir ya da birden çok rol taşır; yetkiler birleşimdir (`organization_member_roles`).
- Şablon roller (salt okunur): `owner`, `admin`, `member`, `merkez_yoneticisi`, `sube_yoneticisi`, `rezervasyon_satis`, `muhasebe`, `teslim_iade`. Firma kendi rollerini Ayarlar → Roller'de oluşturur.
- Şube kapsamı: `all` ya da seçili şubeler (`organization_member_branches`). Kapsam dışı kayıt 404, tüm şube gerektiren işlem 403.
- Davet: Ayarlar → Üyeler; davet rol ve şube taşır.
- `super_admin`'in firma içinde ayrıcalığı yok. Ayrıntı: [backend/docs/auth.md](../../backend/docs/auth.md).

## Paket ve özellikler (platform)

Ekran: Platform → Firmalar → firma → **Paket / özellikler** sekmesi. API: `GET /v1/platform/plans`, `GET|PUT /v1/platform/organizations/{uuid}/plan`, `PUT /v1/platform/organizations/{uuid}/features/{key}` (`enabled`: true / false / null = paket varsayılanı). Yetki: `platform.organizations.plan.read` / `.plan.write` (+ `platform.organizations.read` / `.write`).

Paketler: **Komple** (`complete`, varsayılan, her özellik) ve **Yönetim** (`management`, yalnız panel). Firma bazında override paketi ezer. Panel (müşteri, filo, rezervasyon, kiralama, sözleşme, finans, belgeler, e-posta bildirimleri, müşteri portalı) hiçbir pakette kapanmaz.

| Anahtar | Ad | Kapalıyken kapanan (yazma) |
|---------|----|----------------------------|
| `site` | Markalı web sitesi | Site düzenleme / yayın / alan adı / medya / makale; site host'u 404; `web` kanallı Public API anahtarları |
| `widget` | Rezervasyon widget'ı | `widget` kanallı anahtarların oturumları |
| `public_api` | Açık API | Anahtar / webhook oluşturma ve düzenleme; `api` kanallı çağrılar; webhook gönderimi |
| `channel.yolcu360`, `channel.obilet` | Kanal | Kanal hesabı bağlama, senkron, eşleme |
| `kabis` | KABİS bildirimleri | Hesap bağlama, yeniden deneme, kayıt üretimi |
| `hgs` | HGS / geçiş ücretleri | Hesap bağlama, geçiş çekme |
| `einvoice` | e-Fatura / e-Arşiv gönderimi | Fatura kesinleştirme / gönderme / yeniden deneme / iade faturası, fatura ayarları |
| `whatsapp` | WhatsApp | Oturum bağlama, test, bildirim kanalı (atlanır) |
| `sms` | SMS | Hesap bağlama, SMS rotaları, bildirim kanalı (atlanır) |

Okuma ayrımı: listeler, detaylar, dışa aktarımlar ve yalnız kapatan işlemler (anahtar iptali, hesap devre dışı, WhatsApp çıkışı, KABİS elle kapatma) her pakette açık; özellik kapanınca veri silinmez. Bilinen açıklar (paket dönemi, kuyruktaki işler): [test-ve-bilinenler.md](./test-ve-bilinenler.md#açık-maddeler).

## Sağlayıcı bağlantıları (Ayarlar → Entegrasyonlar)

API `GET /v1/tenant/integrations/meta` sağlayıcıları ve alanlarını döner; bağlama `POST /v1/tenant/integrations`, test `POST …/{uuid}/test`, devre dışı / etkin, iş listesi `GET …/{uuid}/jobs`. Kimlik bilgileri firma başına şifreli kasada; yanıtlarda yalnız maskeli ipucu. Her hesapta "Test (sandbox) modu" anahtarı (sandbox / live).

| Sağlayıcı (panel adı) | Alanlar | Özellik | Durum |
|-----------------------|---------|---------|-------|
| iyzico ödeme | API anahtarı, Güvenlik anahtarı | — (ödeme kapısı yok) | Hesap bekliyor (TEC-684) |
| Netgsm SMS | Kullanıcı kodu, Şifre, Mesaj başlığı (gönderici adı) | `sms` | Hesap bekliyor (TEC-686) |
| WhatsApp (wuzapi ağ geçidi) | Alan yok; bağladıktan sonra Ayarlar → WhatsApp'ta QR okutulur | `whatsapp` | wuzapi prod compose'da; hesap TEC-686 |
| e-Fatura entegratörü (resmi) | Kullanıcı kodu, Şifre | `einvoice` | `waiting_dependency` (TEC-685) |
| KABİS (resmi bağlantı) | Kullanıcı kodu, Şifre | `kabis` | `waiting_dependency` (TEC-682) |
| HGS | Yalnız "HGS test sağlayıcısı (geliştirme)": API anahtarı, Müşteri no | `hgs` | Sağlayıcı seçilmedi (TEC-683) |
| Yolcu360 (kanal) | Tedarikçi kodu, API anahtarı, İmza anahtarı, Sunucu adresi (isteğe bağlı) | `channel.yolcu360` | TEC-680 |
| obilet (kanal) | İş ortağı kodu, API anahtarı (token), Sunucu adresi (isteğe bağlı) | `channel.obilet` | TEC-681 |

Geliştirme / test sağlayıcıları (`kabis_fake`, `einvoice_fake`, `hgs_fake`, `yolcu360_fake`, `obilet_fake`, `whatsapp_fake`, `log`) yalnız `APP_ENV=production` dışında listelenir; KABİS, e-Fatura ve kanal fake'leri "Test senaryosu" alanı alır. Resmî KABİS / e-Fatura ve Netgsm live, `production` dışında reddedilir ([kurulum.md §9](./kurulum.md#9-ortam-ayrımı)). Env'de sağlayıcı kimlik bilgisi yok; yalnız iyzico / Netgsm taban URL'leri (`IYZICO_*_BASE_URL`, `NETGSM_*_BASE_URL`) ve wuzapi (`WUZAPI_*`).

## Bildirim motoru (Ayarlar → Bildirimler)

Sekmeler: **Kurallar** (olay başına açık / kapalı, sıralı kanallar, yedek kanal, hatırlatma dakikaları), **Şablonlar** (olay × kanal × dil, önizleme, varsayılana dön), **Gönderim geçmişi** (dispatch ve kanal denemeleri). Test gönderimi `POST /v1/tenant/notification-tests`. Satır yoksa katalog varsayılanı geçerli. Olaylar ve kanallar: [../design/notification-engine.md](../design/notification-engine.md). Paket SMS / WhatsApp'ı kapatırsa dispatch sıradaki kanala geçer.

## Destek erişimi

Ayarlar → Destek erişimi: sahip / yönetici süreli izin açar (**Salt okunur** ya da **Tam yetki**, 1 saat – 7 gün, not); yeni izin eskisinin yerine geçer; istenince iptal. İzin yoksa platform kişileştirmesi `403 SUPPORT_GRANT_REQUIRED`; salt okunur izinde GET dışı her istek `403 SUPPORT_READ_ONLY`. Platform tarafı: `GET /v1/platform/support-grants` (`platform.users.impersonate`). Kayıtlar `support_access.opened` / `.revoked`.

## Antet (platform)

Sistem Ayarları → Genel → **Marka & Antet** (`/platform/settings/general`), API `GET/PATCH /v1/platform/settings`, `PUT/DELETE /v1/platform/settings/logo`, yetki `platform.settings.read` / `.write`. PDF / XLSX dışa aktarımlarının anteti (`app_settings`). Sözleşme PDF'lerindeki firma bilgisi ise firma ayarlarından (Ticari unvan, vergi no, adres) gelir.

## Site ve tema (Ayarlar → Markalı site)

Sekmeler: Genel, Tema, Menü ve marka, Bölümler, İçerik, Görseller, Katalog, SEO ve iletişim, Widget, Alan adları, Yayın. Taslak düzenlenir, **Yayın** ile canlıya çıkar, sürümler geri yüklenebilir (`/v1/tenant/site/versions`). Katalog sekmesi şube / ürün / ek hizmetin vitrinde görünüp görünmediğini anında değiştirir. Widget sekmesi çerçeveleyebilecek alan adlarını (en çok 20, joker yok) tutar. Alan adı ve HTTPS: [kurulum.md §6](./kurulum.md#6-markalı-site-alan-adı-https). Tasarım: [../design/branded-site.md](../design/branded-site.md).
