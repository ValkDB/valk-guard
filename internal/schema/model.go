// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package schema

import "context"

// ModelSource identifies the source language/framework used to extract a model.
type ModelSource string

const (
	// ModelSourceGo indicates a model extracted from Go source.
	ModelSourceGo ModelSource = "go"
	// ModelSourceSQLAlchemy indicates a model extracted from Python SQLAlchemy source.
	ModelSourceSQLAlchemy ModelSource = "sqlalchemy"
)

// MappingKind identifies whether a model/table/column mapping is explicit in
// source code or inferred by convention.
type MappingKind string

const (
	MappingKindExplicit MappingKind = "explicit"
	MappingKindInferred MappingKind = "inferred"
)

// ModelColumn represents a column mapping in an ORM model.
type ModelColumn struct {
	Name          string // DB column name (from db tag / Column() name)
	Field         string // Source field name (Go field / Python attribute)
	Type          string // ORM type hint if available (e.g. "Integer", "String(255)")
	Line          int
	MappingKind   MappingKind // explicit or inferred
	MappingSource string      // provider/source that produced this mapping
}

// ModelDef represents an ORM model extracted from source code.
type ModelDef struct {
	Table              string      // table name (from __tablename__ or struct name convention)
	TableExplicit      bool        // true when table name is explicitly declared in source
	TableMappingKind   MappingKind // explicit or inferred
	TableMappingSource string      // provider/source that produced this table mapping
	Source             ModelSource
	Columns            []ModelColumn
	File               string
	Line               int
}

// ModelExtractor extracts model definitions from source files.
type ModelExtractor interface {
	ExtractModels(ctx context.Context, paths []string) ([]ModelDef, error)
}
