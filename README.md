<p align="center">
  <h1 align="center">Valk Guard</h1>
  <p align="center">
    <strong>The SQL linter that catches production disasters at PR time.</strong>
  </p>
  <p align="center">
    <code>DELETE FROM orders</code> without a <code>WHERE</code>? <code>SELECT *</code> on a 50M-row table?<br/>
    Valk Guard finds them in your code — before your pager does at 3am.
  </p>
</p>

<p align="center">
  <a href="https://github.com/ValkDB/valk-guard/actions/workflows/ci.yml"><img src="https://github.com/ValkDB/valk-guard/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/github/go-mod/go-version/ValkDB/valk-guard" alt="Go Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-blue.svg" alt="License"></a>
  <a href="https://goreportcard.com/report/github.com/valkdb/valk-guard"><img src="https://goreportcard.com/badge/github.com/valkdb/valk-guard" alt="Go Report Card"></a>
  <a href="https://pkg.go.dev/github.com/valkdb/valk-guard"><img src="https://pkg.go.dev/badge/github.com/valkdb/valk-guard.svg" alt="Go Reference"></a>
</p>

<p align="center">
  <img src="docs/media/demo.svg" alt="valk-guard demo" width="820">
</p>

---

## Why Valk Guard?

**Most SQL linters use regex and only see raw `.sql` files. Valk Guard compiles your code and walks the AST.**

It reads Goqu builder chains, SQLAlchemy ORM calls, and Go `db.Query` invocations — not by pattern-matching strings, but by parsing the actual abstract syntax tree. It synthesizes SQL from your ORM code, feeds it through a real PostgreSQL grammar, and runs every rule against it.

That means: if your ORM builds a `DELETE` without a `WHERE`, Valk Guard catches it — even though no raw SQL exists anywhere in your source.

| What it prevents | Example |
|---|---|
| Accidental mass updates | `UPDATE users SET active = false` (no WHERE) |
| Unbounded queries | `SELECT id, email FROM users` (no LIMIT) |
| Index-killing patterns | `WHERE email LIKE '%@gmail.com'` |
| Dangerous migrations | `DROP TABLE`, `CREATE INDEX` without `CONCURRENTLY` |
| Schema drift | ORM model says `email` exists, but migration dropped it |
| ORM footguns | `session.query(User).delete()` — no raw SQL, still caught |

**Zero config. No database connection. Runs in CI in seconds.**

> **PostgreSQL only.** Valk Guard uses a PostgreSQL parser. MySQL, SQLite, and other dialects are not supported.

---

## Quick Start

```bash
# Install
go install github.com/valkdb/valk-guard/cmd/valk-guard@latest

# Scan your project
valk-guard scan .

# JSON for CI pipelines
valk-guard scan . --format json

# Reviewdog PR comments
valk-guard scan . --format rdjsonl

# GitHub Code Scanning (SARIF)
valk-guard scan . --format sarif --output results.sarif
```

That's it. All 19 rules are enabled by default.

---

## What It Catches

Valk Guard ships with **19 rules** across three categories. Here are the highlights:

| Rule | What it catches | Severity |
|------|----------------|----------|
| VG002 | `UPDATE` without `WHERE` — may wipe entire tables | error |
| VG003 | `DELETE` without `WHERE` — same, but worse | error |
| VG007 | `DROP TABLE`, `TRUNCATE` in application code | error |
| VG001 | `SELECT *` — over-fetching columns | warning |
| VG005 | `LIKE '%...'` — leading wildcard kills indexes | warning |
| VG008 | `CREATE INDEX` without `CONCURRENTLY` — blocks writes | warning |
| VG101 | ORM model references a column that migrations dropped | error |
| VG105 | Query `SELECT`s a column that doesn't exist in schema | error |

**[See all 19 rules](docs/rules.md)** with full descriptions, examples, and severity levels.

---

## Not Regex — Real AST Analysis

Most SQL linters use regex. Valk Guard **compiles and walks the actual AST** of your Go and Python code. It understands ORM builder chains as first-class SQL — no raw strings required.

<table>
<tr>
<td width="50%">

**Go + Goqu** — walks builder chains via `go/ast`

</td>
<td width="50%">

**Python + SQLAlchemy** — parses ORM chains via Python AST

</td>
</tr>
<tr>
<td>

<img src="docs/media/demo-goqu.svg" alt="Goqu AST scanning demo" width="100%">

</td>
<td>

