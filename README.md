# Valk Guard
CI performance linter for application SQL.

## What It Does
- Scans SQL in raw `.sql` files, Go source (`go/ast` extraction), Goqu chains, and Python SQLAlchemy code.
- Parses each statement with [`postgresparser`](https://github.com/ValkDB/postgresparser) into structured query metadata.
- Applies built-in rules `VG001` through `VG008` to detect performance and safety anti-patterns.
- Reports findings in terminal, JSON, or SARIF 2.1.0 format.
- Uses CI-friendly exit codes: `0` (clean), `1` (findings), `2` (config/runtime/parser error).

## Why It Exists
We already review application code in PRs, but we often do not review the SQL that code generates.  
Valk Guard adds a practical gate for SQL regressions so common DB footguns are caught before production.

## Features (Today)

### Scanner Coverage (v1)
Valk Guard v1 supports all three target sources out of the box:
- Raw SQL files (`.sql`)
- Goqu query-builder usage
- Python SQLAlchemy usage

### Raw SQL Scanner
- Splits multi-statement SQL safely (handles comments, strings, dollar-quoted blocks, nested block comments).
- Preserves source file and statement line mapping for accurate findings.

### Go Scanner (`go/ast`)
- Extracts SQL literals from common DB execution methods (`Query`, `Exec`, `QueryRow`, context variants, and more).
- Applies inline suppression directives from Go comments.

### Goqu Scanner
- Extracts raw SQL from `goqu.L("...")`.
- Generates synthetic SQL from builder chains (`From/Join/Where/Limit/Update/Delete`) so rules run without raw literals.

### SQLAlchemy Scanner
- Extracts raw SQL from `text("...")` and `.execute("...")`.
- Generates synthetic SQL from ORM/query chains (`query/select/join/filter/filter_by/update/delete`).

### Configuration and Suppression
- Per-rule enable/disable and severity override in `.valk-guard.yaml`.
- File/path exclusion with glob support (`*` and `**`).
- Inline suppressions for SQL, Go, and Python.

### Runtime Behavior
- Parallel scanner execution across SQL, Go, Goqu, and SQLAlchemy inputs.
- End-to-end context cancellation (for `Ctrl+C` and CI timeout behavior).
- Strict parsing behavior: invalid SQL or unparseable candidate Go/Python source fails the run with exit code `2`.

## Installation

### Requirements
- Go `1.25.6+`
- Python `3.x` (only required if scanning SQLAlchemy/Python files)

### Build From Source (recommended for current v1 work)
```bash
git clone https://github.com/ValkDB/valk-guard.git
cd valk-guard
make build
make install
```

### Optional: Install via Go Tooling
Use this for released module versions:

```bash
go install github.com/valkdb/valk-guard/cmd/valk-guard@latest
```

## Quick Start
```bash
# Scan current directory
valk-guard scan .

# JSON output
valk-guard scan . --format json

# SARIF output for CI/code scanning
valk-guard scan . --format sarif --output results.sarif

# Use a custom config
valk-guard scan . --config .valk-guard.yaml

# Enable debug logs
valk-guard scan . --log-level debug
```

## Configuration
Use `.valk-guard.yaml` to tune scanning behavior:

```yaml
format: terminal

exclude:
  - "vendor/**"
  - "db/migrations/**"
  - "*.gen.sql"

rules:
  VG001:
    enabled: true
    severity: warning
    engines: [all] # all | sql | go | goqu | sqlalchemy
  VG005:
    engines: [goqu, sqlalchemy]
  VG007:
    enabled: false
```

Config controls:
- `rules.<RULE_ID>.enabled`: enable/disable a rule.
- `rules.<RULE_ID>.severity`: override severity (`error`, `warning`, `info`).
- `rules.<RULE_ID>.engines`: restrict a rule to specific engines (`all`, `sql`, `go`, `goqu`, `sqlalchemy`).
- `exclude`: skip matching paths/files from scanning.
- Output file path is configured at runtime with `--output <file>`.

Reference example: [`.valk-guard.yaml.example`](.valk-guard.yaml.example)

## Inline Suppression
Directive syntax:
- SQL: `-- valk-guard:disable ...`
- Go: `// valk-guard:disable ...`
- Python: `# valk-guard:disable ...`

Examples:

```sql
-- valk-guard:disable VG001
SELECT * FROM users;
```

```go
// valk-guard:disable VG001
db.Query("SELECT * FROM users")
```

```python
# valk-guard:disable VG001
session.execute(text("SELECT * FROM users"))
```

Disable all rules for the next statement:

```sql
-- valk-guard:disable
SELECT * FROM orders;
```

## Built-in Rules
| Code  | Name                       | Description                                                | Default Severity |
|-------|----------------------------|------------------------------------------------------------|------------------|
| VG001 | select-star                | Detects `SELECT *` projections.                            | warning          |
| VG002 | missing-where-update       | Detects `UPDATE` statements without `WHERE`.               | error            |
| VG003 | missing-where-delete       | Detects `DELETE` statements without `WHERE`.               | error            |
| VG004 | unbounded-select           | Detects `SELECT` statements without `LIMIT`/`FETCH`.       | warning          |
| VG005 | like-leading-wildcard      | Detects `LIKE`/`ILIKE` predicates with leading wildcard.   | warning          |
| VG006 | select-for-update-no-where | Detects `SELECT ... FOR UPDATE` without `WHERE`.           | error            |
| VG007 | destructive-ddl            | Detects destructive DDL (`DROP`, `TRUNCATE`, etc.).        | error            |
| VG008 | non-concurrent-index       | Detects `CREATE INDEX` without `CONCURRENTLY`.             | warning          |

### ORM-Focused Rules (Planned)
| Planned Code | Name                             | Engine Target      | Intent                                                      |
|--------------|----------------------------------|--------------------|-------------------------------------------------------------|
| VG009        | orm-join-fanout-no-limit         | goqu, sqlalchemy   | Flag join-heavy ORM chains that have no limiting strategy.  |
| VG010        | orm-expensive-order-without-limit| goqu, sqlalchemy   | Flag ordered ORM reads without `LIMIT` on large result sets.|

## CI / GitHub Actions
Minimal SARIF workflow (non-blocking annotations):

```yaml
name: valk-guard

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read
  security-events: write

jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25.6'

      - uses: actions/setup-python@v5
        with:
          python-version: '3.12'

      - name: Build valk-guard
        run: go build -o valk-guard ./cmd/valk-guard/

      - name: Run scan (non-blocking)
        continue-on-error: true
        run: ./valk-guard scan . --format sarif --output valk-guard.sarif

      - name: Upload SARIF
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: valk-guard.sarif
```

## How It Works
```text
Source Files (.sql / .go / .py)
          |
          v
Scanners
- RawSQLScanner
- GoScanner (go/ast)
- Goqu scanner (raw + synthetic SQL)
- SQLAlchemy scanner (raw + synthetic SQL)
          |
          v
SQL Statements + file/line mapping
(synthetic statements are tagged for goqu/sqlalchemy)
          |
          v
postgresparser
          |
          v
Parsed query metadata
          |
          v
Rule Engine (VG001-VG008)
          |
          v
Reporters: terminal | json | sarif
```

## Development
```bash
make build      # build binary
make test       # run tests (-race)
make lint       # golangci-lint
make cover      # coverage report
make check      # fmt + vet + lint + test
```

Local run:

```bash
make run
# or
./valk-guard scan .
```

## Roadmap (Planned)
- Deeper builder semantics (aliases, nested subqueries, richer predicate trees).
- Schema-aware checks and lock/index heuristics.
- Expanded custom rule authoring workflows.
- Stronger PR regression-gating modes (for example changed-files-only policies and severity gates).

## Contributing / Security / License
- Contributing: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Security: [`SECURITY.md`](SECURITY.md)
- License: [`LICENSE`](LICENSE)
