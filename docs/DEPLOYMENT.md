# LOADIFY — Deployment Guide

---

## 1. Local development

**Prerequisites:** Docker 24+ with Compose v2. Nothing else — Go, Postgres and
Redis all run in containers.

```bash
cp .env.example .env
```

Set the two JWT secrets before anything will start; the API refuses to boot with
short ones, which is deliberate:

```bash
openssl rand -base64 48   # JWT_ACCESS_SECRET
openssl rand -base64 48   # JWT_REFRESH_SECRET
```

```bash
docker compose -f deploy/docker-compose.yml up -d
```

The stack comes up in dependency order: Postgres and Redis become healthy, the
bucket is created, migrations run to completion, then the API starts.

```bash
curl http://localhost:8080/health/ready
docker compose -f deploy/docker-compose.yml logs -f api
```

| Service | URL |
|---|---|
| API | http://localhost:8080 |
| MinIO console | http://localhost:9001 |
| Postgres | localhost:5432 |
| Redis | localhost:6379 |

With `SMS_PROVIDER=log` the OTP is written to the API log instead of being sent,
so you can register without an SMS account:

```bash
docker compose -f deploy/docker-compose.yml logs api | grep -i otp
```

### Running the API outside Docker

Keep the infrastructure containers and run Go on the host — much faster to iterate:

```bash
docker compose -f deploy/docker-compose.yml up -d postgres redis minio minio-init
cd backend
go run ./cmd/migrate up
go run ./cmd/api
```

### Useful commands

```bash
cd backend && go test ./...
```

```bash
cd backend && go run ./cmd/migrate status
```

```bash
cd backend && go run ./cmd/seed
```

---

## 2. Production — DigitalOcean

### 2.1 Sizing

| Stage | Droplet | Database |
|---|---|---|
| Launch (< 5k users) | 1 × 4 vCPU / 8 GB | Postgres on the droplet |
| Growth (5–50k) | 2 × 4 vCPU / 8 GB behind a load balancer | Managed Postgres, 4 GB |
| Scale (50k+) | 3+ app nodes, separate WS nodes | Managed Postgres + read replica |

Redis stays on the app node until multiple nodes exist — the moment there are
two, it has to move to a managed instance, because the realtime hub uses it for
cross-node fan-out and the rate limiter needs one shared view of the counters.

### 2.2 Prepare the host

```bash
adduser --disabled-password --gecos "" loadify && usermod -aG docker loadify
```

```bash
ufw default deny incoming && ufw allow OpenSSH && ufw allow 80,443/tcp && ufw enable
```

Postgres, Redis and MinIO must **not** be reachable from the internet. In the
Compose file they are published to the host for local development; on a server,
remove those `ports:` entries so they are reachable only on the Docker network.

### 2.3 Deploy

```bash
git clone git@github.com:<org>/loadify.git /opt/loadify && cd /opt/loadify
```

Create `/opt/loadify/.env` with production values:

```
APP_ENV=production
APP_BASE_URL=https://api.loadify.uz
DB_PASSWORD=<strong>
JWT_ACCESS_SECRET=<48 random bytes>
JWT_REFRESH_SECRET=<48 random bytes, different>
REDIS_PASSWORD=<strong>
S3_ACCESS_KEY=<strong>
S3_SECRET_KEY=<strong>
CORS_ALLOWED_ORIGINS=https://loadify.uz,https://app.loadify.uz
SMS_PROVIDER=eskiz
SMS_ESKIZ_EMAIL=...
SMS_ESKIZ_PASSWORD=...
```

```bash
chmod 600 .env
```

The config validator refuses to start in production with a development secret,
`SMS_PROVIDER=log`, or a wildcard CORS origin. That is the intended behaviour —
fix the value rather than working around the check.

```bash
docker compose -f deploy/docker-compose.yml --profile proxy up -d
```

### 2.4 TLS

```bash
docker run --rm -v /opt/loadify/deploy/nginx/certs:/etc/letsencrypt/live/api.loadify.uz -v /var/www/certbot:/var/www/certbot certbot/certbot certonly --webroot -w /var/www/certbot -d api.loadify.uz --agree-tos -m ops@loadify.uz --non-interactive
```

Renewal, as a daily cron:

```bash
0 3 * * * docker run --rm -v /opt/loadify/deploy/nginx/certs:/etc/letsencrypt -v /var/www/certbot:/var/www/certbot certbot/certbot renew --quiet && docker exec loadify-nginx nginx -s reload
```

