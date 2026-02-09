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
var knownImportPaths = map[string]bool{
	"github.com/doug-martin/goqu":    true,
	"github.com/doug-martin/goqu/v9": true,
}

// goquBuilderMethods are methods that return a new Dataset or builder,
// extending the query chain.
var goquBuilderMethods = map[string]bool{
	"From":          true,
	"Select":        true,
	"Where":         true,
	"Limit":         true,
	"Offset":        true,
	"Order":         true,
	"GroupBy":       true,
	"Having":        true,
	"Join":          true,
	"InnerJoin":     true,
	"LeftJoin":      true,
	"RightJoin":     true,
	"FullJoin":      true,
	"Update":        true,
	"Delete":        true,
	"Set":           true,
	"Prepared":      true,
	"Union":         true,
	"UnionAll":      true,
	"Intersect":     true,
	"IntersectAll":  true,
	"ClearSelect":   true,
	"ClearWhere":    true,
	"ClearOrder":    true,
	"ClearLimit":    true,
	"ClearOffset":   true,
	"Distinct":      true,
	"ForUpdate":     true,
	"ForShare":      true,
	"Returning":     true,
	"With":          true,
	"WithRecursive": true,
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
			parents := buildParentMap(f)
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
					line := pos.Line
					if !yield(scanner.SQLStatement{
						SQL:      sql,
						File:     path,
						Line:     line,
						Disabled: scanner.DisabledRulesForLine(directives, line),
					}, nil) {
						stop = true
						return false
					}
				}

				// Only synthesize from the terminal call in a chain.
				if isChainedSubCall(call, parents) {
					return true
				}

				synthetic := synthesizeFromChain(call, alias)
				if synthetic == "" {
					return true
				}

				pos := fset.Position(call.Pos())
				line := pos.Line

				if !yield(scanner.SQLStatement{
					SQL:      syntheticPrefix + synthetic,
					File:     path,
					Line:     line,
					Disabled: scanner.DisabledRulesForLine(directives, line),
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

func synthesizeFromChain(call *ast.CallExpr, alias string) string {
	chain, ok := flattenMethodChain(call, alias)
	if !ok || len(chain) == 0 {
		return ""
	}

	switch chain[0].Name {
	case "From":
		return synthesizeSelect(chain, alias)
	case "Update":
		return synthesizeUpdate(chain, alias)
	case "Delete":
		return synthesizeDelete(chain, alias)
	default:
		return ""
	}
}

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

func synthesizeSelect(chain []methodCall, alias string) string {
	if len(chain[0].Args) == 0 {
		return ""
	}

	fromTable := tableNameFromExpr(chain[0].Args[0], alias, "synthetic_table")
	columns := []string{"*"} // goqu.From(...) defaults to SELECT *
	hasSelect := false
	hasWhere := false
	hasLimit := false
	limitVal := "1"
	var joins []string
	var predicates []string

	for _, link := range chain[1:] {
		switch link.Name {
		case "Select":
			hasSelect = true
			if cols := extractSelectColumns(link.Args, alias); len(cols) > 0 {
				columns = cols
			}
		case "Join", "InnerJoin", "LeftJoin", "RightJoin", "FullJoin":
			joins = append(joins, joinClause(link.Name, link.Args, alias))
		case "Where":
			hasWhere = true
			conds := extractPredicates(link.Args, alias)
			if len(conds) == 0 {
				conds = []string{"1=1"}
			}
			predicates = append(predicates, conds...)
		case "Limit":
			hasLimit = true
			limitVal = limitFromArgs(link.Args)
		}
	}

	if !hasSelect {
		columns = []string{"*"}
	}

	sql := fmt.Sprintf("SELECT %s FROM %s", strings.Join(columns, ", "), fromTable)
	if len(joins) > 0 {
		sql += " " + strings.Join(joins, " ")
	}
	if hasWhere && len(predicates) > 0 {
		sql += " WHERE " + strings.Join(predicates, " AND ")
	}
	if hasLimit {
		sql += " LIMIT " + limitVal
	}
	return sql
}

func synthesizeUpdate(chain []methodCall, alias string) string {
	if len(chain[0].Args) == 0 {
		return ""
	}

	table := tableNameFromExpr(chain[0].Args[0], alias, "synthetic_table")
	var fromTables []string
	var predicates []string
	hasWhere := false

	for _, link := range chain[1:] {
		switch link.Name {
		case "Join", "InnerJoin", "LeftJoin", "RightJoin", "FullJoin":
			if len(link.Args) > 0 {
				t := tableNameFromExpr(link.Args[0], alias, "synthetic_join")
				fromTables = append(fromTables, t)
			}
		case "Where":
			hasWhere = true
			conds := extractPredicates(link.Args, alias)
			if len(conds) == 0 {
				conds = []string{"1=1"}
			}
			predicates = append(predicates, conds...)
		}
	}

	sql := fmt.Sprintf("UPDATE %s SET synthetic_col = 1", table)
	if len(fromTables) > 0 {
		sql += " FROM " + strings.Join(fromTables, ", ")
	}
	if hasWhere && len(predicates) > 0 {
		sql += " WHERE " + strings.Join(predicates, " AND ")
	}
	return sql
}

func synthesizeDelete(chain []methodCall, alias string) string {
	if len(chain[0].Args) == 0 {
		return ""
	}

	table := tableNameFromExpr(chain[0].Args[0], alias, "synthetic_table")
	var usingTables []string
	var predicates []string
	hasWhere := false

	for _, link := range chain[1:] {
		switch link.Name {
		case "Join", "InnerJoin", "LeftJoin", "RightJoin", "FullJoin":
			if len(link.Args) > 0 {
				t := tableNameFromExpr(link.Args[0], alias, "synthetic_join")
				usingTables = append(usingTables, t)
			}
		case "Where":
			hasWhere = true
			conds := extractPredicates(link.Args, alias)
			if len(conds) == 0 {
				conds = []string{"1=1"}
			}
			predicates = append(predicates, conds...)
		}
	}

	sql := fmt.Sprintf("DELETE FROM %s", table)
	if len(usingTables) > 0 {
		sql += " USING " + strings.Join(usingTables, ", ")
	}
	if hasWhere && len(predicates) > 0 {
		sql += " WHERE " + strings.Join(predicates, " AND ")
	}
	return sql
}

func joinClause(method string, args []ast.Expr, alias string) string {
	joinType := "JOIN"
	switch method {
	case "LeftJoin":
		joinType = "LEFT JOIN"
	case "RightJoin":
		joinType = "RIGHT JOIN"
	case "FullJoin":
		joinType = "FULL JOIN"
	}

	table := "synthetic_join"
	if len(args) > 0 {
		table = tableNameFromExpr(args[0], alias, "synthetic_join")
	}
	return fmt.Sprintf("%s %s ON 1=1", joinType, table)
}

func extractSelectColumns(args []ast.Expr, alias string) []string {
	var columns []string
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
			return fmt.Sprintf("%s LIKE %s", col, sqlValue(firstArg(e.Args), alias))
		case "ILike":
			return fmt.Sprintf("%s ILIKE %s", col, sqlValue(firstArg(e.Args), alias))
		case "Eq":
			return fmt.Sprintf("%s = %s", col, sqlValue(firstArg(e.Args), alias))
		case "Neq":
			return fmt.Sprintf("%s <> %s", col, sqlValue(firstArg(e.Args), alias))
		case "Gt":
			return fmt.Sprintf("%s > %s", col, sqlValue(firstArg(e.Args), alias))
		case "Gte":
			return fmt.Sprintf("%s >= %s", col, sqlValue(firstArg(e.Args), alias))
		case "Lt":
			return fmt.Sprintf("%s < %s", col, sqlValue(firstArg(e.Args), alias))
		case "Lte":
			return fmt.Sprintf("%s <= %s", col, sqlValue(firstArg(e.Args), alias))
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
			predicates = append(predicates, fmt.Sprintf("%s = %s", key, sqlValue(kv.Value, alias)))
		}
		if len(predicates) > 0 {
			return strings.Join(predicates, " AND ")
		}
	}

	return ""
}

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

func sqlValue(expr ast.Expr, alias string) string {
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

	_ = alias
	return "'synthetic_value'"
}

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

func firstArg(args []ast.Expr) ast.Expr {
	if len(args) == 0 {
		return nil
	}
	return args[0]
}

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
			if !(unicode.IsLetter(r) || r == '_') {
				return fallback
			}
			continue
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.') {
			return fallback
		}
	}
	return raw
}

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
	// If the next call in the chain is a known builder method,
	// then the current call is just a sub-component of a larger chain.
	return goquBuilderMethods[sel.Sel.Name]
}
