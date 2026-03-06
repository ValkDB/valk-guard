// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"context"
	"testing"
)

// TestSelectStarRule validates wildcard projection detection.
func TestSelectStarRule(t *testing.T) {
	rule := &SelectStarRule{}

	tests := []struct {
		name      string
		sql       string
		wantCount int
	}{
		{
			name:      "wildcard star projection",
			sql:       "SELECT * FROM users",
			wantCount: 1,
		},
		{
			name:      "table wildcard projection",
			sql:       "SELECT u.* FROM users u",
			wantCount: 1,
		},
		{
			name:      "explicit columns only",
			sql:       "SELECT id, email FROM users",
			wantCount: 0,
		},
		{
			name:      "count star aggregate should not match",
			sql:       "SELECT COUNT(*) FROM users",
			wantCount: 0,
		},
		{
			name:      "star in subquery",
			sql:       "SELECT * FROM (SELECT id FROM users) sub",
			wantCount: 1,
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
				findings = rule.Check(context.Background(), nil, "query.sql", 12, tt.sql)
			} else {
				parsed := parseSQL(t, tt.sql)
				findings = rule.Check(context.Background(), parsed, "query.sql", 12, tt.sql)
			}

			if len(findings) != tt.wantCount {
				t.Fatalf("expected %d findings, got %d: %+v", tt.wantCount, len(findings), findings)
			}
			for _, finding := range findings {
				if finding.RuleID != "VG001" {
					t.Errorf("expected rule ID VG001, got %s", finding.RuleID)
				}
			}
		})
	}
}
