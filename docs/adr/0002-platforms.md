# 2. Windows 11 first, Linux in v1.1

Status: accepted (2026-10-25)

## Decision

v1 targets Windows 11 x64. Windows 10 runs with a solid translucent fallback instead of Acrylic.
Linux ships in v1.1, but OS code sits behind `internal/platform` interfaces from the first commit
and CI builds and tests on Linux on every push.

## Consequences

- Visual polish (glass, focus, hotkey) is perfected on one platform before the next.
- Linux needs adapters, not refactoring. Not every Linux desktop blurs behind windows, and the
  bare Super key belongs to the desktop, so onboarding offers combinations only there.
