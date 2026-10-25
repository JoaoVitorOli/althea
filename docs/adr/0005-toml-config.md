# 5. TOML settings with a schema and hot-reload

Status: accepted (2026-10-25)

## Decision

Settings live in `%APPDATA%\Althea\settings.toml`, commented and validated against a schema built
from the host's and each extension's typed preferences. The settings screen writes the same file;
saving the file by hand reloads it. Secrets are never written to it.

## Consequences

- Power users can version their settings; the GUI and the file never disagree.
- Writing the file must preserve the user's comments and order where possible.
