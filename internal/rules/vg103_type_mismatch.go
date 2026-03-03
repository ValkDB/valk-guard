package rules

import (
	"fmt"
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

// typeCompatibility maps normalized model types to compatible SQL column type prefixes.
var typeCompatibility = map[string][]string{
	"integer":   {"integer", "bigint", "smallint", "serial", "bigserial", "int"},
	"int":       {"integer", "bigint", "smallint", "serial", "bigserial", "int"},
	"int64":     {"integer", "bigint", "smallint", "serial", "bigserial", "int"},
	"int32":     {"integer", "bigint", "smallint", "serial", "bigserial", "int"},
	"string":    {"varchar", "text", "char", "character varying"},
	"str":       {"varchar", "text", "char", "character varying"},
	"float":     {"float", "double precision", "real", "numeric", "decimal"},
	"float64":   {"float", "double precision", "real", "numeric", "decimal"},
	"float32":   {"float", "double precision", "real", "numeric", "decimal"},
	"boolean":   {"boolean", "bool"},
	"bool":      {"boolean", "bool"},
	"datetime":  {"timestamp", "timestamptz"},
	"time.time": {"timestamp", "timestamptz"},
	"timestamp": {"timestamp", "timestamptz"},
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
			modelType := strings.ToLower(mc.Type)
			sqlType := strings.ToLower(col.Type)
			compatible, known := typeCompatibility[modelType]
			if !known {
				continue
			}
			if !matchesSQLType(sqlType, compatible) {
				findings = append(findings, newFinding(
					r.ID(),
					r.DefaultSeverity(),
					fmt.Sprintf("column %q type mismatch: model has %q but migration has %q", mc.Name, mc.Type, col.Type),
					model.File,
					model.Line,
					"",
				))
			}
		}
	}
	return findings
}

// matchesSQLType checks if sqlType starts with any of the compatible prefixes.
func matchesSQLType(sqlType string, compatible []string) bool {
	for _, prefix := range compatible {
		if strings.HasPrefix(sqlType, prefix) {
			return true
		}
	}
	return false
}
