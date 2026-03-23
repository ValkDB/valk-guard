// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

// Package csharp provides a scanner that extracts SQL from EF Core raw SQL
// execution calls in C# source files.
//
// v1 covers raw EF Core SQL execution only:
//   - ExecuteSqlRaw / ExecuteSqlRawAsync
//   - ExecuteSqlInterpolated / ExecuteSqlInterpolatedAsync
//
// Query-builder, LINQ, FromSqlRaw, and SqlQueryRaw patterns are tracked
// separately in issue #17.
//
// Known v1 limitations:
//   - Variable resolution is limited to assignments that appear earlier in the
//     enclosing method body. Cross-method, cross-file, and field/property flow
//     are not tracked.
//   - Only the last assignment to a given variable name before the call is
//     captured. Conditional or branching assignments are not tracked.
//   - Format-string normalization ({0} → $1) is applied unconditionally to
//     ExecuteSqlRaw calls. SQL that contains literal {0} text will be rewritten.
package csharp

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"github.com/valkdb/valk-guard/internal/scanner"
)

// Scanner extracts SQL from EF Core raw SQL execution calls in C# source files.
type Scanner struct{}

var errStop = errors.New("csharp scanner stop")

// efCoreMethodNames lists the EF Core raw SQL execution methods, ordered
// longest-first so prefix ambiguity is resolved correctly during matching.
var efCoreMethodNames = []string{
	"ExecuteSqlInterpolatedAsync",
	"ExecuteSqlInterpolated",
	"ExecuteSqlRawAsync",
	"ExecuteSqlRaw",
}

// interpolatedMethods is the set of methods that expect interpolated strings.
var interpolatedMethods = map[string]struct{}{
	"ExecuteSqlInterpolated":      {},
	"ExecuteSqlInterpolatedAsync": {},
}

// Scan walks .cs files under the given paths and streams extracted SQL statements.
func (s *Scanner) Scan(ctx context.Context, paths []string) iter.Seq2[scanner.SQLStatement, error] {
	return func(yield func(scanner.SQLStatement, error) bool) {
		err := walkCSFiles(ctx, paths, func(path string, src []byte) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			content := string(src)
			if !strings.Contains(content, "ExecuteSql") {
				return nil
			}
			for _, stmt := range extractStatements(path, content) {
				if !yield(stmt, nil) {
					return errStop
				}
			}
			return nil
		})
		if err != nil && !errors.Is(err, errStop) {
			_ = yield(scanner.SQLStatement{}, err)
		}
	}
}

// ---------------------------------------------------------------------------
// File walking
// ---------------------------------------------------------------------------

// walkCSFiles visits .cs inputs under paths and passes each file to fn.
func walkCSFiles(ctx context.Context, paths []string, fn func(string, []byte) error) error {
	for _, root := range paths {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		info, err := os.Stat(root)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if !isCS(root) {
				continue
			}
			data, err := os.ReadFile(root) //nolint:gosec // scanning user-provided source paths
			if err != nil {
				return fmt.Errorf("read %s: %w", root, err)
			}
			if err := fn(filepath.Clean(root), data); err != nil {
				return err
			}
			continue
		}
		if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, we error) error {
			if we != nil {
				return we
			}
			if d.IsDir() || !isCS(p) {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			data, err := os.ReadFile(p) //nolint:gosec // scanning user-provided source paths
			if err != nil {
				return fmt.Errorf("read %s: %w", p, err)
			}
			return fn(filepath.Clean(p), data)
		}); err != nil {
			return err
		}
	}
	return nil
}

// isCS reports whether path has a .cs extension.
func isCS(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".cs")
}

// ---------------------------------------------------------------------------
// Statement extraction
// ---------------------------------------------------------------------------

