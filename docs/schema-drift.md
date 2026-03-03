# Schema-Aware Detection

Valk Guard builds a migration schema snapshot and runs two schema-aware checks:

1. Model schema-drift checks (`VG101`-`VG104`)
2. Query column checks (`VG105`-`VG106`)

## How It Works

Schema-aware detection runs as a post-scan phase:

1. **DDL accumulation**: SQL statements from migration-like paths (`migrations/`, `migration/`, `migrate/`) are parsed by `postgresparser` when present. If none are found, all scanned `.sql` files are used as a fallback. DDL actions (`CREATE TABLE`, `DROP TABLE`, `DROP COLUMN`, `ALTER TABLE ADD COLUMN`) are accumulated into a schema snapshot.
2. **Query-schema checks**: Parsed `SELECT` statements from all scanners are validated for unknown projected columns (`VG105`) and unknown `WHERE`/`JOIN` predicate columns (`VG106`) against:
   - migration schema snapshot (always, when migration SQL exists)
   - model-derived schema for matching engines when present (`go/goqu` -> Go `db` tags, `sqlalchemy` -> SQLAlchemy models)
3. **Model extraction**: Go structs with `db` tags and Python classes with `__tablename__` / `Column()` are extracted from source files.
4. **Model cross-reference**: Model schema-drift rules (`VG101`-`VG104`) compare each model's columns against the migration schema.

`VG101`-`VG104` only fire when **both** SQL migrations and ORM models are present.  
`VG105`-`VG106` require parsed query statements plus at least one schema source (migration snapshot and/or matching model snapshot).

## Supported Model Formats

### Go (via `db` struct tags)

Structs with at least one `db:"column_name"` tag are treated as models:

```go
type User struct {
    ID    int    `db:"id"`
    Email string `db:"email"`
    Name  string `db:"name"`
}
```

Table name is derived from the lowercased struct name (`User` -> `user`). The rule layer tries plural forms (`users`, `useres`) and singular forms automatically for table matching.

Go field types are also extracted (`string`, `int64`, `time.Time`) and used by `VG103`.

### Python (via SQLAlchemy `__tablename__`)

Classes with `__tablename__` and `Column()` definitions:

```python
class User(Base):
    __tablename__ = "users"
    id = Column(Integer, primary_key=True)
    email = Column(String(255), nullable=False)
    name = Column(String(100))
```

Column types are extracted from `Column()` arguments for type-mismatch detection.

## Model Schema-Drift Rules

| Code  | Name             | What It Catches                                                | Severity |
|-------|------------------|----------------------------------------------------------------|----------|
| VG101 | dropped-column   | Model field maps to a column that doesn't exist in DDL         | error    |
| VG102 | missing-not-null | DDL has NOT NULL (no default) column missing from model        | warning  |
| VG103 | type-mismatch    | Model type doesn't match DDL column type                       | warning  |
| VG104 | table-not-found  | Explicit model table mapping has no matching CREATE TABLE      | error    |

## Query-Schema Rules

| Code  | Name                      | What It Catches                                                         | Severity |
|-------|---------------------------|-------------------------------------------------------------------------|----------|
| VG105 | unknown-projection-column | `SELECT` projection references a column missing from migration schema   | error    |
| VG106 | unknown-filter-column     | `WHERE`/`JOIN` predicate references a column missing from migration schema | error |

## Support Matrix

| Rule  | Go `db` tags | Python SQLAlchemy | Notes |
|-------|--------------|-------------------|-------|
| VG101 | yes          | yes               | Column presence check. |
| VG102 | yes          | yes               | Missing required columns check. |
| VG103 | yes          | yes               | Uses normalized model and SQL type matching. |
| VG104 | no (by design) | yes             | Requires explicit table mapping (`__tablename__`) to avoid inferred-name false positives. |

## Query-Schema Support Matrix

| Rule  | sql | go | goqu | sqlalchemy | Notes |
|-------|-----|----|------|------------|-------|
| VG105 | yes | yes | yes | yes | Validates projected columns (`SELECT a, b`) against migration schema; for `go/goqu` and `sqlalchemy`, also validates against model-derived columns when present. |
| VG106 | yes | yes | yes | yes | Validates predicate columns in `WHERE` and `JOIN ... ON ...` (including `INNER JOIN`) against migration schema; for `go/goqu` and `sqlalchemy`, also validates against model-derived columns when present. |

## Configuration

Schema-aware rules use the same config model:

```yaml
rules:
  VG101:
    severity: error
  VG102:
    severity: warning
  VG103:
    severity: warning
  VG104:
    severity: error
    engines: [sqlalchemy] # explicit table mappings only
  VG105:
    severity: error
    engines: [goqu, sqlalchemy]
  VG106:
    severity: error
    engines: [goqu, sqlalchemy]
```

Schema rules honor `engines` filtering by source:

- `go` for Go `db` tag models
- `sqlalchemy` for Python SQLAlchemy models
- `sql`, `go`, `goqu`, `sqlalchemy` for query-schema rules (`VG105`, `VG106`)

## Type Compatibility (VG103)

VG103 fires when the model provides type information (Go field types and SQLAlchemy `Column(...)` types are both supported).

| Model Type                        | Compatible SQL Types                                          |
|-----------------------------------|---------------------------------------------------------------|
| `Integer`, `int`, `int64`         | INTEGER, BIGINT, SMALLINT, SERIAL, BIGSERIAL, INT             |
| `String`, `string`                | VARCHAR, TEXT, CHAR, CHARACTER VARYING                         |
| `Float`, `float64`                | FLOAT, DOUBLE PRECISION, REAL, NUMERIC, DECIMAL               |
| `Boolean`, `bool`                 | BOOLEAN, BOOL                                                 |
| `DateTime`, `time.Time`           | TIMESTAMP, TIMESTAMPTZ                                        |

## Table Name Matching

The `matchTable` helper resolves model table names against the schema snapshot with these strategies (in order):

1. Exact match: `users` -> `users`
2. Add "s": `user` -> `users`
3. Add "es": `address` -> `addresses`
4. Strip "s": `users` -> `user`
5. Strip "es": `addresses` -> `address`

All matching is case-insensitive.

`VG104` only applies to explicit table mappings. Inferred table names (for example from Go struct names) are not used for table-not-found findings.
