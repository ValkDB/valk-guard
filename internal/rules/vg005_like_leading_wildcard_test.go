package rules

import "testing"

// TestLikeLeadingWildcardRule validates leading wildcard LIKE/ILIKE detection.
func TestLikeLeadingWildcardRule(t *testing.T) {
	rule := &LikeLeadingWildcardRule{}

	tests := []struct {
		name      string
		sql       string
		wantCount int
	}{
		{
			name:      "like with leading wildcard",
			sql:       "SELECT id FROM users WHERE name LIKE '%abc'",
			wantCount: 1,
		},
		{
			name:      "ilike with leading wildcard",
			sql:       "SELECT id FROM users WHERE name ILIKE '%abc'",
			wantCount: 1,
		},
		{
			name:      "not like with leading wildcard",
			sql:       "SELECT id FROM users WHERE name NOT LIKE '%abc'",
			wantCount: 1,
		},
		{
			name:      "like with trailing wildcard only",
			sql:       "SELECT id FROM users WHERE name LIKE 'abc%'",
			wantCount: 0,
		},
		{
			name:      "like without wildcard",
			sql:       "SELECT id FROM users WHERE name LIKE 'abc'",
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
				findings = rule.Check(nil, "query.sql", 38, tt.sql)
			} else {
				parsed := parseSQL(t, tt.sql)
				findings = rule.Check(parsed, "query.sql", 38, tt.sql)
			}

			if len(findings) != tt.wantCount {
				t.Fatalf("expected %d findings, got %d: %+v", tt.wantCount, len(findings), findings)
			}
			for _, finding := range findings {
				if finding.RuleID != "VG005" {
					t.Errorf("expected rule ID VG005, got %s", finding.RuleID)
				}
			}
		})
	}
}
