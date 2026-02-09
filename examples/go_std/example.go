package main

import (
	"database/sql"
	"log"
)

func main() {
	db, err := sql.Open("postgres", "postgres://user:pass@localhost/db")
	if err != nil {
		log.Fatal(err)
	}

	// VG001: Select Star detected in string literal
	rows, _ := db.Query("SELECT * FROM users")
	defer rows.Close()

	// VG004: Unbounded Select
	db.QueryRow("SELECT id FROM users WHERE email = $1", "test@example.com")

	// Valid Query
	db.Exec("UPDATE users SET last_login = NOW() WHERE id = $1", 1)

	// Inline Disable
	// valk-guard:disable VG001
	db.Query("SELECT * FROM config")
}
