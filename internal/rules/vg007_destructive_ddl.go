// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"context"

	"github.com/valkdb/postgresparser"
)

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

// CommandTargets limits this rule to DDL statements.
func (r *DestructiveDDLRule) CommandTargets() []postgresparser.QueryCommand {
	return []postgresparser.QueryCommand{postgresparser.QueryCommandDDL}
}

// Check reports findings for destructive DDL actions.
func (r *DestructiveDDLRule) Check(_ context.Context, parsed *postgresparser.ParsedQuery, file string, line int, rawSQL string) []Finding {
	if parsed == nil || parsed.Command != postgresparser.QueryCommandDDL {
		return nil
	}

	findings := make([]Finding, 0, len(parsed.DDLActions))
	for i := range parsed.DDLActions {
		msg, ok := destructiveActionMessage(&parsed.DDLActions[i])
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
