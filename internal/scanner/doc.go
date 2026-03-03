// Package scanner finds SQL statements in source files. It provides shared
// scanner contracts and helpers used by RawSQLScanner (.sql), GoScanner
// (go/ast SQL literals), and language/framework-specific scanners under
// subpackages such as goqu and sqlalchemy. Inline disable directives are
// parsed and attached to each extracted statement.
//
// The goast.go helpers (WalkGoFiles, ExtractStringLiteral, FindImportAlias)
// are also reused by the schema/gomodel extractor for ORM model discovery.
package scanner
