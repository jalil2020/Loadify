# LOADIFY — REST API Reference

Base URL: `/api/v1` · Machine-readable spec: [backend/docs/openapi.yaml](../backend/docs/openapi.yaml)

---

## Conventions

**Envelope.** Every response has the same shape.

```jsonc
// success
{ "success": true, "data": { } }
// list
{ "success": true, "data": [ ], "meta": { "page": 1, "per_page": 20, "total": 134, "total_pages": 7 } }
// failure
{ "success": false, "error": { "code": "VALIDATION_ERROR", "message": "…",
  "details": [ { "field": "phone", "message": "…" } ] }, "trace_id": "…" }
```

Branch on `error.code`, never on `message` — messages are localised and may change.

| Rule | Detail |
|---|---|
| Money | Integer **minor units** (tiyin for UZS) plus an explicit `currency`. Never decimals. |
| Time | RFC 3339, UTC. |
| Coordinates | WGS 84: `{"lat": 41.2995, "lng": 69.2401}` |
| Paging | `?page=1&per_page=20` (max 100) |
| Sorting | `?sort=created_at&order=desc` or `?sort=-created_at`. Fields are allowlisted per endpoint; an unknown field silently uses the default order. |
| Language | `Accept-Language: uz \| ru \| en` |
| Auth | `Authorization: Bearer <access_token>` |

**Tokens.** Access lasts 15 minutes. `POST /auth/refresh` rotates the pair; the presented refresh token dies on use. Presenting an already-rotated one returns `TOKEN_REUSE_DETECTED` and revokes the whole family — the correct client response is to sign in again.

---

## Error codes

| Code | Status | Meaning |
|---|---|---|
| `VALIDATION_ERROR` | 422 | `details` names the failing fields |
| `UNAUTHORIZED` | 401 | No or bad token |
| `TOKEN_EXPIRED` | 401 | Refresh and retry |
| `TOKEN_REUSE_DETECTED` | 401 | Family revoked; sign in again |
| `FORBIDDEN` | 403 | Authenticated but not permitted |
| `ACCOUNT_BLOCKED` | 403 | Suspended |
| `ACCOUNT_LOCKED` | 423 | Too many failed sign-ins |
| `DRIVER_NOT_VERIFIED` | 403 | Documents not approved yet |
| `COMPANY_NOT_VERIFIED` | 403 | Company not approved yet |
| `NO_APPROVED_VEHICLE` | 403 | No usable truck |
| `VEHICLE_UNSUITABLE` | 422 | The truck does not meet the load's requirements |
| `LOAD_NOT_PUBLISHED` | 409 | Not open for reservation |
| `LOAD_ALREADY_BOOKED` | 409 | Assigned to another driver |
| `BOOKING_EXISTS` | 409 | A live booking already exists for this pair |
| `BOOKING_NOT_PENDING` | 409 | Already answered or expired |
| `TRIP_CONFLICT` | 409 | Overlapping trip |
| `INVALID_STATUS_TRANSITION` | 409 | Not allowed by the state machine |
| `QUOTA_EXCEEDED` | 402 | The plan's limit is reached |
| `INSUFFICIENT_FUNDS` | 402 | Not enough available balance |
| `SUBSCRIPTION_ACTIVE` | 409 | Already on this plan |
| `RATE_LIMITED` | 429 | Slow down |

---

## Endpoints

Legend — **Auth**: ⬜ public · 🔑 any signed-in user · 🔒 permission required.

### Authentication — `/auth`

| Method | Path | Auth | Notes |
|---|---|:-:|---|
| POST | `/auth/register` | ⬜ | Sends an SMS code. 5/10 min per phone |
| POST | `/auth/verify-otp` | ⬜ | Returns tokens for `register`/`login` |
| POST | `/auth/resend-otp` | ⬜ | 60s cooldown |
| POST | `/auth/login` | ⬜ | 5 failures → 15 min lock |
| POST | `/auth/refresh` | ⬜ | Rotates the pair |
| POST | `/auth/forgot-password` | ⬜ | Sends a reset code |
| POST | `/auth/reset-password` | ⬜ | Consumes the reset token |
| GET | `/auth/me` | 🔑 | The signed-in principal, roles and permissions |
| POST | `/auth/logout` | 🔑 | Revokes this session, denylists the access JTI |
| POST | `/auth/logout-all` | 🔑 | Revokes every session |
| POST | `/auth/change-password` | 🔑 | |
| GET | `/auth/sessions` | 🔑 | Devices currently signed in |
| DELETE | `/auth/sessions/:id` | 🔑 | Revoke one device |

