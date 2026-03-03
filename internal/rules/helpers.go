// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/valkdb/postgresparser"
)

var (
	// likeLeadingWildcardPattern matches LIKE/ILIKE predicates where the
	// string literal begins with a wildcard ('%...').
	likeLeadingWildcardPattern = regexp.MustCompile(`(?is)\b(?:NOT\s+)?I?LIKE\s+E?'\s*%`)
	// forUpdatePattern matches SELECT ... FOR UPDATE locking clauses.
	forUpdatePattern = regexp.MustCompile(`(?i)\bFOR\s+UPDATE\b`)
)

// newFinding builds a standardized Finding object with default column 1.
func newFinding(ruleID string, severity Severity, message, file string, line int, rawSQL string) Finding {
	return Finding{
		RuleID:   ruleID,
		Severity: severity,
		Message:  message,
		File:     file,
		Line:     line,
		Column:   1,
		SQL:      rawSQL,
	}
}

// isWildcardProjection reports whether a projection expression is "*" or
// a table-qualified wildcard such as "u.*".
func isWildcardProjection(expr string) bool {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "*" {
		return true
	}
	// Match patterns like users.* or u.*.
	return strings.HasSuffix(trimmed, ".*")
}

// hasClause reports whether at least one non-empty clause string exists.
func hasClause(items []string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return true
		}
	}
	return false
}

// hasLimitClause reports whether a parsed SELECT has a top-level LIMIT/FETCH.
func hasLimitClause(parsed *postgresparser.ParsedQuery) bool {
	if parsed == nil || parsed.Limit == nil {
		return false
	}
	return strings.TrimSpace(parsed.Limit.Limit) != ""
}

// queryClauses returns predicate-bearing clause text used for pattern checks.
func queryClauses(parsed *postgresparser.ParsedQuery) []string {
	var clauses []string
	clauses = append(clauses, parsed.Where...)
	clauses = append(clauses, parsed.Having...)
	clauses = append(clauses, parsed.JoinConditions...)
	return clauses
}

// hasLeadingWildcardLike reports whether any clause has LIKE/ILIKE with '%...'.
func hasLeadingWildcardLike(clauses []string) bool {
	for _, clause := range clauses {
		if likeLeadingWildcardPattern.MatchString(clause) {
			return true
		}
	}
	return false
}

// hasForUpdateClause reports whether SQL contains a FOR UPDATE lock clause.
// Comments and quoted segments are stripped first to avoid false positives from
// text like "-- check FOR UPDATE" or string literals containing "FOR UPDATE".
func hasForUpdateClause(sql string) bool {
	return forUpdatePattern.MatchString(stripSQL(sql, true))
}

// stripSQL removes comments from SQL text so regex-based checks do not match
// content inside comments. When stripQuoted is true, it also replaces string
// literals, dollar-quoted strings, and double-quoted identifiers with spaces
// to prevent false matches on quoted content.
func stripSQL(sql string, stripQuoted bool) string {
	var b strings.Builder
	b.Grow(len(sql))

	i := 0
	for i < len(sql) {
		// Single-line comment: skip to end of line.
		if sql[i] == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			i = skipLineComment(sql, i)
			continue
		}

		// Block comment: handle nesting.
		if sql[i] == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			i = skipBlockComment(sql, i, &b)
			continue
		}

		// Dollar-quoted string: $$...$$ or $tag$...$tag$.
		if stripQuoted && sql[i] == '$' {
			if newI, handled := skipDollarQuoted(sql, i, &b); handled {
				i = newI
				continue
			}
		}

		// Single-quoted string literal.
		if sql[i] == '\'' {
			i = skipSingleQuoted(sql, i, &b, stripQuoted)
			continue
		}

		// Double-quoted identifier.
		if stripQuoted && sql[i] == '"' {
			i = skipDoubleQuoted(sql, i, &b)
			continue
		}

		b.WriteByte(sql[i])
		i++
	}

	return b.String()
}

// skipLineComment advances past a -- single-line comment, returning the new
// position. The newline itself is not consumed so line numbers stay correct.
func skipLineComment(sql string, i int) int {
	for i < len(sql) && sql[i] != '\n' {
		i++
	}
	return i
}

