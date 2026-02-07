# Valk Guard

[![Build Status](https://img.shields.io/github/actions/workflow/status/ValkDB/valk-guard/ci.yml?branch=main)](https://github.com/ValkDB/valk-guard/actions)
[![Go Version](https://img.shields.io/badge/Go-1.25.6+-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

**Open-source database performance linter for CI/CD. Statically analyzes SQL in source code and flags performance anti-patterns before they hit production.**

---

> **Early Development** -- Core linting is active with built-in rules `VG001` through `VG008`. Contributions for additional rules and scanners are welcome.

---

## What It Does

Valk Guard scans your codebase for SQL -- both raw `.sql` files and SQL strings embedded in Go source code. Each SQL statement is parsed into structured metadata using [valk-postgres-parser](https://github.com/ValkDB/valk-postgres-parser) (an ANTLR4-based PostgreSQL grammar), then checked against a set of lint rules that catch common performance and safety anti-patterns. Findings are reported as colored terminal output, JSON, or SARIF 2.1.0 for integration with GitHub Code Scanning.

The goal: catch `SELECT *`, missing `WHERE` clauses, unbounded queries, destructive DDL, and other database footguns in CI before they reach production.

## Features (What We Support Today)

The CLI scaffold, scanning pipeline, reporting layer, and initial rules are implemented:

- **Raw SQL file scanning** (`.sql`) with full awareness of quoted strings, dollar-quoting, line comments (`--`), and block comments (`/* */`)
- **Go source scanning** -- extracts SQL string literals from `db.Query`, `db.Exec`, `db.QueryRow`, `db.Prepare`, and other common database method calls via `go/ast`
- **PostgreSQL dialect parsing** via [valk-postgres-parser](https://github.com/ValkDB/valk-postgres-parser) (ANTLR4 grammar)
- **Three output formats**: terminal (colored), JSON, SARIF 2.1.0
- **Inline disable directives** -- `-- valk-guard:disable VG001` in SQL, `// valk-guard:disable VG001` in Go
- **Per-rule configuration** via `.valk-guard.yaml` (enable/disable individual rules, override severity)
- **File exclusion patterns** with glob support (including `**` for recursive matching)
- **`--verbose` mode** for debugging scanner and parser behavior
- **`--output` flag** to write results directly to a file
- **Exit codes**: `0` = clean, `1` = findings, `2` = config/runtime error

**Current status:** Rules `VG001` through `VG008` are implemented.

## Installation

Build from source (requires Go 1.25.6+):

```bash
git clone https://github.com/ValkDB/valk-guard.git
cd valk-guard
make build        # produces ./valk-guard binary
make install      # installs to $GOPATH/bin
```

## Quick Start

```bash
# Scan the current directory
valk-guard scan .

# Scan specific paths
valk-guard scan ./sql/ ./internal/

# Output as JSON
valk-guard scan . --format json

# Output as SARIF (for GitHub Code Scanning)
valk-guard scan . --format sarif

# Use a custom config file
valk-guard scan . --config .valk-guard.yaml

# Enable verbose output for debugging
valk-guard scan . --verbose

# Write results to a file
valk-guard scan . --format sarif --output results.sarif
```

## How It Works

```
valk-guard scan [paths...]
        |
        v
+-------------------------+     Finds SQL in:
|  Scanner                |     - *.sql files (RawSQLScanner)
|                         |     - Go source (GoScanner via go/ast)
+--------+----------------+
         |
         v
+-------------------------+
|  Parser Engine          |     Parses each SQL statement into
|  (valk-postgres-parser) |     structured metadata
+--------+----------------+
         |
         v
+-------------------------+
|  Rule Engine            |     Checks parsed metadata against
|  (VG001-VG008 active)   |     enabled lint rules
+--------+----------------+
         |
         v
+-------------------------+     Output formats:
|  Reporter               |     - Terminal (colored)
|                         |     - JSON
|                         |     - SARIF 2.1.0
+-------------------------+

Exit code: 0 = clean, 1 = findings, 2 = config/runtime error
```

## Scanners

**Raw SQL Scanner** -- Splits `.sql` files on `;` with full awareness of quoted strings, dollar-quoting (`$$...$$`), line comments (`--`), and block comments (`/* */`). Each extracted statement is mapped back to its source file and line number.

**Go Scanner** -- Walks Go source files using `go/ast` and extracts SQL string literals from database method calls: `Query`, `QueryRow`, `Exec`, `QueryContext`, `ExecContext`, `QueryRowContext`, `Prepare`, `Get`, `Select`, `MustExec`, `NamedExec`, `NamedQuery`. Supports inline disable comments on the line above the call.

## Configuration

Rules are configured per-project via `.valk-guard.yaml`:

```yaml
version: 1

# Per-rule overrides
rules:
  VG001:
    enabled: true
    severity: warning
  VG004:
    enabled: false

# File patterns to exclude from scanning (supports ** globs)
exclude:
  - "tests/**"
  - "migrations/seed_*.sql"
  - "vendor/*"
  - "*.gen.sql"
```

### Inline Suppression

Suppress findings for individual statements using disable directives.

In SQL files:

```sql
-- valk-guard:disable VG001
SELECT * FROM users;

-- valk-guard:disable VG002,VG003
UPDATE users SET active = false;

-- valk-guard:disable
SELECT * FROM orders;  -- disables all rules for the next statement
```

In Go files:

```go
// valk-guard:disable VG001
db.Query("SELECT * FROM users")
```

## Built-in Rules

| Rule  | Name                       | What it catches                          | Default Severity |
|-------|----------------------------|------------------------------------------|------------------|
| VG001 | select-star                | `SELECT *` in application code           | warning          |
| VG002 | missing-where-update       | `UPDATE` without `WHERE`                 | error            |
| VG003 | missing-where-delete       | `DELETE` without `WHERE`                 | error            |
| VG004 | unbounded-select           | `SELECT` without `LIMIT`                 | warning          |
| VG005 | like-leading-wildcard      | `LIKE '%...'` leading wildcard           | warning          |
| VG006 | select-for-update-no-where | `SELECT FOR UPDATE` without `WHERE`      | error            |
| VG007 | destructive-ddl            | `DROP TABLE`, `DROP COLUMN`, `TRUNCATE`  | error            |
| VG008 | non-concurrent-index       | `CREATE INDEX` without `CONCURRENTLY`    | warning          |

## Future / Roadmap

**Next**: Expand scanner coverage (ORM-aware extraction and additional languages) and add more advanced rules (schema-aware checks, lock/index heuristics).

**Then**: GitHub Action for PR annotations. Use SARIF output to surface findings directly in pull request diffs.

**Later**: Python SQLAlchemy scanner (detect SQL patterns in ORM code), schema-aware analysis (connect to a live or dumped schema for smarter linting), and custom rule authoring (let users define project-specific rules).

## Development

```bash
make check        # fmt + vet + lint + test (runs all checks)
make test         # run tests with -race
make test-v       # run tests with -race -v (verbose)
make cover        # generate coverage report
make lint         # golangci-lint
make build        # build binary
make install      # install to $GOPATH/bin
make clean        # remove binary and coverage artifacts
make tidy         # go mod tidy
make fmt          # gofmt + goimports
make vet          # go vet
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on reporting issues, submitting pull requests, and setting up a development environment.

## License

Apache 2.0 -- see [LICENSE](LICENSE) for the full text.
