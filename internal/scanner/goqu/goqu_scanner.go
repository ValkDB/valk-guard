// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

// Package goqu provides a scanner that extracts SQL from goqu usage in Go
// source files. It supports both raw literals (goqu.L("...")) and synthetic
// SQL generated from method-chain AST analysis (e.g. goqu.From(...).Join(...)).
package goqu

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"iter"
	"strconv"
	"strings"
	"unicode"

	"github.com/valkdb/valk-guard/internal/scanner"
)

// knownImportPaths lists the import paths recognized as the goqu library.
var knownImportPaths = map[string]struct{}{
	"github.com/doug-martin/goqu":    {},
	"github.com/doug-martin/goqu/v9": {},
}

// goquBuilderMethods are methods that return a new Dataset or builder,
// extending the query chain.
var goquBuilderMethods = map[string]struct{}{
	"From":          {},
	"Select":        {},
	"Where":         {},
	"Limit":         {},
	"Offset":        {},
	"Order":         {},
	"GroupBy":       {},
	"Having":        {},
	"Join":          {},
	"InnerJoin":     {},
	"LeftJoin":      {},
	"RightJoin":     {},
	"FullJoin":      {},
	"Update":        {},
	"Delete":        {},
	"Set":           {},
	"Prepared":      {},
	"Union":         {},
	"UnionAll":      {},
	"Intersect":     {},
	"IntersectAll":  {},
	"ClearSelect":   {},
	"ClearWhere":    {},
	"ClearOrder":    {},
	"ClearLimit":    {},
	"ClearOffset":   {},
	"Distinct":      {},
	"ForUpdate":     {},
	"ForShare":      {},
	"Returning":     {},
	"With":          {},
	"WithRecursive": {},
}

// goquChainTerminalMethods are non-builder methods that still terminate a
// goqu query chain and should suppress synthesis from the immediately nested
// builder call.
var goquChainTerminalMethods = map[string]struct{}{
	"ToSQL": {},
}

const syntheticPrefix = "/* valk-guard:synthetic goqu-ast */ "

// Scanner extracts SQL from goqu usage in Go source files. For raw SQL,
// it extracts goqu.L("...") literals. For query-builder chains, it generates
// synthetic SQL that approximates the built statement so existing SQL rules
// can be reused.
type Scanner struct{}

type methodCall struct {
	Name string
	Args []ast.Expr
}

var errGoquScannerStop = errors.New("goqu scanner stop")

// Scan walks the given paths, finds .go files that import goqu, and streams
// SQL from literals and builder chains.
func (s *Scanner) Scan(ctx context.Context, paths []string) iter.Seq2[scanner.SQLStatement, error] {
	return func(yield func(scanner.SQLStatement, error) bool) {
		err := scanner.WalkGoFiles(ctx, paths, func(path string, fset *token.FileSet, f *ast.File, src []byte) error {
			if err := ctx.Err(); err != nil {
				return err
			}

			alias := scanner.FindImportAlias(f, knownImportPaths)
			if alias == "" {
				return nil // file does not import goqu
			}

			lines := strings.Split(string(src), "\n")
			directives := scanner.ParseDirectives(lines)
			var parents map[ast.Node]ast.Node
			getParents := func() map[ast.Node]ast.Node {
				if parents == nil {
					parents = buildParentMap(f)
				}
				return parents
			}
			stop := false

			ast.Inspect(f, func(n ast.Node) bool {
				if stop {
					return false
				}

				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				if sql := extractGoquLiteral(call, alias); sql != "" {
					pos := fset.Position(call.Pos())
					end := fset.Position(call.End())
					line := pos.Line
					if !yield(scanner.SQLStatement{
						SQL:       sql,
						File:      path,
						Line:      line,
						Column:    pos.Column,
						EndLine:   end.Line,
						EndColumn: end.Column,
						Engine:    scanner.EngineGoqu,
						Disabled:  scanner.DisabledRulesForLine(directives, line),
					}, nil) {
						stop = true
						return false
					}
				}

				// Only synthesize from the terminal call in a chain.
				if isChainedSubCall(call, getParents()) {
					return true
				}

				synthetic := synthesizeFromChain(call, alias)
				if synthetic == "" {
					return true
				}

				pos := fset.Position(call.Pos())
				end := fset.Position(call.End())
				line := pos.Line

				if !yield(scanner.SQLStatement{
					SQL:       syntheticPrefix + synthetic,
					File:      path,
					Line:      line,
					Column:    pos.Column,
					EndLine:   end.Line,
					EndColumn: end.Column,
					Engine:    scanner.EngineGoqu,
					Disabled:  scanner.DisabledRulesForLine(directives, line),
				}, nil) {
					stop = true
					return false
				}
				return true
			})

			if stop {
				return errGoquScannerStop
			}
			return nil
		})
		if err != nil && !errors.Is(err, errGoquScannerStop) {
			_ = yield(scanner.SQLStatement{}, err)
		}
	}
}

