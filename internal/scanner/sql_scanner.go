package scanner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"
)

// RawSQLScanner finds .sql files and splits them into individual statements.
type RawSQLScanner struct{}

const (
	scanStateNormal = iota
	scanStateSingleQuote
	scanStateDoubleQuote
	scanStateDollarQuote
	scanStateLineComment
	scanStateBlockComment
)

var errRawSQLScannerStop = errors.New("raw sql scanner stop")

// Scan walks the given paths, finds .sql files, and streams extracted
// statements.
func (s *RawSQLScanner) Scan(ctx context.Context, paths []string) iter.Seq2[SQLStatement, error] {
	return func(yield func(SQLStatement, error) bool) {
		for _, root := range paths {
			if err := ctx.Err(); err != nil {
				_ = yield(SQLStatement{}, err)
				return
			}

			err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				if d.IsDir() || strings.ToLower(filepath.Ext(path)) != ".sql" {
					return nil
				}

				directives, dirErr := scanDirectives(path)
				if dirErr != nil {
					return dirErr
				}

				err = scanSQLFile(ctx, path, directives, func(stmt SQLStatement) bool {
					return yield(stmt, nil)
				})
				if err != nil {
					if errors.Is(err, errRawSQLScannerStop) {
						return errRawSQLScannerStop
					}
					return fmt.Errorf("scanning sql file %s: %w", path, err)
				}
				return nil
			})
			if err != nil {
				if errors.Is(err, errRawSQLScannerStop) {
					return
				}
				_ = yield(SQLStatement{}, err)
				return
			}
		}
	}
}

// sqlScanState holds all mutable state threaded through the SQL scanner loop.
type sqlScanState struct {
	current    strings.Builder
	line       int
	startLine  int
	state      int
	blockDepth int
	dollarTag  string
	tagWindow  []byte
}

