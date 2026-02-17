package rules

import "github.com/valkdb/postgresparser"

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
