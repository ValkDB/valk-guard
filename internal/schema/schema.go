// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package schema

import "strings"

// ColumnDef describes a column from migration DDL.
type ColumnDef struct {
	Name       string
	Type       string // e.g. "integer", "text", "varchar(255)"
	Nullable   bool
	HasDefault bool
}

// TableDef represents a table as declared by migration DDL.
type TableDef struct {
	Name    string
	Columns map[string]ColumnDef // keyed by lowercase column name
	File    string
	Line    int
}

// Snapshot accumulates DDL state across migration files.
// Snapshot is NOT safe for concurrent use; callers must ensure all
// mutations complete before any concurrent reads.
type Snapshot struct {
	Tables map[string]*TableDef // keyed by lowercase table name
}

// NewSnapshot creates an empty schema snapshot.
func NewSnapshot() *Snapshot {
	return &Snapshot{Tables: make(map[string]*TableDef)}
}

// ApplyCreateTable registers a new table with its columns.
// If the table already exists it is replaced (last writer wins).
func (s *Snapshot) ApplyCreateTable(table string, columns []ColumnDef, file string, line int) {
	key := strings.ToLower(table)
	td := &TableDef{
		Name:    table,
		Columns: make(map[string]ColumnDef, len(columns)),
		File:    file,
		Line:    line,
	}
	for _, col := range columns {
		td.Columns[strings.ToLower(col.Name)] = col
	}
	s.Tables[key] = td
}

// Lookup returns the table definition for the given name, using
// case-insensitive matching. Returns nil if the table is not found.
func (s *Snapshot) Lookup(table string) *TableDef {
	return s.Tables[strings.ToLower(table)]
}

// ApplyDropTable removes a table from the snapshot.
func (s *Snapshot) ApplyDropTable(table string) {
	delete(s.Tables, strings.ToLower(table))
}

// ApplyDropColumn removes a column from a table. No-op if the table or column
// does not exist.
func (s *Snapshot) ApplyDropColumn(table, column string) {
	td, ok := s.Tables[strings.ToLower(table)]
	if !ok {
		return
	}
	delete(td.Columns, strings.ToLower(column))
}

// ApplyAddColumn adds a column to an existing table. No-op if the table does
// not exist.
func (s *Snapshot) ApplyAddColumn(table string, col ColumnDef) {
	td, ok := s.Tables[strings.ToLower(table)]
	if !ok {
		return
	}
	td.Columns[strings.ToLower(col.Name)] = col
}
