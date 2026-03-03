// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/scanner"
)

func TestUnknownProjectionColumnRule(t *testing.T) {
	t.Parallel()

	rule := &UnknownProjectionColumnRule{}

	if rule.ID() != "VG105" {
		t.Fatalf("ID() = %q, want VG105", rule.ID())
	}
	if rule.DefaultSeverity() != SeverityError {
		t.Fatalf("DefaultSeverity() = %q, want %q", rule.DefaultSeverity(), SeverityError)
	}

	snap := testSchemaSnapshot()

	tests := []struct {
		name      string
		sql       string
		wantCount int
		wantMsg   string
	}{
		{
			name:      "qualified unknown projection column",
			sql:       "SELECT users.id, users.eitam FROM users",
			wantCount: 1,
			wantMsg:   `projection column "eitam" not found in table "users" schema; check SELECT list and schema/model mappings`,
		},
		{
			name:      "unqualified unknown projection with single table",
			sql:       "SELECT id, eitam FROM users",
			wantCount: 1,
			wantMsg:   `projection column "eitam" not found in table "users" schema; check SELECT list and schema/model mappings`,
		},
		{
			name:      "unknown unqualified projection in multi-table query skipped",
			sql:       "SELECT eitam FROM users u INNER JOIN orders o ON u.id = o.user_id",
			wantCount: 0,
		},
		{
			name:      "wildcard projection skipped",
			sql:       "SELECT * FROM users",
			wantCount: 0,
		},
		{
			name:      "sqlalchemy-style singular table name has no schema match",
			sql:       `SELECT "User".eitam FROM "User"`,
			wantCount: 0,
		},
		{
			name:      "non-select command skipped",
			sql:       "UPDATE users SET email = 'a' WHERE id = 1",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stmt := scanner.SQLStatement{
				SQL:  tt.sql,
				File: "query.sql",
				Line: 12,
			}
			parsed := parseSQL(t, tt.sql)
			findings := rule.CheckQuerySchema(snap, stmt, parsed)
			if len(findings) != tt.wantCount {
				t.Fatalf("got %d findings, want %d: %+v", len(findings), tt.wantCount, findings)
			}
			if tt.wantMsg != "" && len(findings) > 0 && findings[0].Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", findings[0].Message, tt.wantMsg)
			}
			for _, f := range findings {
				if f.RuleID != "VG105" {
					t.Errorf("RuleID = %q, want VG105", f.RuleID)
				}
				if f.SQL == "" {
					t.Errorf("SQL should be preserved for query-schema rules")
				}
			}
		})
	}
}