### Reference — `/reference`, `/settings`

| Method | Path | Auth |
|---|---|:-:|
| GET | `/reference/countries` | ⬜ |
| GET | `/reference/regions?country=UZ` | ⬜ |
| GET | `/reference/cities?region_id=&q=` | ⬜ |
| GET | `/reference/vehicle-types` | ⬜ |
| GET | `/reference/cargo-types` | ⬜ |
| GET | `/settings` | ⬜ | Public tunables: support phone, tracking intervals |

### Files — `/files`

| Method | Path | Auth | Notes |
|---|---|:-:|---|
| POST | `/files?purpose=` | 🔑 | multipart `file`. Purposes: `avatar`, `logo`, `document`, `vehicle`, `load`, `load_document`, `trip_document`, `chat` |
| POST | `/files/batch?purpose=` | 🔑 | multipart `files`, max 12 |
| GET | `/files/:id` | 🔑 | Metadata + a 30-minute presigned URL |
| DELETE | `/files/:id` | 🔑 | Owner or admin |

Type is decided by sniffing the magic bytes, not the declared `Content-Type`. Limits: images 5–8 MB, documents 15 MB.

### Drivers — `/drivers`

| Method | Path | Auth | Notes |
|---|---|:-:|---|
| GET | `/drivers` | ⬜ | Nearby search. `?lat&lng&radius_km&vehicle_type&min_capacity_kg&online_only`. Contact details need `drivers.search` |
| GET | `/drivers/me` | 🔑 | |
| PUT | `/drivers/me` | 🔒 `drivers.update` | Create or update the profile |
| PATCH | `/drivers/me/status` | 🔒 | `online`/`offline`. `busy` is set by the trip flow |
| POST | `/drivers/me/location` | 🔒 driver | Single fix; the high-frequency path is the socket |
| GET·POST | `/drivers/me/documents` | 🔑·🔒 | Re-uploading reopens verification |
| GET·POST | `/drivers/me/vehicles` | 🔑·🔒 `vehicles.create` | |
| PUT·DELETE | `/drivers/me/vehicles/:id` | 🔒 | Editing returns the truck to review |
| POST | `/drivers/me/vehicles/:id/primary` | 🔒 | |
| GET·POST | `/drivers/me/availability` | 🔑·🔒 driver | Advertise free capacity |
| DELETE | `/drivers/me/availability/:id` | 🔒 driver | |
| GET·PUT | `/drivers/me/preferences` | 🔑·🔒 driver | Matching and push targeting |
| GET | `/drivers/me/report` | 🔑 | Earnings summary |
| GET | `/drivers/:id` | ⬜ | Public profile |
| GET | `/drivers/:id/ratings` | ⬜ | |

### Companies — `/companies`

| Method | Path | Auth | Notes |
|---|---|:-:|---|
| GET | `/companies` | ⬜ | Public directory |
| POST | `/companies` | 🔑 | Caller becomes owner and manager |
| GET·PUT | `/companies/me` | 🔒 company | |
| GET·POST | `/companies/me/documents` | 🔒 | Verification papers |
| GET·POST | `/companies/me/members` | 🔒 `companies.members` | The invitee must already have an account |
| PATCH·DELETE | `/companies/me/members/:id` | 🔒 | The owner cannot be demoted or removed |
| GET | `/companies/me/report` | 🔒 company | Fill rate, spend, cost per km |
| GET | `/companies/:id` | ⬜ | |
| GET | `/companies/:id/ratings` | ⬜ | |

### Loads — `/loads`

| Method | Path | Auth | Notes |
|---|---|:-:|---|
| GET | `/loads` | ⬜ | The marketplace. 20+ filters — see the spec |
| GET | `/loads/my` | 🔒 company | Drafts included |
| GET | `/loads/saved` | 🔑 | Bookmarks |
| POST | `/loads` | 🔒 `loads.create` | `publish: true` goes straight to the board |
| GET | `/loads/:id` | ⬜ | Counts a view |
| PUT | `/loads/:id` | 🔒 `loads.update` | Locked once a driver is assigned |
| DELETE | `/loads/:id` | 🔒 `loads.delete` | Drafts only; otherwise cancel |
| POST | `/loads/:id/publish` | 🔒 `loads.publish` | Plan limit + duplicate check |
| POST | `/loads/:id/unpublish` | 🔒 `loads.publish` | |
| POST | `/loads/:id/cancel` | 🔒 `loads.update` | Closes every open booking |
| POST·DELETE | `/loads/:id/save` | 🔑 | Bookmark |
| POST | `/loads/:id/reserve` | 🔒 `bookings.create` | Driver claims the load |
| GET·POST | `/saved-searches` | 🔑 | Stored filters that can push new matches |
| DELETE | `/saved-searches/:id` | 🔑 | |

