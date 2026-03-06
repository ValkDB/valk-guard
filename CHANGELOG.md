# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-03-06

### Added

#### Scanners
- Raw SQL scanner with multi-statement splitting (comments, dollar-quoting, nested block comments).
- Go scanner using `go/ast` for extracting SQL from `db.Query`, `db.Exec`, `db.QueryRow`, and context variants.
- Goqu scanner for `goqu.L()` raw literals and synthetic SQL from builder chains (`From/Join/Where/Limit/ForUpdate/Update/Delete`).
- SQLAlchemy scanner for `text()`, `.execute()`, and synthetic SQL from ORM chains (`query/select/join/filter/filter_by/with_for_update/update/delete`).

#### Query Rules (VG001-VG008)
- VG001: detect `SELECT *` projections.
- VG002: detect `UPDATE` without `WHERE`.
- VG003: detect `DELETE` without `WHERE`.
- VG004: detect unbounded `SELECT` (no `LIMIT`/`FETCH`).
- VG005: detect `LIKE`/`ILIKE` with leading wildcard.
- VG006: detect `SELECT ... FOR UPDATE` without `WHERE`.
- VG007: detect destructive DDL (`DROP TABLE`, `TRUNCATE`).
- VG008: detect `CREATE INDEX` without `CONCURRENTLY`.

#### Schema-Drift Rules (VG101-VG104, VG109-VG111)
- VG101: model references a column not found in migration schema.
- VG102: NOT NULL column (no default) missing from model.
- VG103: column type mismatch between model and migration DDL.
- VG104: model table has no `CREATE TABLE` in migrations.
- VG109: migration table has no matching model.
- VG110: model maps the same DB column multiple times.
- VG111: Go model relies on inferred table name without explicit mapping.

#### Query-Schema Rules (VG105-VG108)
- VG105: `SELECT` projection references a column not in schema.
- VG106: `WHERE`/`JOIN`/`GROUP BY`/`ORDER BY` references unknown column.
- VG107: `FROM`/`JOIN` references a table not in schema.
- VG108: unqualified column is ambiguous across joined tables.

#### Model Extraction
- Go model extractor: reads `db` and `gorm` struct tags with configurable inference mode (`strict`/`balanced`/`permissive`).
- Python model extractor: reads `__tablename__` and `Column(...)` from SQLAlchemy models via embedded Python script.
- Mapping provenance tracking (`explicit` vs `inferred`) for model table and column mappings.

#### Output and CI
- Terminal, JSON, SARIF 2.1.0, and reviewdog rdjsonl output formats.
- Per-rule enable/disable and severity override via `.valk-guard.yaml`.
- Per-rule engine scoping (`sql`, `go`, `goqu`, `sqlalchemy`, `all`).
- File/path exclusion with glob support (`*` and `**`).
- Inline suppression directives for SQL (`--`), Go (`//`), and Python (`#`).
- Parallel scanner execution with context cancellation support.
- CI-friendly exit codes: `0` (clean), `1` (findings), `2` (config/runtime error).
- GitHub Actions workflow with reviewdog PR review comments, self-scan dogfooding, and cross-platform builds.
- GoReleaser configuration for cross-compiled releases.

[0.1.0]: https://github.com/ValkDB/valk-guard/releases/tag/v0.1.0
