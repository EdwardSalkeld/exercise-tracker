# Workout Service

Go HTTP service for exercise data backed by Postgres.

This is the first cut of moving `workout-data` away from direct local SQLite
access and into its own service. The v1 scope is intentionally narrow:

- Postgres-backed canonical tables for workouts, exercise sets, runs, and run points
- Simple API for recent workouts, workout detail, exercise history, recent runs, run detail, and direct creates
- Health endpoint that also checks database reachability

The initial write surface is deliberately direct and private-network friendly:
nested POST requests for workouts and runs, without auth, background jobs, or
import-specific abstractions yet.

The repo now also contains the first tracker-owned Hevy sync path:

- `sync-hevy` reads the Hevy API directly
- it stores the incremental resume marker in Postgres `sync_state`
- it reconciles Hevy-sourced workouts by `external_id`
- it follows the current versioned update contract, where `PUT` creates a new
  active row and the old row is soft-deleted

## Endpoints

- `GET /healthz`
- `GET /v1/workouts?limit=20`
- `GET /v1/workouts/{id}`
- `POST /v1/workouts`
- `PUT /v1/workouts/{id}`
- `DELETE /v1/workouts/{id}`
- `GET /v1/exercises/{baseName}/history?limit=50`
- `GET /v1/runs?limit=20`
- `GET /v1/runs/{id}`
- `POST /v1/runs`
- `PUT /v1/runs/{id}`
- `DELETE /v1/runs/{id}`

## Write Payloads

`POST /v1/workouts` accepts the top-level workout fields plus nested
`exercises` and `sets`. `order_index` and `set_number` are optional; if omitted
they are assigned sequentially.

`PUT /v1/workouts/{id}` uses a delete-and-recreate flow: the existing workout is
soft-deleted and a new workout row is created from the supplied payload. The
response contains the replacement workout with its new ID.

`POST /v1/runs` accepts the canonical run fields plus optional nested `points`.
`point_index` is optional and is also assigned sequentially when omitted.

`PUT /v1/runs/{id}` behaves the same way: soft-delete the old run, create a new
row, and return the replacement run.

## Configuration

Environment variables:

- `EXERCISE_TRACKER_DATABASE_URL` required, Postgres connection string
- `EXERCISE_TRACKER_LISTEN_ADDR` optional, default `:8080`
- `EXERCISE_TRACKER_READ_TIMEOUT` optional duration, default `5s`
- `EXERCISE_TRACKER_WRITE_TIMEOUT` optional duration, default `10s`

## Local Run

Start Postgres:

```sh
docker compose up -d postgres
```

Apply the initial schema:

```sh
for migration in sql/migrations/*.sql; do
  psql "$EXERCISE_TRACKER_DATABASE_URL" -f "$migration"
done
```

Run the service:

```sh
export EXERCISE_TRACKER_DATABASE_URL=postgres://workout:workout@127.0.0.1:5432/exercise_tracker?sslmode=disable
go run ./cmd/exercise-tracker
```

Run a Hevy sync manually:

```sh
export EXERCISE_TRACKER_HEVY_API_KEY=...
go run ./cmd/exercise-tracker sync-hevy
```

Useful flags:

- `--full` to ignore the stored marker and reconcile from the full Hevy workout list
- `--since <timestamp>` to force an events sync from a specific ISO 8601 timestamp
- `--page-size <n>` to adjust the Hevy API page size

Run formatting and tests:

```sh
go fmt ./...
./scripts/fmt-check.sh
go test ./...
```

## Notes

- The schema is shaped for migration from the current `workout-data` tables, but
  does not try to preserve every import-stage artifact.
- `source_type`, `source_ref`, `external_id`, and `raw_payload` are included so
  we can retain provenance from Hevy text imports, Hevy API sync, and FIT runs.
- Open questions and v1 decisions are documented in [docs/architecture.md](docs/architecture.md).