**`?fits_my_vehicle=true`** restricts results to loads the driver's primary approved truck can legally carry — capacity, volume, body type, ADR, temperature range and every dimension.

### Bookings — `/bookings`

| Method | Path | Auth | Notes |
|---|---|:-:|---|
| GET | `/bookings` | 🔒 `bookings.read` | Your side, whichever that is |
| POST | `/bookings/offer` | 🔒 `bookings.create` | Company offers a load to a driver |
| GET | `/bookings/:id` | 🔒 | Parties only |
| POST | `/bookings/:id/respond` | 🔒 `bookings.respond` | Only the side that did **not** initiate |
| POST | `/bookings/:id/withdraw` | 🔒 `bookings.cancel` | Initiator only |
| POST | `/bookings/:id/cancel` | 🔒 `bookings.cancel` | After acceptance, before loading |

Reservations expire after 24h, company offers after 30 min. Accepting creates the trip and closes every other open booking on the load in one transaction.

### Trips — `/trips`

| Method | Path | Auth | Notes |
|---|---|:-:|---|
| GET | `/trips` | 🔒 `trips.read` | `?active_only=true` |
| GET | `/trips/:id` | 🔒 | Timeline and documents |
| GET | `/trips/:id/track` | 🔒 `trips.track` | Live position + travelled route |
| POST | `/trips/:id/advance` | 🔒 driver | The physical state machine |
| POST | `/trips/:id/confirm` | 🔒 company | Releases escrow, takes commission |
| POST | `/trips/:id/cancel` | 🔒 `trips.cancel` | Refuses once the cargo is loaded |
| POST | `/trips/:id/dispute` | 🔒 `complaints.create` | Freezes for admin resolution |
| POST | `/trips/:id/documents` | 🔒 `trips.update` | CMR, waybill, POD |
| POST | `/trips/:id/rate` | 🔑 | Once per party, after completion |

### Chat — `/chats`

| Method | Path | Auth |
|---|---|:-:|
| GET | `/chats` | 🔒 `chat.read` |
| POST | `/chats/booking/:booking_id` | 🔒 `chat.write` |
| GET | `/chats/:id` | 🔒 |
| GET | `/chats/:id/messages?before=&limit=` | 🔒 |
| POST | `/chats/:id/messages` | 🔒 `chat.write` |
| POST | `/chats/:id/read` | 🔒 |
| PATCH | `/chats/:id/mute?muted=` | 🔒 |

Paging uses a `before` timestamp cursor, not an offset, so arriving messages cannot shift a page. Send `client_msg_id` to make a retry idempotent.

### Notifications — `/notifications`

| Method | Path | Auth |
|---|---|:-:|
| GET | `/notifications?unread_only=` | 🔑 |
| GET | `/notifications/unread-count` | 🔑 |
| POST | `/notifications/read` | 🔑 |
| GET·PUT | `/notifications/preferences` | 🔑 |
| POST·DELETE | `/notifications/devices` | 🔑 |

### Payments — `/wallet`, `/subscriptions`, `/escrow`, `/payments`

| Method | Path | Auth | Notes |
|---|---|:-:|---|
| GET | `/plans?target_role=` | ⬜ | Pricing page |
| GET | `/payments/methods` | ⬜ | Configured providers |
| GET | `/subscriptions/me` | 🔑 | `null` data means the free tier |
| GET | `/subscriptions/entitlements` | 🔑 | Limits + current usage |
| POST | `/subscriptions` | 🔒 `payments.manage` | Wallet settles inline; cards return a checkout URL |
| POST | `/subscriptions/cancel` | 🔒 | Stops renewal; the period stays active |
| GET | `/wallet` | 🔒 `payments.read` | Company members see the company wallet |
| GET | `/wallet/transactions` | 🔒 | Ledger history |
| POST | `/wallet/topup` | 🔒 `payments.manage` | |
| POST | `/wallet/payout` | 🔒 `payments.manage` | Only the last 4 card digits are stored |
| POST | `/escrow/fund` | 🔒 `payments.escrow` | Secures a trip's price |
| GET | `/escrow/:trip_id` | 🔒 `payments.read` | |
| GET | `/payments/orders` | 🔒 | |
| GET | `/payments/invoices` | 🔒 | |
| POST | `/payments/refunds` | 🔒 | Requests admin review |
| POST | `/payments/webhooks/:provider` | ⬜ | Signature-verified; always 200 |

