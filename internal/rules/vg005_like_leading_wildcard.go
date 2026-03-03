// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import "github.com/valkdb/postgresparser"

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

// CommandTargets limits this rule to statements that can contain
// WHERE/HAVING/JOIN clauses where LIKE predicates appear.
// DDL, INSERT, and utility statements are excluded to avoid wasted checks.
func (r *LikeLeadingWildcardRule) CommandTargets() []postgresparser.QueryCommand {
	return []postgresparser.QueryCommand{
		postgresparser.QueryCommandSelect,
		postgresparser.QueryCommandUpdate,
		postgresparser.QueryCommandDelete,
	}
}

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
			"LIKE/ILIKE with leading wildcard may prevent index usage; use a suffix pattern or full-text search",
			file,
			line,
			rawSQL,
		),
	}
}