// extractStatements resolves supported EF Core raw SQL calls from a source file.
func extractStatements(path, src string) []scanner.SQLStatement {
	lines := strings.Split(src, "\n")
	directives := scanner.ParseDirectives(lines)
	offsets := buildLineOffsets(src)
	scopes := collectMethodScopes(src)
	calls := findCalls(src)

	stmts := make([]scanner.SQLStatement, 0, len(calls))
	for _, c := range calls {
		scope := findMethodScope(scopes, c.dotPos)
		if !receiverAllowed(src, c.dotPos, scope) {
			continue
		}

		var vars map[string]varDef
		if scope != nil {
			vars = collectVars(src[scope.bodyStart:c.dotPos])
		}

		isInterp := isInterpolatedMethod(c.method)
		sql, ok := parseExpr(c.firstArg, vars, isInterp, 0)
		if !ok || sql == "" {
			continue
		}
		if !isInterp {
			sql = normalizeFormatPlaceholders(sql)
		}
		if !scanner.LooksLikeSQL(sql) {
			continue
		}

		line, col := offsetToLC(offsets, c.dotPos)
		eLine, eCol := offsetToLC(offsets, c.closePos)

		stmts = append(stmts, scanner.SQLStatement{
			SQL:       sql,
			File:      path,
			Line:      line,
			Column:    col,
			EndLine:   eLine,
			EndColumn: eCol,
			Engine:    scanner.EngineCSharp,
			Disabled:  scanner.DisabledRulesForLine(directives, line),
		})
	}
	return stmts
}

// isInterpolatedMethod reports whether name expects an interpolated SQL string.
func isInterpolatedMethod(name string) bool {
	_, ok := interpolatedMethods[name]
	return ok
}

// ---------------------------------------------------------------------------
// Call finding
// ---------------------------------------------------------------------------

type callSite struct {
	dotPos   int    // position of the '.' before the method name
	method   string // method name
	firstArg string // raw text of the first argument
	closePos int    // position of closing ')'
}

type methodScope struct {
	bodyStart      int
	bodyEnd        int
	dbFacadeParams map[string]struct{}
}

// findCalls locates candidate ExecuteSql* call sites in source text.
func findCalls(src string) []callSite {
	var result []callSite
	n := len(src)

	for i := 0; i < n; {
		if next := skipNonCode(src, i); next > i {
			i = next
			continue
		}
		if src[i] == '.' {
			if c, next := tryMatchCall(src, i); next > 0 {
				result = append(result, c)
				i = next
				continue
			}
		}
		i++
	}
	return result
}

// tryMatchCall matches a supported ExecuteSql* invocation at dotPos.
func tryMatchCall(src string, dotPos int) (call callSite, next int) {
	n := len(src)
	for _, method := range efCoreMethodNames {
		end := dotPos + 1 + len(method)
		if end > n {
			continue
		}
		if src[dotPos+1:end] != method {
			continue
		}
		// Ensure the match is not a prefix of a longer identifier.
		if end < n && isIdentByte(src[end]) {
			continue
		}
		// Skip whitespace to opening '('.
		j := end
		for j < n && isWS(src[j]) {
			j++
		}
		if j >= n || src[j] != '(' {
			continue
		}
		argText, closePos := extractFirstArg(src, j+1)
		if closePos < 0 {
			continue
		}
		return callSite{
			dotPos:   dotPos,
			method:   method,
			firstArg: argText,
			closePos: closePos,
		}, closePos + 1
	}
	return callSite{}, 0
}

// receiverAllowed reports whether the call receiver matches a supported EF Core pattern.
func receiverAllowed(src string, dotPos int, scope *methodScope) bool {
	member, prev, ok := receiverMembers(src, dotPos)
	if !ok {
		return false
	}
	if member == "Database" {
		return prev != ""
	}
	if scope == nil {
		return false
	}

	names := make(map[string]struct{}, len(scope.dbFacadeParams))
	for name := range scope.dbFacadeParams {
		names[name] = struct{}{}
	}
	for name := range collectDatabaseFacadeNames(src[scope.bodyStart:dotPos]) {
		names[name] = struct{}{}
	}

	if _, ok := names[member]; ok {
		return true
	}
	return (prev == "this" || prev == "base") && hasName(names, member)
}

