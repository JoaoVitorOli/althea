# 14. Strict lint, TDD and continuous safety guards

Status: accepted (2026-10-25)

## Decision

- Formatting: gofumpt and gci. Linting: golangci-lint v2 with staticcheck, gosec, errcheck, revive,
  gocritic, govet, bodyclose, noctx and depguard.
- Git hooks with lefthook: format and lint before commit, Conventional Commits on the message.
- CI on Windows and Linux: lint, `go test -race`, benchmarks against budgets, govulncheck.
- Tests are written first, in vertical slices; core coverage of at least 80 %.
- Every test binary checks for leaked goroutines; store tests assert query counts.

## Consequences

- Slower first commits, far fewer regressions; the M8 audit starts from a clean baseline.