// extractGoquLiteral returns the raw SQL string from a goqu.L("...") call
// expression. It returns "" if the call is not a goqu literal call.
func extractGoquLiteral(call *ast.CallExpr, alias string) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != alias || sel.Sel.Name != "L" {
		return ""
	}
	if len(call.Args) < 1 {
		return ""
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok {
		return ""
	}
	return scanner.ExtractStringLiteral(lit)
}

// synthesizeFromChain generates a synthetic SQL statement from a goqu
// method-chain call expression. It inspects the root method name to
// determine the statement type (SELECT, UPDATE, DELETE) and delegates to
// the appropriate render helper. Returns "" if the chain is unrecognized.
func synthesizeFromChain(call *ast.CallExpr, alias string) string {
	chain, ok := flattenMethodChain(call, alias)
	if !ok || len(chain) == 0 {
		return ""
	}

	cp := collectChainParts(chain, alias)

	switch chain[0].Name {
	case "From":
		return renderSelect(&cp)
	case "Update":
		return renderUpdate(&cp)
	case "Delete":
		return renderDelete(&cp)
	default:
		return ""
	}
}

// flattenMethodChain walks a nested method-chain call expression rooted at a
// goqu identifier and returns the calls in order from outermost to innermost.
// ok is false if the chain does not start from the expected alias identifier.
func flattenMethodChain(call *ast.CallExpr, alias string) ([]methodCall, bool) {
	var reversed []methodCall
	current := call

	for {
		sel, ok := current.Fun.(*ast.SelectorExpr)
		if !ok {
			return nil, false
		}

		reversed = append(reversed, methodCall{
			Name: sel.Sel.Name,
			Args: current.Args,
		})

		if nextCall, ok := sel.X.(*ast.CallExpr); ok {
			current = nextCall
			continue
		}

		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != alias {
			return nil, false
		}
		break
	}

	chain := make([]methodCall, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		chain = append(chain, reversed[i])
	}
	return chain, true
}

// chainParts holds the unified output of walking a goqu method chain.
// It captures all clause data needed to render SELECT, UPDATE, or DELETE.
type chainParts struct {
	table      string
	columns    []string
	hasSelect  bool
	joins      []joinPart
	predicates []string
	hasWhere   bool
	hasLimit   bool
	limitVal   string
	forUpdate  bool
}

// joinPart holds a single JOIN clause extracted from the chain.
type joinPart struct {
	joinType string
	table    string
}

