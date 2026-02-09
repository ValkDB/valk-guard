// Package scanner finds SQL statements in source files. It provides shared
// scanner contracts and helpers used by RawSQLScanner (.sql), GoScanner
// (go/ast SQL literals), and language/framework-specific scanners under
// subpackages such as goqu and sqlalchemy. Inline disable directives are
// parsed and attached to each extracted statement.
package scanner
