package rules

import (
	"testing"

	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

func TestUnknownFilterColumnRule(t *testing.T) {
	rule := &UnknownFilterColumnRule{}

	if rule.ID() != "VG106" {
		t.Fatalf("ID() = %q, want VG106", rule.ID())
	}
	if rule.DefaultSeverity() != SeverityError {
		t.Fatalf("DefaultSeverity() = %q, want %q", rule.DefaultSeverity(), SeverityError)
	}

	snap := schema.NewSnapshot()
	snap.ApplyCreateTable("users", []schema.ColumnDef{
		{Name: "id", Type: "integer"},
		{Name: "email", Type: "text"},
	}, "migrations/001.sql", 1)
	snap.ApplyCreateTable("orders", []schema.ColumnDef{
		{Name: "id", Type: "integer"},
		{Name: "user_id", Type: "integer"},
	}, "migrations/002.sql", 1)

	tests := []struct {
		name         string
		sql          string
		wantCount    int
		wantMessages []string
	}{
		{
			name:      "unknown where column",
			sql:       "SELECT id FROM users WHERE users.eitam = 1",
			wantCount: 1,
			wantMessages: []string{
				`filter predicate column "eitam" not found in table "users" schema`,
			},
		},
		{
			name:      "unknown inner join predicate column",
			sql:       "SELECT users.id FROM users INNER JOIN orders ON users.eitam = orders.user_id",
			wantCount: 1,
			wantMessages: []string{
				`join predicate column "eitam" not found in table "users" schema`,
			},
		},
		{
			name:      "unknown where and join columns",
			sql:       "SELECT users.id FROM users INNER JOIN orders ON users.id = orders.eitam WHERE orders.missing = 1",
			wantCount: 2,
			wantMessages: []string{
				`join predicate column "eitam" not found in table "orders" schema`,
				`filter predicate column "missing" not found in table "orders" schema`,
			},
		},
		{
			name:      "unknown unqualified filter in multi-table query skipped",
			sql:       "SELECT users.id FROM users INNER JOIN orders ON users.id = orders.user_id WHERE eitam = 1",
			wantCount: 0,
		},
		{
			name:      "all filter and join columns exist",
			sql:       "SELECT users.id FROM users INNER JOIN orders ON users.id = orders.user_id WHERE users.email = 'x'",
			wantCount: 0,
		},
		{
			name:      "non-select command skipped",
			sql:       "UPDATE users SET email = 'a' WHERE eitam = 1",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmt := scanner.SQLStatement{
				SQL:  tt.sql,
				File: "query.sql",
				Line: 7,
			}
			parsed := parseSQL(t, tt.sql)
			findings := rule.CheckQuerySchema(snap, stmt, parsed)

			if len(findings) != tt.wantCount {
				t.Fatalf("got %d findings, want %d: %+v", len(findings), tt.wantCount, findings)
			}

			byMessage := make(map[string]bool, len(findings))
			for _, f := range findings {
				byMessage[f.Message] = true
				if f.RuleID != "VG106" {
					t.Errorf("RuleID = %q, want VG106", f.RuleID)
				}
				if f.SQL == "" {
					t.Errorf("SQL should be preserved for query-schema rules")
				}
			}
			for _, want := range tt.wantMessages {
				if !byMessage[want] {
					t.Errorf("missing finding message %q in %+v", want, findings)
				}
			}
		})
	}
}