// collectChainParts walks the method chain and populates a chainParts struct.
func collectChainParts(chain []methodCall, alias string) chainParts {
	cp := chainParts{
		limitVal: "1",
	}

	if len(chain) == 0 || len(chain[0].Args) == 0 {
		return cp
	}
	cp.table = tableNameFromExpr(chain[0].Args[0], alias, "synthetic_table")

	for _, link := range chain[1:] {
		switch link.Name {
		case "Select":
			cp.hasSelect = true
			if cols := extractSelectColumns(link.Args, alias); len(cols) > 0 {
				cp.columns = cols
			}
		case "Join", "InnerJoin", "LeftJoin", "RightJoin", "FullJoin":
			jt := "JOIN"
			switch link.Name {
			case "LeftJoin":
				jt = "LEFT JOIN"
			case "RightJoin":
				jt = "RIGHT JOIN"
			case "FullJoin":
				jt = "FULL JOIN"
			}
			table := "synthetic_join"
			if len(link.Args) > 0 {
				table = tableNameFromExpr(link.Args[0], alias, "synthetic_join")
			}
			cp.joins = append(cp.joins, joinPart{joinType: jt, table: table})
		case "Where":
			cp.hasWhere = true
			conds := extractPredicates(link.Args, alias)
			if len(conds) == 0 {
				conds = []string{"1=1"}
			}
			cp.predicates = append(cp.predicates, conds...)
		case "Limit":
			cp.hasLimit = true
			cp.limitVal = limitFromArgs(link.Args)
		case "ForUpdate":
			cp.forUpdate = true
		}
	}

	return cp
}

// renderSelect builds a synthetic SELECT statement from collected chain parts.
func renderSelect(cp *chainParts) string {
	if cp.table == "" {
		return ""
	}

	columns := cp.columns
	if !cp.hasSelect || len(columns) == 0 {
		columns = []string{"*"}
	}

	sql := fmt.Sprintf("SELECT %s FROM %s", strings.Join(columns, ", "), cp.table)
	for _, j := range cp.joins {
		sql += fmt.Sprintf(" %s %s ON 1=1", j.joinType, j.table)
	}
	if cp.hasWhere && len(cp.predicates) > 0 {
		sql += " WHERE " + strings.Join(cp.predicates, " AND ")
	}
	if cp.hasLimit {
		sql += " LIMIT " + cp.limitVal
	}
	if cp.forUpdate {
		sql += " FOR UPDATE"
	}
	return sql
}

// renderUpdate builds a synthetic UPDATE statement from collected chain parts.
func renderUpdate(cp *chainParts) string {
	if cp.table == "" {
		return ""
	}

	sql := fmt.Sprintf("UPDATE %s SET synthetic_col = 1", cp.table)
	if len(cp.joins) > 0 {
		tables := make([]string, len(cp.joins))
		for i, j := range cp.joins {
			tables[i] = j.table
		}
		sql += " FROM " + strings.Join(tables, ", ")
	}
	if cp.hasWhere && len(cp.predicates) > 0 {
		sql += " WHERE " + strings.Join(cp.predicates, " AND ")
	}
	return sql
}

// renderDelete builds a synthetic DELETE statement from collected chain parts.
func renderDelete(cp *chainParts) string {
	if cp.table == "" {
		return ""
	}

	sql := fmt.Sprintf("DELETE FROM %s", cp.table)
	if len(cp.joins) > 0 {
		tables := make([]string, len(cp.joins))
		for i, j := range cp.joins {
			tables[i] = j.table
		}
		sql += " USING " + strings.Join(tables, ", ")
	}
	if cp.hasWhere && len(cp.predicates) > 0 {
		sql += " WHERE " + strings.Join(cp.predicates, " AND ")
	}
	return sql
}

// extractSelectColumns converts a slice of AST expressions from a goqu
// Select() call into SQL column name strings. Falls back to ["*"] if no
// column names can be resolved.
func extractSelectColumns(args []ast.Expr, alias string) []string {
	columns := make([]string, 0, len(args))
	for _, arg := range args {
		col := columnNameFromExpr(arg, alias, "")
		if col == "" {
			continue
		}
		columns = append(columns, col)
	}
	if len(columns) == 0 {
		columns = append(columns, "*")
	}
	return columns
}

// extractPredicates converts a slice of AST expressions from a goqu Where()
// call into SQL predicate strings, skipping any arguments that cannot be
// resolved to a predicate.
func extractPredicates(args []ast.Expr, alias string) []string {
	var predicates []string
	for _, arg := range args {
		p := predicateFromExpr(arg, alias)
		if p != "" {
			predicates = append(predicates, p)
		}
	}
	return predicates
}

