# 6. Themes as full token sets in TOML

Status: accepted (2026-10-25)

## Decision

A theme sets colors (light and dark), material (acrylic, mica, glass, solid; blur, opacity, tint),
radii, spacing, density, fonts and a motion scale. Themes can `extends` another theme. Files are
hot-reloaded; a visual editor writes them in M7. Layout is not themeable.

## Consequences

- Rich customization with one layout to test.
- Tokens are part of the public contract and are versioned (`schema = 1`).
