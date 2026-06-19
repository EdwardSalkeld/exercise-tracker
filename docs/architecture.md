# Exercise Tracker v1

## Goal

Extract session storage and query access into a dedicated Go service backed by
Postgres, while keeping the first implementation small enough to start using and
reviewing immediately.

## Chosen v1 Shape

- Direct HTTP API with reads plus simple create endpoints
- Canonical relational schema for:
  - sessions
  - session exercises
  - exercise sets
  - runs
  - run points
- Source provenance fields on top-level records so existing SQLite data can be
  migrated without losing where it came from

## Why The First Write Cut Stays Simple

The current SQLite exercise data tooling already has a clear query surface:

- recent sessions
- session summary/detail
- exercise history
- runs and run points

What is still not fully settled is the long-term write path:

- keep ingesting Hevy text and FIT files locally, then push to the service
- ingest directly into the service
- sync Hevy API into the service on a schedule

The first write cut therefore stays deliberately narrow:

- direct `POST /v1/sessions` with nested exercises and sets
- direct `POST /v1/runs` with optional run points
- no auth, queueing, importer-specific endpoints, or update/delete semantics yet

That keeps the service usable for a single trusted LAN-facing client without
locking the later importer story too early.

## Open Questions

1. Should the long-term write path be service-native ingestion endpoints, or
   should imports stay in a separate worker that writes to Postgres?
2. Do we want one canonical session model only, or should the service preserve
   a distinction between text-imported sessions and API-backed sessions?
3. Should run GPS points stay inline in Postgres long-term, or move to a
   separate store if route volume grows materially?
4. Should auth be required from day one, or is private-network access enough for
   the first deployment?

## Likely Next Steps

1. Lock the repo name and deployment target.
2. Decide whether update/delete belongs in the service or should wait for a
   richer client.
3. Build the SQLite-to-Postgres migration once the canonical mapping is agreed.
