package scanner

import "github.com/valkdb/postgresparser"

// SQLStatement represents a SQL statement extracted from a source file.
type SQLStatement struct {
	SQL      string   // The raw SQL text.
	File     string   // Source file path.
	Line     int      // 1-based line number where the statement starts.
	Disabled []string // Rule IDs disabled via inline directives.
}

// Scanner is the interface for components that find SQL in source files.
type Scanner interface {
	// Scan walks the given paths and returns all SQL statements found.
	Scan(paths []string) ([]SQLStatement, error)
}

// ParseStatement parses a SQL statement using the postgres parser.
// It returns nil (no error) if the statement is empty or unparseable,
// allowing the caller to skip it gracefully.
func ParseStatement(sql string) (*postgresparser.ParsedQuery, error) {
	if sql == "" {
		return nil, nil
	}
	parsed, err := postgresparser.ParseSQL(sql)
	if err != nil {
		// Unparseable statements are intentionally skipped so the linter
		// can continue processing the remaining statements in the file.
		return nil, nil //nolint:nilerr // intentionally skip unparseable SQL
	}
	return parsed, nil
}
