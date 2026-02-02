# Valk Guard

Open-source database performance linter for CI/CD. Statically analyzes SQL in source code and flags performance anti-patterns.

Runs locally or as a GitHub Action to block PRs that introduce database performance issues.

## Requirements

- Go 1.25.6+
- PostgreSQL SQL dialect

## Installation

```bash
go install github.com/valkdb/valk-guard/cmd/valk-guard@latest
```

Or use as a GitHub Action (see below).

## Status

Under development — Stage 1 (SQL linting for Go and raw SQL files).

## How It Works

Valk Guard scans your codebase for SQL — both raw `.sql` files and SQL strings embedded in Go source code. Each statement is parsed into a structured IR using [valk-postgres-parser](https://github.com/ValkDB/valk-postgres-parser) (an ANTLR4 PostgreSQL parser), then checked against a set of lint rules. Findings are reported as inline PR annotations, terminal output, or SARIF for GitHub Code Scanning.

```
Developer opens PR with SQL changes
        │
        ▼
┌─────────────────────────┐
│  valk-guard scan [path] │
└────────┬────────────────┘
         │
         ▼
┌─────────────────────────┐     Finds SQL in:
│  Scanner                │     • *.sql files (RawSQLScanner)
│                         │     • Go source (GoScanner — go/ast extraction)
└────────┬────────────────┘
         │
         ▼
┌─────────────────────────┐
│  Parser Engine           │     Parses each SQL statement into structured
│  (valk-postgres-parser)  │     metadata: tables, columns, JOINs, WHERE
│                          │     clauses, DDL actions, set operations
└────────┬─────────────────┘
         │
         ▼
┌─────────────────────────┐
│  Rule Engine             │     Checks parsed metadata against enabled
│  (VG001..VG008)          │     rules from .valk-guard.yaml
└────────┬─────────────────┘
         │
         ▼
┌─────────────────────────┐     Output formats:
│  Reporter                │     • Terminal (pretty, with colors)
│                          │     • JSON (machine-readable)
│                          │     • SARIF (GitHub Code Scanning)
└────────┬─────────────────┘     • Inline PR annotations (::error file=...)
         │
         ▼
   Exit code: 0=pass, 1=violations, 2=config error
```

## What's Supported

**SQL parsing** (via valk-postgres-parser):
- DML: SELECT, INSERT, UPDATE, DELETE, MERGE
- DDL: DROP TABLE/COLUMN, ALTER TABLE, CREATE/DROP INDEX, TRUNCATE
- WHERE clause extraction with operator detection
- JOIN relationship inference
- CTE, subquery, and set operation support
- JSONB operator support

**Source code scanning**:
- Raw SQL files (`.sql`)
- Go source files — extracts SQL string literals from `db.Query()`, `db.Exec()`, etc. using `go/ast`

## Rules

### DML Rules

| Rule | Name | What it catches | Severity |
|------|------|-----------------|----------|
| VG001 | select-star | `SELECT *` in application code | warning |
| VG002 | missing-where-update | `UPDATE` without a `WHERE` clause | error |
| VG003 | missing-where-delete | `DELETE` without a `WHERE` clause | error |
| VG004 | unbounded-select | `SELECT` without `LIMIT` | warning |
| VG005 | like-leading-wildcard | `LIKE '%...'` — leading wildcard prevents index usage | warning |
| VG006 | select-for-update-no-where | `SELECT FOR UPDATE` without `WHERE` | error |

### DDL Rules

| Rule | Name | What it catches | Severity |
|------|------|-----------------|----------|
| VG007 | destructive-ddl | `DROP TABLE`, `DROP COLUMN`, or `TRUNCATE` in migrations | error |
| VG008 | non-concurrent-index | `CREATE INDEX` without `CONCURRENTLY` | warning |

## Configuration

Rules are configured per-project via `.valk-guard.yaml`:

```yaml
version: 1
rules:
  VG001:
    enabled: true
    severity: warning
  VG004:
    enabled: false
exclude:
  - "tests/**"
  - "migrations/seed_*.sql"
```

Inline suppression in SQL files:

```sql
-- valk-guard:disable VG001
SELECT * FROM users;
```

## GitHub Action Usage

```yaml
# .github/workflows/valk-guard.yml
name: Valk Guard
on: [pull_request]
jobs:
  lint-sql:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: valkdb/valk-guard@v1
        with:
          config: .valk-guard.yaml
```

Findings appear as inline annotations on the PR diff. SARIF output integrates with GitHub's Code Scanning dashboard.

## Roadmap

**Stage 1** (current): SQL linting for Go and raw SQL files — DML + DDL rules, CLI, GitHub Action, SARIF output.

**Stage 2** (planned): Python SQLAlchemy ORM pattern detection — N+1 query detection, missing eager loads, unbounded `.all()`, raw SQL extraction from `text()`/`execute()`.

**Stage 3** (planned): Schema-aware analysis, additional language spokes, custom rule authoring.

## License

Apache 2.0
