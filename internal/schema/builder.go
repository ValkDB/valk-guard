package schema

import (
	"github.com/valkdb/postgresparser"
	"github.com/valkdb/valk-guard/internal/scanner"
)

// BuildFromStatements constructs a schema snapshot by parsing each SQL
// statement and applying DDL actions (CREATE TABLE, DROP TABLE, DROP COLUMN,
// ALTER TABLE ADD COLUMN) to accumulate the final schema state.
func BuildFromStatements(stmts []scanner.SQLStatement) *Snapshot {
	snap := NewSnapshot()
	for _, stmt := range stmts {
		parsed, err := scanner.ParseStatement(stmt.SQL)
		if err != nil || parsed == nil {
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
		} else if hasFlag(action.Flags, "ADD_COLUMN") {
			// The parser populates Columns (not ColumnDetails) for
			// ALTER TABLE ADD COLUMN. Type metadata is unavailable.
			for _, name := range action.Columns {
				snap.ApplyAddColumn(action.ObjectName, ColumnDef{Name: name})
			}
		}
	}
}

// hasFlag reports whether flags contains the given value.
func hasFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if f == flag {
			return true
		}
	}
	return false
}
