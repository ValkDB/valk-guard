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

// SelectStarRule detects SELECT * usage in query projections.
type SelectStarRule struct{}

// ID returns the unique rule identifier.
func (r *SelectStarRule) ID() string { return "VG001" }

// Name returns the human-readable rule name.
func (r *SelectStarRule) Name() string { return "select-star" }

// Description explains what this rule checks.
func (r *SelectStarRule) Description() string { return "Detects SELECT * projections." }

// DefaultSeverity returns the default severity for this rule.
func (r *SelectStarRule) DefaultSeverity() Severity { return SeverityWarning }

// Check reports a finding when a SELECT statement projects "*" or "<alias>.*".
func (r *SelectStarRule) Check(parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandSelect {
		return nil
	}

	for _, col := range parsed.Columns {
		if !isWildcardProjection(col.Expression) {
			continue
		}
		return []Finding{
			newFinding(
				r.ID(),
				r.DefaultSeverity(),
				"avoid SELECT *; project only required columns",
				file,
				line,
				rawSQL,
			),
		}
	}
	return nil
}

// MissingWhereUpdateRule detects UPDATE statements without a WHERE clause.
type MissingWhereUpdateRule struct{}

// ID returns the unique rule identifier.
func (r *MissingWhereUpdateRule) ID() string { return "VG002" }

// Name returns the human-readable rule name.
func (r *MissingWhereUpdateRule) Name() string { return "missing-where-update" }

// Description explains what this rule checks.
func (r *MissingWhereUpdateRule) Description() string {
	return "Detects UPDATE statements without WHERE."
}

// DefaultSeverity returns the default severity for this rule.
func (r *MissingWhereUpdateRule) DefaultSeverity() Severity { return SeverityError }

// Check reports a finding when UPDATE has no WHERE/CURRENT OF predicate.
func (r *MissingWhereUpdateRule) Check(parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandUpdate {
		return nil
	}
	if hasClause(parsed.Where) {
		return nil
	}
	return []Finding{
		newFinding(
			r.ID(),
			r.DefaultSeverity(),
			"UPDATE without WHERE may affect all rows",
			file,
			line,
			rawSQL,
		),
	}
}

// MissingWhereDeleteRule detects DELETE statements without a WHERE clause.
type MissingWhereDeleteRule struct{}

// ID returns the unique rule identifier.
func (r *MissingWhereDeleteRule) ID() string { return "VG003" }

// Name returns the human-readable rule name.
func (r *MissingWhereDeleteRule) Name() string { return "missing-where-delete" }

// Description explains what this rule checks.
func (r *MissingWhereDeleteRule) Description() string {
	return "Detects DELETE statements without WHERE."
}

// DefaultSeverity returns the default severity for this rule.
func (r *MissingWhereDeleteRule) DefaultSeverity() Severity { return SeverityError }

// Check reports a finding when DELETE has no WHERE/CURRENT OF predicate.
func (r *MissingWhereDeleteRule) Check(parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandDelete {
		return nil
	}
	if hasClause(parsed.Where) {
		return nil
	}
	return []Finding{
		newFinding(
			r.ID(),
			r.DefaultSeverity(),
			"DELETE without WHERE may affect all rows",
			file,
			line,
			rawSQL,
		),
	}
}

// UnboundedSelectRule detects SELECT statements without LIMIT/FETCH bounds.
type UnboundedSelectRule struct{}

// ID returns the unique rule identifier.
func (r *UnboundedSelectRule) ID() string { return "VG004" }

// Name returns the human-readable rule name.
func (r *UnboundedSelectRule) Name() string { return "unbounded-select" }

// Description explains what this rule checks.
func (r *UnboundedSelectRule) Description() string {
	return "Detects SELECT statements without LIMIT/FETCH bounds."
}

// DefaultSeverity returns the default severity for this rule.
func (r *UnboundedSelectRule) DefaultSeverity() Severity { return SeverityWarning }

// Check reports a finding when a SELECT statement has no top-level LIMIT/FETCH.
func (r *UnboundedSelectRule) Check(parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandSelect {
		return nil
	}
	if hasLimitClause(parsed) {
		return nil
	}
	return []Finding{
		newFinding(
			r.ID(),
			r.DefaultSeverity(),
			"SELECT without LIMIT may return unbounded rows",
			file,
			line,
			rawSQL,
		),
	}
}

// LikeLeadingWildcardRule detects LIKE/ILIKE patterns that begin with '%'.
type LikeLeadingWildcardRule struct{}

// ID returns the unique rule identifier.
func (r *LikeLeadingWildcardRule) ID() string { return "VG005" }

// Name returns the human-readable rule name.
func (r *LikeLeadingWildcardRule) Name() string { return "like-leading-wildcard" }