### 2.5 Create the first administrator

There is no bootstrap endpoint by design — an unauthenticated "make me admin"
route is a permanent liability. Register through the API like any user, then
grant the role directly:

```bash
docker exec -it loadify-postgres psql -U loadify -d loadify -c "INSERT INTO user_roles (user_id, role_id) SELECT u.id, r.id FROM users u, roles r WHERE u.phone = '+998901234567' AND r.code = 'super_admin';"
```

---

## 3. Operations

### Migrations

Migrations run as their own Compose job, never on API start-up: two replicas
starting together would otherwise race to migrate the same database.

```bash
docker compose -f deploy/docker-compose.yml run --rm migrate up
```

```bash
docker compose -f deploy/docker-compose.yml run --rm migrate status
```

**Every migration must be backward compatible with the version still running.**
Deploys apply the schema before releasing the new image, so for a short window
the old code runs against the new schema. In practice: add columns nullable,
never rename in one step (add, backfill, switch, drop across three releases),
and never drop a column the running version still selects.

### Backups

```bash
docker exec loadify-postgres pg_dump -U loadify -Fc loadify > /backups/loadify-$(date +%F).dump
```

Nightly, with a 30-day retention:

```bash
0 2 * * * docker exec loadify-postgres pg_dump -U loadify -Fc loadify | gzip > /backups/loadify-$(date +\%F).dump.gz && find /backups -name '*.dump.gz' -mtime +30 -delete
```

Restore:

```bash
gunzip -c /backups/loadify-2026-01-15.dump.gz | docker exec -i loadify-postgres pg_restore -U loadify -d loadify --clean --if-exists
```

A backup nobody has restored is a hypothesis. Run a restore drill into a scratch
database quarterly.

### Scheduled maintenance

The API exposes these as usecases; run them from cron or a worker:

| Job | Frequency | What it does |
|---|---|---|
| Expire loads | hourly | Closes published loads past their window |
| Expire bookings | every 15 min | Closes unanswered bookings, frees the load |
| Auto-confirm deliveries | hourly | Closes delivered trips after 72h so drivers are paid |
| Expire subscriptions | daily | Drops lapsed plans to the free tier |
| Archive chat rooms | daily | Freezes conversations 30 days after a trip closes |
| Purge notifications | weekly | Trims history past 90 days |
| Purge orphan uploads | daily | Deletes files never attached to anything |
| Location retention | daily | Drops GPS traces older than 90 days |
| Wallet reconciliation | daily | Recomputes balances from the ledger, reports drift |

Wallet reconciliation is the one to alert on. A non-zero drift means the cached
balance and the ledger disagree, which is an accounting bug — it should page
someone, not just log.

### Partition maintenance

`driver_locations` is monthly range-partitioned. Partitions for the next three
months are created at migration time; keep creating them ahead:

```bash
docker exec loadify-postgres psql -U loadify -d loadify -c "SELECT ensure_month_partition('driver_locations', (now() + interval '4 months')::date);"
```

Missing a partition means every location write fails, so run this monthly.

### Monitoring

| Signal | Alert when |
|---|---|
| `GET /health/ready` | Fails twice in a row |
| API p95 latency | > 600 ms for 5 minutes |
| 5xx rate | > 1% of requests |
| Postgres connections | > 80% of `max_connections` |
| Replication lag | > 10 s |
| Disk | > 80% |
| WebSocket connections | Drops > 50% in a minute (suggests a node died) |
| Wallet drift | Any non-zero result |
| Failed payment webhooks | Any signature rejection — that is either a config error or an attack |

---

## 4. Troubleshooting

**The API will not start.** It fails fast on bad configuration and says which
key. Read the first log line:

```bash
docker compose -f deploy/docker-compose.yml logs api | head -20
```

**Migrations are stuck "dirty".** A migration failed halfway. Inspect what
applied, fix the SQL, then force the version back and re-run — do not edit the
`schema_migrations` table by hand without knowing which statements landed.

**Realtime works on one node but not another.** The hub falls back to local-only
delivery when Redis is unreachable and logs a warning at start-up. Check that
every node shares one Redis, not one each.

**Uploads fail.** The API degrades rather than refusing to start when MinIO is
down; look for `object storage unavailable` in the log and check the bucket
exists.

**Payments confirm but subscriptions do not activate.** Look at
`payment_webhooks`: every callback is recorded with its signature verdict and
processing error before it is applied.
