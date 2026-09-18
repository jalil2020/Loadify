# 8. MVP rejasi va holati

Holat: 2026-yil sentyabr, `feat/telegram-distribution` branch.

## MVP-1

| # | Imkoniyat | Holati | Qanday tekshirilgan |
|---|---|---|---|
| 1 | Ro‘yxatdan o‘tish / kirish | ✅ Telefon + parol, SMS’siz (sozlanadi) | Unit, web test, lokal API |
| 2 | E’lon yaratish | ✅ Faqat matn; guruhlarga o‘zgarishsiz ketadi | Unit, web test, lokal API (bayt-bayt taqqoslangan) |
| 3 | E’lonni AI bilan o‘qish | ✅ Qoidalar + grounded model | Unit (soxta model); **haqiqiy LLM/Hermes bilan sinalmagan** |
| 4 | Telegram guruh ulash | ✅ Kod tayyor | Unit (soxta Telegram); **haqiqiy Telegram bilan sinalmagan** |
| 5 | Guruh qoidalari | ✅ | Unit, web, lokal API |
| 6 | Guruhlarni tanlash (matching) | ✅ SQL filtr + deterministik ball; AI qayta tartiblash ixtiyoriy, qilinmagan | Unit, haqiqiy bazada SQL |
| 7 | Anti-spam guard | ✅ | Unit (Lua, miniredis) |
| 8 | Redis navbat | ✅ Streams + Postgres relay | Unit, lokal ishga tushirish |
| 9 | Telegram’ga yuborish | ✅ Kod tayyor | Unit (soxta Telegram); **haqiqiy yuborish sinalmagan** |
| 10 | Yuborish tarixi | ✅ | Unit, web, lokal API |
| 11 | Dashboard | ✅ | Unit, haqiqiy bazada SQL, web |
| 12 | Tarif limitlari | ✅ 5 metrika, lock bilan | Unit, lokal API |
| + | Telegram cheklovlari haqida ogohlantirish | ✅ | Unit |
| + | Web kabinet (uz/ru) | ✅ | Typecheck, lint, 17 ta test, CI workflow, brauzerda qisman |

Migratsiyalar haqiqiy PostgreSQL 16 + PostGIS’da up → down → up sinalgan.

## Production’dan oldin qolgan ishlar

1. **Haqiqiy Telegram sinovi.** `.env` ga `TELEGRAM_API_ID/HASH/SESSION_KEK`,
   sinov akkaunti va sinov guruhi bilan: ulash (kod, 2FA), dialoglar,
   guruh ulash, bitta post, `FLOOD_WAIT` va uzish. Bu MVP’dagi eng katta tekshirilmagan qism.
2. **Haqiqiy model sinovi** (`AI_PROVIDER=llm` yoki `hermes`): javob formati,
   narx hisobi, kesh.
3. **To‘liq Docker stack** (`make up`): konteynerlar birga ishga tushishi,
   poster healthcheck’i, nginx orqali web + API.
4. ~~**Web uchun CI**~~ — qo‘shildi: `.github/workflows/web.yml` (typecheck,
   ogohlantirishsiz lint, test, build, `web/dist` artefakti va runtime
   bog‘liqliklar uchun `npm audit`). GitHub’da hali ishga tushib ko‘rilmagan.
5. ~~**Metrikalar**~~ — qo‘shildi: API va poster’da Prometheus `/metrics`
   (ichki portlarda), navbat kechikishi, Telegram xatolari, guard, HTTP va AI;
   `deploy/prometheus/` da namuna sozlama va ogohlantirishlar. Lokal
   tekshirilgan; Prometheus serverining o‘zi hali ishga tushirib ko‘rilmagan.
6. **Tarif sotib olish oqimi** yangi limitlar bilan (to‘lov moduli bor,
   interfeysda ulanmagan).
7. **Qaror kutmoqda:** SMS’siz ro‘yxatdan o‘tish production’da qoladimi
   (raqam egaligi tasdiqlanmaydi).

## MVP-2 (keyingi bosqich)

| # | Imkoniyat | Yondashuv |
|---|---|---|
| 1 | Telegram Load Finder (`POST /telegram/search`) | Foydalanuvchi a’zo bo‘lgan va manba sifatida belgilagan guruhlardan xabarlarni o‘qish (Telegram limitlari ichida), mavjud `nlq` parser bilan yukka aylantirish, qidiruv indeksi |
| 2 | AI load matching | Topilgan yuklarni foydalanuvchi yo‘nalishlari va transportiga moslash: avval SQL, keyin model |
| 3 | Truck matching | Marketplace’dagi haydovchi/transport moduli bilan bog‘lash |
| 4 | Avtomatik bildirishnomalar | Mavjud alert mexanizmi: "yangi mos yuk", "e’lon tarqatildi" |
| 5 | AI assistant | Hermes orqali, backend’ning tor interfeyslari bilan (qaror backend’da) |
| 6 | Analitika | Guruhlar samaradorligi, vaqt bo‘yicha postlar, sabablar tahlili |
| 7 | Telegram manba monitoringi | Guruh faolligi, ban va yozish huquqi o‘zgarishi |
| 8 | Mobil ilova | API tayyor; eski Android ilova o‘chirilgan, yangisi kerak |
