package main

import (
	"fmt"
	"github.com/doug-martin/goqu/v9"
)

func main() {
	// VG001: Implicit Select Star (goqu defaults to SELECT * if no columns specified)
	ds := goqu.From("users")
	sql, _, _ := ds.ToSQL()
	fmt.Println(sql)

	// VG005: Leading Wildcard in Where clause
	// The scanner analyzes the method chain to detect the pattern.
	goqu.From("users").
		Where(goqu.C("email").Like("%@example.com")).
		Select("id")

	// VG002: Update without Where
	goqu.Update("users").
		Set(goqu.Record{"active": false}).
		Exec()

	// Valid Query
	goqu.From("users").
		Select("id", "email").
		Where(goqu.C("active").Eq(true)).
		Limit(10)
}
