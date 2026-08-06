# ─── build ──────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS build

WORKDIR /src

# Dependencies are copied first so a source-only change does not re-download
# the module cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
# CGO off produces a static binary that runs on the scratch-like runtime image.
# The symbol table and DWARF data are stripped: they are dead weight in a
# container and only make the image larger.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X github.com/loadify/backend/internal/bootstrap.Version=${VERSION}" \
    -o /out/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" -o /out/seed ./cmd/seed

# ─── runtime ────────────────────────────────────────────────────────
FROM alpine:3.21

# ca-certificates for the outbound HTTPS calls (FCM, Maps, payment providers);
# tzdata because every timestamp is rendered in Asia/Tashkent.
RUN apk add --no-cache ca-certificates tzdata curl \
 && addgroup -g 1001 -S loadify \
 && adduser -u 1001 -S loadify -G loadify

WORKDIR /app

COPY --from=build /out/api /app/api
COPY --from=build /out/migrate /app/migrate
COPY --from=build /out/seed /app/seed
COPY --from=build /src/migrations /app/migrations

# The process never needs to write to its own image.
USER loadify

ENV TZ=Asia/Tashkent
EXPOSE 8080

# The container is unhealthy the moment it stops serving, not merely when the
# process dies — a wedged server still holds its PID.
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD curl -fsS http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/api"]
