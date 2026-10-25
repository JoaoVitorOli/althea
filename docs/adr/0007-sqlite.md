# 7. One SQLite database through a pure Go driver

Status: accepted (2026-10-25)

## Decision

Use `modernc.org/sqlite` (no cgo, keeping MyGo's cross-compilation), WAL mode, FTS5, numbered
embedded migrations, and per-extension namespaces.

## Consequences

- Somewhat slower than the cgo driver, irrelevant at launcher data sizes.
- A query-counting wrapper in tests guards against N+1 access.
