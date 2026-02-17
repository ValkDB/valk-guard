package rules

import "github.com/valkdb/postgresparser"

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
