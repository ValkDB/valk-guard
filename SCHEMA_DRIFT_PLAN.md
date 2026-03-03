# Schema-Drift Detection: VG101-VG104 — COMPLETE

## Status: All implementation done. Ready to commit.

---

## What Was Done

### Parser Upgrade
- `postgresparser` v1.0.0 → v1.1.5
- v1.1.5 has native `DDLCreateTable` + `ColumnDetails` support
- Eliminated the custom `create_table.go` parser from the original plan entirely

### New Files (19)
| File | Purpose | Tests |
|------|---------|-------|
| `internal/schema/doc.go` | Package doc | — |
| `internal/schema/schema.go` | Snapshot, TableDef, ColumnDef + Apply methods | 10 tests |
| `internal/schema/schema_test.go` | Schema accumulation tests | — |
| `internal/schema/model.go` | ModelDef, ModelColumn, ModelExtractor interface | — |
| `internal/schema/builder.go` | BuildFromStatements using parser DDLActions | 10 tests |
| `internal/schema/builder_test.go` | Builder integration tests | — |
| `internal/schema/gomodel/extractor.go` | Go struct `db` tag extractor | 8 tests |
| `internal/schema/gomodel/extractor_test.go` | Go extractor tests | — |
| `internal/schema/pymodel/extractor.go` | Python model extractor (Go side) | 6 tests |
| `internal/schema/pymodel/extract_models.py` | Python AST model walker | — |
| `internal/schema/pymodel/extractor_test.go` | Python extractor tests | — |
| `internal/rules/schema_rule.go` | SchemaRule interface + matchTable helper | — |
| `internal/rules/vg101_dropped_column.go` | Model references column not in DDL (error) | 5 tests |
| `internal/rules/vg101_dropped_column_test.go` | — | — |
| `internal/rules/vg102_missing_not_null.go` | NOT NULL column missing from model (warning) | 5 tests |
| `internal/rules/vg102_missing_not_null_test.go` | — | — |
| `internal/rules/vg103_type_mismatch.go` | Model/DDL type mismatch (warning) | 8 tests |
| `internal/rules/vg103_type_mismatch_test.go` | — | — |
| `internal/rules/vg104_table_not_found.go` | Model maps to nonexistent table (error) | 5 tests |
| `internal/rules/vg104_table_not_found_test.go` | — | — |
| `docs/schema-drift.md` | Schema-drift feature documentation | — |

### Modified Files (7)
| File | Change |
|------|--------|
| `go.mod` / `go.sum` | postgresparser v1.0.0 → v1.1.5 |
| `internal/rules/registry.go` | Added `schemaRules` map, `RegisterSchema()`, `AllSchema()`, registered VG101-104 |
| `cmd/valk-guard/main.go` | DDL accumulation in scan loop + `runSchemaDrift()` post-scan phase |
| `internal/scanner/doc.go` | Updated doc comment mentioning gomodel reuse |
| `README.md` | VG101-104 rules table, updated mermaid diagram, updated roadmap |
| `docs/adding-rules.md` | Added SchemaRule section |
| `docs/suppression.md` | Added VG101-104 config examples |
| `docs/production-readiness.md` | Added schema-drift checklist items |

### Test Results
All 57+ tests pass:
- `internal/schema/` — 20 tests (schema + builder)
- `internal/schema/gomodel/` — 8 tests
- `internal/schema/pymodel/` — 6 tests
- `internal/rules/` — 23+ tests (including VG101-104)
- `cmd/valk-guard/` — existing tests still pass
- `go vet` clean

---

## valk-guard-example Repo Status

The example repo at `git@github.com:ValkDB/valk-guard-example.git` already has everything needed on main:

**DDL (sql/migrations/001_create_tables.sql):**
- `CREATE TABLE users` (id, email, name, active, created_at, updated_at)
- `CREATE TABLE orders` (id, user_id, status, total, created_at)
- `CREATE TABLE products` (id, name, sku, price, stock, category)

**Go models (with `db` tags):**
- `go/std/models.go` — `User` (6 fields)
- `go/goqu/models.go` — `Order` (5 fields), `Product` (6 fields)

**Python models (with `__tablename__` + `Column()`):**
- `python/sqlalchemy/models.py` — `User`, `Order`, `Product` (all typed)

Main branch is clean — all models match DDL perfectly, zero schema-drift findings. PR branches will introduce violations to demo each rule.

### PR Branch Ideas for Violations
| Branch | What to change | Rule triggered |
|--------|---------------|----------------|
| `pr/dropped-column` | Add `db:"legacy_score"` field to a Go model | VG101 |
| `pr/missing-not-null` | Remove a `db` tag for a NOT NULL column from a model | VG102 |
| `pr/type-mismatch` | Change `Column(Integer)` to `Column(String)` in Python model | VG103 |
| `pr/table-not-found` | Add new Go model `Invoice` with no matching CREATE TABLE | VG104 |

---

## Deferred (post-launch)
- SQLAlchemy 2.0 `mapped_column()` support
- GORM `TableName()` method scanning for Go
- Config option: explicit table name overrides per model
- Schema-qualified table names (`public.users` vs `analytics.events`)
- `// valk-guard:table tablename` directive on Go structs
