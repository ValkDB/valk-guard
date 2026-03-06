// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"context"
	"testing"
)

// TestMissingWhereDeleteRule validates DELETE without WHERE detection.
func TestMissingWhereDeleteRule(t *testing.T) {
	rule := &MissingWhereDeleteRule{}

	tests := []struct {
		name      string
		sql       string
		wantCount int
	}{
		{
			name:      "delete without where",
			sql:       "DELETE FROM users",
			wantCount: 1,
		},
		{
			name:      "delete with where",
			sql:       "DELETE FROM users WHERE id = 1",
			wantCount: 0,
		},
		{
			name:      "delete with constant true where",
			sql:       "DELETE FROM users WHERE 1 = 1",
			wantCount: 1,
		},
		{
			name:      "delete with true literal where",
			sql:       "DELETE FROM users WHERE (TRUE)",
			wantCount: 1,
		},
		{
			name:      "delete with subquery where",
			sql:       "DELETE FROM users WHERE id IN (SELECT id FROM inactive)",
			wantCount: 0,
		},
		{
			name:      "delete with placeholder plus predicate",
			sql:       "DELETE FROM users WHERE 1 = 1 AND id = 1",
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
				findings = rule.Check(context.Background(), nil, "query.sql", 30, tt.sql)
			} else {
				parsed := parseSQL(t, tt.sql)
				findings = rule.Check(context.Background(), parsed, "query.sql", 30, tt.sql)
			}

			if len(findings) != tt.wantCount {
				t.Fatalf("expected %d findings, got %d: %+v", tt.wantCount, len(findings), findings)
			}
			for _, finding := range findings {
				if finding.RuleID != "VG003" {
					t.Errorf("expected rule ID VG003, got %s", finding.RuleID)
				}
			}
		})
	}
}
