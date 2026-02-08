package rules

import (
	"strings"
	"testing"

	"github.com/valkdb/postgresparser"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseSQL(t, tt.sql)
			findings := rule.Check(parsed, "query.sql", 12, tt.sql)
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
			name:      "update with subquery where",
			sql:       "UPDATE users SET active = false WHERE id IN (SELECT id FROM inactive)",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseSQL(t, tt.sql)
			findings := rule.Check(parsed, "query.sql", 20, tt.sql)
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
			name:      "delete with subquery where",
			sql:       "DELETE FROM users WHERE id IN (SELECT id FROM inactive)",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseSQL(t, tt.sql)
			findings := rule.Check(parsed, "query.sql", 30, tt.sql)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseSQL(t, tt.sql)
			findings := rule.Check(parsed, "query.sql", 35, tt.sql)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseSQL(t, tt.sql)
			findings := rule.Check(parsed, "query.sql", 38, tt.sql)
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
			name:      "for update after block comment is still detected",
			sql:       "SELECT id FROM users /* comment */ FOR UPDATE",
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseSQL(t, tt.sql)
			findings := rule.Check(parsed, "query.sql", 42, tt.sql)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseSQL(t, tt.sql)
			findings := rule.Check(parsed, "query.sql", 40, tt.sql)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseSQL(t, tt.sql)
			findings := rule.Check(parsed, "query.sql", 45, tt.sql)
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

// TestStripSQLComments validates the comment-stripping helper.
func TestStripSQLComments(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "no comments",
			sql:  "SELECT 1",
			want: "SELECT 1",
		},
		{
			name: "line comment stripped",
			sql:  "SELECT 1 -- a comment",
			want: "SELECT 1 ",
		},
		{
			name: "block comment stripped",
			sql:  "SELECT /* hidden */ 1",
			want: "SELECT  1",
		},
		{
			name: "nested block comment stripped",
			sql:  "SELECT /* outer /* inner */ end */ 1",
			want: "SELECT  1",
		},
		{
			name: "string not stripped",
			sql:  "SELECT '-- not a comment' FROM t",
			want: "SELECT '-- not a comment' FROM t",
		},
		{
			name: "mixed comments and strings",
			sql:  "SELECT '/* ok */' -- line comment\nFROM t /* block */",
			want: "SELECT '/* ok */' \nFROM t ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripSQLComments(tt.sql)
			if got != tt.want {
				t.Errorf("stripSQLComments(%q)\n  got  %q\n  want %q", tt.sql, got, tt.want)
			}
		})
	}
}

// TestNilParsedQueryRules verifies that all rules handle nil parsed input gracefully.
func TestNilParsedQueryRules(t *testing.T) {
	allRules := []Rule{
		&SelectStarRule{},
		&MissingWhereUpdateRule{},
		&MissingWhereDeleteRule{},
		&UnboundedSelectRule{},
		&LikeLeadingWildcardRule{},
		&SelectForUpdateNoWhereRule{},
		&DestructiveDDLRule{},
		&NonConcurrentIndexRule{},
	}

	for _, rule := range allRules {
		t.Run(rule.ID()+"_nil", func(t *testing.T) {
			findings := rule.Check(nil, "test.sql", 1, "")
			if len(findings) != 0 {
				t.Errorf("%s: expected 0 findings for nil input, got %d", rule.ID(), len(findings))
			}
		})
	}
}

// parseSQL parses SQL in tests and fails fast on parser errors.
func parseSQL(t *testing.T, sql string) *postgresparser.ParsedQuery {
	t.Helper()

	parsed, err := postgresparser.ParseSQL(sql)
	if err != nil {
		t.Fatalf("ParseSQL(%q) error: %v", sql, err)
	}
	if parsed == nil {
		t.Fatalf("ParseSQL(%q) returned nil parsed query", sql)
	}
	return parsed
}
