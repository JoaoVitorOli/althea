# 4. Declarative extension views with a custom escape hatch

Status: accepted (2026-10-25)

## Decision

Extensions return view values (List, Grid, Detail, Form, actions) that the host renders.
`sdk/custom` offers a view drawn with MyGo's `ui` package for the rare cases a declarative view
cannot express, such as the theme editor or a color picker.

## Consequences

- Theme, materials, motion and keyboard behavior are consistent everywhere and improve in one place.
- MyGo's API leaks only into extensions that opt into `sdk/custom`.
