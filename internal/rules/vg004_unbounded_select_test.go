// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import "testing"

// TestUnboundedSelectRule validates SELECT without LIMIT/FETCH detection.
func TestUnboundedSelectRule(t *testing.T) {
	rule := &UnboundedSelectRule{}

	tests := []struct {
		name      string
		sql       string
		wantCount int
	}{
		{
			name:      "select without limit",
			sql:       "SELECT id FROM users",
			wantCount: 1,
		},
		{
			name:      "select with limit",
			sql:       "SELECT id FROM users LIMIT 10",
			wantCount: 0,
		},
		{
			name:      "select with fetch first",
			sql:       "SELECT id FROM users FETCH FIRST 10 ROWS ONLY",
			wantCount: 0,
		},
		{
			name:      "count aggregate without group by",
			sql:       "SELECT COUNT(*) FROM users",
			wantCount: 0,
		},
		{
			name:      "multiple aggregates without group by",
			sql:       "SELECT COUNT(*), MAX(id) FROM users",
			wantCount: 0,
		},
		{
			name:      "count aggregate with group by remains bounded by caller",
			sql:       "SELECT COUNT(*) FROM users GROUP BY status",
			wantCount: 1,
		},
		{
			name:      "parenthesized aggregate without group by",
			sql:       "SELECT (COUNT(*)) FROM users",
			wantCount: 0,
		},
		{
			name:      "constant select without table source",
			sql:       "SELECT 1",
			wantCount: 0,
		},
		{
			name:      "nil parsed query",
			sql:       "",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var findings []Finding
			if tt.name == "nil parsed query" {
				findings = rule.Check(nil, "query.sql", 35, tt.sql)
			} else {
				parsed := parseSQL(t, tt.sql)
				findings = rule.Check(parsed, "query.sql", 35, tt.sql)
			}

			if len(findings) != tt.wantCount {
				t.Fatalf("expected %d findings, got %d: %+v", tt.wantCount, len(findings), findings)
			}
			for _, finding := range findings {
				if finding.RuleID != "VG004" {
					t.Errorf("expected rule ID VG004, got %s", finding.RuleID)
				}
			}
		})
	}
}
