// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"strings"

	"github.com/valkdb/valk-guard/internal/schema"
)

// DroppedColumnRule flags model columns that reference a column not present in
// the migration schema.
type DroppedColumnRule struct{}

// ID returns the unique rule identifier.
func (r *DroppedColumnRule) ID() string { return "VG101" }

// Name returns the human-readable rule name.
func (r *DroppedColumnRule) Name() string { return "dropped-column" }

// Description explains what this rule checks.
func (r *DroppedColumnRule) Description() string {
	return "Detects model columns that reference a column not found in the migration schema."
}

// DefaultSeverity returns the default severity for this rule.
func (r *DroppedColumnRule) DefaultSeverity() Severity { return SeverityError }

// CheckSchema compares model columns against the migration schema snapshot.
func (r *DroppedColumnRule) CheckSchema(_ context.Context, snap *schema.Snapshot, models []schema.ModelDef) []Finding {
	var findings []Finding
	for _, model := range models {
		td := matchTable(snap, model.Table)
		if td == nil {
			continue
		}
		for _, col := range model.Columns {
			if _, ok := td.Columns[strings.ToLower(col.Name)]; !ok {
				findings = append(findings, newFinding(
					r.ID(),
					r.DefaultSeverity(),
					fmt.Sprintf("model %q references column %q not found in table %q schema; check migration DDL or update model mapping", model.Table, col.Name, td.Name),
					model.File,
					col.Line,
					"",
				))
			}
		}
	}
	return findings
}