// receiverMembers returns the immediate receiver member and its parent member.
func receiverMembers(src string, dotPos int) (member, prev string, ok bool) {
	j := dotPos - 1
	for j >= 0 && isWS(src[j]) {
		j--
	}
	if j < 0 || !isIdentByte(src[j]) {
		return "", "", false
	}
	end := j + 1
	for j >= 0 && isIdentByte(src[j]) {
		j--
	}
	member = src[j+1 : end]

	k := j
	for k >= 0 && isWS(src[k]) {
		k--
	}
	if k < 0 || src[k] != '.' {
		return member, "", true
	}
	k--
	for k >= 0 && isWS(src[k]) {
		k--
	}
	if k < 0 || !isIdentByte(src[k]) {
		return member, "", true
	}
	prevEnd := k + 1
	for k >= 0 && isIdentByte(src[k]) {
		k--
	}
	return member, src[k+1 : prevEnd], true
}

// extractFirstArg extracts the raw text of the first argument from a call.
// pos is the position after '('. Returns (argText, closeParenPos).
func extractFirstArg(src string, pos int) (argText string, closeParenPos int) {
	n := len(src)
	depth := 1

	start := pos
	for start < n && isWS(src[start]) {
		start++
	}
	argEnd := -1

	for i := start; i < n; {
		if next := skipNonCode(src, i); next > i {
			i = next
			continue
		}
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				if argEnd < 0 {
					argEnd = i
				}
				return strings.TrimSpace(src[start:argEnd]), i
			}
		case ',':
			if depth == 1 && argEnd < 0 {
				argEnd = i
			}
		}
		i++
	}
	return "", -1
}

// collectMethodScopes finds method-like bodies and the DatabaseFacade params they declare.
func collectMethodScopes(src string) []methodScope {
	var scopes []methodScope
	for i := 0; i < len(src); i++ {
		if next := skipNonCode(src, i); next > i {
			i = next - 1
			continue
		}
		if src[i] != '{' {
			continue
		}
		closeParen := prevNonWS(src, i-1)
		if closeParen < 0 || src[closeParen] != ')' {
			continue
		}
		openParen := findMatchingOpenParen(src, closeParen)
		if openParen < 0 {
			continue
		}
		name := identBeforePos(src, openParen)
		if name == "" || isMethodBlockKeyword(name) {
			continue
		}
		closeBrace := findMatchingCloseBrace(src, i)
		if closeBrace < 0 {
			continue
		}
		scopes = append(scopes, methodScope{
			bodyStart:      i + 1,
			bodyEnd:        closeBrace,
			dbFacadeParams: collectDatabaseFacadeNames(src[openParen+1 : closeParen]),
		})
	}
	return scopes
}

// findMethodScope returns the innermost method scope containing pos.
func findMethodScope(scopes []methodScope, pos int) *methodScope {
	var best *methodScope
	bestLen := 0
	for i := range scopes {
		scope := &scopes[i]
		if pos < scope.bodyStart || pos > scope.bodyEnd {
			continue
		}
		length := scope.bodyEnd - scope.bodyStart
		if best == nil || length < bestLen {
			best = scope
			bestLen = length
		}
	}
	return best
}

