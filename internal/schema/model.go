package schema

import "context"

// ModelColumn represents a column mapping in an ORM model.
type ModelColumn struct {
	Name  string // DB column name (from db tag / Column() name)
	Field string // Source field name (Go field / Python attribute)
	Type  string // ORM type hint if available (e.g. "Integer", "String(255)")
	Line  int
}

// ModelDef represents an ORM model extracted from source code.
type ModelDef struct {
	Table   string // table name (from __tablename__ or struct name convention)
	Columns []ModelColumn
	File    string
	Line    int
}

// ModelExtractor extracts model definitions from source files.
type ModelExtractor interface {
	ExtractModels(ctx context.Context, paths []string) ([]ModelDef, error)
}
