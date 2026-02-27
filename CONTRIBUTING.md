# Contributing to Valk Guard

Thanks for your interest in contributing!

## Getting Started

1. Fork the repo and clone your fork
2. Ensure Go 1.25.6 is installed
3. Ensure `python3` is available (required for SQLAlchemy scanner tests)
4. Run `make check` to verify everything builds and passes

## Development Workflow

```bash
make build        # build the binary
make test         # run tests with race detector
make lint         # run golangci-lint
make check        # all of the above
```

## Submitting Changes

1. Create a branch from `main`
2. Make your changes
3. Add or update tests as needed
4. Run `make check` — all checks must pass
5. Open a pull request with a clear description of the change

## Adding a New Rule

1. Create `internal/rules/vgXXX_rule_name.go` implementing the `Rule` interface
2. Register it in `internal/rules/registry.go` via `DefaultRegistry()`
3. Add tests in `internal/rules/vgXXX_rule_name_test.go`
4. Add a test fixture in `testdata/` if helpful
5. Document the rule in README.md

Detailed guide: see [`docs/adding-rules.md`](docs/adding-rules.md).

## Adding a New Scanner

1. Create a subfolder under `internal/scanner/` (e.g. `internal/scanner/myorm/`) with `myorm_scanner.go`
2. Implement the `scanner.Scanner` interface
3. Use shared helpers from `internal/scanner/goast.go` (for Go-based scanners) and `scanner.DisabledRulesForLine` for directive support
4. Register it in the scanner list inside `collectAndAnalyze()` in `cmd/valk-guard/main.go`
5. Add scanner tests in `internal/scanner/myorm/myorm_scanner_test.go` and fixtures under `testdata/`

Existing scanners in subfolders: `internal/scanner/goqu/`, `internal/scanner/sqlalchemy/`.

Detailed guide: see [`docs/adding-scanners.md`](docs/adding-scanners.md).

## Code Style

- Follow standard Go conventions (`gofmt`, `goimports`)
- All exported types and functions need doc comments
- Tests use the standard `testing` package (no external test frameworks)

## Reporting Issues

Open an issue on GitHub with:
- What you expected to happen
- What actually happened
- Steps to reproduce
- Go version and OS

## License

By contributing, you agree that your contributions will be licensed under the Apache 2.0 License.
