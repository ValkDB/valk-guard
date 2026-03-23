# Valk Guard Overview

**Valk Guard** is an open-source database performance linter for CI/CD. It statically analyzes SQL in your source code to flag performance anti-patterns and safety "footguns" before they hit production.

---

## What It Does

Valk Guard acts as a static gatekeeper for your database. It scans your codebase (Go, Python, SQL, C#), extracts database interactions, parses them using a formal PostgreSQL grammar, and runs them against a suite of performance-focused rules.

### Core Value Proposition
- **Prevent Outages**: Catch `UPDATE` or `DELETE` statements missing a `WHERE` clause.
- **Optimize Performance**: Identify `SELECT *` and unbounded queries (missing `LIMIT`).
- **Index Efficiency**: Flag `LIKE` patterns with leading wildcards that bypass indexes.
- **Safe Migrations**: Ensure indexes are created `CONCURRENTLY` to avoid table locks.

---

## Technical Architecture

Valk Guard is built for speed and accuracy, leveraging Go 1.25+ features like iterators for memory-efficient streaming.

```mermaid
graph TD
    subgraph "1. Extraction Phase"
        A[Filesystem] --> B{File Type?}
        B -->|.sql| C[Raw SQL Scanner]
        B -->|.go| D[Go AST Scanner]
        B -->|.go| E[Goqu Synth Scanner]
        B -->|.py| F[SQLAlchemy Scanner]
        B -->|.cs| G[C# EF Core Scanner]
    end

    subgraph "2. Analysis Engine"
        C & D & E & F & G -->|SQL Stream| H[Valk PG Parser]
        H -->|AST| I[Rule Engine]
        I -->|Findings| J[Deduplicator]
    end

    subgraph "3. Reporting"
        J --> K{Output Format}
        K -->|Terminal| L[Human Readable]
        K -->|JSON| M[Machine Readable]
        K -->|SARIF| N[GitHub Code Scanning]
        K -->|rdjsonl| O[reviewdog PR Review]
    end
```

---

## Supported Scanners

| Scanner | Method | Description |
| :--- | :--- | :--- |
| **Raw SQL** | Character Stream | Parses `.sql` files, respecting comments, dollar-quoting, and nested blocks. |
| **Go Standard** | `go/ast` | Extracts SQL literals from `db.Query`, `db.Exec`, `sqlx`, etc. |
| **Goqu** | Synthesis | Analyzes Goqu method chains to generate synthetic SQL for analysis. |
| **SQLAlchemy** | Python AST | Invokes a Python sub-process to extract SQL from `text()` and ORM chains. |
| **C# (EF Core)** | Text Analysis | Extracts SQL from `ExecuteSqlRaw`, `ExecuteSqlInterpolated`, and async variants. v1: raw SQL execution only. |

---

## Built-in Rules

Valk Guard ships with 19 production-first rules. See [docs/rules.md](rules.md) for the full reference.

### Query Rules (VG001-VG008)

| ID | Name | Severity | Catch |
| :--- | :--- | :--- | :--- |
| **VG001** | `select-star` | Warning | Usage of `SELECT *` instead of specific columns. |
| **VG002** | `missing-where-update` | Error | `UPDATE` statements without a `WHERE` clause. |
| **VG003** | `missing-where-delete` | Error | `DELETE` statements without a `WHERE` clause. |
| **VG004** | `unbounded-select` | Warning | `SELECT` without `LIMIT` (exempts aggregate-only and dual queries). |
| **VG005** | `like-leading-wildcard` | Warning | `LIKE '%...'` patterns that prevent index usage. |
| **VG006** | `select-for-update-no-where` | Error | Locking entire tables with `FOR UPDATE` (exempts `LIMIT`). |
| **VG007** | `destructive-ddl` | Error | `DROP` or `TRUNCATE` commands in application code. |
| **VG008** | `non-concurrent-index` | Warning | Creating indexes without `CONCURRENTLY`. |

### Schema-Drift Rules (VG101-VG104, VG109-VG111)

| ID | Name | Severity | Catch |
| :--- | :--- | :--- | :--- |
| **VG101** | `dropped-column` | Error | Model field maps to a column absent from migration DDL. |
| **VG102** | `missing-not-null` | Warning | NOT NULL column (no default) missing from model. |
| **VG103** | `type-mismatch` | Warning | Model type doesn't match DDL column type. |
| **VG104** | `table-not-found` | Error | Explicit model table mapping has no CREATE TABLE. |
| **VG109** | `orphan-migration-table` | Warning | Migration table has no matching model. |
| **VG110** | `duplicate-model-column-mapping` | Warning | Model maps the same DB column multiple times. |
| **VG111** | `go-inferred-table-name-risk` | Warning | Go model uses inferred table name without explicit mapping. |

### Query-Schema Rules (VG105-VG108)

| ID | Name | Severity | Catch |
| :--- | :--- | :--- | :--- |
| **VG105** | `unknown-projection-column` | Error | SELECT projection references a column not in schema. |
| **VG106** | `unknown-filter-column` | Error | WHERE/JOIN/GROUP BY/ORDER BY references unknown column. |
| **VG107** | `unknown-table-reference` | Error | FROM/JOIN references a table not in schema. |
| **VG108** | `ambiguous-unqualified-column` | Warning | Unqualified column is present in multiple joined tables. |

---

## CI/CD Workflow

Valk Guard is designed to be a "blocking" step in your CI pipeline.

```mermaid
sequenceDiagram
    participant Dev as Developer
    participant Git as GitHub/GitLab
    participant CI as CI Runner (Valk Guard)
    participant Sec as Security/Insights

    Dev->>Git: Push Pull Request
    Git->>CI: Trigger "DB Lint" Job
    CI->>CI: valk-guard scan . --format sarif

    alt Findings Detected
        CI->>Git: Upload SARIF Report
        Git->>Dev: Show inline PR Annotations
        CI-->>Git: Exit Code 1 (Fail Build)
    else Clean
        CI-->>Git: Exit Code 0 (Pass Build)
    end
```

---

## Configuration

Control Valk Guard via a `.valk-guard.yaml` file:

```yaml
exclude:
  - "vendor/**"
  - "db/migrations/*.gen.sql"

rules:
  VG001:
    enabled: true
    severity: error
  VG004:
    enabled: false
```

### Inline Suppression
You can disable rules for specific lines using comments:
- **SQL**: `-- valk-guard:disable VG001`
- **Go**: `// valk-guard:disable VG001`
- **Python**: `# valk-guard:disable VG001`
- **C#**: `// valk-guard:disable VG001`