// scanSQLFile streams a SQL file and splits it into individual statements
// while respecting semicolons inside strings and comments.
func scanSQLFile(ctx context.Context, path string, directives []Directive, yield func(SQLStatement) bool) error {
	f, err := os.Open(path) //nolint:gosec // scanning user-provided source paths
	if err != nil {
		return fmt.Errorf("opening sql file %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // best-effort close

	reader := bufio.NewReader(f)
	ss := &sqlScanState{
		line:      1,
		tagWindow: make([]byte, 0, 16),
	}

	emitStatement := func(sql string, stmtLine int) error {
		stmt := strings.TrimSpace(sql)
		if stmt == "" {
			return nil
		}
		if stmtLine == 0 {
			stmtLine = ss.line
		}
		if !yield(SQLStatement{
			SQL:      stmt,
			File:     path,
			Line:     stmtLine,
			Engine:   EngineSQL,
			Disabled: DisabledRulesForLine(directives, stmtLine),
		}) {
			return errRawSQLScannerStop
		}
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		ch, readErr := reader.ReadByte()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return fmt.Errorf("reading sql file %s: %w", path, readErr)
		}

		switch ss.state {
		case scanStateNormal:
			if err := handleNormalState(reader, ss, ch, path, emitStatement); err != nil {
				return err
			}

		case scanStateLineComment:
			ss.current.WriteByte(ch)
			if ch == '\n' {
				ss.line++
				ss.state = scanStateNormal
			}

		case scanStateBlockComment:
			if err := handleBlockCommentState(reader, ss, ch, path); err != nil {
				return err
			}

		case scanStateSingleQuote:
			if err := handleQuoteState(reader, ss, ch, '\'', path); err != nil {
				return err
			}

		case scanStateDoubleQuote:
			if err := handleQuoteState(reader, ss, ch, '"', path); err != nil {
				return err
			}

		case scanStateDollarQuote:
			ss.current.WriteByte(ch)
			if ch == '\n' {
				ss.line++
			}
			ss.tagWindow = append(ss.tagWindow, ch)
			if len(ss.tagWindow) > len(ss.dollarTag) {
				ss.tagWindow = ss.tagWindow[1:]
			}
			if len(ss.tagWindow) == len(ss.dollarTag) && string(ss.tagWindow) == ss.dollarTag {
				ss.state = scanStateNormal
				ss.tagWindow = ss.tagWindow[:0]
			}
		}
	}

	if err := emitStatement(ss.current.String(), ss.startLine); err != nil {
		return err
	}

	return nil
}

// handleNormalState processes one byte while in the normal (unquoted) scan
// state, transitioning to comment, string, or dollar-quote states as needed.
// It delegates statement emission to emitStatement.
func handleNormalState(
	reader *bufio.Reader,
	ss *sqlScanState,
	ch byte,
	path string,
	emitStatement func(string, int) error,
) error {
	if ch == '\n' {
		ss.current.WriteByte(ch)
		ss.line++
		return nil
	}

	if ch == ';' {
		if err := emitStatement(ss.current.String(), ss.startLine); err != nil {
			return err
		}
		ss.current.Reset()
		ss.startLine = 0
		return nil
	}

	if ch == '-' {
		next, ok, err := peekByte(reader)
		if err != nil {
			return fmt.Errorf("reading sql file %s: %w", path, err)
		}
		if ok && next == '-' {
			if ss.startLine == 0 {
				ss.startLine = ss.line
			}
			ss.current.WriteByte(ch)
			_, _ = reader.ReadByte()
			ss.current.WriteByte(next)
			ss.state = scanStateLineComment
			return nil
		}
	}

	if ch == '/' {
		next, ok, err := peekByte(reader)
		if err != nil {
			return fmt.Errorf("reading sql file %s: %w", path, err)
		}
		if ok && next == '*' {
			if ss.startLine == 0 {
				ss.startLine = ss.line
			}
			ss.current.WriteByte(ch)
			_, _ = reader.ReadByte()
			ss.current.WriteByte(next)
			ss.state = scanStateBlockComment
			ss.blockDepth = 1
			return nil
		}
	}

	if ch == '\'' {
		if ss.startLine == 0 {
			ss.startLine = ss.line
		}
		ss.current.WriteByte(ch)
		ss.state = scanStateSingleQuote
		return nil
	}

	if ch == '"' {
		if ss.startLine == 0 {
			ss.startLine = ss.line
		}
		ss.current.WriteByte(ch)
		ss.state = scanStateDoubleQuote
		return nil
	}

	if ch == '$' {
		if ss.startLine == 0 {
			ss.startLine = ss.line
		}
		tag, ok, err := consumeDollarTag(reader, &ss.current)
		if err != nil {
			return fmt.Errorf("reading sql file %s: %w", path, err)
		}
		if ok {
			ss.state = scanStateDollarQuote
			ss.dollarTag = tag
			ss.tagWindow = ss.tagWindow[:0]
		}
		return nil
	}

	if ss.startLine == 0 && !isSpace(ch) {
		ss.startLine = ss.line
	}
	ss.current.WriteByte(ch)
	return nil
}

// handleBlockCommentState processes one byte while inside a /* ... */ block
// comment, tracking nesting depth and transitioning back to normal when the
// comment closes.
func handleBlockCommentState(reader *bufio.Reader, ss *sqlScanState, ch byte, path string) error {
	ss.current.WriteByte(ch)
	if ch == '\n' {
		ss.line++
	}

	if ch == '/' {
		next, ok, err := peekByte(reader)
		if err != nil {
			return fmt.Errorf("reading sql file %s: %w", path, err)
		}
		if ok && next == '*' {
			_, _ = reader.ReadByte()
			ss.current.WriteByte(next)
			ss.blockDepth++
		}
		return nil
	}

	if ch == '*' {
		next, ok, err := peekByte(reader)
		if err != nil {
			return fmt.Errorf("reading sql file %s: %w", path, err)
		}
		if ok && next == '/' {
			_, _ = reader.ReadByte()
			ss.current.WriteByte(next)
			ss.blockDepth--
			if ss.blockDepth == 0 {
				ss.state = scanStateNormal
			}
		}
	}

	return nil
}

// handleQuoteState processes one byte while inside a single-quoted or
// double-quoted string literal. The quote parameter is the delimiter character
// ('\'' or '"'). It handles escaped-quote sequences and transitions back to
// normal state when the closing delimiter is found.
func handleQuoteState(reader *bufio.Reader, ss *sqlScanState, ch, quote byte, path string) error {
	ss.current.WriteByte(ch)
	if ch == '\n' {
		ss.line++
	}
	if ch != quote {
		return nil
	}

	next, ok, err := peekByte(reader)
	if err != nil {
		return fmt.Errorf("reading sql file %s: %w", path, err)
	}
	if ok && next == quote {
		_, _ = reader.ReadByte()
		ss.current.WriteByte(next)
		return nil
	}
	ss.state = scanStateNormal
	return nil
}

// scanDirectives reads the SQL file at path and returns all inline disable
// directives found in it, without parsing any SQL statements.
func scanDirectives(path string) ([]Directive, error) {
	f, err := os.Open(path) //nolint:gosec // scanning user-provided source paths
	if err != nil {
		return nil, fmt.Errorf("opening sql file %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // best-effort close

	reader := bufio.NewReader(f)
	lineNumber := 1
	var directives []Directive

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, fmt.Errorf("reading sql file %s: %w", path, readErr)
		}

		trimmed := strings.TrimSuffix(line, "\n")
		trimmed = strings.TrimSuffix(trimmed, "\r")
		if d, ok := ParseDirectiveLine(trimmed, lineNumber); ok {
			directives = append(directives, d)
		}

		if errors.Is(readErr, io.EOF) {
			break
		}
		lineNumber++
	}

	return directives, nil
}

// consumeDollarTag attempts to read a PostgreSQL dollar-quoted tag (e.g. "$$"
// or "$tag$") from reader. On success it writes the tag bytes to current and
// returns the full tag string with ok=true. If the bytes do not form a valid
// dollar tag, consumed bytes are written to current and ok=false is returned.
func consumeDollarTag(reader *bufio.Reader, current *strings.Builder) (tag string, ok bool, err error) {
	buf := []byte{'$'}

	for {
		ch, readErr := reader.ReadByte()
		if errors.Is(readErr, io.EOF) {
			current.Write(buf)
			return "", false, nil
		}
		if readErr != nil {
			return "", false, readErr
		}

		if ch == '$' {
			buf = append(buf, ch)
			tag := string(buf)
			current.Write(buf)
			return tag, true, nil
		}

		if isDollarTagChar(ch, len(buf) == 1) {
			buf = append(buf, ch)
			continue
		}

		if err := reader.UnreadByte(); err != nil {
			return "", false, err
		}
		current.Write(buf)
		return "", false, nil
	}
}

// peekByte reads the next byte from reader without consuming it. It returns
// ok=false at EOF and an error for any other read failure.
func peekByte(reader *bufio.Reader) (b byte, ok bool, err error) {
	ch, readErr := reader.ReadByte()
	if errors.Is(readErr, io.EOF) {
		return 0, false, nil
	}
	if readErr != nil {
		return 0, false, readErr
	}
	if unreadErr := reader.UnreadByte(); unreadErr != nil {
		return 0, false, unreadErr
	}
	return ch, true, nil
}

// isDollarTagChar reports whether ch is a valid character in a PostgreSQL
// dollar-quote tag. The first character must be a letter or underscore;
// subsequent characters may also be digits.
func isDollarTagChar(ch byte, first bool) bool {
	if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' {
		return true
	}
	if !first && ch >= '0' && ch <= '9' {
		return true
	}
	return false
}

// isSpace reports whether ch is an ASCII whitespace character
// (space, tab, carriage return, or newline).
func isSpace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}
