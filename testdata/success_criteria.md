# Success Criteria: ORM Synthetic SQL

Every case below exercises a join or complex structure so scanners prove they
feed realistic SQL into the existing parser and rule engine.

## Group A: Raw SQL Baseline Complex Queries

1. VG001 select-star with joins:

```sql
SELECT * FROM users u LEFT JOIN orders o ON u.id = o.uid JOIN logs l ON u.id = l.uid;
```

2. VG005 invalid LIKE with joins:

```sql
SELECT u.id FROM users u JOIN comments c ON u.id = c.uid WHERE c.body LIKE '%error%';
```

3. VG002 update with join and missing WHERE:

```sql
UPDATE inventory i FROM shipments s SET i.stock = 0;
```

## Group B: Goqu Go AST to Synthetic SQL

4. VG001 select-star with join:

```go
goqu.From("users").Join(goqu.T("orders"), goqu.On(goqu.Ex{"users.id": goqu.I("orders.uid")})).Select(goqu.Star())
// Synthetic: SELECT * FROM users JOIN orders ON 1=1
```

5. VG005 leading wildcard LIKE:

```go
goqu.From("users").Join(goqu.T("profiles"), goqu.On(goqu.Ex{"users.id": goqu.I("profiles.uid")})).Where(goqu.C("email").Like("%@gmail.com")).Select("users.id")
// Synthetic: SELECT users.id FROM users JOIN profiles ON 1=1 WHERE email LIKE '%@gmail.com'
```

6. VG004 unbounded select with joins:

```go
goqu.From("logs").LeftJoin(goqu.T("users"), goqu.On(goqu.Ex{"logs.uid": goqu.I("users.id")})).Select("logs.id", "logs.msg")
// Synthetic: SELECT logs.id, logs.msg FROM logs LEFT JOIN users ON 1=1
```

## Group C: SQLAlchemy Python AST to Synthetic SQL

7. VG001 ORM select-star with join:

```python
session.query(User).join(Address).all()
# Synthetic: SELECT * FROM User JOIN Address ON 1=1
```

8. VG005 invalid LIKE pattern:

```python
session.query(User).join(Address).filter(Address.street.like("%Main%")).all()
# Synthetic: SELECT * FROM User JOIN Address ON 1=1 WHERE street LIKE '%Main%'
```

9. VG003 delete with join and missing filter:

```python
session.query(User).join(Roles).delete()
# Synthetic: DELETE FROM User
```

## Group D: C# EF Core Roslyn AST to Synthetic SQL

10. VG001 select-star with include join:

```csharp
db.Users.Include(u => u.Orders).ToList();
// Synthetic: SELECT * FROM Users LEFT JOIN Orders ON 1=1
```

11. VG005 invalid LIKE pattern:

```csharp
db.Users.Include(u => u.Profile).Where(u => u.Email.Contains("@gmail.com")).ToList();
// Synthetic: SELECT * FROM Users LEFT JOIN Profile ON 1=1 WHERE Email LIKE '%@gmail.com%'
```

12. VG003 delete with join-like include and missing filter:

```csharp
db.Users.Include(u => u.Roles).ExecuteDelete();
// Synthetic: DELETE FROM Users
```
