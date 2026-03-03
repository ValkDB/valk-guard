package main

import (
	"fmt"

	"github.com/doug-martin/goqu/v9"
)

func main() {
	// VG001: select-star (implicit SELECT *)
	ds := goqu.From("users")
	sql, _, _ := ds.ToSQL()
	_, _ = fmt.Println(sql)

	// VG002: missing-where-update
	goqu.Update("users").Set(goqu.Record{"active": false})

	// VG003: missing-where-delete
	goqu.Delete("sessions")

	// VG004: unbounded-select
	goqu.From("logs").Select("id")

	// VG005: like-leading-wildcard
	goqu.From("users").
		Where(goqu.C("email").Like("%@example.com")).
		Select("id")

	// VG006: select-for-update-no-where (ORM chain; requires scanner FOR UPDATE synthesis)
	goqu.From("accounts").ForUpdate(goqu.Wait).Select("id")

	// VG007: destructive-ddl (raw literal from goqu.L)
	goqu.L("DROP TABLE archived_users")

	// VG008: non-concurrent-index (raw literal from goqu.L)
	goqu.L("CREATE INDEX idx_users_email ON users(email)")

	// Valid query
	goqu.From("users").
		Select("id", "email").
		Where(goqu.C("active").Eq(true)).
		Limit(10)

	// Inline disable example
	// valk-guard:disable VG001
	goqu.From("config").Select(goqu.Star())
}
