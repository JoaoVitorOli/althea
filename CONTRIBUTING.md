# Contributing

Thanks for helping with Althea. Read [docs/spec.md](docs/spec.md) and
[docs/architecture.md](docs/architecture.md) first: they explain what goes where and why.

## Setup

Requires Go 1.27+. Every tool is pinned in a `go.mod`, so there is nothing to install globally.

```bash
go tool lefthook install                                 # git hooks: format, lint, commit messages
go tool mygo dev                                         # run with live reload
go test -race ./...                                      # tests (the race detector needs cgo)
go tool -modfile=tools/go.mod golangci-lint fmt ./...    # format
go tool -modfile=tools/go.mod golangci-lint run ./...    # lint
go tool -modfile=tools/go.mod govulncheck ./...          # known vulnerabilities
```

To lint for Linux from Windows, build the linter for your system first, then point it at Linux:

```bash
go build -modfile=tools/go.mod -o bin/golangci-lint.exe github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOOS=linux bin/golangci-lint.exe run ./...
```

## Conventions

- **Tests first.** Write a failing test for the behavior, make it pass, then refactor. Test
  through public interfaces; prefer real implementations and `fake` platform services to mocks.
- **Layering.** `depguard` enforces the dependency rules in the architecture doc: the core never
  imports MyGo, and extensions never import `internal/`.
- **Commits** follow [Conventional Commits](https://www.conventionalcommits.org/):
  `<type>(<scope>): <description>`, with types `feat fix docs style refactor perf test build ci
  chore revert`. The commit-msg hook checks it.
- **Code and docs are in English.** User-facing strings go through the i18n catalogs.
- **Comments** explain why, not what. Every exported name has a doc comment.
