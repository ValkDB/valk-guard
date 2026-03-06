// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

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

// CommandTargets limits this rule to UPDATE statements.
func (r *MissingWhereUpdateRule) CommandTargets() []postgresparser.QueryCommand {
	return []postgresparser.QueryCommand{postgresparser.QueryCommandUpdate}
}

// Check reports a finding when UPDATE has no restrictive WHERE/CURRENT OF
// predicate.
func (r *MissingWhereUpdateRule) Check(_ context.Context, parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandUpdate {
		return nil
	}
	if hasRestrictiveClause(parsed.Where) {
		return nil
	}
	return []Finding{
		newFinding(
			r.ID(),
			r.DefaultSeverity(),
			"UPDATE without WHERE may affect all rows; add a WHERE clause",
			file,
			line,
			rawSQL,
		),
	}
}
