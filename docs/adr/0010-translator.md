# 10. Offline translation with native Bergamot downloaded on demand

Status: accepted (2026-10-25)

## Decision

As in Latte, translation uses Bergamot with `tiny` models. CI builds the native
`bergamot-translator` for each platform; turning the translator on downloads the engine and the
chosen models, verified by sha256, and runs it as a persistent subprocess over stdio. Online
providers are optional.

## Consequences

- The app binary stays pure Go and small.
- We maintain a small build of a C++ project in CI.
