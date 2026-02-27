# Valk Guard 100% Readiness Checklist

This document defines what must be true for Valk Guard to be considered fully operational in local usage and GitHub PR reviewer mode.

## 1) Runtime Prerequisites

1. Go `1.25.6` installed.
2. Python `3.x` available when scanning SQLAlchemy code.
3. `postgresparser` dependency pinned to a stable release version.

## 2) Core Functional Coverage

1. Raw SQL scanning works (`.sql`).
2. Go scanner works for standard DB call extraction.
3. Goqu scanner works for:
   - raw literals (`goqu.L`)
   - synthetic SQL from builder chains
4. SQLAlchemy scanner works for:
   - raw SQL (`text`, `execute`)
   - synthetic SQL from ORM/query chains
5. Rule engine runs VG001-VG008 over parsed SQL with deterministic output ordering.

## 3) Reliability and Exit Semantics

1. Exit code `0` when no findings exist.
2. Exit code `1` when findings exist.
3. Exit code `2` on config/runtime/parser failures.
4. Invalid SQL and unparseable Go/Python input fail fast as designed.

## 4) Noise-Control and Governance

1. Exclude patterns are documented and tested.
2. Rule severity/enable overrides are documented and tested.
3. Inline suppression directives are documented for SQL/Go/Python.
4. Rule extension path is documented (`docs/adding-rules.md`).

See `docs/suppression.md` for suppression strategy details.

## 5) GitHub PR Reviewer Mode (Optional, Non-Blocking)

1. JSON findings output is generated in CI (`valk-guard.json`).
2. JSON findings are uploaded as a workflow artifact.
3. PR review comments are posted from findings (reviewdog mode).
4. Workflow permissions include:
   - `contents: read`
   - `pull-requests: write`
5. Workflow is non-blocking unless intentionally enforced by branch protection.
6. Preferred PR behavior: changed-files-only scanning for `.sql`, `.go`, `.py`.

See `docs/ci-reviewer-mode.md` for workflow details.

## 6) Validation Commands

```bash
make fmt
make vet
make lint
go test -race ./...
valk-guard scan . --format json
valk-guard scan . --format sarif --output valk-guard.sarif
```

## 7) Done Definition

Valk Guard is "100% ready" when all checklist items above are satisfied in both:

1. Local CLI usage (no CI required).
2. GitHub Actions reviewer mode (PR comments visible and JSON artifacts exported).
