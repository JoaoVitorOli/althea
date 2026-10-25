# 1. Go and MyGo for the whole app

Status: accepted (2026-10-25)

## Context

Althea started in Rust + Slint and moved to Go + MyGo on 2026-10-05. Before v1 we compared them
again against the v1 needs: Windows then Linux, GPU UI at 60 fps, runtime themes, glass, global
hotkeys, tray, installers and updates, and a codebase forks can extend easily.

## Decision

Build everything in Go with MyGo v0.2.x native UI (no webview). MyGo ships the desktop APIs a
launcher needs (global shortcuts including the Wayland portal, tray, single instance, start at
login, installers, signed delta updates), a runtime-switchable `ui.Theme`, headless view tests
and a cross-platform glass shader.

## Consequences

- MyGo is young and has one maintainer: the version is pinned, the core never imports it
  (depguard), and we are ready to carry a fork with fixes.
- Go keeps contributions approachable for people forking Althea.
- If MyGo fails a hard requirement, only `internal/ui`, `internal/shell` and the platform
  adapters change.
