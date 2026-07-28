# Khan's Bike Zone API

Backend for Khan's Bike Zone — a **catalogue** (not a shop) for a bicycle and
bicycle-parts retailer/wholesaler in Trinidad & Tobago. Visitors browse, search,
and filter stock; the owner manages the catalogue from a web admin and a paired
phone. There is no cart, checkout, or payment — by design.

Ships as a single cross-compiled Windows `.exe` (React build embedded), fronted
by a Cloudflare Tunnel, backed by PostgreSQL 17.

## Stack

Go 1.23+ · PostgreSQL 17 · `pgx/v5` + `pgxpool` · `sqlc` · `goose` · `chi/v5` ·
`log/slog`. Pure Go only — `CGO_ENABLED=0` must build the whole tree.

## Quick start (development)

```sh
cp .env.example .env                       # then set CSRF_KEY (openssl rand -base64 32)
make db-up                                 # start dev Postgres 17 (docker compose)
make migrate-up                            # apply migrations
make run                                   # serve on HTTP_ADDR (default 127.0.0.1:8080)

curl http://127.0.0.1:8080/healthz         # {"status":"ok"}
curl http://127.0.0.1:8080/readyz
```

## Common tasks

| Command | Does |
|---|---|
| `make build` | Build all binaries (`CGO_ENABLED=0`) |
| `make check` | `go vet` + `go test -race` — the pre-commit gate |
| `make sqlc` | Regenerate typed queries into `internal/store/gen` |
| `make migrate-up` / `migrate-down` / `migrate-status` | Migrations (needs `DATABASE_URL`) |
| `make test-integration` | Integration tests against a real Postgres (`-tags=integration`) |

## Layout

- `cmd/` — `api` (server + in-process worker), `migrate`, `worker`, `importer`.
- `internal/` — `domain` (pure types), `store` (pgx + sqlc), `service`, `http`
  (router/handlers/dto/middleware), `media`, `auth`, `jobs`, `platform`, `config`.
- `db/` — `migrations/` (goose, embedded) and `queries/` (sqlc input).

Configuration is validated at startup and fails fast, reporting **every** invalid
or missing variable at once. See `.env.example` for the full documented set.
