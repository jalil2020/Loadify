# 3. API spetsifikatsiyasi

To‘liq spetsifikatsiya: [`backend/docs/openapi.yaml`](../backend/docs/openapi.yaml)
(OpenAPI 3.0.3; `go run ./cmd/speccheck docs/openapi.yaml` barcha `$ref`
havolalarini tekshiradi). Bu yerda — qoidalar va asosiy endpointlar xaritasi.

## Qoidalar

- **Base URL:** `/api/v1`. Autentifikatsiya: `Authorization: Bearer <access_token>`
  (15 daqiqa), yangilash — `POST /auth/refresh` (refresh token har safar
  almashadi; eski token qayta kelsa butun sessiya oilasi bekor qilinadi).
- **Konvert:** `{"success": true, "data": …, "meta": …}` yoki
  `{"success": false, "error": {"code", "message", "details"}}`.
- **Pul** — minor birlikda butun son (`250000` USD = 2 500 USD) va alohida valyuta.
- **Til:** `Accept-Language: uz | ru | en`.
- **Xato kodlari** barqaror: `VALIDATION_ERROR` (422, `details` bilan),
  `QUOTA_EXCEEDED` (402), `RATE_LIMITED` (429), `CONFLICT` (409),
  `FORBIDDEN` (403), `SERVICE_UNAVAILABLE` (503) va boshqalar.
- **Idempotentlik:** `POST /load-ads/{id}/publish` uchun `Idempotency-Key`
  sarlavhasi majburiy; bir xil kalit bilan qayta so‘rov o‘sha run’ni qaytaradi.

## Spec’dagi minimal endpointlar → amalga oshirilgani

| Spec | Amalda | Izoh |
|---|---|---|
| `POST /api/auth/register` | `POST /auth/register` | Telefon (login) + parol; standart holatda SMS’siz darhol sessiya |
| `POST /api/auth/login` | `POST /auth/login` | |
| `POST /api/loads` | `POST /load-ads` | Odatda faqat `raw_text`: server uni o‘qiydi (guruh tanlash uchun) va guruhlarga aynan shu matnni yuboradi. `POST /load-ads/parse` — o‘qishni saqlamasdan ko‘rish (API mijozlari uchun) |
| `GET /api/loads` | `GET /load-ads` | `status`, `search`, `mine`, sahifalash |
| `GET /api/loads/:id` | `GET /load-ads/{id}` | Matnli e’londa `missing` doim bo‘sh; faqat maydonlar bilan (matnsiz) yaratilganda yo‘nalish va telefon talab qilinadi |
| `DELETE /api/loads/:id` | `DELETE /load-ads/{id}` | Arxivlaydi va navbatdagi postlarni bekor qiladi |
| `POST /api/loads/:id/publish` | `POST /load-ads/{id}/publish/preview` → `POST /load-ads/{id}/publish` | Ikki qadam: ko‘rib chiqish (hech narsa yubormaydi) va tasdiqlash (202) |
| `GET /api/telegram/groups` | `GET /telegram/groups` | Qoidalar va yo‘nalishlar bilan |
| `POST /api/telegram/groups` | `POST /telegram/groups` | Faqat akkaunt a’zo bo‘lgan guruhdan |
| `DELETE /api/telegram/groups/:id` | `DELETE /telegram/groups/{id}` | |
| `POST /api/telegram/search` | — | MVP-2 (Telegram Load Finder), [08-mvp-plan.md](08-mvp-plan.md) |
| `GET /api/posting-history` | `GET /posting-history` | `status`, `load_ad_id`, `group_id`, `mine` |
| `GET /api/dashboard` | `GET /dashboard` | `scope=company|mine` |
| `GET /api/subscription` | `GET /subscriptions/me`, `GET /subscriptions/usage` | Tarif va oylik foydalanish. Pullik obuna bo‘lmasa `{"subscription": null}` — bu xato emas, bepul tarif |
| — | `GET /plans`, `GET /payments/methods` | Tariflar (`limits` da kvota metrikalari) va to‘lov usullari |
| — | `POST /subscriptions`, `POST /subscriptions/cancel` | Sotib olish (provayder sahifasiga `checkout_url`) va avto-yangilashni o‘chirish |

`/load-ads` va `/loads` alohida: `/loads` — marketplace’dagi yuk (bron, reys),
`/load-ads` — Telegram guruhlariga tarqatiladigan e’lon. Ularning shakli va
qoidalari har xil.

## Qo‘shimcha endpointlar

| Endpoint | Vazifa |
|---|---|
| `GET /load-ads/{id}/posts` | E’lonning har bir guruhdagi posti va holati |
| `POST /posting-jobs/{id}/cancel` | Navbatdagi postni to‘xtatish |
| `GET/POST /telegram/accounts…` | O‘z akkauntini ulash: `login` → `{id}/code` → `{id}/password`; `DELETE /telegram/accounts/{id}` uzadi va Telegram’dagi sessiyani yopadi |
| `GET /telegram/accounts/{id}/dialogs` | Akkaunt a’zo bo‘lgan guruhlar (yozish mumkinligi bilan) |
| `PUT /telegram/groups/{id}/rules` | Guruh qoidalari: limitlar, jim soatlar, kuzov/yuk, og‘irlik, yo‘nalishlar |
| `GET /notifications…` | Ichki bildirishnomalar (Telegram ogohlantirishlari shu yerga tushadi) |

## Ruxsatlar (RBAC)

| Ruxsat | Kim | Nima uchun |
|---|---|---|
| `load_ads.create/view/delete/publish` | company_manager, dispatcher | E’lonlar va tarqatish |
| `telegram.connect` | company_manager, dispatcher | O‘z akkauntini ulash |
| `telegram.groups.manage` | company_manager, dispatcher | Guruhlar va qoidalar |
| `posting.view`, `dashboard.view` | company_manager, dispatcher | Tarix va dashboard |

Kim qaysi akkauntdan yubora olishi ruxsat bilan emas, **egalik** bilan
aniqlanadi: akkauntdan faqat uni ulagan odam yubora oladi va uza oladi.
Menejer hamkasblarining akkauntlarini ko‘radi, lekin ular nomidan yubora olmaydi.
