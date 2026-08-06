# LOADIFY — Build progress

Modules are built one at a time and each is finished — schema, repository,
usecase, handler, routes, tests — before the next one starts.

| # | Module | Status | Verified by |
|---|---|---|---|
| 1 | Monorepo scaffold, env template, git | ✅ Done | directory tree, `git status` |
| 2 | PRD · user flows · ER diagram · architecture | ✅ Done | [docs/](.) |
| 3 | PostgreSQL schema — 11 migrations, 50 tables | ✅ Done | reviewed; **not yet executed** (see caveat) |
| 4 | Backend core — config, DB, Redis, MinIO, SMS, logging, errors, DI, middleware | ✅ Done | `go build ./...`, `go vet ./...` |
| 5 | Auth module — register, OTP, login, refresh rotation, RBAC, sessions, password reset | ✅ Done | `go test ./...` (all green) |
| 6 | Users · drivers · vehicles · documents · companies · dispatchers · file upload | ⬜ Next | |
| 7 | Loads · geo search · bookings · trips · ratings | ⬜ Planned | |
| 8 | WebSocket hub · live tracking · chat · FCM notifications | ⬜ Planned | |
| 9 | Wallet · escrow · commission · subscriptions · Click/Payme/Uzum | ⬜ Planned | |
| 10 | Admin panel API · reports · audit · CMS · OpenAPI spec | ⬜ Planned | |
| 11 | Next.js web — design system, auth, dashboards, maps, chat, i18n | ⬜ Planned | |
| 12 | Flutter driver app | ⬜ Planned | |
| 13 | Docker Compose · NGINX · GitHub Actions · integration tests · deployment guide | ⬜ Planned | |

## What runs today

```bash
cd backend
go build ./...      # compiles
go vet ./...        # clean
go test ./...       # passes
```

Endpoints live right now:

| Method | Path | Auth |
|---|---|---|
| GET | `/health` | public |
| GET | `/health/ready` | public |
| POST | `/api/v1/auth/register` | public |
| POST | `/api/v1/auth/verify-otp` | public |
| POST | `/api/v1/auth/resend-otp` | public |
| POST | `/api/v1/auth/login` | public |
| POST | `/api/v1/auth/refresh` | public |
| POST | `/api/v1/auth/forgot-password` | public |
| POST | `/api/v1/auth/reset-password` | public |
| GET | `/api/v1/auth/me` | bearer |
| POST | `/api/v1/auth/logout` | bearer |
| POST | `/api/v1/auth/logout-all` | bearer |
| POST | `/api/v1/auth/change-password` | bearer |
| GET | `/api/v1/auth/sessions` | bearer |
| DELETE | `/api/v1/auth/sessions/:id` | bearer |

## Verification caveat

Docker, PostgreSQL and Flutter are **not installed on this machine**. Consequences:

- The 11 migration files have been written and reviewed but **never executed against a live PostgreSQL**. They must be run once before the schema can be called verified. The first `docker compose up` (task 13) will do this.
- Repository and handler code compiles and vets clean, but the SQL it emits has not been exercised against a real database. Integration tests covering that arrive with task 13.
- Flutter code (task 12) will be written but cannot be compiled here.

Everything claimed as "Done" above was verified with the tool named in the last column — nothing is marked done on the strength of inspection alone except migration SQL, which is explicitly flagged.
