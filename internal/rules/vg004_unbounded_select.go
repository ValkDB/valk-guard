package rules

import "github.com/valkdb/postgresparser"

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
