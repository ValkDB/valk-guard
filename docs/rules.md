# Rules Reference

Valk Guard ships with **19 rules** across three categories.

## Query Rules (VG001-VG008)

Structural checks on every SQL statement, regardless of source.

| Code  | Name                       | What it flags                                      | Severity |
|-------|----------------------------|----------------------------------------------------|----------|
| VG001 | select-star                | `SELECT *` projections                             | warning  |
| VG002 | missing-where-update       | `UPDATE` without `WHERE`                           | error    |
| VG003 | missing-where-delete       | `DELETE` without `WHERE`                           | error    |
| VG004 | unbounded-select           | `SELECT` without `LIMIT` (exempts aggregate-only and dual queries) | warning  |
| VG005 | like-leading-wildcard      | `LIKE`/`ILIKE` with leading `%`                    | warning  |
| VG006 | select-for-update-no-where | `SELECT ... FOR UPDATE` without `WHERE` (exempts queries with `LIMIT`) | error    |
| VG007 | destructive-ddl            | `DROP TABLE`, `TRUNCATE`, etc.                     | error    |
| VG008 | non-concurrent-index       | `CREATE INDEX` without `CONCURRENTLY`              | warning  |

> **Note:** VG004 does not distinguish between tables and views/materialized views. Queries that select from intentionally small views or lookup tables will still be flagged as unbounded. Use inline suppression (`-- valk-guard:disable VG004` or `// valk-guard:disable VG004`) to silence these.

> **Goqu and VG001:** The Goqu scanner synthesizes `SELECT *` for any `goqu.From(...)` chain that does not explicitly call `.Select()`, since omitting `.Select()` is idiomatic Goqu. To avoid noise, scope VG001 to specific engines with `engines: [sql, go, sqlalchemy]`. See [suppression](suppression.md#goqu-select--noise) for details.

## Schema-Drift Rules (VG101-VG104, VG109-VG111)

Cross-reference your ORM models against migration DDL to catch drift.

| Code  | Name                          | What it flags                                             | Severity |
|-------|-------------------------------|-----------------------------------------------------------|----------|
| VG101 | dropped-column                | Model references a column not in migrations               | error    |
| VG102 | missing-not-null              | NOT NULL column (no default) missing from model           | warning  |
| VG103 | type-mismatch                 | Column type mismatch between model and DDL                | warning  |
| VG104 | table-not-found               | Model table has no `CREATE TABLE` in migrations           | error    |
| VG109 | orphan-migration-table        | Migration table has no matching model                     | warning  |
| VG110 | duplicate-model-column-mapping| Model maps the same DB column twice                       | warning  |
| VG111 | go-inferred-table-name-risk   | Go model relies on inferred table name                    | warning  |

## Query-Schema Rules (VG105-VG108)

Validate that columns and tables referenced in queries actually exist in your schema.

| Code  | Name                         | What it flags                                          | Severity |
|-------|------------------------------|--------------------------------------------------------|----------|
| VG105 | unknown-projection-column    | `SELECT` references a column not in schema             | error    |
| VG106 | unknown-filter-column        | `WHERE`/`JOIN`/`GROUP BY`/`ORDER BY` uses unknown column | error  |
| VG107 | unknown-table-reference      | `FROM`/`JOIN` references a table not in schema         | error    |
| VG108 | ambiguous-unqualified-column | Unqualified column is ambiguous across joined tables   | warning  |

## Details

- Full schema-drift behavior and type compatibility: [`schema-drift.md`](schema-drift.md)
- Suppression and per-rule config: [`suppression.md`](suppression.md)
- Output format contracts: [`output-formats.md`](output-formats.md)
