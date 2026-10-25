# 3. Compiled Go extensions plus Script Commands

Status: accepted (2026-10-25)

## Decision

Every feature, built-in or not, is a Go package implementing `sdk.Extension`, listed in
`extensions/builtin.go`. A fork adds a feature by adding a package and one line. Script Commands
(any executable with metadata comments) cover users who do not want to compile.
Runtime-installable extensions (WASM via wazero) are deferred to v2.

## Consequences

- Built-ins use only the public `sdk`, so the API is proven by real features.
- Extension code runs in-process with full trust; acceptable because it is compiled into the
  user's own build. Script Commands run as child processes.
