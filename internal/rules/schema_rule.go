// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"

	"github.com/valkdb/valk-guard/internal/schema"
)

// SchemaRule checks ORM models against migration DDL.
type SchemaRule interface {
	ID() string
	Name() string
	Description() string
	DefaultSeverity() Severity
	CheckSchema(snap *schema.Snapshot, models []schema.ModelDef) []Finding
}

// matchTable resolves a model table name to a schema table using exact
// case-insensitive matching only.
func matchTable(snap *schema.Snapshot, modelTable string) *schema.TableDef {
	if snap == nil {
		return nil
	}
	return snap.Lookup(strings.ToLower(modelTable))
}
