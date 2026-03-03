// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"log/slog"

	"github.com/valkdb/postgresparser"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/textutil"
)

// BuildFromStatements constructs a schema snapshot by parsing each SQL
// statement and applying DDL actions (CREATE TABLE, DROP TABLE, DROP COLUMN,
// ALTER TABLE ADD COLUMN) to accumulate the final schema state.
//
// Parse errors are logged at Debug level via logger so callers can diagnose
// silently skipped migrations. Passing a nil logger is safe (a no-op logger
// is used).
func BuildFromStatements(stmts []scanner.SQLStatement, logger *slog.Logger) *Snapshot {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	snap := NewSnapshot()
	for _, stmt := range stmts {
		parsed, err := scanner.ParseStatement(stmt.SQL)
		if err != nil {
			logger.Debug("skipping unparseable DDL statement", "file", stmt.File, "line", stmt.Line, "error", err)
			continue
		}
		if parsed == nil {
			continue
		}
		if parsed.Command != postgresparser.QueryCommandDDL {
			continue
		}
		for _, action := range parsed.DDLActions {
			applyDDLAction(snap, &action, stmt.File, stmt.Line)
		}
	}
	return snap
}

// applyDDLAction applies a single DDL action to the snapshot.
func applyDDLAction(snap *Snapshot, action *postgresparser.DDLAction, file string, line int) {
	switch action.Type {
	case postgresparser.DDLCreateTable:
		cols := make([]ColumnDef, len(action.ColumnDetails))
		for i, dc := range action.ColumnDetails {
			cols[i] = ColumnDef{
				Name:       dc.Name,
				Type:       dc.Type,
				Nullable:   dc.Nullable,
				HasDefault: dc.Default != "",
			}
		}
		snap.ApplyCreateTable(action.ObjectName, cols, file, line)

	case postgresparser.DDLDropTable:
		snap.ApplyDropTable(action.ObjectName)

	case postgresparser.DDLDropColumn:
		for _, col := range action.Columns {
			snap.ApplyDropColumn(action.ObjectName, col)
		}

	case postgresparser.DDLAlterTable:
		if len(action.ColumnDetails) > 0 {
			for _, dc := range action.ColumnDetails {
				snap.ApplyAddColumn(action.ObjectName, ColumnDef{
					Name:       dc.Name,
					Type:       dc.Type,
					Nullable:   dc.Nullable,
					HasDefault: dc.Default != "",
				})
			}
		} else if textutil.ContainsFoldTrim(action.Flags, "ADD_COLUMN") {
			// The parser populates Columns (not ColumnDetails) for
			// ALTER TABLE ADD COLUMN. Type metadata is unavailable.
			for _, name := range action.Columns {
				snap.ApplyAddColumn(action.ObjectName, ColumnDef{Name: name})
			}
		}
	}
}
