package rules

import "github.com/valkdb/postgresparser"

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
