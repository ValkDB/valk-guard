# Valk Guard

Open-source database performance linter for CI/CD that finds dangerous SQL **inside your application code** — not just in migration files.

Most SQL linters only analyze standalone `.sql` files. Valk Guard goes further: it reaches into your Go source code, extracts SQL from `db.Query()`, `db.Exec()`, and other database calls, and catches performance and data-safety issues before they reach production.

It's the difference between linting your migrations and linting **what your application actually does to your database**.

## Why Valk Guard

**Application-code SQL extraction.** Your most dangerous queries aren't in migration files — they're buried in application code. A `DELETE` without a `WHERE` inside a handler. An unbounded `SELECT` on a table that grew from 1K to 10M rows. Valk Guard uses real language-level AST parsing (not regex) to find and analyze these queries wherever they live in your codebase.

**DML analysis, not just DDL.** Migration linters check whether your schema changes are safe. Valk Guard checks whether your **queries** are safe — missing WHERE clauses, unbounded reads, full table scans, dangerous updates. These are the queries that cause outages at 3am.

**Org-level policies for critical tables.** Define protected tables and enforce rules across your team. Mark `users` or `payments` as protected and automatically block destructive operations, require WHERE clauses, or enforce LIMIT on reads. Turn tribal knowledge ("never run DELETE on the audit table") into automated guardrails.

**CI-native, not bolted on.** Runs as a GitHub Action with inline PR annotations, SARIF integration for Code Scanning dashboards, and clear exit codes. Developers get feedback in the PR, not in a separate dashboard they'll never check.

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

### Policies — Protect What Matters

Policies let you enforce table-specific guardrails across your org. Instead of relying on code review to remember "don't touch the audit table," encode it in config:

```yaml
policies:
  - name: "protect-core-tables"
    description: "Critical tables — block destructive operations"
    match:
      tables: [users, accounts, payments]
    rules:
      VG002: { severity: error }   # UPDATE without WHERE → error
      VG003: { severity: error }   # DELETE without WHERE → error
      VG007: { severity: error }   # DROP/TRUNCATE → error

  - name: "audit-is-append-only"
    description: "Audit log is immutable"
    match:
      tables: [audit_log, event_log]
    deny: [DELETE, UPDATE, TRUNCATE, DROP]

  - name: "large-tables-need-limits"
    match:
      tables: [events, transactions, logs]
    rules:
      VG004: { severity: error }   # unbounded SELECT → error
```

Policies compose — multiple policies can match the same table, and the strictest rule wins. This turns team conventions into automated enforcement.

### Inline Suppression

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

**Stage 1** (current): Go + raw SQL analysis — DML + DDL rules, table-level policies, CLI, GitHub Action, SARIF output.

**Stage 2** (planned): Python SQLAlchemy ORM pattern detection — N+1 query detection, missing eager loads, unbounded `.all()`, raw SQL extraction from `text()`/`execute()`. Additional language scanners (Java/JDBC, Node/Knex).

**Stage 3** (planned): Schema-aware analysis — connect to a live or dumped schema to detect missing indexes, full table scans, and type mismatches. Custom policy expressions.

## License

Apache 2.0
