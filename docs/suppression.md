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

## Current Limitation

Path-scoped per-rule overrides (for example: downgrade `VG008` only for `db/migrations/**`) are not implemented yet.