// findMatchingOpenParen returns the matching '(' for a ')' at closePos.
func findMatchingOpenParen(src string, closePos int) int {
	depth := 0
	for i := closePos; i >= 0; i-- {
		switch src[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// findMatchingCloseBrace returns the matching '}' for a '{' at openPos.
func findMatchingCloseBrace(src string, openPos int) int {
	depth := 0
	for i := openPos; i < len(src); i++ {
		if next := skipNonCode(src, i); next > i {
			i = next - 1
			continue
		}
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

var methodBlockKeywords = map[string]struct{}{
	"if": {}, "for": {}, "foreach": {}, "while": {}, "switch": {},
	"catch": {}, "using": {}, "lock": {}, "checked": {}, "unchecked": {},
}

// isMethodBlockKeyword reports whether name begins a control-flow block, not a method.
func isMethodBlockKeyword(name string) bool {
	_, ok := methodBlockKeywords[name]
	return ok
}

// ---------------------------------------------------------------------------
// SQL resolution
// ---------------------------------------------------------------------------

type varDef struct {
	expr string // raw RHS expression text
}

// parseExpr tries to resolve a C# expression to SQL text.
// It handles string literals, variable references, and simple concatenation.
func parseExpr(expr string, vars map[string]varDef, isInterp bool, depth int) (string, bool) {
	if depth > 5 {
		return "", false
	}
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", false
	}

	var result strings.Builder
	remaining := expr

	for remaining != "" {
		remaining = strings.TrimSpace(remaining)
		if remaining == "" {
			break
		}

		// Try string literal.
		if content, rest, ok := tryParseStringLiteral(remaining); ok {
			result.WriteString(content)
			remaining = strings.TrimSpace(rest)
			if strings.HasPrefix(remaining, "+") {
				remaining = strings.TrimSpace(remaining[1:])
				continue
			}
			if remaining == "" {
				return result.String(), true
			}
			return "", false
		}

		// Try identifier (variable reference).
		name := readIdent(remaining)
		if name != "" {
			rest := strings.TrimSpace(remaining[len(name):])
			if v, ok := vars[name]; ok {
				resolved, rok := parseExpr(v.expr, vars, isInterp, depth+1)
				if !rok {
					return "", false
				}
				result.WriteString(resolved)
				remaining = rest
				if strings.HasPrefix(remaining, "+") {
					remaining = strings.TrimSpace(remaining[1:])
					continue
				}
				if remaining == "" {
					return result.String(), true
				}
				return "", false
			}
			return "", false // unresolved variable
		}

		return "", false // unrecognized expression
	}

	if result.Len() == 0 {
		return "", false
	}
	return result.String(), true
}

// tryParseStringLiteral attempts to parse a C# string literal from the start
// of s. Returns (content, rest, ok). The content has interpolation expressions
// replaced with $1, $2 placeholders and escape sequences decoded.
func tryParseStringLiteral(s string) (content, rest string, ok bool) {
	// $@"..." or @$"..." (verbatim interpolated)
	if strings.HasPrefix(s, `$@"`) || strings.HasPrefix(s, `@$"`) {
		content, rest := readVerbInterpStr(s[3:])
		return content, rest, true
	}
	// $"""...""" (raw interpolated)
	if len(s) >= 4 && s[0] == '$' && s[1] == '"' && s[2] == '"' && s[3] == '"' {
		content, rest := readRawInterpStr(s[4:])
		return content, rest, true
	}
	// $"..." (interpolated)
	if len(s) >= 2 && s[0] == '$' && s[1] == '"' {
		content, rest := readInterpStr(s[2:])
		return content, rest, true
	}
	// @"..." (verbatim)
	if len(s) >= 2 && s[0] == '@' && s[1] == '"' {
		content, rest := readVerbStr(s[2:])
		return content, rest, true
	}
	// """...""" (raw)
	if len(s) >= 6 && s[0] == '"' && s[1] == '"' && s[2] == '"' {
		content, rest := readRawStr(s[3:])
		return content, rest, true
	}
	// regular string literal
	if len(s) >= 1 && s[0] == '"' {
		content, rest := readRegStr(s[1:])
		return content, rest, true
	}
	return "", s, false
}

// ---------------------------------------------------------------------------
// String content reading — each function starts after the opening delimiter
// and returns (content, rest) where rest is the text after the closing delimiter.
// ---------------------------------------------------------------------------

// readRegStr reads a regular C# string ("...") starting after the opening ".
func readRegStr(s string) (content, rest string) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) {
				i++
				switch s[i] {
				case 'n':
					b.WriteByte('\n')
				case 'r':
					b.WriteByte('\r')
				case 't':
					b.WriteByte('\t')
				case '\\':
					b.WriteByte('\\')
				case '"':
					b.WriteByte('"')
				case '0':
					b.WriteByte(0)
				default:
					b.WriteByte('\\')
					b.WriteByte(s[i])
				}
			}
		case '"':
			return b.String(), strings.TrimSpace(s[i+1:])
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String(), ""
}

// readVerbStr reads a verbatim C# string (@"...") starting after @".
func readVerbStr(s string) (content, rest string) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			if i+1 < len(s) && s[i+1] == '"' {
				b.WriteByte('"')
				i++
				continue
			}
			return b.String(), strings.TrimSpace(s[i+1:])
		}
		b.WriteByte(s[i])
	}
	return b.String(), ""
}

// readRawStr reads a raw C# string ("""...""") starting after the opening """.
func readRawStr(s string) (content, rest string) {
	for i := 0; i+2 < len(s); i++ {
		if s[i] == '"' && s[i+1] == '"' && s[i+2] == '"' {
			return trimRawContent(s[:i]), strings.TrimSpace(s[i+3:])
		}
	}
	return trimRawContent(s), ""
}

