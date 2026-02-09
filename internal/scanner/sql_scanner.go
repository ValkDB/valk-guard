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

// scanSQLFile streams a SQL file and splits it into individual statements
// while respecting semicolons inside strings and comments.
func scanSQLFile(ctx context.Context, path string, directives []Directive, yield func(SQLStatement) bool) error {
	f, err := os.Open(path) //nolint:gosec // scanning user-provided source paths
	if err != nil {
		return fmt.Errorf("opening sql file %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // best-effort close

	reader := bufio.NewReader(f)
	var current strings.Builder

	line := 1
	startLine := 0
	state := scanStateNormal
	blockDepth := 0
	dollarTag := ""
	tagWindow := make([]byte, 0, 16)

	emitStatement := func(sql string, stmtLine int) error {
		stmt := strings.TrimSpace(sql)
		if stmt == "" {
			return nil
		}
		if stmtLine == 0 {
			stmtLine = line
		}
		if !yield(SQLStatement{
			SQL:      stmt,
			File:     path,
			Line:     stmtLine,
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

		switch state {
		case scanStateNormal:
			if ch == '\n' {
				current.WriteByte(ch)
				line++
				continue
			}

			if ch == ';' {
				if err := emitStatement(current.String(), startLine); err != nil {
					return err
				}
				current.Reset()
				startLine = 0
				continue
			}

			if ch == '-' {
				next, ok, err := peekByte(reader)
				if err != nil {
					return fmt.Errorf("reading sql file %s: %w", path, err)
				}
				if ok && next == '-' {
					if startLine == 0 {
						startLine = line
					}
					current.WriteByte(ch)
					_, _ = reader.ReadByte()
					current.WriteByte(next)
					state = scanStateLineComment
					continue
				}
			}

			if ch == '/' {
				next, ok, err := peekByte(reader)
				if err != nil {
					return fmt.Errorf("reading sql file %s: %w", path, err)
				}
				if ok && next == '*' {
					if startLine == 0 {
						startLine = line
					}
					current.WriteByte(ch)
					_, _ = reader.ReadByte()
					current.WriteByte(next)
					state = scanStateBlockComment
					blockDepth = 1
					continue
				}
			}

			if ch == '\'' {
				if startLine == 0 {
					startLine = line
				}
				current.WriteByte(ch)
				state = scanStateSingleQuote
				continue
			}

			if ch == '"' {
				if startLine == 0 {
					startLine = line
				}
				current.WriteByte(ch)
				state = scanStateDoubleQuote
				continue
			}

			if ch == '$' {
				if startLine == 0 {
					startLine = line
				}
				tag, ok, err := consumeDollarTag(reader, &current)
				if err != nil {
					return fmt.Errorf("reading sql file %s: %w", path, err)
				}
				if ok {
					state = scanStateDollarQuote
					dollarTag = tag
					tagWindow = tagWindow[:0]
				}
				continue
			}

			if startLine == 0 && !isSpace(ch) {
				startLine = line
			}
			current.WriteByte(ch)

		case scanStateLineComment:
			current.WriteByte(ch)
			if ch == '\n' {
				line++
				state = scanStateNormal
			}

		case scanStateBlockComment:
			current.WriteByte(ch)
			if ch == '\n' {
				line++
			}

			if ch == '/' {
				next, ok, err := peekByte(reader)
				if err != nil {
					return fmt.Errorf("reading sql file %s: %w", path, err)
				}
				if ok && next == '*' {
					_, _ = reader.ReadByte()
					current.WriteByte(next)
					blockDepth++
				}
				continue
			}

			if ch == '*' {
				next, ok, err := peekByte(reader)
				if err != nil {
					return fmt.Errorf("reading sql file %s: %w", path, err)
				}
				if ok && next == '/' {
					_, _ = reader.ReadByte()
					current.WriteByte(next)
					blockDepth--
					if blockDepth == 0 {
						state = scanStateNormal
					}
				}
			}

		case scanStateSingleQuote:
			current.WriteByte(ch)
			if ch == '\n' {
				line++
			}
			if ch != '\'' {
				continue
			}

			next, ok, err := peekByte(reader)
			if err != nil {
				return fmt.Errorf("reading sql file %s: %w", path, err)
			}
			if ok && next == '\'' {
				_, _ = reader.ReadByte()
				current.WriteByte(next)
				continue
			}
			state = scanStateNormal

		case scanStateDoubleQuote:
			current.WriteByte(ch)
			if ch == '\n' {
				line++
			}
			if ch != '"' {
				continue
			}

			next, ok, err := peekByte(reader)
			if err != nil {
				return fmt.Errorf("reading sql file %s: %w", path, err)
			}
			if ok && next == '"' {
				_, _ = reader.ReadByte()
				current.WriteByte(next)
				continue
			}
			state = scanStateNormal

		case scanStateDollarQuote:
			current.WriteByte(ch)
			if ch == '\n' {
				line++
			}
			tagWindow = append(tagWindow, ch)
			if len(tagWindow) > len(dollarTag) {
				tagWindow = tagWindow[1:]
			}
			if len(tagWindow) == len(dollarTag) && string(tagWindow) == dollarTag {
				state = scanStateNormal
				tagWindow = tagWindow[:0]
			}
		}
	}

	if err := emitStatement(current.String(), startLine); err != nil {
		return err
	}

	return nil
}

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

func consumeDollarTag(reader *bufio.Reader, current *strings.Builder) (string, bool, error) {
	buf := []byte{'$'}

	for {
		ch, err := reader.ReadByte()
		if errors.Is(err, io.EOF) {
			current.Write(buf)
			return "", false, nil
		}
		if err != nil {
			return "", false, err
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

func peekByte(reader *bufio.Reader) (byte, bool, error) {
	ch, err := reader.ReadByte()
	if errors.Is(err, io.EOF) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if err := reader.UnreadByte(); err != nil {
		return 0, false, err
	}
	return ch, true, nil
}

func isDollarTagChar(ch byte, first bool) bool {
	if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' {
		return true
	}
	if !first && ch >= '0' && ch <= '9' {
		return true
	}
	return false
}

func isSpace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}
