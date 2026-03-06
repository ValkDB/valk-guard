// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import "github.com/valkdb/postgresparser"

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

// CommandTargets limits this rule to SELECT statements.
func (r *SelectForUpdateNoWhereRule) CommandTargets() []postgresparser.QueryCommand {
	return []postgresparser.QueryCommand{postgresparser.QueryCommandSelect}
}

// Check reports a finding for SELECT FOR UPDATE statements lacking WHERE.
func (r *SelectForUpdateNoWhereRule) Check(_ context.Context, parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandSelect {
		return nil
	}
	if !hasForUpdateClause(rawSQL) || hasClause(parsed.Where) {
		return nil
	}
	// Bounded locking queries are common worker patterns and are intentionally
	// not flagged (for example: SELECT ... FOR UPDATE LIMIT 1).
	if hasLimitClause(parsed) {
		return nil
	}
	return []Finding{
		newFinding(
			r.ID(),
			r.DefaultSeverity(),
			"SELECT FOR UPDATE without WHERE may lock too many rows; add a WHERE clause",
			file,
			line,
			rawSQL,
		),
	}
}