// Description explains what this rule checks.
func (r *LikeLeadingWildcardRule) Description() string {
	return "Detects LIKE/ILIKE predicates with a leading wildcard."
}

// DefaultSeverity returns the default severity for this rule.
func (r *LikeLeadingWildcardRule) DefaultSeverity() Severity { return SeverityWarning }

// Check reports a finding when any parsed predicate uses LIKE/ILIKE '%...'.
func (r *LikeLeadingWildcardRule) Check(parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
	if parsed == nil {
		return nil
	}
	if !hasLeadingWildcardLike(queryClauses(parsed)) {
		return nil
	}
	return []Finding{
		newFinding(
			r.ID(),
			r.DefaultSeverity(),
			"LIKE/ILIKE with leading wildcard may prevent index usage",
			file,
			line,
			rawSQL,
		),
	}
}

// SelectForUpdateNoWhereRule detects SELECT FOR UPDATE without WHERE.
type SelectForUpdateNoWhereRule struct{}

// ID returns the unique rule identifier.
func (r *SelectForUpdateNoWhereRule) ID() string { return "VG006" }

// Name returns the human-readable rule name.
func (r *SelectForUpdateNoWhereRule) Name() string { return "select-for-update-no-where" }

// Description explains what this rule checks.
func (r *SelectForUpdateNoWhereRule) Description() string {
	return "Detects SELECT FOR UPDATE statements without WHERE."
}

// DefaultSeverity returns the default severity for this rule.
func (r *SelectForUpdateNoWhereRule) DefaultSeverity() Severity { return SeverityError }

// Check reports a finding for SELECT FOR UPDATE statements lacking WHERE.
func (r *SelectForUpdateNoWhereRule) Check(parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandSelect {
		return nil
	}
	if !hasForUpdateClause(rawSQL) || hasClause(parsed.Where) {
		return nil
	}
	return []Finding{
		newFinding(
			r.ID(),
			r.DefaultSeverity(),
			"SELECT FOR UPDATE without WHERE may lock too many rows",
			file,
			line,
			rawSQL,
		),
	}
}

// DestructiveDDLRule detects destructive schema operations.
type DestructiveDDLRule struct{}

// ID returns the unique rule identifier.
func (r *DestructiveDDLRule) ID() string { return "VG007" }

// Name returns the human-readable rule name.
func (r *DestructiveDDLRule) Name() string { return "destructive-ddl" }

// Description explains what this rule checks.
func (r *DestructiveDDLRule) Description() string {
	return "Detects destructive DDL operations (DROP TABLE, DROP COLUMN, TRUNCATE)."
}

// DefaultSeverity returns the default severity for this rule.
func (r *DestructiveDDLRule) DefaultSeverity() Severity { return SeverityError }

// Check reports findings for destructive DDL actions.
func (r *DestructiveDDLRule) Check(parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandDDL {
		return nil
	}

	var findings []Finding
	for _, action := range parsed.DDLActions {
		msg, ok := destructiveActionMessage(action)
		if !ok {
			continue
		}
		findings = append(findings, newFinding(
			r.ID(),
			r.DefaultSeverity(),
			msg,
			file,
			line,
			rawSQL,
		))
	}
	return findings
}

// NonConcurrentIndexRule detects CREATE INDEX statements without CONCURRENTLY.
type NonConcurrentIndexRule struct{}

// ID returns the unique rule identifier.
func (r *NonConcurrentIndexRule) ID() string { return "VG008" }

// Name returns the human-readable rule name.
func (r *NonConcurrentIndexRule) Name() string { return "non-concurrent-index" }

// Description explains what this rule checks.
func (r *NonConcurrentIndexRule) Description() string {
	return "Detects CREATE INDEX statements that omit CONCURRENTLY."
}

// DefaultSeverity returns the default severity for this rule.
func (r *NonConcurrentIndexRule) DefaultSeverity() Severity { return SeverityWarning }

// Check reports findings for CREATE INDEX operations missing CONCURRENTLY.
func (r *NonConcurrentIndexRule) Check(parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandDDL {
		return nil
	}

	var findings []Finding
	for _, action := range parsed.DDLActions {
		if action.Type != postgresparser.DDLCreateIndex || hasFlag(action.Flags, "CONCURRENTLY") {
			continue
		}

		message := "CREATE INDEX without CONCURRENTLY may block writes"
		if action.ObjectName != "" {
			message = fmt.Sprintf("CREATE INDEX %s without CONCURRENTLY may block writes", action.ObjectName)
		}
		findings = append(findings, newFinding(
			r.ID(),
			r.DefaultSeverity(),
			message,
			file,
			line,
			rawSQL,
		))
	}

	return findings
}

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
func hasForUpdateClause(sql string) bool {
	return forUpdatePattern.MatchString(sql)
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
