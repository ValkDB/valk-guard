package rules

import (
	"testing"

	"github.com/valkdb/postgresparser"
)

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
