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

## 4) Schema-Drift Rule Config

Schema-drift rules (VG101-VG104) use the same per-rule config as query rules:

```yaml
rules:
  VG101:
    severity: error
  VG102:
    severity: warning
  VG103:
    enabled: false  # opt-in until type mapping matures
  VG104:
    severity: error
```

Schema-drift rules only fire when both SQL migrations (with `CREATE TABLE` DDL) and ORM models (Go structs with `db` tags or Python classes with `__tablename__`) are present. Projects with only one or the other produce no schema-drift findings.

## Current Limitation

Path-scoped per-rule overrides (for example: downgrade `VG008` only for `db/migrations/**`) are not implemented yet.
