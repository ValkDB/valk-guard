# Contributing to Valk Guard

Thanks for your interest in contributing!

## Getting Started

1. Fork the repo and clone your fork
2. Ensure Go 1.25.6+ is installed
3. Clone [valk-postgres-parser](https://github.com/ValkDB/valk-postgres-parser) alongside this repo (the `go.mod` `replace` directive expects `../valk-postgres-parser`)
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

1. Create `rules/vgXXX_rule_name.go` implementing the `Rule` interface
2. Register it in `rules/registry.go` via `DefaultRegistry()`
3. Add tests in `rules/vgXXX_rule_name_test.go`
4. Add a test fixture in `testdata/` if helpful
5. Document the rule in README.md

Detailed guide: see [`docs/adding-rules.md`](docs/adding-rules.md).

## Adding a New Scanner

1. Implement the `scanner.Scanner` interface in `scanner/<name>_scanner.go`
2. Reuse directive parsing/suppression mapping for consistent behavior
3. Register it in `configuredScanners()` in `cmd/valk-guard/main.go`
4. Add scanner tests and fixtures under `scanner/` and `testdata/`

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