// readInterpStr reads an interpolated C# string ($"...") starting after $".
// Replaces {expr} with $1, $2, etc.
func readInterpStr(s string) (content, rest string) {
	var b strings.Builder
	ph := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) {
				i++
				switch s[i] {
				case 'n':
					b.WriteByte('\n')
				case 'r':
					b.WriteByte('\r')
				case 't':
					b.WriteByte('\t')
				case '\\':
					b.WriteByte('\\')
				case '"':
					b.WriteByte('"')
				case '0':
					b.WriteByte(0)
				default:
					b.WriteByte('\\')
					b.WriteByte(s[i])
				}
			}
		case '{':
			if i+1 < len(s) && s[i+1] == '{' {
				b.WriteByte('{')
				i++
				continue
			}
			end := findCloseBrace(s, i+1)
			ph++
			fmt.Fprintf(&b, "$%d", ph)
			i = end
		case '}':
			if i+1 < len(s) && s[i+1] == '}' {
				b.WriteByte('}')
				i++
				continue
			}
			b.WriteByte('}')
		case '"':
			return b.String(), strings.TrimSpace(s[i+1:])
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String(), ""
}

// readVerbInterpStr reads a verbatim interpolated string ($@"..." or @$"...").
func readVerbInterpStr(s string) (content, rest string) {
	var b strings.Builder
	ph := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			if i+1 < len(s) && s[i+1] == '{' {
				b.WriteByte('{')
				i++
				continue
			}
			end := findCloseBrace(s, i+1)
			ph++
			fmt.Fprintf(&b, "$%d", ph)
			i = end
		case '}':
			if i+1 < len(s) && s[i+1] == '}' {
				b.WriteByte('}')
				i++
				continue
			}
			b.WriteByte('}')
		case '"':
			if i+1 < len(s) && s[i+1] == '"' {
				b.WriteByte('"')
				i++
				continue
			}
			return b.String(), strings.TrimSpace(s[i+1:])
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String(), ""
}

// readRawInterpStr reads a raw interpolated string ($"""...""").
func readRawInterpStr(s string) (content, rest string) {
	var b strings.Builder
	ph := 0
	for i := 0; i < len(s); i++ {
		if i+2 < len(s) && s[i] == '"' && s[i+1] == '"' && s[i+2] == '"' {
			return trimRawContent(b.String()), strings.TrimSpace(s[i+3:])
		}
		switch s[i] {
		case '{':
			if i+1 < len(s) && s[i+1] == '{' {
				b.WriteByte('{')
				i++
				continue
			}
			end := findCloseBrace(s, i+1)
			ph++
			fmt.Fprintf(&b, "$%d", ph)
			i = end
		case '}':
			if i+1 < len(s) && s[i+1] == '}' {
				b.WriteByte('}')
				i++
				continue
			}
			b.WriteByte('}')
		default:
			b.WriteByte(s[i])
		}
	}
	return trimRawContent(b.String()), ""
}

// findCloseBrace finds the matching } for an interpolation expression.
// It handles all C# string literal types (regular, verbatim, raw,
// interpolated) inside the expression by reusing skipStringLit.
func findCloseBrace(s string, pos int) int {
	depth := 1
	for i := pos; i < len(s) && depth > 0; {
		// Skip any string/char literal that starts at i.
		if next := skipStringLit(s, i); next > i {
			i = next
			continue
		}
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
		i++
	}
	return len(s) - 1
}

// trimRawContent strips the required leading/trailing newlines from raw string content.
func trimRawContent(s string) string {
	if strings.HasPrefix(s, "\r\n") {
		s = s[2:]
	} else if strings.HasPrefix(s, "\n") {
		s = s[1:]
	}
	if strings.HasSuffix(s, "\r\n") {
		s = s[:len(s)-2]
	} else if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	return s
}

// ---------------------------------------------------------------------------
// Code region skipping — used to identify code vs non-code (comments/strings)
// ---------------------------------------------------------------------------

