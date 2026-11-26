# Validation

All database exercises used a dedicated local Docker PostgreSQL 17 database and synthetic data. No production store, provider or user data was accessed. Authentication was exercised with locally signed test tokens. The full-domain HTTP scenarios use real HTTP handlers, the real queue/worker and PostgreSQL as explicitly required by the task.

## Executed checks

- Fresh-schema migration and repeat migration.
- Immutable import, checksum/key/reference/snapshot validation, rollback of invalid imports, prepared order/route/reservation restoration.
- Import refuses incomplete original dataset snapshots. `expected.json` never supplies domain state.
- Stock/capacity atomicity, physical versus salable stock, local issue, whole-basket reservations and last-unit concurrency.
- Mesh/hub/hybrid planning, cutoffs, stop order, per-leg transport capacity, timezone/DST and point opening hours, candidate origin index queries.
- Order versions, shipment and segment ownership, route revision preservation, simulated action replay, cancellation resource release.
- Returns, quarantined stock, partial financial reversals, incidents and reroutes, long storage, deferral, actual transfer and slot release.
- HTTP E2E: delivery, local issue, mixed basket, race/conflict, partial damaged return, incident, storage threshold and hub transfer, run isolation, JWT roles/delegation, cursors and response schema checks.
- Worker crash injected after domain changes and before completion: rollback preserves stock; retry produces exactly one issue event.
- Exact monetary int64 persistence above 2^53, integer overflow rejection and test DSN isolation.
- `go test -race ./... -count=1`, `go vet ./...`, `go build ./cmd/mesh`.
- Actual API and worker processes plus `python3 scripts/demo_order.py`: order issued, 4/5 units remain, 8 ledger entries. `/healthz` returned 200.

## Measured slice

Generated deterministic synthetic CSV slice: 150 pickup points, 20,000 products, 20,000 batches, 2,990,191 input bytes. Local macOS arm64 host, Go 1.27.4, Docker PostgreSQL 17, fixed model clock. Import uses bounded batches of 512 records. These are single observed runs, not production latency guarantees. No federal complete dataset was available with sufficient input snapshots.

| Operation | Observed elapsed time |
|---|---:|
| Import | 30.343082292 s |
| New clean run | 5.198750 ms |
| Quote | 22.567291 ms |
| Baseline rows copied for run | 0 |

Reproduce from `backend/` with the isolated test DSN configured:

```sh
MESH_RUN_SCALE=1 go test ./internal/core -run '^TestImportLargeSlice$' -count=1 -v
```

The test generates the large inputs in a temporary directory; no generated megabytes are committed. Existing dataset source files and expected outputs remain unchanged. Parquet files are absent from the supplied ready data; unsupported file formats return explicit import errors. There is no claim of verified federal-scale throughput.

## Final verification boundary

The full race-enabled suite passed before the last arithmetic, transport-accounting, simulation-identity and import-validation refinements. The domain refinements received focused passing tests from their owners. The user explicitly requested no further test runs, so the final combined committed snapshot was not rerun as a whole. The last attempted full run was rejected before execution.
