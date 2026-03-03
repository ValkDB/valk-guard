// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/valkdb/valk-guard/internal/schema"
)

// OrphanMigrationTableRule flags migration tables that have no matching model.
type OrphanMigrationTableRule struct{}

// ID returns the unique rule identifier.
func (r *OrphanMigrationTableRule) ID() string { return "VG109" }

// Name returns the human-readable rule name.
func (r *OrphanMigrationTableRule) Name() string { return "orphan-migration-table" }

// Description explains what this rule checks.
func (r *OrphanMigrationTableRule) Description() string {
	return "Detects migration tables that are not represented by any extracted model."
}

// DefaultSeverity returns the default severity for this rule.
func (r *OrphanMigrationTableRule) DefaultSeverity() Severity { return SeverityWarning }

// CheckSchema compares migration tables against extracted model mappings.
func (r *OrphanMigrationTableRule) CheckSchema(snap *schema.Snapshot, models []schema.ModelDef) []Finding {
	if len(snap.Tables) == 0 || len(models) == 0 {
		return nil
	}

	covered := make(map[string]struct{})
	for _, model := range models {
		td := matchTable(snap, model.Table)
		if td == nil {
			continue
		}
		covered[strings.ToLower(td.Name)] = struct{}{}
	}

	tableKeys := make([]string, 0, len(snap.Tables))
	for key := range snap.Tables {
		tableKeys = append(tableKeys, key)
	}
	slices.Sort(tableKeys)

	var findings []Finding
	for _, key := range tableKeys {
		td := snap.Tables[key]
		if td == nil {
			continue
		}
		if _, ok := covered[key]; ok {
			continue
		}
		findings = append(findings, newFinding(
			r.ID(),
			r.DefaultSeverity(),
			fmt.Sprintf("migration table %q has no matching model mapping", td.Name),
			td.File,
			td.Line,
			"",
		))
	}

	return findings
}
