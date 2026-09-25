# Khan's Bike Zone API

Backend for Khan's Bike Zone — a **catalogue** (not a shop) for a bicycle and
bicycle-parts retailer/wholesaler in Trinidad & Tobago. Visitors browse, search,
and filter stock; the owner manages the catalogue from a web admin and a paired
phone. There is no cart, checkout, or payment — by design.

Ships as a single cross-compiled Windows `.exe` (React build embedded), fronted
by a Cloudflare Tunnel, backed by PostgreSQL 17.

## Stack

Go 1.23+ · PostgreSQL 17 · `pgx/v5` + `pgxpool` · `sqlc` · `goose` · `chi/v5` ·
`log/slog`. Pure Go only — `CGO_ENABLED=0` must build the whole tree (WebP
encoding uses a pure-Go build of libwebp; spreadsheets use `excelize`).

## Quick start (development)

```sh
cp .env.example .env                  # then set CSRF_KEY (openssl rand -base64 32) and MEDIA_FS_ROOT
make db-up                            # start dev Postgres 17 (docker compose)
make run                              # migrates, then serves on HTTP_ADDR (default 127.0.0.1:8080)

make admin EMAIL=you@example.com NAME="Your Name"   # create the owner account (prompts for a password)
make seed                             # optional: load a sample catalogue into the empty database

curl http://127.0.0.1:8080/healthz    # {"status":"ok"}
curl http://127.0.0.1:8080/readyz     # also checks the database
curl http://127.0.0.1:8080/api/v1/categories
```

The Makefile loads `.env` for every target. The binaries themselves only read
the process environment.

## Common tasks

