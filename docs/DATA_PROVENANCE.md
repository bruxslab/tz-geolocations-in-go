# Data provenance

- Reference: https://github.com/unctom/tz-locations
- Exact source commit: `d835ea318dbb17f789d4b7195b600e2e07e20f1e`
- Original ingestion source: https://www.tanzaniapostcode.com
- Snapshot files: `scraper/data/{regions,districts,wards,streets}.json` from that commit, copied byte-for-byte to this project's `data/`.
- The reference npm copies are byte-identical to the scraper copies.
- License: MIT; upstream copyright attribution is preserved in `LICENSE`.
- No live refresh was performed. Counts describe this snapshot, not an assertion about current official administrative boundaries.

## Verified snapshot

| Level | Records | SHA-256 of original JSON bytes |
|---|---:|---|
| Regions | 31 | `1efe90052c314274739ced9ad920b7f441896fc7cb28330d87c126c2d9f0318a` |
| Districts | 168 | `227df5cdbf4f78c68e1bf69b97e9390fef434ba830e06b6a32522f095cf00743` |
| Wards | 4,054 | `e3adfe6ad7e7f1b9ed26c0e1d656643ed7079ccc9170ebbc4974651cb2737be5` |
| Streets | 75,061 | `f8651b2c572ca70dff3642fff886cbe944641b4daec68166fb494cae9aea5ad9` |

The combined import fingerprint is `fc4fcb00d9b8f0d6c7b161cc589f93ae7de4cb144e14d8dc6199983452a56cd0`. It hashes filename, byte length and bytes in region/district/ward/street order. `data/manifest.json` records the source and individual checksums/counts; `go run ./cmd/validate` verifies them, structural validity, uniqueness, postcodes, and parent-qualified relationships.

There are 235 repeated ward slugs across parents, no duplicate ward slugs within a district, no globally ambiguous district slugs, and no duplicate street names within a ward in this snapshot. All ward/street postcodes are populated. Valid repetition is retained.

IDs reproduce the committed legacy seed: one-based array position within each entity type. The SQLite migration/seed was executed to verify counts and ID assignment before implementation. The source search seed has 31 region, 168 district and 4,054 ward documents, but no streets. This implementation deliberately adds all 75,061 street documents; expected total search documents are **79,314**.

## Source limitations and refresh policy

The Python scraper combines region HTML with gzip sitemaps, infers hierarchy using longest matching prefixes, derives some display names from slugs, deduplicates parent-qualified records, and sorts the result. It logs validation warnings and still writes outputs. This project retains it unchanged as a reference tool; the Go importer independently enforces integrity and aborts invalid imports. We do not correct names, infer new records, or equate source postal data with an official current administrative registry.

A future refresh requires review of all four files, counts, checksums, parent relationships, source IDs, API/offline compatibility, and updated contract fixtures as appropriate. Never edit the Go results to make a mismatching snapshot appear compatible. A changed dataset is rejected by the importer until explicit `--replace`; the original remains intact until the new import commits. External replacement can change IDs when array ordering changes, and does not update the offline package without a rebuild.
