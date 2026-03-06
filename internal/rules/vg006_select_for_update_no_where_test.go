// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"context"
	"testing"
)

// TestSelectForUpdateNoWhereRule validates FOR UPDATE without WHERE detection.
func TestSelectForUpdateNoWhereRule(t *testing.T) {
	rule := &SelectForUpdateNoWhereRule{}

	tests := []struct {
		name      string
		sql       string
		wantCount int
	}{
		{
			name:      "for update without where",
			sql:       "SELECT id FROM users FOR UPDATE",
			wantCount: 1,
		},
		{
			name:      "for update with where",
			sql:       "SELECT id FROM users WHERE id = 1 FOR UPDATE",
			wantCount: 0,
		},
		{
			name:      "for share does not match",
			sql:       "SELECT id FROM users FOR SHARE",
			wantCount: 0,
		},
		{
			name:      "for update inside line comment is not a match",
			sql:       "SELECT id FROM users WHERE id = 1 -- FOR UPDATE",
			wantCount: 0,
		},
		{
			name:      "for update inside block comment is not a match",
			sql:       "SELECT id FROM users WHERE id = 1 /* FOR UPDATE */",
			wantCount: 0,
		},
		{
			name:      "for update inside quoted identifier is not a match",
			sql:       `SELECT 1 AS "FOR UPDATE"`,
			wantCount: 0,
		},
		{
			name:      "for update inside string literal is not a match",
			sql:       "SELECT 'FOR UPDATE'",
			wantCount: 0,
		},
		{
			name:      "for update after block comment is still detected",
			sql:       "SELECT id FROM users /* comment */ FOR UPDATE",
			wantCount: 1,
		},
		{
			name:      "for update with limit one is not flagged",
			sql:       "SELECT id FROM jobs FOR UPDATE LIMIT 1",
			wantCount: 0,
		},
		{
			name:      "for update skip locked limit one worker pattern",
			sql:       "SELECT id FROM jobs FOR UPDATE SKIP LOCKED LIMIT 1",
			wantCount: 0,
		},
		{
			name:      "for update skip locked without limit still flagged",
			sql:       "SELECT id FROM jobs FOR UPDATE SKIP LOCKED",
			wantCount: 1,
		},
		{
			name:      "for update with constant true WHERE 1=1 is flagged",
			sql:       "SELECT * FROM users WHERE 1 = 1 FOR UPDATE",
			wantCount: 1,
		},
		{
			name:      "for update with constant true WHERE TRUE is flagged",
			sql:       "SELECT * FROM users WHERE TRUE FOR UPDATE",
			wantCount: 1,
		},
		{
			name:      "for update with 1=1 AND real predicate is not flagged",
			sql:       "SELECT * FROM users WHERE 1 = 1 AND id = 5 FOR UPDATE",
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
				findings = rule.Check(context.Background(), nil, "query.sql", 42, tt.sql)
			} else {
				parsed := parseSQL(t, tt.sql)
				findings = rule.Check(context.Background(), parsed, "query.sql", 42, tt.sql)
			}

			if len(findings) != tt.wantCount {
				t.Fatalf("expected %d findings, got %d: %+v", tt.wantCount, len(findings), findings)
			}
			for _, finding := range findings {
				if finding.RuleID != "VG006" {
					t.Errorf("expected rule ID VG006, got %s", finding.RuleID)
				}
			}
		})
	}
}
