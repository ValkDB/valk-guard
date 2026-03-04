// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/scanner"
)

func TestUnknownTableReferenceRule(t *testing.T) {
	t.Parallel()

	rule := &UnknownTableReferenceRule{}
	if rule.ID() != "VG107" {
		t.Fatalf("ID() = %q, want VG107", rule.ID())
	}

	snap := testSchemaSnapshot()
	tests := []struct {
		name      string
		sql       string
		wantCount int
		wantMsg   string
	}{
		{
			name:      "unknown join table",
			sql:       "SELECT users.id FROM users INNER JOIN ghost_orders ON users.id = ghost_orders.user_id",
			wantCount: 1,
			wantMsg:   `table "ghost_orders" referenced in query not found in schema`,
		},
		{
			name:      "known tables",
			sql:       "SELECT users.id FROM users INNER JOIN orders ON users.id = orders.user_id",
			wantCount: 0,
		},
		{
			name:      "cte table skipped",
			sql:       "WITH q AS (SELECT id FROM users) SELECT id FROM q",
			wantCount: 0,
		},
		{
			name:      "sqlalchemy singular table does not match plural table",
			sql:       `SELECT "User".id FROM "User"`,
			wantCount: 1,
			wantMsg:   `table "user" referenced in query not found in schema`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stmt := scanner.SQLStatement{SQL: tt.sql, File: "query.sql", Line: 11}
			parsed := parseSQL(t, tt.sql)
			findings := rule.CheckQuerySchema(snap, &stmt, parsed)
			if len(findings) != tt.wantCount {
				t.Fatalf("got %d findings, want %d: %+v", len(findings), tt.wantCount, findings)
			}
			if tt.wantMsg != "" && len(findings) > 0 && findings[0].Message != tt.wantMsg {
				t.Fatalf("message = %q, want %q", findings[0].Message, tt.wantMsg)
			}
		})
	}
}
