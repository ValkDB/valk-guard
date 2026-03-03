// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/valkdb/valk-guard/internal/schema"
)

// TypeMismatchRule flags column type mismatches between ORM models and
// migration DDL.
type TypeMismatchRule struct{}

// ID returns the unique rule identifier.
func (r *TypeMismatchRule) ID() string { return "VG103" }

// Name returns the human-readable rule name.
func (r *TypeMismatchRule) Name() string { return "type-mismatch" }

// Description explains what this rule checks.
func (r *TypeMismatchRule) Description() string {
	return "Detects column type mismatches between ORM model and migration schema."
}

// DefaultSeverity returns the default severity for this rule.
func (r *TypeMismatchRule) DefaultSeverity() Severity { return SeverityWarning }

// typeCompatibility maps normalized model types to compatible normalized SQL types.
var typeCompatibility = map[string][]string{
	"integer":   {"integer", "bigint", "smallint", "serial", "bigserial", "int", "int2", "int4", "int8"},
	"string":    {"varchar", "text", "char", "character", "character varying"},
	"float":     {"float", "double precision", "real", "numeric", "decimal"},
	"boolean":   {"boolean", "bool"},
	"timestamp": {"timestamp", "timestamp with time zone", "timestamp without time zone", "timestamptz"},
}

var modelTypeAliases = map[string]string{
	"integer":   "integer",
	"int":       "integer",
	"int8":      "integer",
	"int16":     "integer",
	"int32":     "integer",
	"int64":     "integer",
	"smallint":  "integer",
	"bigint":    "integer",
	"serial":    "integer",
	"bigserial": "integer",

	"string": "string",
	"str":    "string",
	"char":   "string",
	"text":   "string",

	"float":   "float",
	"float32": "float",
	"float64": "float",
	"double":  "float",
	"numeric": "float",
	"decimal": "float",
	"real":    "float",

	"bool":    "boolean",
	"boolean": "boolean",

	"nullint64":   "integer",
	"nullint32":   "integer",
	"nullint16":   "integer",
	"nullfloat64": "float",
	"nullstring":  "string",
	"nullbool":    "boolean",
	"nulltime":    "timestamp",

	"datetime":  "timestamp",
	"timestamp": "timestamp",
	"time.time": "timestamp",
}

// CheckSchema compares model column types against the migration schema.
func (r *TypeMismatchRule) CheckSchema(snap *schema.Snapshot, models []schema.ModelDef) []Finding {
	var findings []Finding
	for _, model := range models {
		td := matchTable(snap, model.Table)
		if td == nil {
			continue
		}
		for _, mc := range model.Columns {
			if mc.Type == "" {
				continue
			}
			col, ok := td.Columns[strings.ToLower(mc.Name)]
			if !ok {
				continue
			}
			modelType := normalizeModelType(mc.Type)
			if modelType == "" {
				continue
			}
			sqlType := normalizeSQLType(col.Type)
			compatible, known := typeCompatibility[modelType]
			if !known {
				continue
			}
			if !matchesSQLType(sqlType, compatible) {
				findings = append(findings, newFinding(
					r.ID(),
					r.DefaultSeverity(),
					fmt.Sprintf("column %q type mismatch: model has %q but migration has %q; align model type or migration column type", mc.Name, mc.Type, col.Type),
					model.File,
					mc.Line,
					"",
				))
			}
		}
	}
	return findings
}

// normalizeModelType canonicalizes model-side type names so equivalent forms
// (for example, "String(255)" and "string") resolve to a shared key.
func normalizeModelType(modelType string) string {
	t := strings.ToLower(strings.TrimSpace(modelType))
	if t == "" {
		return ""
	}
	// Strip pointer (*) and slice ([]) prefixes in any combination.
	for strings.HasPrefix(t, "*") || strings.HasPrefix(t, "[]") {
		t = strings.TrimPrefix(t, "*")
		t = strings.TrimPrefix(t, "[]")
	}
	if idx := strings.IndexByte(t, '('); idx >= 0 {
		t = t[:idx]
	}
	t = strings.TrimSpace(t)
	if canonical, ok := modelTypeAliases[t]; ok {
		return canonical
	}
	if idx := strings.LastIndexByte(t, '.'); idx >= 0 {
		if canonical, ok := modelTypeAliases[t[idx+1:]]; ok {
			return canonical
		}
	}
	return ""
}

// normalizeSQLType canonicalizes SQL type strings for exact matching.
func normalizeSQLType(sqlType string) string {
	t := strings.ToLower(strings.TrimSpace(sqlType))
	if t == "" {
		return ""
	}
	if idx := strings.IndexByte(t, '('); idx >= 0 {
		t = t[:idx]
	}
	return strings.Join(strings.Fields(strings.TrimSpace(t)), " ")
}

// matchesSQLType checks if sqlType exactly matches one of the compatible types.
func matchesSQLType(sqlType string, compatible []string) bool {
	return slices.Contains(compatible, sqlType)
}
