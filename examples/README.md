# Valk Guard Examples

This directory contains examples demonstrating how Valk Guard scans different languages and patterns.

## Structure

*   `raw_sql/`: Standard `.sql` files.
*   `go_std/`: Go code using the standard `database/sql` package.
*   `go_goqu/`: Go code using the `goqu` query builder (AST analysis).
*   `python_sqlalchemy/`: Python code using SQLAlchemy ORM/Core (AST analysis).
*   `schema_drift/`: Go models + SQL migrations demonstrating schema-drift detection (VG101+).

## Running the Examples

You can run `valk-guard` against these folders to see the linter in action.

```bash
# Scan all examples
valk-guard scan examples/

# Scan a specific example
valk-guard scan examples/go_goqu/

# See schema-drift findings
valk-guard scan examples/schema_drift/
```

## Expected Output

The linter will flag deliberate anti-patterns included in these files (e.g., `SELECT *`, missing `WHERE`, etc.). This confirms the scanner is working correctly across all supported modes.

**Note:** The example files are scan targets, not compilable programs. Some files use simplified signatures to focus on the SQL patterns being demonstrated.

## Rule Coverage Matrix

Each example folder contains explicit labeled snippets for all built-in rules `VG001` through `VG008`.

| Rule | `raw_sql` | `go_std` | `go_goqu` | `python_sqlalchemy` |
| --- | --- | --- | --- | --- |
| VG001 `select-star` | yes | yes | yes | yes |
| VG002 `missing-where-update` | yes | yes | yes | yes |
| VG003 `missing-where-delete` | yes | yes | yes | yes |
| VG004 `unbounded-select` | yes | yes | yes | yes |
| VG005 `like-leading-wildcard` | yes | yes | yes | yes |
| VG006 `select-for-update-no-where` | yes | yes | yes (ORM chain) | yes (ORM chain) |
| VG007 `destructive-ddl` | yes | yes | yes (raw `goqu.L`) | yes (raw `text(...)`) |
| VG008 `non-concurrent-index` | yes | yes | yes (raw `goqu.L`) | yes (raw `text(...)`) |

The `schema_drift/` example demonstrates VG101 (dropped-column) by including a model field that references a column not present in migrations.

Notes:
- In ORM folders, VG007 and VG008 are shown via raw SQL execution APIs.
- VG006 is shown via ORM query-chain APIs (`ForUpdate`, `with_for_update`).