// predicateFromExpr converts a single AST expression representing a goqu
// predicate (such as goqu.C("col").Eq(val) or a goqu.Ex{} composite literal)
// into a SQL condition string. Returns "" if the expression cannot be
// recognized as a predicate.
func predicateFromExpr(expr ast.Expr, alias string) string {
	switch e := expr.(type) {
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		if !ok {
			return ""
		}

		col := columnNameFromExpr(sel.X, alias, "synthetic_col")
		switch sel.Sel.Name {
		case "Like":
			return fmt.Sprintf("%s LIKE %s", col, sqlValue(firstArg(e.Args)))
		case "ILike":
			return fmt.Sprintf("%s ILIKE %s", col, sqlValue(firstArg(e.Args)))
		case "Eq":
			return fmt.Sprintf("%s = %s", col, sqlValue(firstArg(e.Args)))
		case "Neq":
			return fmt.Sprintf("%s <> %s", col, sqlValue(firstArg(e.Args)))
		case "Gt":
			return fmt.Sprintf("%s > %s", col, sqlValue(firstArg(e.Args)))
		case "Gte":
			return fmt.Sprintf("%s >= %s", col, sqlValue(firstArg(e.Args)))
		case "Lt":
			return fmt.Sprintf("%s < %s", col, sqlValue(firstArg(e.Args)))
		case "Lte":
			return fmt.Sprintf("%s <= %s", col, sqlValue(firstArg(e.Args)))
		}

	case *ast.CompositeLit:
		var predicates []string
		for _, elt := range e.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key := literalOrIdent(kv.Key)
			key = safeIdent(key, "synthetic_col")
			predicates = append(predicates, fmt.Sprintf("%s = %s", key, sqlValue(kv.Value)))
		}
		if len(predicates) > 0 {
			return strings.Join(predicates, " AND ")
		}
	}

	return ""
}

// tableNameFromExpr resolves an AST expression to a safe SQL table name
// string. It handles string literals, identifiers, selector expressions, and
// goqu.T()/goqu.I() calls. Returns fallback when the expression cannot be
// resolved to a safe identifier.
func tableNameFromExpr(expr ast.Expr, alias, fallback string) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			return safeIdent(scanner.ExtractStringLiteral(e), fallback)
		}
	case *ast.Ident:
		return safeIdent(e.Name, fallback)
	case *ast.SelectorExpr:
		return safeIdent(e.Sel.Name, fallback)
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		if !ok {
			return fallback
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok || x.Name != alias {
			return fallback
		}
		if len(e.Args) == 0 {
			return fallback
		}
		switch sel.Sel.Name {
		case "T", "I":
			return safeIdent(literalOrIdent(e.Args[0]), fallback)
		}
	}
	return fallback
}

// columnNameFromExpr resolves an AST expression to a safe SQL column name
// string. It handles string literals, identifiers, qualified selectors, and
// goqu.C()/goqu.I()/goqu.Star() calls. Returns fallback when the expression
// cannot be resolved to a safe identifier.
func columnNameFromExpr(expr ast.Expr, alias, fallback string) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			return safeIdent(scanner.ExtractStringLiteral(e), fallback)
		}
	case *ast.Ident:
		return safeIdent(e.Name, fallback)
	case *ast.SelectorExpr:
		if left, ok := e.X.(*ast.Ident); ok {
			return safeIdent(left.Name+"."+e.Sel.Name, fallback)
		}
		return safeIdent(e.Sel.Name, fallback)
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		if !ok {
			return fallback
		}

		// goqu.C("col"), goqu.I("tbl.col"), goqu.Star()
		if x, ok := sel.X.(*ast.Ident); ok && x.Name == alias {
			switch sel.Sel.Name {
			case "Star":
				return "*"
			case "C", "I":
				if len(e.Args) > 0 {
					return safeIdent(literalOrIdent(e.Args[0]), fallback)
				}
			}
		}

		// Preserve receiver identity for helper methods like As().
		return columnNameFromExpr(sel.X, alias, fallback)
	}

	return fallback
}

