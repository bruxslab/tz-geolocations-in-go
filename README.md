# Tanzania administrative locations in Go

Read-only REST API and an offline Go package for 31 regions, 168 districts, 4,054 wards, and 75,061 streets. Data and legacy API behavior are pinned to [unctom/tz-locations commit d835ea3](https://github.com/unctom/tz-locations/tree/d835ea318dbb17f789d4b7195b600e2e07e20f1e).

## Requirements

- Docker Engine with Compose v2 or newer.
- Go **1.27.3** for host development (the Docker build pins this version).
- OpenSSL for the optional local credential generator.
- Python 3.12+ for OpenAPI validation; Python scraping is optional.
- A C compiler and `CGO_ENABLED=1` for host race tests. The Docker test target includes the compiler.

## Start with Docker

```sh
sh scripts/init-env.sh
# Review .env; it is ignored by Git and contains separate generated secrets.
make db
make migrate
make import
make up
curl http://127.0.0.1:8080/v1/stats
```

`init-env.sh` refuses to overwrite an existing `.env`. Alternatively copy `.env.example` and replace every secret placeholder with separate URL-safe random values. Passwords containing URL-reserved characters must be percent-encoded in connection URLs.

Compose creates a dedicated PostgreSQL 18 service with a named volume, an owner role, a read-only API role, and **tz_locations / tz_locations_test** databases. PostgreSQL binds to loopback port **55432** and the API to loopback **8080**. The API image runs as a non-root user with a read-only filesystem. Data initialization is explicit; run migrations and import before starting the API.

The approved Docker setup supersedes the initial request to use Lerd PostgreSQL. **No Lerd services or databases are used or changed.** Production is independent of Docker and Lerd: provide a PostgreSQL `DATABASE_URL`, use verified TLS as required by your host, and run the built binaries. `sslmode=disable` is for this isolated local setup.

`docker compose down` stops this project's services and keeps its data volume. Do not remove the volume unless you intend to delete this project's databases.

## Host development

The Go commands read exported environment variables; they do not implicitly load `.env`.

```sh
set -a
. ./.env
set +a

go run ./cmd/migrate   # MIGRATION_DATABASE_URL; grants SELECT to RUNTIME_DATABASE_ROLE

go run ./cmd/import    # same owner connection, embedded pinned dataset

go run ./cmd/api       # DATABASE_URL; restricted reader connection
```

`MIGRATION_DATABASE_URL` takes precedence over `DATABASE_URL` in the migration/import commands. Only those commands require write privileges. The HTTP server neither migrates nor imports automatically.

All application tables live in the dedicated `locations` schema. Versioned SQL is embedded in the migration executable; checksums reject edits to applied migrations. PostgreSQL advisory locks serialize migrations and imports. A missing migration is an operational error, never a request-time schema change.

## Import and validation

```sh
go run ./cmd/validate
# Repeat the pinned import safely; identical healthy data is a no-op.
docker compose run --rm import
# Explicitly replace a different dataset, or repair failed count/index verification:
docker compose run --rm import --replace
# Host command for a separately reviewed, complete dataset:
go run ./cmd/import --data /absolute/path/to/json --source SOURCE_COMMIT_OR_VERSION --replace
```

External data must contain `regions.json`, `districts.json`, `wards.json`, and `streets.json`. The importer reports structural errors together, validates full parent-qualified keys, preserves legitimate repeated names, and aborts without partial data. An unversioned or different populated dataset is rejected without `--replace`. Replacement touches only this application's schema; all writes, search documents, provenance, and integrity checks commit together.

IDs are one-based source array positions per level, reproducing the legacy seed. Reimports of the pinned files preserve IDs. A **different** dataset may change IDs if its ordering changes; review compatibility before replacing it. The offline package always uses the embedded pinned snapshot and changes only when that snapshot is deliberately rebuilt.

## Endpoints

| Method | Path | Behavior |
|---|---|---|
| GET | `/` | Unwrapped API discovery |
| GET | `/v1/regions` | Regions with district counts |
| GET | `/v1/regions/{slug}` | Region and hierarchy counts |
| GET | `/v1/regions/{slug}/districts` | Districts in region |
| GET | `/v1/districts/{slug}` | District and hierarchy counts |
| GET | `/v1/districts/{slug}/wards` | Wards in district |
| GET | `/v1/wards/{slug}` | Ward, postcode and parent references |
| GET | `/v1/wards/{slug}/streets` | Streets; `limit=100` (max 500), `offset=0` |
| GET | `/v1/streets/{id}` | Street by stable source ID |
| GET | `/v1/postcodes/{postcode}` | First matching ward and all matching streets |
| GET | `/v1/search?q=...` | Full-text search; optional `region_slug`, `type`, `limit=10` (max 50) |
| GET | `/v1/autocomplete?q=...` | Phrase prefix, minimum two characters; optional `region_slug`, `type`; `limit=5` (max 20) |
| GET | `/v1/stats` | Counts at all four levels |
| GET | `/v1/docs` | OpenAPI 3.1 document |

Responses retain `{ "data": ..., "meta": ... }`, omitting `meta` when the source omits it. Root discovery is unwrapped. Errors use `{ "error": { "code": "NOT_FOUND", "message": "..." } }`. On street pages, `meta.total` is the **returned page size**, preserving the source contract.

Ambiguous slugs select the lowest source ID. Add `?region_slug=...` on district routes and `?region_slug=...&district_slug=...` on ward routes, including their child lists. Qualified responses preserve parent context in relevant links. Unqualified legacy link strings remain unchanged.

API search uses PostgreSQL's `simple` full-text configuration with a GIN index, phrase matching, and final-token prefix autocomplete. It searches names, postcodes, and paths at all four levels. Ranking differs from SQLite FTS5 porter; streets are included as the explicitly approved fix. Offline search keeps the npm package's case-insensitive name substring matching. See [parity decisions](docs/FEATURE_PARITY.md).

## Checkout address selection

Default the country to Tanzania and populate the region dropdown from `/v1/regions`. Pass the selected region's **slug** to autocomplete while the customer types:

```http
GET /v1/autocomplete?q=ibad&type=street&region_slug=shinyanga&limit=10
```

For full-word searches with stable entity IDs, use:

```http
GET /v1/search?q=ibadakuli&type=street&region_slug=shinyanga&limit=10
```

Matching includes parent ward/district names, so `ibadakuli` can return **Airport** in Ibadakuli ward, as well as streets named Ibadakuli. Region filtering uses the actual parent relationship, not text in `full_path`. Both endpoints retain their existing response shapes; omit `region_slug` to search nationwide. A valid but unknown region returns an empty list; a supplied empty or malformed slug returns `BAD_REQUEST`. Autocomplete now supports the same optional `type` values as search.

Display suggestions with the postcode first, using the two existing fields:

```js
const label = [result.postcode, result.full_path].filter(Boolean).join(", ");
// "37113, Airport, Ibadakuli, Shinyanga CBD, Shinyanga"
```

Store the selected postcode/path (and street ID when using search) alongside the selected region. Clear the address selection when the region changes. Debounce typing requests and cancel or ignore stale responses so an old region/query cannot overwrite current suggestions. The API supports Tanzania only; no country query parameter is needed. This repository provides the API, not a checkout frontend.

Existing installations must run `make migrate` before starting the updated API. Migration `0002` backfills a separate, indexed containing-region ID for every existing search document; it preserves IDs, names, postcodes and paths and does not require reimporting the dataset. The importer also populates this field for fresh imports/replacements.

## Offline package

```go
import "github.com/bruxslab/tz-geolocations-in-go/pkg/locations"

regions := locations.GetRegions()
wards := locations.GetWardsByDistrict("dar-es-salaam", "ilala")
ward, found := locations.GetWard("buguruni", "dar-es-salaam", "ilala")
streets := locations.GetStreets("dar-es-salaam", "ilala", "buguruni")
postcode := locations.GetByPostcode("12102")
results := locations.SearchLocations("buguruni")
limited := locations.SearchLocations("a", locations.SearchOptions{Type: "ward", Limit: 5})
stats := locations.GetStats()
```

Also exported: `GetRegion`, `GetDistricts`, `GetDistrictsByRegion`, `GetDistrict`, and `GetWards`. Singular getters return `(value, bool)`. Lists are non-nil copies in original JSON order. `GetDistricts()` returns all districts; an optional region filters them. `GetWards()` returns all wards and filters only when **both** region and district are supplied, preserving npm behavior. Optional parents on singular district/ward getters disambiguate them.

Omitted search options use limit 10. With an explicit `SearchOptions`, `Limit <= 0` returns no results. Search results retain region/district/ward/street order. Calls are concurrency-safe, and mutating returned values cannot corrupt shared data. No database, HTTP server, or network is required.

## Configuration and security

See `.env.example` for all settings. Defaults: pool max/min 10/2, rate limit 100 requests per 60-second fixed window, wildcard CORS, no trusted proxies. Use `CORS_ORIGINS=https://app.example,https://other.example` to restrict origins. The API accepts GET/HEAD and CORS preflight; it has no write endpoints or authentication.

Client identity comes from the socket peer. `TRUSTED_PROXIES` accepts comma-separated CIDRs; only peers in those networks may provide forwarding headers. X-Forwarded-For is processed right-to-left until an untrusted hop. CF-Connecting-IP is a fallback only from a trusted immediate peer. Do not trust a Docker network or proxy range without configuring your ingress to sanitize forwarding headers.

Rate limits are process-local, with bounded client bookkeeping; replicas do not share quotas. Responses include X-RateLimit-Limit/Remaining/Reset. Successful v1 data responses retain `Cache-Control: public, max-age=86400`; errors do not. Requests have a 64 KiB body limit, 1 MiB header limit, connection/server timeouts, and a 10-second database read budget. Migrations and imports have a separate two-minute statement budget and five-minute command budget. SIGINT/SIGTERM drains HTTP requests before closing the pool. Structured logs omit credentials and unexpected error details from public responses.

## Tests

```sh
# Full unit, integration, contract, race, vet, format and data checks in Docker:
make verify

# Host equivalents, after exporting .env:
REQUIRE_DATABASE_TESTS=1 go test ./...
CGO_ENABLED=1 REQUIRE_DATABASE_TESTS=1 go test -race ./...
go vet ./...
gofmt -l .
go run ./cmd/validate

python3 -m venv /tmp/tz-openapi-venv
/tmp/tz-openapi-venv/bin/pip install -r scripts/requirements-dev.txt
CONTRACT_RESULTS_PATH=/tmp/tz-go-contracts.json REQUIRE_DATABASE_TESTS=1 go test ./tests/contract
/tmp/tz-openapi-venv/bin/python scripts/validate-openapi.py /tmp/tz-go-contracts.json
```

Without `TEST_DATABASE_URL`, database suites explicitly skip; set `REQUIRE_DATABASE_TESTS=1` to make missing configuration fail. Database tests refuse any database name other than `tz_locations_test` and verify the actual connected name before writes. Suites serialize access to that database and never reset development. They exercise real SQL, all 75,061 street IDs/relationships, constraints, search, parent filters, transactional rollback, replacement, cancellation, and sanitized database failures.

Contract fixtures were generated by executing the pinned **original Hono handlers** against their committed SQLite migration and seed, plus all twelve original npm functions. Tests compare canonicalized JSON; approved broad-search changes have separately documented assertions. CI validates the OpenAPI 3.1 specification and source/actual Go responses against its schemas.

To regenerate fixtures, download/extract the pinned reference to a temporary directory, run `npm ci --ignore-scripts` in its `api/`, then run from this project's root:

```sh
REFERENCE_DIR=/absolute/path/to/pinned-reference \
  /absolute/path/to/pinned-reference/api/node_modules/.bin/tsx scripts/reference-contracts.mts
```

Review fixture changes against the pinned source; never regenerate them from the Go API. GitHub Actions runs on PRs targeting `main` and pushes to `main`, with isolated CI PostgreSQL. Local Lerd PostgreSQL is never used by CI. This delivery does not push a branch, so remote Actions execution is unverified.

## Scraper and provenance

The original Python scraper/parser/tests remain in `scraper/` as reference ingestion tools. Install their requirements in a virtual environment to use them. Scraping is manual, writes to ignored `scraper/data/`, and never replaces embedded/API data automatically. The original tool infers some names and hierarchy from URLs; preserving its output is not independent verification of official current administrative boundaries. See [data provenance](docs/DATA_PROVENANCE.md) and [architecture](docs/ARCHITECTURE.md).

## Troubleshooting

- **Docker permission denied:** enable access to your installed daemon for your user, then refresh group membership or restart your terminal/app. No sudo is needed by the application itself.
- **Credential-helper error pulling public images:** fix your Docker credential store, or use an isolated empty Docker client configuration for anonymous public pulls. Do not change Lerd settings.
- **Port already in use:** change `POSTGRES_PORT` / `HTTP_PORT` in `.env`; update host connection URLs to the same PostgreSQL port. Containers still use `postgres:5432`.
- **Authentication failed after changing passwords:** PostgreSQL initialization runs only on a new data volume. Changing `.env` does not rotate existing database role passwords. Rotate them explicitly and update configuration together.
- **Relation missing / empty data:** run `make migrate`, then `make import`, before starting the API.
- **Different dataset / invalid counts:** review provenance and validation output before using `--replace`. Replacement is explicit and transactional.
- **Race tests cannot find gcc:** use `make verify` or install a host C compiler and enable cgo.
- **Missing test URL:** export `TEST_DATABASE_URL` from your local configuration; do not point tests at development.

MIT license; upstream attribution is retained in `LICENSE`.
