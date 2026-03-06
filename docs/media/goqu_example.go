package demo

import "github.com/doug-martin/goqu/v9"

func dangerousQueries() {
	// No raw SQL here — just Goqu builder chains.
	// Valk Guard walks the AST and catches the problems.

	goqu.From("users")

	goqu.Update("orders").Set(goqu.Record{"status": "cancelled"})

	goqu.Delete("sessions")

	goqu.From("users").
		Where(goqu.C("email").Like("%@gmail.com")).
		Select("id")
}
