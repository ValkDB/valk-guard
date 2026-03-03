# Schema Drift Example

This example demonstrates what happens when a Go model diverges from the
corresponding database migration.

## The drift

`migrations/001_create_users.sql` defines the `users` table with three columns:
`id`, `email`, and `name`.

`models.go` declares a `User` struct with a fourth `db:"legacy_field"` tag that
has no matching column in the migration. This is schema drift: the code
references a column the database does not have, which will cause runtime errors.

## Running valk-guard

```bash
valk-guard scan examples/schema_drift/
```

Expected output includes findings such as:

- `VG001 select-star` — `SELECT *` will silently include or exclude columns as
  the schema changes.
- `VG004 unbounded-select` — querying without `LIMIT` can cause full-table
  scans as the dataset grows.
- `VG101` (schema-aware rules, if enabled) — flags `legacy_field` as a column
  present in the model but absent from the migration.

## What this illustrates

Models and migration files can fall out of sync over time. The `db:"legacy_field"`
tag in the `User` struct has no corresponding column in
`migrations/001_create_users.sql`. Running valk-guard against both the Go source
and the migration file surfaces this mismatch before it reaches production.