// literalOrIdent extracts the raw string representation of a basic literal,
// identifier, or selector expression. Returns "" for any other expression type.
func literalOrIdent(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			return scanner.ExtractStringLiteral(e)
		}
		return e.Value
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok {
			return x.Name + "." + e.Sel.Name
		}
		return e.Sel.Name
	}
	return ""
}

// sqlValue converts an AST expression to a SQL value literal suitable for
// embedding in a synthetic query. String literals are single-quoted, numeric
// literals are emitted as-is, boolean/nil identifiers are mapped to SQL
// keywords, and all other expressions are replaced with 'synthetic_value'.
func sqlValue(expr ast.Expr) string {
	if expr == nil {
		return "NULL"
	}

	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.STRING:
			s := scanner.ExtractStringLiteral(e)
			s = strings.ReplaceAll(s, "'", "''")
			return "'" + s + "'"
		case token.INT, token.FLOAT:
			return e.Value
		}
	case *ast.Ident:
		switch strings.ToLower(e.Name) {
		case "true":
			return "TRUE"
		case "false":
			return "FALSE"
		case "nil":
			return "NULL"
		}
	case *ast.UnaryExpr:
		if e.Op == token.SUB {
			if lit, ok := e.X.(*ast.BasicLit); ok && (lit.Kind == token.INT || lit.Kind == token.FLOAT) {
				return "-" + lit.Value
			}
		}
	}

	return "'synthetic_value'"
}

// limitFromArgs extracts the LIMIT value from the AST arguments of a goqu
// Limit() call. Returns "1" if no argument is present or the value cannot be
// statically determined.
func limitFromArgs(args []ast.Expr) string {
	if len(args) == 0 {
		return "1"
	}

	if lit, ok := args[0].(*ast.BasicLit); ok {
		if lit.Kind == token.INT {
			return lit.Value
		}
		if lit.Kind == token.STRING {
			s := scanner.ExtractStringLiteral(lit)
			if _, err := strconv.Atoi(s); err == nil {
				return s
			}
		}
	}
	return "1"
}

// firstArg returns the first element of args, or nil if args is empty.
func firstArg(args []ast.Expr) ast.Expr {
	if len(args) == 0 {
		return nil
	}
	return args[0]
}

// safeIdent validates that raw is a safe SQL identifier (letters, digits,
// underscores, or dots with a letter/underscore as the first character) and
// returns it trimmed. Returns fallback if raw is empty or contains unsafe
// characters. The special token "*" is always returned as-is.
func safeIdent(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "\"`")
	if raw == "" {
		return fallback
	}

	if fields := strings.Fields(raw); len(fields) > 0 {
		raw = fields[0]
	}

	if raw == "*" {
		return raw
	}

	for i, r := range raw {
		if i == 0 {
			if unicode.IsLetter(r) || r == '_' {
				continue
			}
			return fallback
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' {
			continue
		}
		return fallback
	}
	return raw
}

// buildParentMap traverses the AST rooted at root and builds a map from each
// node to its direct parent node. This is used to determine whether a call
// expression is a sub-expression within a larger method chain.
func buildParentMap(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node

	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			return false
		}
		if len(stack) > 0 {
			parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
	return parents
}

// isChainedSubCall reports whether call is an intermediate node in a goqu
// builder method chain rather than the terminal call. A call is considered
// a sub-call when its parent selector's method name is another goqu chain
// method (builder or terminal sink such as ToSQL) and that selector is the
// function of a grandparent call expression.
func isChainedSubCall(call *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	parent, ok := parents[call]
	if !ok {
		return false
	}
	sel, ok := parent.(*ast.SelectorExpr)
	if !ok || sel.X != call {
		return false
	}
	grandParent, ok := parents[parent]
	if !ok {
		return false
	}
	grandCall, ok := grandParent.(*ast.CallExpr)
	if !ok || grandCall.Fun != sel {
		return false
	}
	// If the next call in the chain is another recognized goqu chain method,
	// then the current call is just a sub-component of a larger chain.
	if _, ok = goquBuilderMethods[sel.Sel.Name]; ok {
		return true
	}
	_, ok = goquChainTerminalMethods[sel.Sel.Name]
	return ok
}
