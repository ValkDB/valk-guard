// Package scanner finds SQL statements in source files. It provides a
// RawSQLScanner for .sql files and a GoScanner that extracts SQL string
// literals from Go source using go/ast. Inline disable directives are
// parsed and attached to each extracted statement.
package scanner
