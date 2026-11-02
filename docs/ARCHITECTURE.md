# Architecture

The HTTP service is a single Go process with `net/http`, `slog`, and a bounded `pgxpool` connection pool. The only runtime dependency outside the standard library is pgx and its transitive dependencies. No ORM, cache service, metrics platform, or authentication service is introduced.

## Responsibilities and request flow

`cmd/api` loads validated environment configuration, verifies connectivity, installs routes/middleware, and manages signal-driven shutdown. Middleware handles CORS/preflight, security headers, a bounded fixed-window per-client limiter, request body limits, panic recovery, and request deadlines. Handlers validate inputs, call repositories, and serialize the legacy response shapes. Domain input conversion lives in `internal/service`, safe full-text inputs in `internal/search`, and typed database read models in `internal/models`.

Repositories issue parameterized, context-aware PostgreSQL queries. Hierarchy lists use joins or indexed correlated aggregates in bounded round trips, not a per-record query loop. Region/district/ward list responses are intentionally complete, matching the finite source hierarchy. Street pages and searches are bounded; postcode results remain complete as required by the original contract. Name sorting uses PostgreSQL `COLLATE "C"` and ID tie-breaks to reproduce SQLite's byte ordering and deterministic ties.

## Schema and imports

All dataset tables live in `locations`. Regions have globally unique slugs; district and ward slugs are unique only under their parents; street names are unique within a ward. Foreign keys protect hierarchy and search-document references. Postcodes stay five-character strings. Parent/slug/postcode/name indexes serve lookups and lists. Search documents have one row per entity, a generated `simple` tsvector and a GIN index. A type index helps filtered search. Trigram indexes are unnecessary for the approved API phrase/prefix behavior; offline substring matching does not query PostgreSQL.

`cmd/migrate` applies embedded versioned SQL with checksums and a session advisory lock; each migration and its record commit together. The migration ledger is the dedicated `public.tz_locations_migrations` table. `cmd/import` validates all JSON before writing, takes a transaction advisory lock, performs COPY in parent order, builds all search documents, checks counts/index coverage, records provenance, analyzes tables, and commits. Explicit replacement truncates only the listed application tables inside that same transaction. Failure restores the previous dataset. A healthy identical fingerprint is a no-op; invalid count/index verification requires explicit replacement to repair.

The database owner performs migrations/imports. The runtime role has CONNECT, schema USAGE and table SELECT only, with read-only transactions by default. Compose's administrator initializes roles/databases; owner credentials are not supplied to the API container. The HTTP service never writes schema or data.

## Offline access

The `data` package embeds the four unmodified JSON files and a checksum/count manifest. The offline package lazily validates them once, constructs immutable parent/postcode indexes, and returns copies. It has no PostgreSQL or HTTP dependency. Source ordering and substring search behavior remain separate from API ordering and full-text search. External imports do not mutate the embedded snapshot; update/rebuild it deliberately to move both interfaces to a new dataset release.

## Operational boundaries

Compose has dedicated PostgreSQL, API, migration/import tools, and a compiler-equipped test target. API images use a non-root distroless runtime and no writable filesystem. PostgreSQL uses a named volume and loopback host port; this project never accesses Lerd. Production connection details come entirely from environment variables; database TLS is deployment-specific. Quotas are per process and reset on restart. No remote deployment or remote CI run is part of local delivery.

Database tests verify the target database name twice, serialize suites with their own advisory lock, and operate exclusively on `tz_locations_test`. Contract fixtures come from original TypeScript handlers and SQLite, not a reconstructed Go oracle. OpenAPI response validation uses the same fixtures and captures from actual Go handlers. No benchmarking or load-test infrastructure is included.