// skipNonCode skips past comments, strings, and char literals at position i.
// Returns the position after the skipped element, or i if nothing to skip.
func skipNonCode(src string, i int) int {
	n := len(src)
	// Line comment
	if i+1 < n && src[i] == '/' && src[i+1] == '/' {
		j := i + 2
		for j < n && src[j] != '\n' {
			j++
		}
		if j < n {
			j++
		}
		return j
	}
	// Block comment
	if i+1 < n && src[i] == '/' && src[i+1] == '*' {
		j := i + 2
		for j+1 < n {
			if src[j] == '*' && src[j+1] == '/' {
				return j + 2
			}
			j++
		}
		return n
	}
	return skipStringLit(src, i)
}

// skipStringLit checks if position i starts a string or char literal.
func skipStringLit(src string, i int) int {
	n := len(src)
	// $@"..." or @$"..."
	if i+2 < n &&
		((src[i] == '$' && src[i+1] == '@' && src[i+2] == '"') ||
			(src[i] == '@' && src[i+1] == '$' && src[i+2] == '"')) {
		return skipVerbInterpStrLit(src, i+3)
	}
	// $"""..."""
	if i+3 < n && src[i] == '$' && src[i+1] == '"' && src[i+2] == '"' && src[i+3] == '"' {
		return skipRawStrLit(src, i+4)
	}
	// $"..."
	if i+1 < n && src[i] == '$' && src[i+1] == '"' {
		return skipInterpStrLit(src, i+2)
	}
	// @"..."
	if i+1 < n && src[i] == '@' && src[i+1] == '"' {
		return skipVerbStrLit(src, i+2)
	}
	// """..."""
	if i+2 < n && src[i] == '"' && src[i+1] == '"' && src[i+2] == '"' {
		return skipRawStrLit(src, i+3)
	}
	// "..."
	if i < n && src[i] == '"' {
		return skipRegStrLit(src, i+1)
	}
	// '...'
	if i < n && src[i] == '\'' {
		return skipCharLit(src, i+1)
	}
	return i
}

// skipRegStrLit skips a regular string literal and returns the next position.
func skipRegStrLit(src string, pos int) int {
	for i := pos; i < len(src); i++ {
		if src[i] == '\\' {
			i++
			continue
		}
		if src[i] == '"' {
			return i + 1
		}
	}
	return len(src)
}

// skipVerbStrLit skips a verbatim string literal and returns the next position.
func skipVerbStrLit(src string, pos int) int {
	for i := pos; i < len(src); i++ {
		if src[i] == '"' {
			if i+1 < len(src) && src[i+1] == '"' {
				i++
				continue
			}
			return i + 1
		}
	}
	return len(src)
}

// skipInterpStrLit skips an interpolated string literal and returns the next position.
func skipInterpStrLit(src string, pos int) int {
	for i := pos; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '{':
			if i+1 < len(src) && src[i+1] == '{' {
				i++
				continue
			}
			i = skipBraceExprLit(src, i+1) - 1
		case '"':
			return i + 1
		}
	}
	return len(src)
}

// skipVerbInterpStrLit skips a verbatim interpolated string literal.
func skipVerbInterpStrLit(src string, pos int) int {
	for i := pos; i < len(src); i++ {
		switch src[i] {
		case '{':
			if i+1 < len(src) && src[i+1] == '{' {
				i++
				continue
			}
			i = skipBraceExprLit(src, i+1) - 1
		case '"':
			if i+1 < len(src) && src[i+1] == '"' {
				i++
				continue
			}
			return i + 1
		}
	}
	return len(src)
}

// skipRawStrLit skips a raw string literal and returns the next position.
func skipRawStrLit(src string, pos int) int {
	for i := pos; i+2 < len(src); i++ {
		if src[i] == '"' && src[i+1] == '"' && src[i+2] == '"' {
			return i + 3
		}
	}
	return len(src)
}

// skipCharLit skips a character literal and returns the next position.
func skipCharLit(src string, pos int) int {
	for i := pos; i < len(src); i++ {
		if src[i] == '\\' {
			i++
			continue
		}
		if src[i] == '\'' {
			return i + 1
		}
	}
	return len(src)
}

