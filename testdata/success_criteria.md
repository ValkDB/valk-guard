# Valk Guard Strategic Refactor Success Criteria

This document defines 9 validation cases for the AST-to-Synthetic-SQL refactor focused on Goqu and SQLAlchemy.

## Group A: Raw SQL (Baseline Complex Queries)

### 1. VG001 (Select Star w/ Joins)

```sql
SELECT * FROM users u LEFT JOIN orders o ON u.id = o.uid JOIN logs l ON u.id = l.uid;
```

Expected: `VG001` (select-star) should trigger.

### 2. VG005 (Invalid LIKE w/ Joins)

```sql
SELECT u.id FROM users u JOIN comments c ON u.id = c.uid WHERE c.body LIKE '%error%';
```

Expected: `VG005` (leading wildcard LIKE) should trigger.

### 3. VG002 (Update w/ Join & Missing Where)

```sql
UPDATE inventory i FROM shipments s SET i.stock = 0; -- Postgres supports UPDATE ... FROM
```

Expected: `VG002` (update without where) should trigger.

## Group B: Goqu (Go AST -> Synthetic SQL)

### 4. VG001 (Select Star w/ Join)

Input (builder chain):

```go
goqu.From("users").Join(goqu.T("orders"), ...).Select(goqu.Star())
```

Synthetic target shape:

```sql
SELECT * FROM users JOIN orders ON 1=1
```

Expected: `VG001` should trigger.

### 5. VG005 (Leading Wildcard LIKE)

Input (builder chain):

```go
goqu.From("users").Join(...).Where(goqu.C("email").Like("%@gmail.com"))
```

Synthetic target shape:

```sql
SELECT ... FROM users JOIN ... ON 1=1 WHERE email LIKE '%@gmail.com'
```

Expected: `VG005` should trigger.

### 6. VG004 (Unbounded Select w/ Joins)

Input (builder chain):

```go
goqu.From("logs").LeftJoin("users", ...).Select("id", "msg")
```

Synthetic target shape:

```sql
SELECT id, msg FROM logs LEFT JOIN users ON 1=1
```

Expected: `VG004` should trigger due to missing `LIMIT`.

## Group C: SQLAlchemy (Python AST -> Synthetic SQL)

### 7. VG001 (ORM Select Star w/ Join)

Input (ORM chain):

```python
session.query(User).join(Address).all()
```

Synthetic target shape:

```sql
SELECT * FROM User JOIN Address ON 1=1
```

Expected: `VG001` should trigger.

### 8. VG005 (Invalid LIKE Pattern)

Input (ORM chain):

```python
session.query(User).join(Address).filter(Address.street.like("%Main%"))
```

Synthetic target shape:

```sql
SELECT * FROM User JOIN Address ON 1=1 WHERE Address.street LIKE '%Main%'
```

Expected: `VG005` should trigger.

### 9. VG003 (Delete w/ Join & Missing Filter)

Input (ORM chain):

```python
session.query(User).join(Roles).delete()
```

Synthetic target shape:

```sql
DELETE FROM User USING Roles
```

Expected: `VG003` should trigger due to missing filter/where.
