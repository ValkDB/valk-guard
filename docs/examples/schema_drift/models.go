//go:build ignore

// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"log"
)

// User maps to the "users" table defined in migrations/001_create_users.sql.
// The LegacyField column does not exist in the migration — this drift is
// intentional and demonstrates the kind of mismatch valk-guard can surface
// when migration files and Go models diverge.
type User struct {
	ID          int    `db:"id"`
	Email       string `db:"email"`
	Name        string `db:"name"`
	LegacyField string `db:"legacy_field"` // schema drift: column absent from migration
}

// TableName returns the database table this model maps to.
func (User) TableName() string { return "users" }

func main() {
	db, err := sql.Open("postgres", "postgres://localhost/example?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}

	// VG001: select-star — fetches legacy_field even though it doesn't exist
	_, _ = db.Query("SELECT * FROM users")

	// VG004: unbounded-select — no LIMIT, full table scan risk
	_, _ = db.Query("SELECT id, email, name, legacy_field FROM users WHERE active = true")
}
