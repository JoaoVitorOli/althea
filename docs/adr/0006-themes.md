# 6. Themes as full token sets in TOML

Status: accepted (2026-10-25)

## Decision

A theme has one appearance (`light` or `dark`) and sets colors, material (acrylic, mica, glass,
solid; opacity), radii, spacing, font and a motion scale. Themes can `extends` another theme; a
user's theme with a built-in theme's name shadows it. Settings pick a theme by name, or `system`
to follow the OS between a light and a dark theme. Files are hot-reloaded; a
visual editor writes them in M7. Layout is not themeable.

## Consequences

- Rich customization with one layout to test.
- Tokens are part of the public contract and are versioned (`schema = 1`).
