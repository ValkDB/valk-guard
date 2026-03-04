# Schema-Aware Detection

Valk Guard builds a migration schema snapshot and runs two schema-aware checks:

1. Model schema checks (`VG101`-`VG104`, `VG109`-`VG111`)
2. Query-schema checks (`VG105`-`VG108`)

## How It Works

Schema-aware detection runs as a post-scan phase:

1. **DDL accumulation**: SQL statements from migration-like paths (`migrations/`, `migration/`, `migrate/`) are parsed by `postgresparser` when present. If none are found, all scanned `.sql` files are used as a fallback. DDL actions (`CREATE TABLE`, `DROP TABLE`, `DROP COLUMN`, `ALTER TABLE ADD COLUMN`) are accumulated into a schema snapshot.
2. **Query-schema checks**: Parsed statements are validated for unknown/ambiguous table and column usage (`VG105`-`VG108`) against:
   - migration schema snapshot (always, when migration SQL exists)
   - model-derived schema for matching engines when present (`go/goqu` -> Go model extractor, `sqlalchemy` -> SQLAlchemy models)
3. **Model extraction**: Go structs and Python SQLAlchemy classes are extracted into generic `schema.ModelDef`.
4. **Model cross-reference**: Model schema rules (`VG101`-`VG104`, `VG109`-`VG111`) compare model metadata against migration schema.

    If a statement cannot be parsed, Valk Guard logs a warning and skips only that statement. Warning logs include remediation guidance to exclude the file path in `.valk-guard.yaml`.

Source-to-engine mapping is controlled in `cmd/valk-guard/source_bindings.go`:

- `configEngines` map model sources to rule-engine filters for model schema rules.
- `queryEngines` map statement engines to model snapshots for query-schema rules.

`VG101`-`VG104` and `VG109`-`VG111` only fire when **both** SQL migrations and ORM models are present.  
`VG105`-`VG108` require parsed query statements plus at least one schema source (migration snapshot and/or matching model snapshot).

## Supported Model Formats

### Go (provider pipeline)

Go model extraction supports provider-based mapping:

```go
type User struct {
    ID    int    `db:"id"`
    Name  string `gorm:"column:name"`
    Email string `db:"email"`
}
```

Table mapping precedence:

1. Explicit `TableName() string` method
2. Inferred type name fallback (`User` -> `user`)

Column mapping precedence:

1. Explicit `db:"column"` tag
2. Explicit `gorm:"column:column_name"` tag
3. Optional inference from field names (mode-dependent)

Go mapping mode is configured in `.valk-guard.yaml`:

```yaml
go_model:
  mapping_mode: strict # strict | balanced | permissive
```

- `strict`: explicit mappings only
- `balanced`: infer exported field names when explicit mapping is absent
- `permissive`: infer all named fields when explicit mapping is absent

Extracted mappings are tagged as `explicit` or `inferred` metadata for downstream rules.

Go field types are also extracted (`string`, `int64`, `time.Time`) and used by `VG103`.

## Mapping Provenance (Explicit vs Inferred)

All model extractors normalize mappings to common provenance metadata:

- `ModelDef.TableMappingKind`: `explicit` or `inferred`
- `ModelDef.TableMappingSource`: source token (for example `table_name_method`, `type_name`, `sqlalchemy_ast`)
- `ModelColumn.MappingKind`: `explicit` or `inferred`
- `ModelColumn.MappingSource`: source token (for example `db_tag`, `gorm_tag`, `field_name`, `sqlalchemy_ast`)

Meaning:

- `explicit`: mapping is declared by framework/source metadata.
- `inferred`: mapping is derived by fallback naming rules.

Rule behavior that depends on this metadata:

- `VG104` (`table-not-found`) runs only for explicit table mappings.
- `VG111` (`go-inferred-table-name-risk`) reports Go models that rely on inferred table mapping.

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
| VG109 | orphan-migration-table | Migration table has no matching model mapping           | warning  |
| VG110 | duplicate-model-column-mapping | Model maps the same DB column multiple times    | warning  |
| VG111 | go-inferred-table-name-risk | Go model uses inferred mapping without explicit table mapping | warning |

## Query-Schema Rules

| Code  | Name                      | What It Catches                                                         | Severity |
|-------|---------------------------|-------------------------------------------------------------------------|----------|
| VG105 | unknown-projection-column | `SELECT` projection references a column missing from migration schema   | error    |
| VG106 | unknown-filter-column     | `WHERE`/`JOIN`/`GROUP BY`/`ORDER BY` references a column missing from migration schema | error |
| VG107 | unknown-table-reference   | `FROM`/`JOIN` references a table missing from migration/model schema      | error |
| VG108 | ambiguous-unqualified-column | Unqualified column is present in multiple joined tables               | warning |

## Support Matrix

| Rule  | Go provider pipeline | Python SQLAlchemy | Notes |
|-------|----------------------|-------------------|-------|
| VG101 | yes                  | yes               | Column presence check. |
| VG102 | yes                  | yes               | Missing required columns check. |
| VG103 | yes                  | yes               | Uses normalized model and SQL type matching. |
| VG104 | yes (explicit table only) | yes         | Requires explicit table mapping (`TableName()` / `__tablename__`). |
| VG109 | yes                  | yes               | Migration table coverage check. |
| VG110 | yes                  | yes               | Duplicate model column mapping check. |
| VG111 | yes                  | no                | Inference-risk warning for Go models. |

## Query-Schema Support Matrix

| Rule  | sql | go | goqu | sqlalchemy | Notes |
|-------|-----|----|------|------------|-------|
| VG105 | yes | yes | yes | yes | Validates projected columns (`SELECT a, b`) against migration schema; for `go/goqu` and `sqlalchemy`, also validates against model-derived columns when present. |
| VG106 | yes | yes | yes | yes | Validates columns in `WHERE`, `JOIN ... ON ...` (including `INNER JOIN`), `GROUP BY`, and `ORDER BY` against migration schema; for `go/goqu` and `sqlalchemy`, also validates against model-derived columns when present. |
| VG107 | yes | yes | yes | yes | Validates referenced tables in `FROM`/`JOIN` against schema sources. |
| VG108 | yes | yes | yes | yes | Validates that unqualified columns are not ambiguous across joined tables. |

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
  VG107:
    severity: error
    engines: [goqu, sqlalchemy]
  VG108:
    severity: warning
    engines: [sql, go, goqu, sqlalchemy]
```

Schema rules honor `engines` filtering by source:

- `go` for Go `db` tag models
- `sqlalchemy` for Python SQLAlchemy models
- `sql`, `go`, `goqu`, `sqlalchemy` for query-schema rules (`VG105`-`VG108`)

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

The `matchTable` helper resolves model table names against the schema snapshot using exact
case-insensitive matching only.

Examples:

- `users` -> `users` (match)
- `Users` -> `users` (match)
- `user` -> `users` (no match)
- `addresses` -> `address` (no match)

`VG104` only applies to explicit table mappings. Inferred table names are not used for table-not-found findings.
