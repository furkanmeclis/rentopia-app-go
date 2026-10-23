# Teslim / iade personeli rehberi (mobil)

Panel telefonda tarayıcıdan açılır: `/t/{kısa-ad}`. Etiketler panelde göründüğü gibidir. Kaynak: `frontend/src/features/{field-tasks,inspections,rentals}`, tr metinleri `frontend/src/locales/tr/`. Tasarım: [../design/inspections.md](../design/inspections.md), [../design/field-tasks.md](../design/field-tasks.md).

Ekran görüntüleri: kabul çalıştırmasının kanıt klasöründen (`frontend/e2e/evidence/`) eklenecek.

## Görev listesi

Kiralamalar → **Görevlerim** (`/t/{slug}/field`, yetki `tenant.field_tasks.execute`): size atanan görevler **Bugün** ve **Yarın** başlıkları altında; geciken görev **Gecikti** etiketli. Görev türleri: Teslim, İade, Teslimat, Araç toplama, Transfer, Temizlik, Hazırlık, Bakım, Depozito, Diğer. Durumlar: Planlandı, Atandı, Yolda, Sorun bildirildi, Tamamlandı, İptal.

Görev detayı (**Saha görevi**): yer, zaman, iletişim, uçuş no, araç, rezervasyon; **Yol tarifi**. Sıradaki adım düğmeleri:

| Düğme | Ne olur |
|-------|---------|
| **Yola çık** | Durum Yolda |
| **Teslim formuna geç** / **İade formuna geç** | Kiralamanın mobil formu açılır. Teslim / iade görevi elle tamamlanmaz; form tamamlanınca görev kapanır |
| **Sorun bildir** | Not planlayıcıya gider, görev açık kalır |
| **Tamamla** | Temizlik / hazırlık / bakım / transfer gibi formsuz görevler |
| **Depozitoyu sonuçlandır** | Depozito görevi |

"Plaka kesinleşip kiralama açılınca teslim formu kullanılabilir": plaka atanmamış rezervasyonda form açılmaz.

## Kiralamayı açma

| Yol | Nasıl |
|-----|-------|
| Görevden | Görevlerim → görev → Teslim / İade formuna geç |
| Referans / plaka ile | Kiralamalar → **Kiralamalar** listesinde arama (referans, plaka, müşteri) → kiralama → **Mobil teslim** / **Mobil iade** |
| QR | Panelde QR üretme / okutma yok (kodda bulunmadı). Formun adresi `/t/{slug}/field/handover/{kiralama}` ve `/t/{slug}/field/return/{kiralama}`; bağlantı paylaşılarak açılabilir |

## Teslim (Mobil teslim, 8 adım)

Üstte "Adım N/8" ve ilerleme çubuğu; **İleri** / **Geri**.

| # | Adım | Yapılacak |
|---|------|-----------|
| 1 | **Araç ve sürücü** | Araç: "Plaka doğru araç", "Araç genel durumu kontrol edildi". Müşteri ve sürücü: "Aracı teslim alan sürücü" seçilir, "Kimlik kontrol edildi", "Ehliyet kontrol edildi" |
| 2 | **Tutarlar** | Toplam, Ödenen, Kalan ödeme, Ödeme durumu, Depozito / provizyon, Depozito durumu (yalnız gösterim). Kalan ödemeyi aracı teslim etmeden önce tahsil edin; depozitoyu **Depozito al** ile kaydedin |
| 3 | **Km, yakıt, ekipman** | Kilometre, Yakıt seviyesi, "Ekipman ve kontrol listesi" (her madde yanıtlanmalı; "Kalanları mevcut işaretle") |
| 4 | **Hasar** | Mevcut hasarları araç şemasında işaretleyin: bölgeye dokun → tür (Çizik, Göçük, Çatlak, Kırık, Leke, Eksik parça, Diğer), Derece, açıklama → **Hasarı kaydet** → fotoğraf. Her hasar için fotoğraf zorunlu |
| 5 | **Fotoğraflar** | "Zorunlu fotoğraflar": şablondaki her slot için **Fotoğraf çek** (JPEG, PNG, WebP, HEIC; en çok 15 MB); **Yeniden çek**, ek fotoğraf |
| 6 | **Müşteri incelemesi** | Özet (araç, km, yakıt, eksik ekipman, hasar, fotoğraf, zaman); "Müşteri beyanı" (isteğe bağlı); müşteri formu inceleyince "Müşteri formu inceledi" |
| 7 | **İmza** | "Müşteri imzası": İmzalayan adı, "Buraya imzalayın", **İmzayı kaydet** (Temizle / Geri al). Sözleşme onay bekliyorsa sözleşme sürümü gösterilir; "Bu imza, gösterilen sözleşme sürümünün onayıdır" ve aynı imza sözleşmeyi de onaylar |
| 8 | **Tamamla** | Kontrol listesi: zorunlu fotoğraflar, her hasarın fotoğrafı, tüm fotoğraflar yüklendi ve doğrulandı, km / yakıt / ekipman / doğrulama / müşteri incelemesi, müşteri imzası. Eksik varsa **Tamamla** kapalı. Sonuç: "Araç teslim edildi, kiralama aktif." |

