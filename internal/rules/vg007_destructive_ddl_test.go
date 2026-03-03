// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"
)

// TestDestructiveDDLRule validates destructive DDL detection.
func TestDestructiveDDLRule(t *testing.T) {
	rule := &DestructiveDDLRule{}

	tests := []struct {
		name          string
		sql           string
		wantCount     int
		wantMessageIn string
	}{
		{
			name:          "drop table",
			sql:           "DROP TABLE users",
			wantCount:     1,
			wantMessageIn: "DROP TABLE",
		},
		{
			name:          "truncate table",
			sql:           "TRUNCATE TABLE users",
			wantCount:     1,
			wantMessageIn: "TRUNCATE",
		},
		{
			name:          "drop column",
			sql:           "ALTER TABLE users DROP COLUMN email",
			wantCount:     1,
			wantMessageIn: "DROP COLUMN",
		},
		{
			name:      "non destructive ddl",
			sql:       "CREATE INDEX idx_users_email ON users (email)",
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
				findings = rule.Check(nil, "query.sql", 40, tt.sql)
			} else {
				parsed := parseSQL(t, tt.sql)
				findings = rule.Check(parsed, "query.sql", 40, tt.sql)
			}

			if len(findings) != tt.wantCount {
				t.Fatalf("expected %d findings, got %d: %+v", tt.wantCount, len(findings), findings)
			}
			for _, finding := range findings {
				if finding.RuleID != "VG007" {
					t.Errorf("expected rule ID VG007, got %s", finding.RuleID)
				}
				if tt.wantMessageIn != "" && !strings.Contains(finding.Message, tt.wantMessageIn) {
					t.Errorf("expected message to contain %q, got %q", tt.wantMessageIn, finding.Message)
				}
			}
		})
	}
}