| Command | Does |
|---|---|
| `make run` / `make worker` | Run the API (with the in-process job worker) / a standalone worker |
| `make build` / `make build-windows` | Build every binary into `bin/` / cross-compile the `.exe` files into `bin/windows/` |
| `make check` | `go vet` + unit tests with `-race` — the pre-commit gate |
| `make test-integration` | Unit + integration tests against a real Postgres (see [Testing](#testing)) |
| `make lint` | `golangci-lint` v2 with `.golangci.yml` |
| `make sqlc` / `make sqlc-check` | Regenerate typed queries into `internal/store/gen` / fail if they are stale |
| `make migrate-up` / `migrate-down` / `migrate-status` | Drive migrations by hand (the servers also migrate on start) |
| `go run ./cmd/migrate create <name> sql` | Scaffold the next numbered migration in `db/migrations` |
| `make admin EMAIL=… NAME=…` | Create an admin account; `go run ./cmd/admin reset-password -email …` resets one |

## Layout

```
cmd/
  api/        HTTP server; also runs the job worker unless WORKER_ENABLED=false
  worker/     standalone job worker (for running jobs in a separate process)
  migrate/    goose CLI over the embedded migrations
  importer/   stage / review / commit the price-list workbook from the shell
  admin/      create-user and reset-password
db/
  migrations/ goose SQL migrations — the single source of truth for the schema
  queries/    SQL for sqlc
internal/
  app/        wiring: builds the store, services, router and job runner from Config
  config/     environment parsing and validation (reports every problem at once)
  domain/     plain types, errors and rules shared by every layer
  platform/   small utilities: money, slugs, cursors
  store/      pgx + sqlc; hand-written dynamic catalogue SQL; database-error translation
  service/    business logic: catalogue, taxonomy, products, media, mailing list, audit
  importer/   workbook parsing, staging, review and commit
  auth/       passwords, sessions, CSRF, device pairing
  media/      image processing and blob storage (filesystem or Cloudflare R2)
  email/      SMTP and log senders
  jobs/       Postgres-backed job queue runner
  http/       router, handlers, DTOs, middleware, problem+json errors
  testutil/   integration-test helpers (throwaway databases)
web/          embeds the front-end build (web/dist) and serves it as an SPA
scripts/      seed.go — the sample catalogue
```

Requests flow **handler → service → store**. Handlers only translate HTTP;
services own the rules and transactions; the store owns SQL. DTOs in
`internal/http/dto` are the only shapes that reach the wire, and public and
admin responses use separate types so admin-only data cannot leak into the
public API.

## API

Everything lives under `/api/v1` and speaks JSON.

### Conventions

- **Errors** are `application/problem+json` ([RFC 9457](https://www.rfc-editor.org/rfc/rfc9457)):
  `{"type","title","status","detail"}`. Validation failures are `422` with
  `"errors": [{"field": "...", "message": "..."}]`. Other statuses: `400`
  malformed request, `401`/`403` auth, `404`, `409` conflict (e.g. a slug is
  taken, or a record is still in use), `412` stale edit, `413` too large,
  `429` rate limited.
- **Request bodies** are limited to 1 MiB (uploads excepted), and unknown fields
  are rejected.
- **Lists** return `{"items": [...], "nextCursor": "..."}`. Pass `?cursor=` to
  get the next page and `?limit=` to set its size. Cursors are opaque.
- **Concurrent edits**: admin `GET`s of a product, category or attribute return
  an `ETag`. Send it back as `If-Match` on the `PUT`; if someone saved in
  between, the answer is `412` instead of a silent overwrite.
- **Caching**: the public catalogue is `Cache-Control: public, max-age=60`, and
  everything authenticated is `no-store`.

### Public catalogue (anonymous, read-only)

| Endpoint | Returns |
|---|---|
| `GET /categories` | The category tree |
| `GET /categories/{slug}` | A category, its breadcrumb and children |
| `GET /categories/{slug}/facets` | Filter options with counts for the current filters |
| `GET /products` | Product cards (filterable, sortable, paginated) |
| `GET /products/{slug}` | Product detail with variants, attributes and images |
| `GET /search?q=` | Matches grouped by category (typo tolerant, also matches supplier codes) |
| `GET /search/suggest?q=` | Type-ahead suggestions |
| `GET /fitment/{wheelSize}` | Everything that fits a wheel size, grouped by category |
| `GET /brands` | Brands, with logos |

`/products` and `/facets` accept:

- `category=<slug>` (includes subcategories), `brand=<slug>`, `q=<text>`
- `attr.<key>=a,b` — any of the listed values. Different attributes combine
  with AND, and a product matches when **one of its variants** satisfies them
  all.
- `attr.<key>_min=` / `attr.<key>_max=` — a numeric range
- `sort=featured|name|-created|-updated|relevance`. The default is
  `relevance` when `q` is given and `featured` otherwise.

Facet counts are *disjunctive*: each attribute's counts ignore that attribute's
own filter, so picking "Red" still shows how many "Blue" items there are.

Only `active` products in visible categories appear. Prices are never public
except the retail price of products marked `retailPriceIsPublic`. Cost,
landed and wholesale prices exist only in the admin API.

### Mailing list

Double opt-in: `POST /subscribers {"email","name?","source?"}` always answers
`202` (whether or not the address is already known), and a confirmation email
links to the front-end pages `/subscribe/confirm?token=…` and
`/subscribe/unsubscribe?token=…`. Those pages call
`POST /subscribers/confirm {"token"}` and `POST /subscribers/unsubscribe {"token"}`.

### Authentication

- **Browser admin**: `POST /auth/login {"email","password"}` sets an HttpOnly
  session cookie and returns a `csrfToken`. Every unsafe request (`POST`, `PUT`,
  `DELETE`) must send it as `X-CSRF-Token`. `GET /auth/session` returns it again
  after a page reload. `POST /auth/logout` ends the session, and
  `PUT /auth/password` changes the password and signs out every other session.
- **Paired phone**: in the admin, `POST /admin/devices/pairing-codes` makes an
  8-character code (valid for 10 minutes) and a `/pair?code=` link. The phone
  redeems it with `POST /auth/devices {"code","name"}` and then sends
  `Authorization: Bearer <token>`. Devices are listed and revoked under
  `/admin/devices`, which only a browser session can reach.
- Five wrong passwords in a row lock the account for 1 minute. Each further
  failure doubles the lock, up to 1 hour. Login, pairing and mailing-list signups are rate limited per IP.

### Admin (`/admin`, admin role)

| Area | Endpoints |
|---|---|
| Categories | `GET/POST /categories`, `GET/PUT/DELETE /categories/{id}` (moving is a `PUT` with a new `parentId`) |
| Category attributes | `GET /categories/{id}/attributes`, `PUT/DELETE /categories/{id}/attributes/{attributeId}`, `GET /categories/{id}/form-schema` |
| Attributes | `GET/POST /attributes`, `GET/PUT/DELETE /attributes/{id}`, `POST /attributes/{id}/options`, `PUT/DELETE /attributes/{id}/options/{optionId}` |
| Brands, suppliers | `GET/POST /brands`, `PUT/DELETE /brands/{id}`, and the same for `/suppliers` |
| Products | `GET/POST /products`, `GET/PUT/DELETE /products/{id}` — a product is saved as one document: fields, variants, attribute values and prices |
| Media | `POST /media` (multipart field `file`), `GET /media/{id}`, `POST /products/{id}/media`, `PUT/DELETE /products/{id}/media/{mediaId}` |
| Imports | `GET/POST /imports`, `GET /imports/{id}`, `GET /imports/{id}/rows`, `PUT /imports/{id}/rows/{rowId}`, `POST /imports/{id}/commit`, `POST /imports/{id}/abort` |
| Mailing list | `GET /subscribers/stats`, `GET /subscribers/export` (CSV of confirmed subscribers) |
| Audit | `GET /audit?entityType=&entityId=` — every admin change, who made it, and before/after |

Every price change is kept as history. The form schema tells the admin UI
which fields a category's products have, and its `ETag` changes whenever the
category's attributes change.

## Importing the price list

The importer turns the owner's `.xlsx` price list into products. It works in
two steps, and staging never changes the catalogue.

1. **Stage** (`POST /admin/imports` with the workbook as `file`, or
   `importer stage list.xlsx`). Each sheet is matched to the category with the
   same name. The header row may be anywhere in the first 10 rows, and columns
   are recognised by name:

   | Field | Accepted headers (case and punctuation ignored) |
   |---|---|
   | Name | name, description, item, product |
   | Brand | brand, make, manufacturer |
   | SKU | sku, code, item code, stock code (generated when missing) |
   | Supplier item no. | supplier code, item no, part no |
   | Stock | stock, qty, quantity, availability |
   | Prices | cost / usd (cost_usd), landed, wholesale / trade, retail / price |
   | Attributes | the attribute's key or label, e.g. `Wheel Size`, `Colour` |

   Every row gets a proposal, a list of issues and a suggested decision.
   Examples of issues: `"Prestaa" is not an option of Valve Type; did you mean
   "Presta"?`, a price that had to be rounded, or a duplicate row. Rows with an
   **error** wait for a person (`pending`), and exact duplicates are skipped.
   Rows with the same name, brand and product-level attributes become the
   variants of one product (differing in, say, colour). Rows whose SKU or
   supplier code matches an existing variant are proposed as a **merge**,
   which updates that variant's prices and stock.
2. **Review**: list rows with `GET /admin/imports/{id}/rows?issues=true`, and
   set a row's decision with `PUT …/rows/{rowId} {"decision":
   "accept|skip|merge"}`. Rows left `pending` block the commit.
3. **Commit** (`POST …/commit {"retailPricesPublic": false}` or
   `importer commit <batch-id>`). This applies everything in one transaction,
   through the same validation as the admin product editor. New products are
   `active`, except products with a **warning** on any of their rows: those
   start in `needs_review` and stay hidden from the public until someone checks
   them.

## Media

Uploads (JPEG, PNG or WebP, up to `MEDIA_MAX_UPLOAD_BYTES`) are
deduplicated by content hash. A background job then generates WebP and JPEG
renditions at `MEDIA_RENDITION_WIDTHS`, a blurhash placeholder and a dominant
colour. Photos are rotated upright according to their EXIF data, and only
renditions are ever served. Originals, which may carry GPS metadata, are never
served. Files go to `MEDIA_FS_ROOT` or to Cloudflare R2 (`MEDIA_BACKEND=r2`).

## Background jobs

Jobs live in the `jobs` table: media processing, search reindexing and
confirmation emails. `cmd/api` runs them in-process. Set `WORKER_ENABLED=false`
to run `cmd/worker` separately instead. Failed jobs retry with backoff. Jobs
stuck on a crashed worker are picked up again, and a job that keeps failing
ends up `dead` so it can be inspected.

## Deployment (Windows host)

1. Build the front end and copy its output into `web/dist/`. It is embedded
   into the binary and served for every path the API does not own.
2. `make build-windows`, then copy `bin/windows/*.exe` to the host.
3. Set the environment (see `.env.example`): `APP_ENV=production` (secure
   cookies, JSON logs), a fresh `CSRF_KEY`, `PUBLIC_BASE_URL`, `DATABASE_URL`
   and media and email settings. Keep `HTTP_ADDR` on `127.0.0.1` and
   `TRUST_CLOUDFLARE_IPS=true`.
4. Run `api.exe` as a service. It applies pending migrations on start, and
   concurrent starts are safe because migrations take a lock.
5. Point a Cloudflare Tunnel at `HTTP_ADDR`. TLS ends at Cloudflare, and the
   real client IP comes from `CF-Connecting-IP`, trusted only from Cloudflare
   and loopback addresses.
6. Create the owner account once with `admin.exe create-user -email … -name …`.

## Testing

- `make check` — vet and unit tests; no database needed.
- `make test-integration` — also runs the tests tagged `integration`: every API
  flow end to end (auth, CSRF, catalogue filters and facets, price leaks,
  concurrent edits, media, mailing list, pairing, imports, jobs). Each test
  creates and drops its own database on the server in `TEST_DATABASE_URL`
  (falling back to `DATABASE_URL`), so that user needs `CREATEDB`.

CI (`.github/workflows/ci.yml`) runs on the Go version `go.mod` declares:
formatting, `go mod tidy -diff`, `sqlc diff`, lint (which includes vet), the
unit and integration tests against Postgres 17, and the Linux and Windows
builds.
