// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/valkdb/postgresparser"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

// UnknownTableReferenceRule flags FROM/JOIN table references that do not exist
// in the resolved schema snapshot.
type UnknownTableReferenceRule struct{}

// ID returns the unique rule identifier.
func (r *UnknownTableReferenceRule) ID() string { return "VG107" }

// Name returns the human-readable rule name.
func (r *UnknownTableReferenceRule) Name() string { return "unknown-table-reference" }

// Description explains what this rule checks.
func (r *UnknownTableReferenceRule) Description() string {
	return "Detects FROM/JOIN table references that are not found in the schema snapshot."
}

// DefaultSeverity returns the default severity for this rule.
func (r *UnknownTableReferenceRule) DefaultSeverity() Severity { return SeverityError }

// CheckQuerySchema validates referenced tables against the schema snapshot.
func (r *UnknownTableReferenceRule) CheckQuerySchema(
	_ context.Context,
	snap *schema.Snapshot,
	stmt *scanner.SQLStatement,
	parsed *postgresparser.ParsedQuery,
) []Finding {
	if parsed == nil || len(snap.Tables) == 0 || len(parsed.Tables) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	findings := make([]Finding, 0, len(parsed.Tables))

	for _, tbl := range parsed.Tables {
		if tbl.Type != "" && tbl.Type != postgresparser.TableTypeBase {
			continue
		}

		tableName := normalizeIdentifier(tbl.Name)
		if tableName == "" {
			continue
		}
		if matchTable(snap, tableName) != nil {
			continue
		}
		if _, ok := seen[tableName]; ok {
			continue
		}
		seen[tableName] = struct{}{}

		findings = append(findings, newFinding(
			r.ID(),
			r.DefaultSeverity(),
			fmt.Sprintf("table %q referenced in query not found in schema", tableName),
			stmt.File,
			stmt.Line,
			stmt.SQL,
		))
	}

	return findings
}