<img src="docs/media/demo-sqlalchemy.svg" alt="SQLAlchemy AST scanning demo" width="100%">

</td>
</tr>
</table>

No raw SQL in those files. Valk Guard synthesizes SQL from the ORM calls, parses it with a PostgreSQL grammar, and runs all 19 rules against it (a handful of checks use targeted regex on parser-extracted clauses when the AST doesn't expose the needed field, but source scanning is always AST-based).

| Source | How it works |
|--------|-------------|
| **Raw SQL** (`.sql`) | Multi-statement parser with dollar-quoting, nested block comments |
| **Go** (`go/ast`) | Extracts SQL from `db.Query`, `db.Exec`, `db.QueryRow` and context variants |
| **Goqu** | Walks builder chains (`From`/`Join`/`Where`/`Limit`/`ForUpdate`) via Go AST |
| **SQLAlchemy** | Parses ORM chains (`query`/`select`/`join`/`filter`) via Python AST |

For schema-drift rules (VG101+), it also reads **ORM model definitions** — Go struct tags (`db`, `gorm`) and Python `__tablename__` / `Column(...)` — and cross-references them against your migration DDL.

---

## How It Compares

| | Valk Guard | sqlfluff | squawk | pganalyze |
|---|---|---|---|---|
| **No DB connection needed** | Yes | Yes | Yes | No |
| **Scans Go/Python source** | Yes | No | No | No |
| **ORM-aware (Goqu, SQLAlchemy)** | Yes | No | No | No |
| **Schema-drift detection** | Yes | No | No | Yes |
| **CI-native (SARIF, reviewdog)** | Yes | Partial | Yes | Yes |
| **Fix/auto-correct** | No | Yes | No | No |
| **Multi-dialect** | PG only | Yes | PG only | PG only |
| **PostgreSQL parser** | Yes | Multi-dialect | Yes | Yes |

Valk Guard's niche: **static analysis across SQL + ORM code with schema-drift detection, no infrastructure required.**

---

## CI / GitHub Actions

Valk Guard is built for CI. Findings post as inline PR review comments via reviewdog:

```yaml
permissions:
  contents: read
  pull-requests: write

jobs:
  pr-review:
    if: github.event_name == 'pull_request'
    steps:
      - uses: reviewdog/action-setup@v1

      - name: Run valk-guard
        run: |
          valk-guard scan . --format rdjsonl > valk-guard.rdjsonl || exit_code=$?
          if [ "${exit_code:-0}" -gt 1 ]; then exit $exit_code; fi

      - name: Post review comments
        env:
          REVIEWDOG_GITHUB_API_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          reviewdog \
            -f=rdjsonl \
            -name="valk-guard" \
            -reporter=github-pr-review \
            -filter-mode=added \
            -fail-level=none \
            < valk-guard.rdjsonl
```

Findings (exit 1) are non-blocking. Config/parser errors (exit 2+) fail the job.

Copy-paste workflows:
- [Full-repo PR scan](docs/ci-example-full-scan.md)
- [Changed-files-only PR scan](docs/ci-example-changed-files.md)
- [Full guide with SARIF + reviewdog + JSON artifacts](docs/ci-reviewer-mode.md)

---

## Live Demo PRs

See valk-guard reviewing real code in [`ValkDB/valk-guard-example`](https://github.com/ValkDB/valk-guard-example):

- [Query rules (SELECT *, missing WHERE, unbounded queries)](https://github.com/ValkDB/valk-guard-example/pull/2)
- [Index and locking rules (leading wildcard, FOR UPDATE)](https://github.com/ValkDB/valk-guard-example/pull/3)
- [Schema-drift and DDL rules](https://github.com/ValkDB/valk-guard-example/pull/4)
- [Query-schema validation (unknown columns/tables)](https://github.com/ValkDB/valk-guard-example/pull/5)
- [Suppression showcase (inline + global config)](https://github.com/ValkDB/valk-guard-example/pull/6)

---

## Installation

### Download a Binary (easiest)

Grab a pre-built binary from [GitHub Releases](https://github.com/ValkDB/valk-guard/releases) for Linux, macOS, or Windows (amd64/arm64).

### Install via Go

```bash
go install github.com/valkdb/valk-guard/cmd/valk-guard@latest
```

### Pin in CI (recommended)

```bash
go install github.com/valkdb/valk-guard/cmd/valk-guard@vX.Y.Z
```

Why pin: avoids surprise behavior changes, keeps output processing stable, makes builds reproducible.

### Build From Source

```bash
git clone https://github.com/ValkDB/valk-guard.git
cd valk-guard
make build
make install
```

### Requirements

- **Go >= 1.25.6** for building from source
- **Python 3.x** only for SQLAlchemy scanning. No pip packages needed — Valk Guard ships an embedded script using only stdlib (`ast`, `json`). If `python3` is not on your `PATH`, scanning `.py` files will fail with an error. Non-Python rules still run normally on `.sql` and `.go` files.

---

## Configuration

Zero config works out of the box. To customize, create a `.valk-guard.yaml`:

```yaml
exclude:
  - "vendor/**"
  - "db/migrations/**"

rules:
  VG001:
    severity: warning
    engines: [all]       # all | sql | go | goqu | sqlalchemy
  VG007:
    enabled: false

go_model:
  mapping_mode: strict   # strict | balanced | permissive
```

Reference: [`.valk-guard.yaml.example`](.valk-guard.yaml.example)

### Inline Suppression

```sql
-- valk-guard:disable VG001
SELECT * FROM users;
```

Works in Go (`//`) and Python (`#`) too. Full guide: [`docs/suppression.md`](docs/suppression.md)

### Exit Codes

| Code | Meaning |
|------|---------|
| `0`  | No findings |
| `1`  | Findings reported (any severity) |
| `2`  | Config, runtime, or parser error |

---

## How It Works

```mermaid
flowchart LR
  subgraph S1["1. Source Inputs"]
    A1[".sql files"]
    A2["Go code"]
    A3["Goqu usage"]
    A4["Python SQLAlchemy"]
  end

  subgraph S2["2. Statement Extraction"]
    B1["Raw SQL Scanner"]
    B2["Go AST Scanner"]
    B3["Goqu Scanner"]
    B4["SQLAlchemy Scanner"]
    B5["Statements with file/line mapping"]
  end

  subgraph S3["3. Parsing and Schema Context"]
    C1["postgresparser"]
    C2["DDL -> Schema Snapshot"]
    C3["Go Model Extractor"]
    C4["Python Model Extractor"]
    C5["Model Snapshots"]
  end

  subgraph S4["4. Rule Evaluation"]
    D1["Query Rules VG001-VG008"]
    D2["Query-Schema Rules VG105-VG108"]
    D3["Model Schema Rules VG101-VG104 and VG109-VG111"]
  end

  subgraph S5["5. Output"]
    E0["Findings"]
    E1["terminal"]
    E2["json"]
    E3["sarif"]
    E4["rdjsonl"]
  end

  A1 --> B1 --> B5
  A2 --> B2 --> B5
  A3 --> B3 --> B5
  A4 --> B4 --> B5

  B5 --> C1
  C1 --> D1
  C1 --> C2
  C1 --> D2

  A2 --> C3 --> C5
  A4 --> C4 --> C5

  C2 --> D2
  C5 --> D2
  C2 --> D3
  C5 --> D3

  D1 --> E0
  D2 --> E0
  D3 --> E0
  E0 --> E1
  E0 --> E2
  E0 --> E3
```

---

## Roadmap

Track progress and vote on what matters to you:

- GORM scanner — AST-based scanning for GORM builder chains and model extraction
- Deeper builder semantics — aliases, nested subqueries, richer predicate trees
- SQLAlchemy 2.0 `mapped_column()` support — modern model extraction
- Custom rule authoring — define your own rules in YAML or Go
- Severity-gated CI — block PRs only on errors, not warnings

---

## Documentation

- [All 19 rules — full reference](docs/rules.md)
- [Schema-drift detection](docs/schema-drift.md)
- [Suppression and noise control](docs/suppression.md)
- [Output formats (terminal, JSON, rdjsonl, SARIF)](docs/output-formats.md)
- [CI reviewer mode](docs/ci-reviewer-mode.md)
- [Adding new rules](docs/adding-rules.md)
- [Adding new scanners/sources](docs/adding-sources.md)

---

## Development

```bash
make build      # build binary
make test       # run tests (-race)
make lint       # golangci-lint
make cover      # coverage report
make check      # fmt + vet + lint + test
```

---

## Contributing / Security / License

- Contributing: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Security: [`SECURITY.md`](SECURITY.md) | [Report a vulnerability](https://github.com/ValkDB/valk-guard/security/advisories/new)
- License: [Apache 2.0](LICENSE)
