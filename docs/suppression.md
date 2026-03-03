# Suppression and Noise Control

Valk Guard supports multiple suppression levels so teams can tune signal vs noise without disabling the tool.

## 1) Per-File Exclusions

Use `exclude` in `.valk-guard.yaml` to skip whole paths (for example generated files or migrations):

```yaml
exclude:
  - "vendor/**"
  - "db/migrations/**"
  - "*.gen.sql"
```

## 2) Per-Rule Tuning

Use `rules` in `.valk-guard.yaml` to enable/disable rules, change severity, or scope rules to engines:

```yaml
rules:
  VG001:
    enabled: true
    severity: warning
    engines: [all] # all | sql | go | goqu | sqlalchemy
  VG005:
    engines: [goqu, sqlalchemy]
  VG007:
    enabled: false
```

## 3) Inline Suppression

Use inline directives for one-off cases near the SQL source.

The directive must appear on the same line as the statement or the line immediately before it. Blank lines between the directive and the statement will prevent suppression.

SQL:

```sql
-- valk-guard:disable VG001
SELECT * FROM users;
```

Go:

```go
// valk-guard:disable VG001
db.Query("SELECT * FROM users")
```

Python:

```python
# valk-guard:disable VG001
session.execute(text("SELECT * FROM users"))
```

## 4) Schema-Aware Rule Config

Schema-aware rules (`VG101`-`VG111`) use the same per-rule config:

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
  VG109:
    severity: warning
  VG110:
    severity: warning
  VG111:
    severity: warning
```

Model schema-drift rules (`VG101`-`VG104`, `VG109`-`VG111`) only fire when both SQL migrations (with `CREATE TABLE` DDL) and ORM models (Go structs with `db` tags or Python classes with `__tablename__`) are present.

Query-schema rules (`VG105`-`VG108`) fire when parsed query statements are present and at least one schema source is available:

- migration schema snapshot (from SQL DDL)
- model-derived schema for matching engines (`go/goqu` from Go `db` tags, `sqlalchemy` from SQLAlchemy models)

Schema-aware rules honor per-rule `engines` filtering:

- `go` applies to models extracted from Go `db` tags.
- `sqlalchemy` applies to models extracted from Python SQLAlchemy code.
- `sql`, `go`, `goqu`, and `sqlalchemy` apply to query-schema statement sources.

## Current Limitation

Path-scoped per-rule overrides (for example: downgrade `VG008` only for `db/migrations/**`) are not implemented yet.
