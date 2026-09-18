# Loadify — Telegram yuk e’lonlarini mas’uliyat bilan tarqatish

Logist yuk e’lonini **bir marta** oddiy matn bilan yozadi. Guruhlarga **aynan
shu matn, o‘zgarishsiz** ketadi. Tizim matnni faqat ichkarida o‘qiydi — mos
Telegram guruhlarini tanlash va takrorni tanish uchun — so‘ng anti-spam
limitlaridan o‘tkazib, **logistning o‘z Telegram akkauntidan** navbat orqali,
oralatib yuboradi. Har bir
post uchun nima bo‘lgani — yuborildi, kutmoqda, o‘tkazib yuborildi yoki xato —
sababi bilan yozib boriladi.

Tizim spam-bot emas: Telegram limitlarini aylanib o‘tmaydi, guruhlarga o‘zi
qo‘shilmaydi, faqat foydalanuvchi o‘zi ulagan akkaunt va u o‘zi tanlagan
guruhlar bilan ishlaydi. Telegram cheklov qo‘ysa, yuborish to‘xtaydi va sababi
foydalanuvchiga aniq ko‘rsatiladi.

## Tarkibi

| Qism | Papka | Nima |
|---|---|---|
| API | `backend/cmd/api` | Go (Fiber), REST `/api/v1`, JWT + RBAC |
| Poster | `backend/cmd/poster` | Navbatdagi postlarni Telegram’ga yuboradigan worker (MTProto) |
| Migratsiyalar | `backend/migrations`, `backend/cmd/migrate` | PostgreSQL + PostGIS |
| Web | `web/` | React + Vite, o‘zbek/rus tilida logist kabineti |
| Deploy | `deploy/` | Docker Compose, nginx |

Hujjatlar: [`docs/`](docs/) — arxitektura, ERD, API, Telegram va AI
strategiyasi, navbat va limitlar, xavfsizlik, MVP rejasi.
API spetsifikatsiyasi: [`backend/docs/openapi.yaml`](backend/docs/openapi.yaml).

## Tez ishga tushirish

### Docker bilan

```bash
cp .env.example .env
make up
```

`make up` Postgres, Redis, MinIO, migratsiyalar, API va poster’ni ko‘taradi.
Web ilova uchun alohida:

```bash
make web-install && make web-dev
```

`http://localhost:3000` — web ilova, `http://localhost:8080/health` — API.

### Docker’siz (Windows’da sinalgan)

Kerak: PostgreSQL 16 + PostGIS, Go 1.26, Node 24.

1. Bazani bir marta tayyorlang (postgres superuser paroli so‘raladi):
   `psql -U postgres -f deploy/local/postgres-setup.sql`
2. `.env` da `DB_PORT` ni o‘z Postgres’ingiz portiga moslang.
3. Alohida terminallarda:

```bash
make dev-redis
```

```bash
make migrate && make run
```

```bash
make run-poster
```

```bash
make web-dev
```

`dev-redis` — xotirada ishlaydigan Redis o‘rnini bosuvchi (faqat lokal
ishlab chiqish uchun, to‘xtatilsa limit hisoblagichlari nolga tushadi).

## Telegram’ni ulash

Poster Telegram’ga yuborishi uchun `.env` ga uchta qiymat kerak:

- `TELEGRAM_API_ID`, `TELEGRAM_API_HASH` — my.telegram.org’dan o‘zingiz olasiz;
- `TELEGRAM_SESSION_KEK` — sessiyalarni shifrlovchi kalit: `openssl rand -base64 32`.

Uchalasi bo‘sh bo‘lsa tizim ishlayveradi, faqat hech narsa yuborilmaydi.
Uchalasi birga berilishi shart — qisman sozlama ishga tushishda rad etiladi.

## Tekshiruvlar

```bash
make test
```

```bash
make web-check
```

Haqiqiy bazada SQL testlari: `LOADIFY_TEST_DSN="host=... user=... dbname=..." go test ./internal/repository/`.
