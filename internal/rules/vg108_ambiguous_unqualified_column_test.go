// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/valkdb/valk-guard/internal/scanner"
)

func TestAmbiguousUnqualifiedColumnRule(t *testing.T) {
	t.Parallel()

	rule := &AmbiguousUnqualifiedColumnRule{}
	if rule.ID() != "VG108" {
		t.Fatalf("ID() = %q, want VG108", rule.ID())
	}

	snap := testSchemaSnapshot()
	tests := []struct {
		name      string
		sql       string
		wantCount int
		wantPart  string
	}{
		{
			name:      "ambiguous projection column",
			sql:       "SELECT id FROM users u INNER JOIN orders o ON u.id = o.user_id",
			wantCount: 1,
			wantPart:  `projection column "id" is ambiguous`,
		},
		{
			name:      "qualified projection not ambiguous",
			sql:       "SELECT u.id FROM users u INNER JOIN orders o ON u.id = o.user_id",
			wantCount: 0,
		},
		{
			name:      "single-owner unqualified filter",
			sql:       "SELECT users.id FROM users INNER JOIN orders ON users.id = orders.user_id WHERE email = 'x'",
			wantCount: 0,
		},
		{
			name:      "single table query",
			sql:       "SELECT id FROM users",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stmt := scanner.SQLStatement{SQL: tt.sql, File: "query.sql", Line: 4}
			parsed := parseSQL(t, tt.sql)
			findings := rule.CheckQuerySchema(snap, stmt, parsed)
			if len(findings) != tt.wantCount {
				t.Fatalf("got %d findings, want %d: %+v", len(findings), tt.wantCount, findings)
			}
			if tt.wantPart != "" && len(findings) > 0 && !strings.Contains(findings[0].Message, tt.wantPart) {
				t.Fatalf("message = %q, want to contain %q", findings[0].Message, tt.wantPart)
			}
		})
	}
}
