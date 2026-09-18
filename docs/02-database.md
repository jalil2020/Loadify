# 2. Ma’lumotlar bazasi (ERD)

PostgreSQL 16 + PostGIS. Migratsiyalar `backend/migrations`, har biri `up`/`down`
jufti bilan; 000002–000014 ortga qaytarilib qayta qo‘llanishi sinalgan.

Tarqatish moduli migratsiyalari:

| Migratsiya | Nima qo‘shadi |
|---|---|
| `000012_telegram_distribution` | Telegram akkauntlar, guruhlar, qoidalar, yo‘nalishlar, e’lonlar, publish run, joblar, tarix, spam hodisalari, AI vazifalar, ruxsatlar |
| `000013_cis_geography` | MDH va qo‘shni davlatlar viloyat/shaharlari (RU, KZ, KG, TJ, TM, AF, TR, CN) |
| `000014_distribution_quotas` | Kompaniya tariflariga tarqatish limitlari |

## ERD

```mermaid
erDiagram
    companies ||--o{ company_members : "a’zolar"
    users ||--o{ company_members : ""
    companies ||--o{ telegram_accounts : ""
    users ||--o{ telegram_accounts : "egasi (rozilik bergan)"
    telegram_accounts ||--o{ telegram_groups : "a’zo bo‘lgan guruhlar"
    telegram_groups ||--|| telegram_group_rules : "qoidalar"
    telegram_groups ||--o{ telegram_group_routes : "yo‘nalishlar"
    regions ||--o{ telegram_group_routes : ""
    companies ||--o{ load_ads : ""
    users ||--o{ load_ads : "yaratgan"
    cities ||--o{ load_ads : "qayerdan/qayerga"
    cargo_types ||--o{ load_ads : ""
    load_ads ||--o{ publish_runs : "har bir 'Tarqatish'"
    telegram_accounts ||--o{ publish_runs : "qaysi akkauntdan"
    publish_runs ||--o{ posting_jobs : ""
    telegram_groups ||--o{ posting_jobs : ""
    posting_jobs ||--o{ posting_history : "har bir o‘tish"
    posting_jobs ||--o{ spam_events : "guard sabablari"
    companies ||--o{ ai_tasks : "model chaqiruvlari"
    companies ||--o{ subscriptions : ""
    subscription_plans ||--o{ subscriptions : "limits JSONB"

    telegram_accounts {
        uuid id PK
        uuid company_id FK
        uuid user_id FK
        bigint tg_user_id
        varchar phone
        bytea session_ciphertext "AES-256-GCM"
        smallint session_key_version
        varchar status "connecting|active|paused|restricted|revoked|disabled"
        timestamptz paused_until
        varchar consent_version
        timestamptz consent_at
    }
    telegram_groups {
        uuid id PK
        uuid account_id FK
        bigint tg_peer_id "-100... ko‘rinishida"
        bigint access_hash
        varchar title
        varchar language "uz|ru|en"
        int slowmode_seconds
        bool can_send
        bool posting_enabled
        bool active
    }
    telegram_group_rules {
        uuid group_id PK
        text_arr vehicle_types
        text_arr cargo_type_codes
        int min_weight_kg
        int max_weight_kg
        smallint hourly_limit
        smallint daily_limit
        int cooldown_minutes
        smallint quiet_from_hour
        smallint quiet_to_hour
    }
    load_ads {
        uuid id PK
        uuid company_id FK
        text raw_text
        char origin_country
        uuid origin_region_id
        uuid origin_city_id
        int weight_kg
        varchar vehicle_type
        date loading_date
        bigint price_amount "minor units"
        char price_currency
        varchar contact_phone
        jsonb field_sources "qiymat qaysi so‘zdan"
        char fingerprint "ma’no bo‘yicha SHA-256"
        varchar status "draft|ready|publishing|published|archived"
    }
    posting_jobs {
        uuid id PK
        uuid run_id FK
        uuid load_ad_id FK
        uuid account_id FK
        uuid group_id FK
        text message_text
        varchar status "pending|processing|sent|failed|skipped|rate_limited|cancelled"
        varchar skip_reason
        bigint random_id "Telegram dublikatdan himoya"
        smallint attempt
        timestamptz next_attempt_at
        timestamptz lease_until
        bigint tg_message_id
        varchar error_code
    }
    ai_tasks {
        uuid id PK
        uuid company_id FK
        varchar kind
        char input_hash
        bool cache_hit
        varchar status "ok|rejected|failed"
        int tokens_in
        int tokens_out
        bigint cost_micros
    }
```

## Muhim qoidalar bazada

- **Tenant izolyatsiyasi kompozit FK bilan.** `telegram_groups`, `publish_runs`,
  `posting_jobs` va boshqalar `(id, company_id)` juftligiga bog‘lanadi: boshqa
  kompaniyaning akkaunti yoki guruhiga job yaratish bazaning o‘zida mumkin emas.
- **Rozilik va sessiya.** `status = 'active'` bo‘lishi uchun
  `session_ciphertext` va `consent_at` majburiy (CHECK). Shifrlangan sessiya
  kalit versiyasisiz saqlanmaydi.
- **Bitta e’lon — bitta guruh navbatida bir marta:** `posting_jobs (load_ad_id, group_id)`
  bo‘yicha `pending/processing/rate_limited` holatlar uchun unikal indeks.
- **`random_id` akkaunt bo‘yicha unikal** — qayta urinish xabarni ikki marta chiqarmaydi.
- **Tarix faqat qo‘shiladi.** `posting_history` har bir holat o‘tishini
  yozadi; Telegram javobi access_hash va telefon raqamlarisiz saqlanadi.
- **Foydalanish yozuvlarning o‘zidan sanaladi.** Alohida hisoblagich jadvali
  yo‘q: e’lonlar, joblar, akkauntlar, guruhlar va `ai_tasks` bo‘yicha oylik
  so‘rov ([06-queue-rate-limits.md](06-queue-rate-limits.md)).
