# Local verification

Verified on 2026-11-02 with Go 1.27.3, pgx/v5 5.12.0, Docker Engine 29.9.2, and dedicated Docker PostgreSQL 18. No Lerd configuration, service, or database was modified.

| Check | Result |
|---|---|
| Full Go tests with required PostgreSQL | Passed |
| Race detector with cgo, including PostgreSQL | Passed in Docker compiler image |
| `go vet ./...` | Passed |
| `gofmt -l .` / `git diff --check` | Clean |
| `go mod verify` | Passed |
| Pinned data checksums, counts, relationships | Passed; 79,314 entities |
| Migrations and repeated execution | Passed |
| Import, no-op repeat, replacement and injected rollback | Passed |
| All 75,061 street IDs and parent paths | Matched pinned source array ordering |
| Original API contracts | 60 cases; 55 exact, 5 approved broad-search comparisons |
| Original offline package | 20 exact fixtures covering all 12 functions |
| Live Docker HTTP contracts | All 60 passed |
| OpenAPI 3.1 and response schemas | Passed; 180 source/Go/live response checks |
| Runtime database permissions | Read allowed; INSERT/UPDATE/DELETE denied |
| GIN usage | Confirmed by PostgreSQL EXPLAIN ANALYZE |
| Multi-stage Docker images and CLI tools | Built and exercised successfully |
| Remote GitHub Actions | Not run; no push authorized |
| Live scrape / deployment | Not performed; outside delivery |

The API is available locally at `http://127.0.0.1:8085`; `.env` selects this host port while the container listens on 8080. PostgreSQL is loopback-bound at port 55432. Local passwords were generated separately and remain in ignored `.env` with restrictive permissions.

Initial environment issues were resolved without changing Lerd: Docker required refreshed group membership; the installed Docker credential helper could not decrypt saved credentials, so public images were fetched with an isolated empty client configuration in `/tmp/tz-docker-config`. The host has no gcc, so race tests ran in the compiler-equipped Docker test image. Normal host tests, static analysis and data validation also passed.

The retained search differences, deterministic ambiguous-slug policy, additional parent filters, and source limitations are documented in FEATURE_PARITY.md and DATA_PROVENANCE.md. No claim is made that PostgreSQL ranking is identical to SQLite FTS5 porter or that this pinned postal snapshot reflects today's official boundaries.

## Region-scoped checkout search

Verified on 2026-11-02 after adding migration 0002, optional `region_slug` on search/autocomplete, and optional `type` on autocomplete:

- Full Go/PostgreSQL suites and source contracts passed. The Docker suite also passed race detection, vet, formatting and data checks; migration timeout changes received an additional database/integration test and vet run.
- Tests validate containing-region mappings for every search document and exercise all four entity types, parent-ward street matches, repeated street names, partial prefix input, malformed/empty filters, and unknown regions.
- Existing populated application and test databases migrated successfully without reimporting the dataset or changing its fingerprint. Backfills use a two-minute statement budget rather than the HTTP read budget; the migration command has a five-minute overall deadline.
- Five new live checkout cases and all 60 legacy live API contracts passed. The Airport result produces exactly `37113, Airport, Ibadakuli, Shinyanga CBD, Shinyanga` when the existing postcode and full_path fields are joined.
- PostgreSQL EXPLAIN ANALYZE confirmed a BitmapAnd combining `search_document_gin` with `search_region_type_idx` for Shinyanga/Ibadakuli street search.
- The updated API image is running on the existing port 8085. No checkout UI, remote push, or deployment was added.

## Postcode-first response correction

Search and autocomplete now return the postcode prefix directly inside `full_path`, as requested, while retaining the separate `postcode` field. Regions/districts without postcodes keep their original paths. This explicitly approved presentation change is applied after querying, so it changes neither ranking nor stored search data. Original source fixtures are retained; contract comparisons apply only the approved prefix to expected search/suggestion paths.

PostgreSQL integration and source contract suites, vet, formatting, and OpenAPI validation passed after the correction. Regression checks cover the exact Airport path and postcode prefixes on street autocomplete/search results, while source comparisons cover ward postcodes and unchanged region paths. The corrected image was rebuilt for local port 8085.
