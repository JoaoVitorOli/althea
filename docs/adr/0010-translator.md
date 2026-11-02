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

## How it is built (2026-11-02)

- Upstream's program reads stdin to its end, so [`tools/bergamot/main.cpp`](../../tools/bergamot/main.cpp)
  takes its place, with a small protocol of its own, and
  [`.github/workflows/bergamot.yml`](../../.github/workflows/bergamot.yml) builds it by hand. It links
  OpenBLAS rather than upstream's MKL, for the license. Its size and sha256 are pinned in
  `extensions/translator/engine.json`, and checked again each time it starts.
- Models come from Mozilla's attachments CDN, as Latte's do: the GitHub repository's LFS objects are
  gone. [`tools/bergamotmodels`](../../tools/bergamotmodels/main.go) pins the `tiny` 1.0 records in
  `extensions/translator/models.json`. Pairs with no model of their own go through English.
- Those records have no Japanese, Korean, Chinese, Arabic or Thai, whose models are only larger
  ones, and some languages go one way only. An online provider covers them.
- Marian opens files with Windows' ANSI functions, which fail on a path with an accent. The engine
  runs in the models folder and its configs use relative paths.
- The engine stops after 5 minutes without use, and when Althea quits or its stdin closes.