### Complaints & admin

| Method | Path | Auth |
|---|---|:-:|
| POST | `/complaints` | 🔒 `complaints.create` |
| GET | `/admin/dashboard` | 🔒 `admin.dashboard` |
| GET | `/admin/analytics/series?metric=` | 🔒 `admin.reports` |
| GET | `/admin/analytics/corridors` | 🔒 `admin.reports` |
| GET | `/admin/users`, `/admin/users/:id` | 🔒 `admin.users` |
| POST | `/admin/users/:id/block`, `/unblock` | 🔒 `admin.users` |
| POST·DELETE | `/admin/users/:id/roles` | 🔒 `admin.roles` (super admin) |
| GET | `/admin/driver-documents/pending` | 🔒 `drivers.verify` |
| POST | `/admin/driver-documents/:id/review` | 🔒 `drivers.verify` |
| GET·POST | `/admin/vehicles/pending`, `/admin/vehicles/:id/review` | 🔒 `vehicles.verify` |
| GET·POST | `/admin/company-documents/...`, `/admin/companies/:id/review` | 🔒 `companies.verify` |
| GET | `/admin/complaints`, `/admin/complaints/:id` | 🔒 `complaints.manage` |
| POST | `/admin/complaints/:id/assign`, `/resolve` | 🔒 `complaints.manage` |
| GET·POST | `/admin/refunds`, `/admin/refunds/:id/review` | 🔒 `payments.refund` |
| GET | `/admin/audit-logs` | 🔒 `admin.audit` |
| GET | `/admin/audit-logs/:resource_type/:id` | 🔒 `admin.audit` |
| GET·PUT | `/admin/settings`, `/admin/settings/:key` | 🔒 `admin.settings` |
| GET·PUT | `/admin/feature-flags`, `/admin/feature-flags/:key` | 🔒 `admin.settings` |

Blocking a user revokes every live session immediately — without that they keep working until their access token expires.

Resolving a complaint on a trip may settle the escrow in the same action: `settlement: "refund" | "release"`.

---

## WebSocket — `/api/v1/ws`

Connect with `?token=<access_token>` (browsers cannot set headers on a handshake). Server pings every 25s; the connection is closed after 60s of silence. A client that cannot keep up with its send queue is disconnected rather than allowed to stall the hub.

**Client → server**

| Type | Payload | Notes |
|---|---|---|
| `ping` | — | Answered with `pong` |
| `location.update` | `lat, lng, speed_kmh, heading, accuracy_m, battery_level, trip_id, recorded_at` | Drivers only. Back-dated points accepted from an offline queue; future timestamps rejected |
| `trip.watch` / `trip.unwatch` | `trip_id` | Authorised against trip participation |
| `chat.send` | `room_id, type, body, file_id, lat, lng, client_msg_id` | |
| `chat.read` | `room_id, message_id` | |
| `chat.typing` | `room_id` | Not persisted |

**Server → client**

`connected` · `pong` · `error` · `load.published` · `booking.created` · `booking.offered` · `booking.accepted` · `booking.rejected` · `booking.cancelled` · `booking.expired` · `trip.status_changed` · `trip.location` · `chat.message` · `chat.read` · `chat.typing` · `notification.new`

Every frame: `{ "type": "...", "payload": { }, "ts": "2026-01-01T12:00:00Z" }`

---

## Rate limits

| Scope | Limit |
|---|---|
| Global | 120 req/min per IP |
| `/auth/login`, `/refresh`, `/verify-otp`, `/reset-password` | 10 req/min |
| `/auth/register`, `/resend-otp`, `/forgot-password` | 5 per 10 min |
| File uploads | 60/hour |
| Payment webhooks | 600/min (providers retry) |
