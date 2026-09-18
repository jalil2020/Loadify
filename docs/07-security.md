# 7. Xavfsizlik modeli

## Kimdan himoyalanamiz

| Tahdid | Himoya |
|---|---|
| Boshqa kompaniya ma’lumotini ko‘rish yoki o‘zgartirish | Har bir repository so‘rovi `company_id` bilan; bazada kompozit FK `(id, company_id)` |
| Hamkasb akkauntidan uning nomidan post yuborish | Akkauntdan faqat uning egasi (`user_id`) yubora oladi va uza oladi — ruxsat emas, egalik tekshiriladi |
| Baza yoki backup sizib chiqishi | Telegram sessiyalari AES-256-GCM bilan shifrlangan; kalit bazada emas |
| Loglarda maxfiy ma’lumot | `secret.Value` tipi o‘zini `[REDACTED]` qilib chiqaradi; telefonlar niqoblanadi (`+99890 *** ** 67`); Telegram javobi access_hash va telefonsiz saqlanadi |
| Parol yoki kodni taxmin qilish | Login lockout (5 urinish → 15 daqiqa), rate limit, Telegram kodi uchun 5 urinish |
| Token o‘g‘irlanishi | Qisqa access token (15 daqiqa), refresh token rotatsiyasi, qayta ishlatilgan eski token butun sessiya oilasini bekor qiladi, logout’da access token denylist’ga tushadi |
| Spam va Telegram bloki | Anti-spam guard, tarif limitlari, akkaunt pauzasi va to‘xtatilishi ([06](06-queue-rate-limits.md)) |
| Model xarajati | Tarif limiti, kunlik byudjet, kesh ([05](05-hermes-ai.md)) |

## Telegram sessiyalarini shifrlash

- **Envelope encryption:** har bir sessiya o‘z tasodifiy data-key’i bilan
  AES-256-GCM’da shifrlanadi; data-key esa KEK bilan o‘raladi.
- **AAD** yozuvga bog‘langan (`telegram_account:{id}:{company}`): bir
  akkauntning shifrlangan sessiyasini boshqa qatorga ko‘chirsa ochilmaydi.
- **KEK** faqat muhit o‘zgaruvchisida (`TELEGRAM_SESSION_KEK`), bazada emas.
  Rotatsiya: yangi kalit `TELEGRAM_SESSION_KEK` ga, eskisi
  `TELEGRAM_SESSION_KEK_PREVIOUS` ga (versiyasi bilan) — eski sessiyalar
  ochilishda davom etadi.
- Sessiya faqat poster (va ulash vaqtida API) xotirasida, yuborish davomida
  ochiladi va keyin baytlari nollanadi.
- Ikki bosqichli parol faqat Telegram’ning SRP almashinuvi uchun bir marta
  ishlatiladi, hech qayerda saqlanmaydi va loglanmaydi.

## Autentifikatsiya va avtorizatsiya

- JWT (HS256), `JWT_ACCESS_SECRET` va `JWT_REFRESH_SECRET` kamida 32 belgi va
  bir-biridan farqli — aks holda ilova ishga tushmaydi.
- Parollar bcrypt (cost ≥ 10, standart 12).
- RBAC: rol → ruxsat kodlari; middleware `RequirePermission`, `RequireCompany`.
- **Ro‘yxatdan o‘tish SMS’siz** (standart). Oqibati: raqam egasi tasdiqlanmaydi
  (`phone_verified=false`). Telegram akkaunt ulashda Telegram o‘z kodini
  so‘raydi, shuning uchun postlar baribir haqiqiy egasining akkauntidan
  chiqadi. Kerak bo‘lsa `AUTH_REQUIRE_PHONE_VERIFICATION=true` SMS’ni qaytaradi.

## Tarmoq va kirish

- HTTPS nginx’da (TLS sertifikatlari `deploy/nginx/certs`), HTTP → HTTPS.
- Rate limit ikki qatlamda: nginx (IP bo‘yicha) va ilova (foydalanuvchi yoki
  IP bo‘yicha, endpoint guruhlari uchun alohida: auth, e’lon o‘qish, Telegram ulash).
- CORS ruxsat etilgan manbalar ro‘yxati bilan; web ilova production’da API
  bilan bir domenda turadi, shuning uchun CORS kerak emas.
- Kirish ma’lumotlari: `go-playground/validator` va DTO chegaralari; SQL faqat
  parametrlangan (GORM yoki nomlangan parametrlar, qo‘lda satr yig‘ilmaydi).

## Audit va kuzatuv

- `audit_logs`: akkaunt ulash/uzish, guruh ulash, qoidalarni o‘zgartirish,
  e’lon yaratish/arxivlash, publish, post bekor qilish.
- `posting_history`: har bir post uchun kim, qaysi e’lon, qaysi guruh, qachon,
  holat, xato, urinish va Telegram javobi.
- Strukturali loglar (zap), har bir so‘rovda `request_id`.
- Telegram cheklovlari haqida egasi, menejerlar va platforma adminlariga
  avtomatik bildirishnoma.

## Maxfiy qiymatlar

Kodda hech qanday sir yo‘q: hammasi `.env` orqali. `.env` git’ga kirmaydi
(`.gitignore`). `.env.example` faqat bo‘sh yoki xavfsiz lokal qiymatlarni saqlaydi.
