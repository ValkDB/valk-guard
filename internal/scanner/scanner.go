// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/valkdb/postgresparser"
)

// Engine identifies where a SQL statement originated.
type Engine string

const (
	// EngineUnknown is used when the statement source is not specified.
	EngineUnknown Engine = ""
	// EngineSQL represents raw .sql files.
	EngineSQL Engine = "sql"
	// EngineGo represents SQL literals extracted from standard Go DB calls.
	EngineGo Engine = "go"
	// EngineGoqu represents SQL extracted/synthesized from goqu usage.
	EngineGoqu Engine = "goqu"
	// EngineSQLAlchemy represents SQL extracted/synthesized from SQLAlchemy usage.
	EngineSQLAlchemy Engine = "sqlalchemy"
)

// SQLStatement represents a SQL statement extracted from a source file.
type SQLStatement struct {
	SQL      string   // The raw SQL text.
	File     string   // Source file path.
	Line     int      // 1-based line number where the statement starts.
	Engine   Engine   // Statement source engine (sql/go/goqu/sqlalchemy).
	Disabled []string // Rule IDs disabled via inline directives.
}

// Scanner is the interface for components that find SQL in source files.
type Scanner interface {
	// Scan walks the given paths and streams extracted SQL statements.
	// The second value yields non-nil errors. Implementations must stop work
	// when the context is canceled.
	Scan(ctx context.Context, paths []string) iter.Seq2[SQLStatement, error]
}

// ErrParserFailure wraps errors returned by the SQL parser.
var ErrParserFailure = errors.New("sql parser failure")

// ParseStatement parses a SQL statement using the postgres parser.
// It returns nil (no error) only for empty statements.
func ParseStatement(sql string) (*postgresparser.ParsedQuery, error) {
	if sql == "" {
		return nil, nil
	}
	parsed, err := postgresparser.ParseSQL(sql)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParserFailure, err)
	}
	return parsed, nil
}

// Collect drains a statement stream into a slice and returns the first error.
func Collect(seq iter.Seq2[SQLStatement, error]) ([]SQLStatement, error) {
	out := make([]SQLStatement, 0, 16)
	for stmt, err := range seq {
		if err != nil {
			return nil, err
		}
		out = append(out, stmt)
	}
	return out, nil
}
