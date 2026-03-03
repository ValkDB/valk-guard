// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/valkdb/postgresparser"
	"github.com/valkdb/valk-guard/internal/schema"
)

// testSchemaSnapshot returns a pre-built schema snapshot with common tables
// (users, orders) for use in query-schema rule tests (VG105, VG106, etc.).
func testSchemaSnapshot() *schema.Snapshot {
	snap := schema.NewSnapshot()
	snap.ApplyCreateTable("users", []schema.ColumnDef{
		{Name: "id", Type: "integer"},
		{Name: "email", Type: "text"},
	}, "migrations/001.sql", 1)
	snap.ApplyCreateTable("orders", []schema.ColumnDef{
		{Name: "id", Type: "integer"},
		{Name: "user_id", Type: "integer"},
	}, "migrations/002.sql", 1)
	return snap
}

// parseSQL parses SQL in tests and fails fast on parser errors.
func parseSQL(t *testing.T, sql string) *postgresparser.ParsedQuery {
	t.Helper()

	parsed, err := postgresparser.ParseSQL(sql)
	if err != nil {
		t.Fatalf("ParseSQL(%q) error: %v", sql, err)
	}
	if parsed == nil {
		t.Fatalf("ParseSQL(%q) returned nil parsed query", sql)
	}
	return parsed
}
