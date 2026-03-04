// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

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
	// CheckQuerySchema validates a parsed query statement's column usage
	// against the given schema snapshot and returns any findings.
	CheckQuerySchema(snap *schema.Snapshot, stmt *scanner.SQLStatement, parsed *postgresparser.ParsedQuery) []Finding
}
