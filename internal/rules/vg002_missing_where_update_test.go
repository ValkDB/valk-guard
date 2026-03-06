// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"context"
	"testing"
)

// TestMissingWhereUpdateRule validates UPDATE without WHERE detection.
func TestMissingWhereUpdateRule(t *testing.T) {
	rule := &MissingWhereUpdateRule{}

	tests := []struct {
		name      string
		sql       string
		wantCount int
	}{
		{
			name:      "update without where",
			sql:       "UPDATE users SET active = false",
			wantCount: 1,
		},
		{
			name:      "update with where",
			sql:       "UPDATE users SET active = false WHERE id = 1",
			wantCount: 0,
		},
		{
			name:      "update with constant true where",
			sql:       "UPDATE users SET active = false WHERE 1 = 1",
			wantCount: 1,
		},
		{
			name:      "update with true literal where",
			sql:       "UPDATE users SET active = false WHERE (TRUE)",
			wantCount: 1,
		},
		{
			name:      "update with subquery where",
			sql:       "UPDATE users SET active = false WHERE id IN (SELECT id FROM inactive)",
			wantCount: 0,
		},
		{
			name:      "update with placeholder plus predicate",
			sql:       "UPDATE users SET active = false WHERE 1 = 1 AND id = 1",
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
				findings = rule.Check(context.Background(), nil, "query.sql", 20, tt.sql)
			} else {
				parsed := parseSQL(t, tt.sql)
				findings = rule.Check(context.Background(), parsed, "query.sql", 20, tt.sql)
			}

			if len(findings) != tt.wantCount {
				t.Fatalf("expected %d findings, got %d: %+v", tt.wantCount, len(findings), findings)
			}
			for _, finding := range findings {
				if finding.RuleID != "VG002" {
					t.Errorf("expected rule ID VG002, got %s", finding.RuleID)
				}
			}
		})
	}
}
