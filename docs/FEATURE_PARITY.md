# Feature parity specification

Baseline: [unctom/tz-locations](https://github.com/unctom/tz-locations/tree/d835ea318dbb17f789d4b7195b600e2e07e20f1e), commit `d835ea318dbb17f789d4b7195b600e2e07e20f1e`.

## Audit completed before business implementation

Reviewed all eight route modules, root routing, both response/validation helpers, cache/CORS/rate middleware, Drizzle schema, SQL migration, seed generator and generated seed, API tests, npm implementation/types/tests/docs, scraper/parser/tests, data validator, CI, manifests, and documentation. The scraper and npm copies of all four JSON files are byte-identical. Executing the committed migration and seed with SQLite confirms the JSON counts and source ID assignment (one-based array order, separately per entity).

| Behavior | Source contract and Go target | Verification |
|---|---|---|
| `GET /` | Unwrapped discovery object, original version/name/description/endpoints, docs link `/v1/docs` | Verified: source fixtures, Go handlers and live Docker HTTP |
| `/v1/regions` | Name-ordered list; id/name/slug/district_count/links; meta.total | Verified: source fixtures, Go handlers and live Docker HTTP |
| `/v1/regions/{slug}` | Region plus district/ward/street counts and links | Verified: source fixtures, Go handlers and live Docker HTTP |
| `/v1/regions/{slug}/districts` | Name-ordered districts with ward_count, region, links; meta.total/region | Verified: source fixtures, Go handlers and live Docker HTTP |
| `/v1/districts/{slug}` | District with ward/street counts, region and links | Verified: source fixtures, Go handlers and live Docker HTTP |
| `/v1/districts/{slug}/wards` | Name-ordered wards with postcode, street_count, parents and links; meta.total/district | Verified: source fixtures, Go handlers and live Docker HTTP |
| `/v1/wards/{slug}` | Ward with postcode, street_count, parents and links | Verified: source fixtures, Go handlers and live Docker HTTP |
| `/v1/wards/{slug}/streets` | Name-ordered streets with postcode, parents and links; limit=100, max=500, offset=0; meta.total is **page size**, plus ward/limit/offset | Verified: source fixtures, Go handlers and live Docker HTTP |
| `/v1/streets/{id}` | Stable numeric ID, postcode, full parents and links; invalid IDs yield 404 `Invalid street ID` | Verified: source fixtures, Go handlers and live Docker HTTP |
| `/v1/postcodes/{postcode}` | Exactly five ASCII digits; first matching ward plus all name-ordered streets; missing=404 | Verified: source fixtures, Go handlers and live Docker HTTP |
| `/v1/search` | q required; type optional (region/district/ward/street); limit=10, max=50; name/type/postcode (nullable)/full_path/entity_id; meta.total/query | Verified: exact fixtures plus approved search-difference and integration assertions |
| `/v1/autocomplete` | q min two UTF-16 code units before sanitization; limit=5, max=20; prefix results without entity_id or meta | Verified: exact fixtures plus approved search-difference and integration assertions |
| `/v1/stats` | Four exact counts, no meta | Verified: source fixtures, Go handlers and live Docker HTTP |
| Success/error envelopes | `{data,meta?}`; `{error:{code,message}}`; BAD_REQUEST/NOT_FOUND/RATE_LIMITED/INTERNAL_ERROR | Verified: unit and contract suites |
| Validation | Slug `[a-z0-9-]+`, max 100; postcode 5 digits; search strips non-ASCII-alphanumeric/non-whitespace and trims; source numeric defaults/clamping | Verified: validation units and source contracts |
| Middleware | CORS wildcard default; nosniff; cache public/max-age=86400 on successful v1 responses; 100 requests/60-second fixed window with three X-RateLimit headers | Verified: middleware unit/race tests and live headers |
| Offline getters | All 12 exports: GetRegions, GetRegion, GetDistricts, GetDistrictsByRegion, GetDistrict, GetWards, GetWardsByDistrict, GetWard, GetStreets, GetByPostcode, SearchLocations, GetStats | Verified: getter/search/copy/concurrency tests |
| Offline semantics | Original JSON order; first slug match; optional region for districts; wards filter only with both parents; postcode ward derived from first matching street; case-insensitive name substring search in region/district/ward/street order, default limit=10 | Verified: 20 original npm function fixtures covering all twelve exports |
| Import/data | 31 / 168 / 4,054 / 75,061; unchanged strings/relationships; deterministic IDs; transactional repeatable importer replaces unsafe SQLite seed execution | Verified: real PostgreSQL migration/import/rollback/ID/constraint/index tests |
| Scraper | Retained Python reference: async retry/backoff, checkpoints/resume, region HTML and gzip sitemaps, longest-prefix hierarchy inference, parent-aware deduplication, deterministic sorting, validation and flat/nested JSON export | Source retained; no live scraping planned |

## Approved compatibility decisions

- There are 235 globally ambiguous ward slugs; no duplicate parent-qualified ward keys or globally ambiguous district slugs in this snapshot. Select the lowest preserved source ID for unqualified requests. Optional `region_slug` (district routes) and `region_slug`/`district_slug` (ward routes) disambiguate without changing existing calls.
- Preserve original link strings for legacy calls. Qualified requests carry resolved parent context on district/ward links so following them retains context, including ward names repeated within one region.
- The seed generator indexes regions/districts/wards only: street search is advertised but absent. Index streets as the approved bug fix. PostgreSQL full-text rank/tokenization differs from SQLite FTS5 porter; document and test those differences rather than claim byte-identical search ranking.
- SQLite searches all indexed fields, including full_path and postcode. PostgreSQL will index those searchable text fields with native full-text vectors; query syntax remains safely constructed from sanitized text.
- The npm package uses substring name matching, independent of API full-text search. Preserve both contracts. Go returns copies and uses concurrency-safe indexes instead of exposing mutable shared arrays.
- The source validators.ts Zod schemas are unused by routes; executable route behavior is authoritative. Extremely large inputs/non-finite pagination are rejected safely; fractional limits/offsets produce sanitized server errors as SQLite does. Deterministic ID tie-breaks replace unspecified ordering ties.
- Source root advertises `/v1/docs` but never implements it. Provide OpenAPI documentation at that URL as an additive fix.
- Source tests mock empty D1 results and do not prove data parity; this project must execute source handlers against the committed SQLite seed to generate contract fixtures.
- Source seed warns and skips missing parents, while validation checks several references without full parent context. Go rejects all such integrity errors; it never silently skips records.

## Delivery boundaries

Docker Compose for the Go app and dedicated PostgreSQL explicitly supersedes the original Lerd database requirement and application-container exclusion. No existing Lerd service, database, or global setting is changed. Local commits only; remote CI execution and deployment are outside this delivery.

## Verification record — 2026-11-02

- `go test ./...` passed with `REQUIRE_DATABASE_TESTS=1` against dedicated Docker PostgreSQL; no database suite was skipped.
- `go test -race ./...` passed with cgo in the compiler-equipped Docker test image, including both PostgreSQL suites.
- `go vet ./...`, formatting checks, `go mod verify`, and `go run ./cmd/validate` passed.
- 60 original API fixtures were generated by executing the pinned Hono handlers with the actual committed SQLite migration/seed; 55 compare exact canonicalized JSON and five broad-search cases permit only the documented search changes. All 60 also passed against the running Docker API.
- 20 source npm fixtures cover all twelve public functions; exact offline comparisons passed.
- OpenAPI 3.1 validation passed, along with 180 response-schema checks (60 source, 60 Go-handler captures, 60 live HTTP captures).
- Fresh migrations/import, no-op repeat import, atomic replacement, failure rollback after writes began, parent-qualified relationships, all 75,061 street IDs, search across every level, and complete index counts were verified.
- Live runtime privilege checks confirmed CONNECT/USAGE/SELECT and denied INSERT/UPDATE/DELETE. The migration/import binaries also ran successfully inside their non-root images.
- `EXPLAIN (ANALYZE, BUFFERS)` confirmed a Bitmap Index Scan using `search_document_gin` for a street query; no benchmark infrastructure was added.
- Multi-stage runtime and compiler/test Docker images built successfully. The local API uses host port 8085 (container port 8080).
- GitHub Actions configuration is committed; remote execution is **unverified** because no branch was pushed. Live scraping and production deployment were not performed.
