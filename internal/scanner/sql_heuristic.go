// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package scanner

import "strings"

// sqlKeywordSet maps known SQL statement-opening keywords to an empty struct
// for O(1) lookup. The LooksLikeSQL function extracts the first word from the
// input and checks for membership here instead of iterating a slice.
var sqlKeywordSet = map[string]struct{}{
	"SELECT":   {},
	"INSERT":   {},
	"UPDATE":   {},
	"DELETE":   {},
	"CREATE":   {},
	"DROP":     {},
	"ALTER":    {},
	"TRUNCATE": {},
	"WITH":     {},
	"GRANT":    {},
	"REVOKE":   {},
	"BEGIN":    {},
	"COMMIT":   {},
	"ROLLBACK": {},
	"SET":      {},
	"COPY":     {},
	"VACUUM":   {},
	"ANALYZE":  {},
	"EXPLAIN":  {},
	"MERGE":    {},
}

// LooksLikeSQL reports whether the string appears to be a SQL statement
// by checking for common starting keywords or comments.
// It extracts only the first word (letters A-Z) and performs an O(1) map
// lookup instead of a linear scan over all keywords.
func LooksLikeSQL(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// Allow SQL line/block comments as valid statement starts.
	if strings.HasPrefix(s, "--") || strings.HasPrefix(s, "/*") {
		return true
	}
	upper := strings.ToUpper(s)
	// Extract the leading alphabetic word (SQL keywords are all letters).
	end := 0
	for end < len(upper) && upper[end] >= 'A' && upper[end] <= 'Z' {
		end++
	}
	if end == 0 {
		return false
	}
	// Ensure the keyword is not a prefix of a longer identifier (e.g. "SELECTOR").
	if end < len(upper) && isSQLIdentChar(upper[end]) {
		return false
	}
	_, ok := sqlKeywordSet[upper[:end]]
	return ok
}

func isSQLIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_'
}
