# 8. OS vault for secrets, encryption at rest for personal data

Status: accepted (2026-10-25)

## Decision

API keys are stored in Windows Credential Manager (libsecret on Linux). Clipboard history and notes
are encrypted in SQLite with AES-GCM using a random key kept in the vault. Clipboard capture skips
content flagged by password managers and apps on an ignore list, and retention is configurable.

## Consequences

- No master password slows down opening the launcher.
- Data copied off the machine without the vault key is unreadable.
