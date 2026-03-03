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

// matchTable tries to find a schema table for a model's table name.
// It attempts exact match, plural forms (+"s", +"es"), and singular
// forms (strip trailing "s", "es"). All lookups are case-insensitive.
func matchTable(snap *schema.Snapshot, modelTable string) *schema.TableDef {
	name := strings.ToLower(modelTable)

	// exact
	if td, ok := snap.Tables[name]; ok {
		return td
	}
	// name + "s"
	if td, ok := snap.Tables[name+"s"]; ok {
		return td
	}
	// name + "es"
	if td, ok := snap.Tables[name+"es"]; ok {
		return td
	}
	// strip trailing "s"
	if strings.HasSuffix(name, "s") {
		if td, ok := snap.Tables[strings.TrimSuffix(name, "s")]; ok {
			return td
		}
	}
	// strip trailing "es"
	if strings.HasSuffix(name, "es") {
		if td, ok := snap.Tables[strings.TrimSuffix(name, "es")]; ok {
			return td
		}
	}
	return nil
}