// skipBlockComment advances past a /* ... */ block comment (handling nesting),
// preserving any embedded newlines in b. Returns the new position.
func skipBlockComment(sql string, i int, b *strings.Builder) int {
	depth := 1
	i += 2
	for i < len(sql) && depth > 0 {
		switch {
		case sql[i] == '/' && i+1 < len(sql) && sql[i+1] == '*':
			depth++
			i += 2
		case sql[i] == '*' && i+1 < len(sql) && sql[i+1] == '/':
			depth--
			i += 2
		default:
			if sql[i] == '\n' {
				b.WriteByte('\n')
			}
			i++
		}
	}
	return i
}

// skipDollarQuoted attempts to skip a dollar-quoted string starting at i.
// If a valid dollar-quote tag is found, it writes a single space placeholder
// into b (preserving embedded newlines) and returns (newI, true).
// If sql[i] is not the start of a dollar-quote, it returns (i, false).
func skipDollarQuoted(sql string, i int, b *strings.Builder) (int, bool) {
	tag := scanDollarQuoteTag(sql, i)
	if tag == "" {
		return i, false
	}
	i += len(tag)
	for i < len(sql) {
		if sql[i] == '\n' {
			b.WriteByte('\n')
		}
		if sql[i] == '$' && strings.HasPrefix(sql[i:], tag) {
			i += len(tag)
			break
		}
		i++
	}
	b.WriteByte(' ')
	return i, true
}

// skipSingleQuoted advances past a single-quoted string literal starting at i.
// When stripQuoted is true, a space placeholder is written into b and the
// literal content is discarded; otherwise the literal is copied verbatim.
// Returns the new position.
func skipSingleQuoted(sql string, i int, b *strings.Builder, stripQuoted bool) int {
	if stripQuoted {
		i++
		for i < len(sql) {
			if sql[i] == '\n' {
				b.WriteByte('\n')
			}
			if sql[i] == '\'' {
				i++
				if i < len(sql) && sql[i] == '\'' {
					i++
					continue
				}
				break
			}
			i++
		}
		b.WriteByte(' ')
		return i
	}

	b.WriteByte(sql[i])
	i++
	for i < len(sql) {
		b.WriteByte(sql[i])
		if sql[i] == '\'' {
			i++
			if i < len(sql) && sql[i] == '\'' {
				b.WriteByte(sql[i])
				i++
				continue
			}
			break
		}
		i++
	}
	return i
}

// skipDoubleQuoted advances past a double-quoted identifier starting at i,
// writing a space placeholder into b and preserving embedded newlines.
// Returns the new position.
func skipDoubleQuoted(sql string, i int, b *strings.Builder) int {
	i++
	for i < len(sql) {
		if sql[i] == '\n' {
			b.WriteByte('\n')
		}
		if sql[i] == '"' {
			i++
			if i < len(sql) && sql[i] == '"' {
				i++
				continue
			}
			break
		}
		i++
	}
	b.WriteByte(' ')
	return i
}

// stripSQLComments is a convenience wrapper that removes only comments,
// preserving string literals and identifiers.
func stripSQLComments(sql string) string {
	return stripSQL(sql, false)
}

// scanDollarQuoteTag checks whether sql[pos] starts a valid dollar-quote tag.
// It returns the full tag token (e.g., "$$" or "$tag$"), or "" if not found.
func scanDollarQuoteTag(sql string, pos int) string {
	if pos >= len(sql) || sql[pos] != '$' {
		return ""
	}
	j := pos + 1
	for j < len(sql) {
		ch := sql[j]
		if ch == '$' {
			return sql[pos : j+1]
		}
		if (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			ch == '_' ||
			(ch >= '0' && ch <= '9' && j > pos+1) {
			j++
			continue
		}
		break
	}
	return ""
}

// destructiveActionMessage returns a user-facing message for destructive DDL
// actions and false for non-destructive actions.
func destructiveActionMessage(action *postgresparser.DDLAction) (string, bool) {
	if action == nil {
		return "", false
	}

	switch action.Type {
	case postgresparser.DDLDropTable:
		if action.ObjectName != "" {
			return fmt.Sprintf("destructive DDL detected: DROP TABLE %s", action.ObjectName), true
		}
		return "destructive DDL detected: DROP TABLE", true
	case postgresparser.DDLDropColumn:
		if len(action.Columns) > 0 {
			return fmt.Sprintf("destructive DDL detected: DROP COLUMN %s", strings.Join(action.Columns, ", ")), true
		}
		return "destructive DDL detected: DROP COLUMN", true
	case postgresparser.DDLTruncate:
		if action.ObjectName != "" {
			return fmt.Sprintf("destructive DDL detected: TRUNCATE %s", action.ObjectName), true
		}
		return "destructive DDL detected: TRUNCATE", true
	default:
		return "", false
	}
}
