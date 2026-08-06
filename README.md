# LOADIFY

Freight marketplace connecting logistics companies and truck drivers in real time.
First market: Uzbekistan. Roadmap: Central Asia → Europe → USA.

## Monorepo layout

```
Loadify/
├── backend/      Go 1.26 + Fiber v2 REST API + WebSocket (Clean Architecture)
├── frontend/     Next.js 15 + React 19 + TypeScript + Tailwind + shadcn/ui
├── android/      Flutter mobile app (driver + company)
├── deploy/       Docker Compose, NGINX, Postgres init
├── docs/         PRD, user flows, ER diagram, API docs, deployment guide
└── .github/      CI/CD workflows
```

## Stack

| Layer | Technology |
|---|---|
| Web | React 19, Next.js 15, TypeScript, TailwindCSS, shadcn/ui, TanStack Query, React Hook Form + Zod |
| Mobile | Flutter, Riverpod, Dio, Google Maps, FCM |
| API | Go 1.26, Fiber v2, Clean Architecture, Repository pattern, DI |
| Database | PostgreSQL 16 + PostGIS, GORM |
| Cache / Pub-Sub | Redis 7 |
| Auth | JWT access + refresh rotation, RBAC |
| Storage | MinIO (S3-compatible) |
| Realtime | WebSocket hub + Redis pub/sub |
| Push | Firebase Cloud Messaging |
| Payments | Click, Payme, Uzum Bank (Stripe later) |
| Maps | Google Maps Platform |
| Infra | Docker, Docker Compose, NGINX, DigitalOcean, GitHub Actions |

## Quick start

```bash
cp .env.example .env
docker compose -f deploy/docker-compose.yml up -d
```

Backend only (local Go):

```bash
cd backend && go run ./cmd/api
```

## Documentation

| Document | Path |
|---|---|
| Product Requirements | [docs/PRD.md](docs/PRD.md) |
| User Flows | [docs/USER_FLOWS.md](docs/USER_FLOWS.md) |
| ER Diagram | [docs/ER_DIAGRAM.md](docs/ER_DIAGRAM.md) |
| Architecture | [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) |
| REST API | [docs/API.md](docs/API.md) |
| Design System | [docs/DESIGN_SYSTEM.md](docs/DESIGN_SYSTEM.md) |
| Deployment | [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) |

## Build status

Progress is tracked module by module. See [docs/PROGRESS.md](docs/PROGRESS.md).
