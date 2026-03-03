// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package schema

import "strings"

// BuildModelSnapshotForSource converts extracted models for one source engine
// into a lightweight table/column snapshot used by query-schema rules.
func BuildModelSnapshotForSource(models []ModelDef, source ModelSource) *Snapshot {
	snap := NewSnapshot()

	for _, model := range models {
		if model.Source != source {
			continue
		}
		tableName := strings.TrimSpace(model.Table)
		if tableName == "" {
			continue
		}

		var cols []ColumnDef
		for _, mc := range model.Columns {
			colName := strings.TrimSpace(mc.Name)
			if colName == "" {
				continue
			}
			cols = append(cols, ColumnDef{
				Name: colName,
				Type: mc.Type,
			})
		}

		if snap.Lookup(tableName) != nil {
			// Table already registered by another model; add columns individually.
			for _, col := range cols {
				snap.ApplyAddColumn(tableName, col)
			}
			continue
		}
		snap.ApplyCreateTable(tableName, cols, model.File, model.Line)
	}

	return snap
}
