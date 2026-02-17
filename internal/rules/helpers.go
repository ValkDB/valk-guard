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
	likeLeadingWildcardPattern = regexp.MustCompile(`(?is)\b(?:NOT\s+)?I?LIKE\s+E?'[[:space:]]*%`)
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
// Comments are stripped first to avoid false positives from text like
// "-- check FOR UPDATE" or "/* FOR UPDATE */".
func hasForUpdateClause(sql string) bool {
	return forUpdatePattern.MatchString(stripSQLForUpdateCheck(sql))
}

// stripSQLForUpdateCheck removes comments and quoted segments that may contain
// arbitrary text so regex checks do not match phrases like "FOR UPDATE" inside
// string literals or quoted identifiers.
func stripSQLForUpdateCheck(sql string) string {
	var b strings.Builder
	b.Grow(len(sql))

	i := 0
	for i < len(sql) {
		// Single-line comment: skip to end of line.
		if sql[i] == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			continue
		}

		// Block comment: handle nesting.
		if sql[i] == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			depth := 1
			i += 2
			for i < len(sql) && depth > 0 {
				if sql[i] == '/' && i+1 < len(sql) && sql[i+1] == '*' {
					depth++
					i += 2
				} else if sql[i] == '*' && i+1 < len(sql) && sql[i+1] == '/' {
					depth--
					i += 2
				} else {
					if sql[i] == '\n' {
						b.WriteByte('\n')
					}
					i++
				}
			}
			continue
		}

		// Dollar-quoted string: $$...$$ or $tag$...$tag$.
		if sql[i] == '$' {
			tag := scanDollarQuoteTag(sql, i)
			if tag != "" {
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
				continue
			}
		}

		// Single-quoted string literal.
		if sql[i] == '\'' {
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
			continue
		}

		// Double-quoted identifier.
		if sql[i] == '"' {
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
			continue
		}

		b.WriteByte(sql[i])
		i++
	}

	return b.String()
}

// stripSQLComments removes single-line (--) and block (/* */) comments from
// SQL text so that regex-based checks do not match content inside comments.
// Nested block comments (valid in PostgreSQL) are handled correctly.
func stripSQLComments(sql string) string {
	var b strings.Builder
	b.Grow(len(sql))
	i := 0
	for i < len(sql) {
		// Single-line comment: skip to end of line.
		if sql[i] == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			continue
		}
		// Block comment: handle nesting.
		if sql[i] == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			depth := 1
			i += 2
			for i < len(sql) && depth > 0 {
				if sql[i] == '/' && i+1 < len(sql) && sql[i+1] == '*' {
					depth++
					i += 2
				} else if sql[i] == '*' && i+1 < len(sql) && sql[i+1] == '/' {
					depth--
					i += 2
				} else {
					if sql[i] == '\n' {
						b.WriteByte('\n') // preserve line numbers
					}
					i++
				}
			}
			continue
		}
		// Single-quoted string: pass through (don't strip inside strings).
		if sql[i] == '\'' {
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
			continue
		}
		b.WriteByte(sql[i])
		i++
	}
	return b.String()
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

// hasFlag reports whether flags contains a specific token, case-insensitively.
func hasFlag(flags []string, want string) bool {
	for _, flag := range flags {
		if strings.EqualFold(strings.TrimSpace(flag), want) {
			return true
		}
	}
	return false
}

// destructiveActionMessage returns a user-facing message for destructive DDL
// actions and false for non-destructive actions.
func destructiveActionMessage(action postgresparser.DDLAction) (string, bool) {
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
