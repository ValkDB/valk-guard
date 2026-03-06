// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"context"
	"testing"
)

// TestNonConcurrentIndexRule validates CREATE INDEX without CONCURRENTLY detection.
func TestNonConcurrentIndexRule(t *testing.T) {
	rule := &NonConcurrentIndexRule{}

	tests := []struct {
		name      string
		sql       string
		wantCount int
	}{
		{
			name:      "create index without concurrently",
			sql:       "CREATE INDEX idx_users_email ON users (email)",
			wantCount: 1,
		},
		{
			name:      "create index concurrently",
			sql:       "CREATE INDEX CONCURRENTLY idx_users_email ON users (email)",
			wantCount: 0,
		},
		{
			name:      "create unique index concurrently",
			sql:       "CREATE UNIQUE INDEX CONCURRENTLY idx_users_email ON users (email)",
			wantCount: 0,
		},
		{
			name:      "non create index ddl",
			sql:       "DROP INDEX idx_users_email",
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
				findings = rule.Check(context.Background(), nil, "query.sql", 45, tt.sql)
			} else {
				parsed := parseSQL(t, tt.sql)
				findings = rule.Check(context.Background(), parsed, "query.sql", 45, tt.sql)
			}

			if len(findings) != tt.wantCount {
				t.Fatalf("expected %d findings, got %d: %+v", tt.wantCount, len(findings), findings)
			}
			for _, finding := range findings {
				if finding.RuleID != "VG008" {
					t.Errorf("expected rule ID VG008, got %s", finding.RuleID)
				}
			}
		})
	}
}
