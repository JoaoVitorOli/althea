# 12. No telemetry; network only on explicit action

Status: accepted (2026-10-25)

## Decision

Althea collects no telemetry. All HTTP goes through `internal/netpolicy`, which allows a request
only for a feature the user enabled or an action they took, and logs it. Update checks are opt-in
in onboarding. Crash reports stay local.

## Consequences

- Users can audit every request in the log.
- No usage data to guide decisions; we rely on issues.
- Typing counts as asking when an answer needs data for what was typed, as the Calculator's
  exchange rates: an `Answer` may fetch it with `ctx.Go`, never sending the query (2026-11-06).
