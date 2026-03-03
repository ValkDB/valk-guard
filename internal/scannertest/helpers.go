// Package scannertest provides shared test helpers for scanner sub-package tests.
package scannertest

import (
	"strings"
	"testing"

	"github.com/valkdb/valk-guard/internal/rules"
	"github.com/valkdb/valk-guard/internal/scanner"
)

// HasSQL reports whether any statement has exactly the given SQL text.
func HasSQL(stmts []scanner.SQLStatement, want string) bool {
	for _, stmt := range stmts {
		if stmt.SQL == want {
			return true
		}
	}
	return false
}

// HasSQLContaining reports whether any statement's SQL contains the substring.
func HasSQLContaining(stmts []scanner.SQLStatement, want string) bool {
	for _, stmt := range stmts {
		if strings.Contains(stmt.SQL, want) {
			return true
		}
	}
	return false
}

// CollectFindingsByRule parses each statement and runs all rules, returning
// a map of rule ID to finding count.
func CollectFindingsByRule(t *testing.T, stmts []scanner.SQLStatement) map[string]int {
	t.Helper()

	findingsByRule := make(map[string]int)
	reg := rules.DefaultRegistry()

	for _, stmt := range stmts {
		parsed, err := scanner.ParseStatement(stmt.SQL)
		if err != nil {
			t.Fatalf("failed to parse SQL %q: %v", stmt.SQL, err)
		}
		if parsed == nil {
			continue
		}
		for _, rule := range reg.All() {
			finds := rule.Check(parsed, stmt.File, stmt.Line, stmt.SQL)
			if len(finds) > 0 {
				findingsByRule[rule.ID()] += len(finds)
			}
		}
	}

	return findingsByRule
}
