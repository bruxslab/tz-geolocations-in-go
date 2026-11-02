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
