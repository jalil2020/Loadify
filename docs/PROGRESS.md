# LOADIFY — Build progress

Modules are built one at a time and each is finished — schema, repository,
usecase, handler, routes, tests — before the next one starts.

Updated: 2026-07-31

---

## Status

| # | Module | Status |
|---|---|---|
| 1 | Monorepo scaffold, env template, git | ✅ Done |
| 2 | PRD · user flows · ER diagram · architecture | ✅ Done |
| 3 | PostgreSQL schema — 11 migrations, 50 tables | ✅ Done |
| 4 | Backend core — config, DB, Redis, MinIO, SMS, logging, errors, DI, middleware | ✅ Done |
| 5 | Auth — register, OTP, login, refresh rotation, RBAC, sessions, password reset | ✅ Done |
| 6 | Drivers, vehicles, documents, companies, members, files, reference data | ✅ Done |
| 7 | Loads, bookings, trips, ratings — the marketplace core | ✅ Done |
| 8 | Realtime hub, chat, notifications, FCM push | ✅ Done |
| 9 | Payments, double-entry wallet, escrow, subscriptions | ✅ Done |
| 10 | Admin, moderation, analytics, audit trail, API docs | ✅ Done |
| 11 | Docker, NGINX, CI/CD, deployment guide | ✅ Done |
| 12 | **Next.js web frontend** | ⬜ Not started |
| 13 | **Flutter mobile app** | ⬜ Not started |

**The backend is feature-complete for Phase 1 and Phase 2 of the roadmap.**

---

## What exists

```
backend/          ~13,000 lines of Go
  cmd/            api · migrate · seed
  internal/
    domain/       entities, state machines, ports (imports no framework)
    usecase/      business rules — 11 usecases
    repository/   GORM adapters — 16 repositories
    delivery/     149 HTTP routes + the WebSocket hub
    infrastructure/ postgres · redis · minio · sms · fcm · click/payme/uzum
  migrations/     11 up/down pairs
  docs/           openapi.yaml
deploy/           docker-compose · nginx
docs/             PRD · user flows · ER diagram · architecture · API · deployment
.github/          CI (lint · test · security · image) + deploy
```

### Verified locally

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./...` — every package passes

### Not verified locally

Docker and PostgreSQL are not installed on this machine, so the following is
written but has **never been executed**:

- The 11 SQL migrations have not run against a real PostGIS database.
- Repository SQL is unexercised — the tests cover pure domain logic only.
- The Docker Compose stack has not been brought up.

`make up` on a machine with Docker closes that gap. Expect to fix a handful of
SQL details on the first run; that is normal for a schema this size, and the CI
job already runs migrations up, down and up again against real PostGIS.

---

## Test coverage

Tests cover the logic where a mistake is expensive and a unit test can actually
catch it.

| Area | What is asserted |
|---|---|
| Vehicle ↔ load matching | Capacity, volume, body type, ADR, refrigeration, temperature range, every dimension |
| Load & trip state machines | Happy paths, terminal states, forbidden skips, the cancel-after-loading refusal, driver permissions |
| Booking statuses | Live/open predicates, and that `LiveBookingStatuses` matches `IsLive()` — the schema's partial unique index is built from that slice, so drift would silently allow double bookings |
| Realtime routing | User, company, trip-watcher and broadcast fan-out; deduplication; a failed member lookup targets nobody rather than everybody |
| Payment signatures | Click md5 over the exact signed field order, tampered-field rejection, prepare↔complete signature separation; Payme constant-time Basic auth and protocol error codes |
| Money arithmetic | Held balance excluded from available, payee + commission = escrow amount, plan limits default to unlimited |
| PostgreSQL array codec | Round trips including commas, quotes, backslashes and a literal `"NULL"` |
| JWT, hashing, pagination, geo, validation | Round trips and edge cases |

---

## Design decisions worth knowing

**Concurrency is handled in the database, not in Go.** Every status change is a
guarded `UPDATE` whose `WHERE` names the expected current status, so a
double-tapped button affects zero rows instead of overwriting newer state.
Booking uniqueness is a partial unique index; ledger idempotency is a unique
constraint. The Go checks are for good error messages — the database is what
makes it correct.

**The ledger is the truth; balances are a cache.** Every movement writes a debit
and a credit sharing a group id. `Reconcile` recomputes a balance from the ledger
and reports the drift, which turns an accounting bug into a number somebody can
alert on rather than quietly wrong money.

**Vehicle compatibility is one rule expressed twice.** `Vehicle.CanCarry` in Go
is the authority and gates every reservation. The SQL predicate in the load
repository mirrors it so filtering, paging and totals stay correct — filtering in
Go would break both. The duplication is deliberate, commented, and constrained:
the SQL may only ever be more permissive.

**Contact details are earned, not given.** A guest browses the board with phone
numbers hidden. The shipper's number appears once a live booking exists, or for
company members and admins. Without that the marketplace is a scraping target.

**Cross-module dependencies go through one-method ports.** The trip flow settles
escrow through `EscrowSettler`; the load flow checks plan limits through
`QuotaChecker`. Neither imports the payment usecase, and a deployment without
payments simply leaves them nil.

**Blocking a user revokes their sessions.** Without that they keep working until
their access token expires — up to 15 minutes of a blocked account still trading.

---

## Clients

### Next.js web frontend — built
Nine App Router routes over the live API: board, bookings, trips, my-loads,
drivers, wallet, admin dashboard, login. uz/ru/en. Board, bookings and trips are
single components that invert by role rather than duplicated per-role pages, and
the UI gates on permissions rather than role names.

Verified end to end against the running stack: browse, filter, reserve, accept,
run the trip machine, confirm delivery — with contact masking and 5% commission
settling as designed.

### Flutter driver app — built
Four tabs: board with filters and "only loads my vehicle fits", bookings from
both sides, trips with the seven-stage driver state machine, profile with
availability, vehicles and wallet. uz/ru/en. Clean architecture per feature —
domain entities import no framework; composition happens only in
`app/providers.dart`.

Foreground location reporting runs only while a trip is actually moving.
`flutter analyze` is clean under strict-casts and strict-inference; 24 tests
cover refresh behaviour, auth parsing, the trip machine, booking permissions and
filter semantics.

Not yet wired: FCM push, chat, in-app map rendering, registration/OTP.

### What building the clients taught the backend

**A wrong contract is worse than no contract.** The OpenAPI document described
verify-otp, login and refresh as returning a bare `TokenPair`; all three actually
return `{user, tokens}`. The web client faithfully implemented the document, so
every session died silently fifteen minutes after sign-in — refresh returned 200,
stored `undefined`, and every later request went out unauthenticated. The
document is now correct and `cmd/speccheck` fails CI on a `$ref` that does not
resolve. Both clients now parse the payload strictly, with regression tests.

**`Scan` does not populate associations.** The load list returned every row with
no pickup, no delivery and no shipper — the three things a board exists to show.
Fixed with a batched hydrate.

### Backend follow-ups
- Run the migrations against real PostGIS and fix what surfaces
- Integration tests against a live database (the CI job is already configured)
- A scheduled-job runner for the maintenance tasks in [DEPLOYMENT.md](DEPLOYMENT.md#scheduled-maintenance)
- AI matching and price prediction — the seam exists, the rules-based scorer does not