İstisnalar:

- İmzasız teslim: "İmzasız teslim gerekçesi" + imza feragati yetkisi (`tenant.signature.waive`) gerekir.
- Kiralama sözleşmesi onaysızsa: "Sözleşme onayı olmadan teslim gerekçesi".
- Tamamlanan form değiştirilemez; düzeltme yetkilinin "yeni sürüm + gerekçe" işlemiyle yapılır (`tenant.rentals.close`) ve müşteri yeniden imzalar.

## İade (Mobil iade, 8 adım)

| # | Adım | Yapılacak |
|---|------|-----------|
| 1 | **Araç ve sürücü** | Teslimdeki gibi doğrulama |
| 2 | **Km, yakıt, ekipman** | Teslimdeki değerler yanında görünür ("Teslimde: … km", "Teslimde: %…"). Km teslimdekinden düşük olamaz. Eksik ekipman: "Teslimde vardı, şimdi eksik" |
| 3 | **Hasar** | Yalnız yeni hasarları işaretleyin; teslimdeki hasarlar turuncu halkayla görünür. Yeni hasar "Yeni hasar (teslimde yoktu)"; müşteri itiraz ediyorsa "Müşteri bu bulguya itiraz ediyor" |
| 4 | **Fotoğraflar** | Aynı slotlar; "Teslimde" ve "Şimdi" yan yana |
| 5 | **Ücretler** | Koşullara göre hesaplanan satırlar: Geç iade, Kilometre aşımı, Eksik yakıt, Eksik ekipman; hasar tutarı elle. **Tutarı düzenle** + Gerekçe (`tenant.price.override`). "Müşteri ücretlere / bulgulara itiraz ediyor" + "İtiraz notu"; "İade sonrası araç" seçimi |
| 6 | **Müşteri incelemesi** | Özet, Müşteri beyanı, "Müşteri formu inceledi" |
| 7 | **İmza** | Müşteri imzası |
| 8 | **Tamamla** | Normal: "Araç iade alındı."; depozito görevi açılır, araç temizlik / bakım / hazırlığa gider |

İtiraz / imzasız yol: imza yoksa "Müşteri neden imzalamadı?" yazılır. İade yine kaydedilir, kiralama **İade kontrolü**'nde kalır, form yetkili inceleme kuyruğuna düşer ("İade yetkili incelemesinde"), tüm ücretler **İtirazlı** olarak bekler, depozito görevi bekletme notu alır. İtirazlı ücret kendiliğinden borca ya da karttan çekime dönüşmez; yetkili onayı (`tenant.damage.approve` + not) gerekir.

## Bağlantı yokken

| Durum | Davranış |
|-------|----------|
| Alan değişikliği | Gönderilemezse cihazda saklanır ("Bağlantı yok: değişiklik cihazda saklandı, bağlantı gelince gönderilecek."); üstte **Çevrimdışı** / **Gönderilmemiş değişiklik** |
| Fotoğraf | Önce cihaza (IndexedDB) yazılır, sonra yüklenir. **Yükleme kuyruğu** durumları: Cihazda, bağlantı bekleniyor → Yükleniyor → Sunucuda doğrulanıyor → Yüklendi ve doğrulandı / Başarısız (**Yeniden dene**). Sayfa yenilenince ve bağlantı gelince kuyruk devam eder; aynı fotoğraf iki kez kaydedilmez |
| Adım | Cihaz formun adımını hatırlar |
| Tamamla | Sunucu gerekir; kuyrukta fotoğraf varken **Tamamla** kapalı |
| Gizli sekme / engelli depolama | Form çalışır ama cihazda saklama olmaz: sayfayı yenilemeyin |

## Saha görevleri (teslimat, toplama, transfer)

- Teslimat / toplama: adresli ya da hizmet noktalı (havalimanı vb.) teslim / iade görevidir; form aynıdır. İletişim, uçuş no ve buluşma notu görev detayında.
- Transfer: **Tamamla** aracın şubesini hedef şube yapar ("Tamamlanınca aracın bulunduğu şube hedef şube olur.").
- Temizlik / hazırlık / bakım: **Tamamla** aracı müsait yapar.
- Sorun varsa **Sorun bildir**; planlayıcı Görev panosunda görür.
