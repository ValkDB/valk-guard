# Valk Guard Weekly Goals

## Week Focus
Ship a reliable CI/CD integration on GitHub Actions and ensure scanner coverage is solid for:
- Raw SQL files
- Goqu usage (raw + AST-synthetic SQL)
- SQLAlchemy usage (raw + AST-synthetic SQL)

## Dependency Note
- Current parser source is tied to `postgresparser` PR branch: https://github.com/ValkDB/postgresparser/pull/30
- Action after merge: replace the PR branch reference with the latest released/stable version and re-run the full CI suite.

## This Week Goals
1. GitHub Actions CI/CD runs cleanly end-to-end for `valk-guard`.
2. Scanning works consistently across raw SQL, Goqu, and SQLAlchemy paths.
3. CI scan scope is changed files only (PR diff), not full-repo by default.
4. ORM scans verify findings against the final synthesized SQL structure (post-AST transform).
5. Rule framework remains easy to extend with custom rules when needed.

## Success Criteria
1. `main` and PR workflows pass for lint, tests, and build jobs.
2. Running `valk-guard scan` in CI returns expected exit codes:
   - `0` for no findings
   - `1` for findings
   - `2` for runtime/config/parser errors
3. Existing scanner tests pass for:
   - `internal/scanner/sql_scanner_test.go`
   - `internal/scanner/goqu/goqu_scanner_test.go`
   - `internal/scanner/sqlalchemy/sqlalchemy_scanner_test.go`
4. CI validates only changed files in pull requests (with deterministic path filtering).
5. Goqu/SQLAlchemy synthetic SQL tests assert the final SQL shape used by rules (projection, joins, predicates, limits, update/delete structure).
6. SARIF/JSON outputs are generated correctly in CI when requested.
7. Rule extension flow is documented and validated with one example path:
   - Add a new rule struct
   - Register in `internal/rules/registry.go`
   - Add tests
   - Run `go test ./...`

## Deliverables
1. Stable GitHub Actions pipeline for lint, test, self-scan, and build.
2. PR-mode scan pipeline that evaluates changed files only.
3. Verified scanner behavior for raw SQL, Goqu, and SQLAlchemy in CI, including final synthetic SQL structure checks for ORM paths.
4. Clear custom-rule contribution path (docs + passing example workflow).

## Stretch (If Time Allows)
1. Add a dedicated CI check for SARIF upload simulation.
2. Add one extra rule stub (`VG009`) to prove extension velocity.