// skipBraceExprLit skips an interpolated brace expression and returns the next position.
func skipBraceExprLit(src string, pos int) int {
	depth := 1
	for i := pos; i < len(src) && depth > 0; i++ {
		if next := skipStringLit(src, i); next > i {
			i = next - 1
			continue
		}
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(src)
}

// ---------------------------------------------------------------------------
// Variable collection
// ---------------------------------------------------------------------------

// collectVars gathers simple string-backed variable assignments from src.
func collectVars(src string) map[string]varDef {
	vars := make(map[string]varDef)
	n := len(src)

	for i := 0; i < n; {
		if next := skipNonCode(src, i); next > i {
			i = next
			continue
		}
		if src[i] == '=' && !isCompoundOrComparison(src, i, n) {
			name := identBefore(src, i)
			if name == "" || isCSharpKeyword(name) {
				i++
				continue
			}
			rhsStart := i + 1
			for rhsStart < n && isWS(src[rhsStart]) {
				rhsStart++
			}
			rhsEnd := findSemicolon(src, rhsStart)
			if rhsEnd < 0 {
				i++
				continue
			}
			rhs := strings.TrimSpace(src[rhsStart:rhsEnd])
			if rhs != "" && isStringLiteralStart(rhs) {
				vars[name] = varDef{expr: rhs}
			}
			i = rhsEnd + 1
			continue
		}
		i++
	}
	return vars
}

// collectDatabaseFacadeNames extracts identifiers declared with DatabaseFacade type text.
func collectDatabaseFacadeNames(src string) map[string]struct{} {
	names := make(map[string]struct{})
	n := len(src)

	for i := 0; i < n; {
		if next := skipNonCode(src, i); next > i {
			i = next
			continue
		}
		typeLen := matchDatabaseFacadeType(src, i)
		if typeLen == 0 {
			i++
			continue
		}

		j := i + typeLen
		for j < n && isWS(src[j]) {
			j++
		}
		if j < n && src[j] == '?' {
			j++
			for j < n && isWS(src[j]) {
				j++
			}
		}

		name := readIdent(src[j:])
		if name != "" && !isCSharpKeyword(name) {
			names[name] = struct{}{}
			i = j + len(name)
			continue
		}
		i++
	}

	return names
}

// matchDatabaseFacadeType reports the length of a DatabaseFacade type token at pos.
func matchDatabaseFacadeType(src string, pos int) int {
	candidates := []string{
		"Microsoft.EntityFrameworkCore.Infrastructure.DatabaseFacade",
		"DatabaseFacade",
	}
	for _, candidate := range candidates {
		end := pos + len(candidate)
		if end > len(src) || src[pos:end] != candidate {
			continue
		}
		if pos > 0 && (isIdentByte(src[pos-1]) || src[pos-1] == '.') {
			continue
		}
		if end < len(src) && (isIdentByte(src[end]) || src[end] == '.') {
			continue
		}
		return len(candidate)
	}
	return 0
}

// isCompoundOrComparison reports whether '=' at i belongs to a non-assignment operator.
func isCompoundOrComparison(src string, i, n int) bool {
	if i+1 < n && (src[i+1] == '=' || src[i+1] == '>') {
		return true
	}
	if i > 0 {
		p := src[i-1]
		if p == '!' || p == '<' || p == '>' || p == '=' ||
			p == '+' || p == '-' || p == '*' || p == '/' ||
			p == '%' || p == '&' || p == '|' || p == '^' || p == '?' {
			return true
		}
	}
	return false
}

// identBefore returns the identifier immediately before pos, excluding member assignments.
func identBefore(src string, pos int) string {
	j := pos - 1
	for j >= 0 && (src[j] == ' ' || src[j] == '\t') {
		j--
	}
	if j < 0 || !isIdentByte(src[j]) {
		return ""
	}
	end := j + 1
	for j >= 0 && isIdentByte(src[j]) {
		j--
	}
	name := src[j+1 : end]
	// Reject member/property assignments like obj.Query = "..." — the dot
	// means this is not a local variable declaration.
	if j >= 0 && src[j] == '.' {
		return ""
	}
	return name
}

// identBeforePos returns the identifier immediately preceding pos.
func identBeforePos(src string, pos int) string {
	j := prevNonWS(src, pos-1)
	if j < 0 || !isIdentByte(src[j]) {
		return ""
	}
	end := j + 1
	for j >= 0 && isIdentByte(src[j]) {
		j--
	}
	return src[j+1 : end]
}

// findSemicolon returns the next statement terminator starting from pos.
func findSemicolon(src string, pos int) int {
	for i := pos; i < len(src); {
		if next := skipNonCode(src, i); next > i {
			i = next
			continue
		}
		if src[i] == ';' {
			return i
		}
		if src[i] == '{' || src[i] == '}' {
			return -1
		}
		i++
	}
	return -1
}

// isStringLiteralStart reports whether s begins with a supported C# string literal.
func isStringLiteralStart(s string) bool {
	if s == "" {
		return false
	}
	switch s[0] {
	case '"':
		return true
	case '@':
		return len(s) > 1 && (s[1] == '"' || (s[1] == '$' && len(s) > 2 && s[2] == '"'))
	case '$':
		return len(s) > 1 && (s[1] == '"' || (s[1] == '@' && len(s) > 2 && s[2] == '"'))
	}
	return false
}

var csharpKeywords = map[string]struct{}{
	"if": {}, "else": {}, "for": {}, "foreach": {}, "while": {},
	"do": {}, "switch": {}, "case": {}, "break": {}, "continue": {},
	"return": {}, "new": {}, "null": {}, "true": {}, "false": {},
	"this": {}, "base": {}, "class": {}, "struct": {}, "interface": {},
	"void": {}, "typeof": {}, "sizeof": {}, "nameof": {},
	"throw": {}, "try": {}, "catch": {}, "finally": {},
	"lock": {}, "using": {}, "checked": {}, "unchecked": {},
	"default": {}, "delegate": {}, "event": {}, "fixed": {},
	"goto": {}, "implicit": {}, "explicit": {}, "extern": {},
	"operator": {}, "params": {}, "ref": {}, "out": {},
	"stackalloc": {}, "unsafe": {}, "volatile": {},
}

// isCSharpKeyword reports whether s is a reserved C# keyword used for filtering.
func isCSharpKeyword(s string) bool {
	_, ok := csharpKeywords[s]
	return ok
}

// hasName reports whether name exists in the identifier set.
func hasName(names map[string]struct{}, name string) bool {
	_, ok := names[name]
	return ok
}

// ---------------------------------------------------------------------------
// Placeholder normalization
// ---------------------------------------------------------------------------

// normalizeFormatPlaceholders converts .NET format string placeholders ({0},
// {1}, ...) to PostgreSQL-style placeholders ($1, $2, ...).
func normalizeFormatPlaceholders(sql string) string {
	var b strings.Builder
	changed := false
	for i := 0; i < len(sql); i++ {
		if sql[i] == '{' {
			j := i + 1
			for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
				j++
			}
			if j > i+1 && j < len(sql) && sql[j] == '}' {
				num := 0
				for k := i + 1; k < j; k++ {
					num = num*10 + int(sql[k]-'0')
				}
				fmt.Fprintf(&b, "$%d", num+1)
				i = j
				changed = true
				continue
			}
		}
		b.WriteByte(sql[i])
	}
	if !changed {
		return sql
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Utility helpers
// ---------------------------------------------------------------------------

// buildLineOffsets builds a line-start offset table for source positions.
func buildLineOffsets(src string) []int {
	offsets := []int{0}
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

// offsetToLC converts a byte offset into 1-based line and column coordinates.
func offsetToLC(offsets []int, pos int) (line, col int) {
	lo, hi := 0, len(offsets)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if offsets[mid] <= pos {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo + 1, pos - offsets[lo] + 1
}

// prevNonWS returns the previous non-whitespace byte index before or at pos.
func prevNonWS(src string, pos int) int {
	for pos >= 0 && isWS(src[pos]) {
		pos--
	}
	return pos
}

// isIdentByte reports whether c is valid in a C# identifier tail.
func isIdentByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_'
}

// readIdent reads the leading C# identifier from s.
func readIdent(s string) string {
	if s == "" {
		return ""
	}
	c := s[0]
	if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && c != '_' {
		return ""
	}
	i := 1
	for i < len(s) && isIdentByte(s[i]) {
		i++
	}
	return s[:i]
}

// isWS reports whether c is recognized as scanner whitespace.
func isWS(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
