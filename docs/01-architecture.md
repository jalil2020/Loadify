# 1. Arxitektura

## Jarayonlar

```mermaid
flowchart LR
    subgraph Client
        WEB[Web ilova<br/>React + Vite]
    end

    subgraph Edge
        NGINX[nginx<br/>TLS, rate limit,<br/>web/dist]
    end

    subgraph App["Go ilovalar"]
        API[API<br/>cmd/api]
        POSTER[Poster worker<br/>cmd/poster]
    end

    subgraph Data
        PG[(PostgreSQL + PostGIS<br/>yagona haqiqat manbai)]
        RD[(Redis<br/>navbat, limitlar,<br/>lock, kesh)]
        S3[(MinIO / S3<br/>fayllar)]
    end

    subgraph External
        TG[Telegram MTProto]
        LLM[LLM yoki Hermes<br/>ixtiyoriy]
        FCM[FCM push]
    end

    WEB -->|/api/v1| NGINX --> API
    API --> PG
    API --> RD
    API --> S3
    API -->|faqat ulash: kod, parol,<br/>dialoglar| TG
    API -->|e’lonni o‘qish,<br/>faqat kerak bo‘lsa| LLM
    POSTER --> PG
    POSTER --> RD
    POSTER -->|messages.sendMessage| TG
    POSTER -->|ogohlantirishlar| FCM
    RD -.->|pub/sub realtime| API
```

| Jarayon | Vazifasi | Nima uchun alohida |
|---|---|---|
| **API** | Auth, e’lonlar, preview va publish, Telegram akkaunt/guruh ulash, dashboard, tarif | HTTP trafik bilan masshtablanadi |
| **Poster** | Postgres’dagi navbatni Redis Streams’ga o‘tkazadi (relay) va Telegram’ga yuboradi (sender) | Telegram sessiyalari faqat shu yerda ochiladi; API deploy qilinsa yuborish uzilmaydi; akkauntlar soni bilan masshtablanadi |
| **PostgreSQL** | E’lonlar, joblar, tarix, akkauntlar, guruhlar, tarif, AI hisobi | Har qanday qaror shu yerga yoziladi, Redis yo‘qolsa ham holat tiklanadi |
| **Redis** | Anti-spam hisoblagichlari (Lua), job navbati (Streams), preview tokeni, lock, kesh, realtime pub/sub | Qisqa umrli arifmetika; Postgres o‘rnini bosmaydi |

## Kod qatlamlari (clean architecture)

```
backend/internal/
  domain/          ← entity, interface (port), biznes qoidalari; tashqi paketsiz
  usecase/         ← stsenariylar: LoadAd, Publish, Telegram, Quota, Dashboard, Alerts
  nlq/             ← e’lon matnini qoidalar bilan o‘qish (tokenizator, joylar, raqamlar)
  ai/              ← model ustidagi "grounded" o‘qish va hisob-kitob (Metered)
  matching/        ← guruhlarni deterministik ballash
  worker/          ← relay va sender (poster jarayoni)
  repository/      ← GORM + raw SQL, har bir so‘rov company_id bilan
  infrastructure/  ← redis, telegram (gotd/td), crypto, queue, ratelimit, storage, push
  delivery/http/   ← handler, route, middleware, dto
  bootstrap/       ← DI: container.go (API), poster.go (worker)
```

Bog‘liqlik yo‘nalishi faqat ichkariga: `delivery → usecase → domain ← repository/infrastructure`.
Usecase’lar interfeyslarni (`domain.PostingGuard`, `domain.TelegramSender`,
`domain.QuotaGuard`…) oladi, shuning uchun har biri soxta (fake) implementatsiya
bilan unit-test qilinadi.

## Asosiy oqim: e’londan postgacha

```mermaid
sequenceDiagram
    actor L as Logist
    participant W as Web
    participant A as API
    participant P as Postgres
    participant R as Redis
    participant PO as Poster
    participant T as Telegram

    L->>W: e’lon matni
    W->>A: POST /load-ads/parse
    A-->>W: maydonlar + qaysi so‘zdan olingani
    L->>W: tekshiradi, saqlaydi
    W->>A: POST /load-ads
    A->>P: load_ads (fingerprint, status)
    L->>W: "Tarqatish"
    W->>A: POST /load-ads/{id}/publish/preview
    A->>P: SQL filtr (yo‘nalish, kuzov, yuk, og‘irlik)
    A->>A: matching: ball va sabab
    A->>R: guard.Plan (hisoblagichlarni faqat o‘qiydi)
    A->>R: preview token (10 daqiqa, bir martalik)
    A-->>W: yuboriladi / yuborilmaydi (sababi bilan)
    L->>W: tasdiqlaydi
    W->>A: POST /publish (Idempotency-Key)
    A->>P: tarif limiti (lock bilan) + publish_run + posting_jobs (pending)
    A-->>W: 202 Accepted
    loop har soniya
        PO->>P: muddati kelgan joblar (FOR UPDATE SKIP LOCKED)
        PO->>R: XADD posting:jobs
    end
    PO->>R: XREADGROUP
    PO->>P: claim (lease)
    PO->>R: guard.Reserve (Lua, atomik)
    PO->>T: sendMessage (random_id)
    T-->>PO: natija yoki xato
    PO->>P: holat + posting_history + spam_events
```

## Muhim tamoyillar

- **Postgres — haqiqat manbai.** Redis’dagi navbat yo‘qolsa, relay muddati
  kelgan joblarni bazadan qayta chiqaradi. Job holati faqat Postgres’da.
- **Qaror va yuborish ajratilgan.** API hech qachon Telegram’ga post
  yubormaydi: u nimani yuborishni yozadi, poster esa limitlar ichida yuboradi.
- **Idempotentlik uch qatlamda:** publish uchun `Idempotency-Key`; bitta
  e’lon bitta guruh navbatida faqat bir marta (unikal indeks); Telegram
  `random_id` — qayta urinishda xabar ikki marta chiqmaydi.
- **Model ixtiyoriy.** AI bo‘lmasa ham tizim to‘liq ishlaydi
  ([05-hermes-ai.md](05-hermes-ai.md)).
- **Graceful shutdown.** Poster to‘xtatilganda yo‘ldagi yuborish tugaydi va
  yoziladi; tugamagani lease muddati o‘tgach "natija noma’lum" deb belgilanadi
  va avtomatik qayta yuborilmaydi (dublikat spam hisoblanadi).
