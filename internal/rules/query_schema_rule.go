package rules

import (
	"github.com/valkdb/postgresparser"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

// QuerySchemaRule checks parsed query column usage against migration DDL schema.
type QuerySchemaRule interface {
	ID() string
	Name() string
	Description() string
	DefaultSeverity() Severity
	CheckQuerySchema(snap *schema.Snapshot, stmt scanner.SQLStatement, parsed *postgresparser.ParsedQuery) []Finding
}
