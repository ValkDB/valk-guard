# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [1.0.0] - Unreleased

### Added
- Raw SQL scanner with multi-statement splitting (comments, dollar-quoting, nested block comments).
- Go scanner using `go/ast` for extracting SQL from `db.Query`, `db.Exec`, `db.QueryRow`, and context variants.
- Goqu scanner for `goqu.L()` raw literals and synthetic SQL from builder chains (`From/Join/Where/Limit/ForUpdate/Update/Delete`).
- SQLAlchemy scanner for `text()`, `.execute()`, and synthetic SQL from ORM chains (`query/select/join/filter/filter_by/with_for_update/update/delete`).
- Rules VG001-VG008 covering SELECT *, missing WHERE, unbounded SELECT, leading wildcard LIKE, FOR UPDATE without WHERE, destructive DDL, and non-concurrent index creation.
- Terminal, JSON, and SARIF 2.1.0 output formats.
- Per-rule enable/disable and severity override via `.valk-guard.yaml`.
- File/path exclusion with glob support (`*` and `**`).
- Inline suppression directives for SQL (`--`), Go (`//`), and Python (`#`).
- Parallel scanner execution with context cancellation support.
- CI-friendly exit codes: `0` (clean), `1` (findings), `2` (config/runtime error).
- GitHub Actions workflow with reviewdog PR review comments, self-scan dogfooding, and cross-platform builds.
- GoReleaser configuration for cross-compiled releases.
