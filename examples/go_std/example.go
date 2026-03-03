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

	// VG001: select-star
	_, _ = db.Query("SELECT * FROM users")

	// VG002: missing-where-update
	_, _ = db.Exec("UPDATE users SET active = false")

	// VG003: missing-where-delete
	_, _ = db.Exec("DELETE FROM sessions")

	// VG004: unbounded-select
	_ = db.QueryRow("SELECT id FROM users WHERE email = $1", "test@example.com")

	// VG005: like-leading-wildcard
	_, _ = db.Query("SELECT id FROM users WHERE email LIKE '%@example.com'")

	// VG006: select-for-update-no-where
	_, _ = db.Query("SELECT * FROM accounts FOR UPDATE")

	// VG007: destructive-ddl
	_, _ = db.Exec("DROP TABLE archived_users")

	// VG008: non-concurrent-index
	_, _ = db.Exec("CREATE INDEX idx_users_email ON users(email)")

	// Valid query
	_, _ = db.Exec("UPDATE users SET last_login = NOW() WHERE id = $1", 1)

	// Inline disable
	// valk-guard:disable VG001
	_, _ = db.Query("SELECT * FROM config")
}
