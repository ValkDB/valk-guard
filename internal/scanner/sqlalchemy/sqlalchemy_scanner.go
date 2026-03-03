// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

// Package sqlalchemy provides a scanner that extracts raw SQL from
// SQLAlchemy text() and .execute() calls in Python source files.
//
// Unlike the Go-based scanners which use go/ast, this scanner shells out to
// a Python subprocess that walks the Python AST. The subprocess is only
// invoked when .py files containing SQLAlchemy usage are found, keeping
// the cost at zero when no Python code is present.
package sqlalchemy

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"strings"
	"time"

	"github.com/valkdb/valk-guard/internal/pyrunner"
	"github.com/valkdb/valk-guard/internal/scanner"
)

//go:embed extract_sql.py
var extractScript embed.FS

// sqlalchemyMarkers are the quick-reject keywords used to determine whether a
// .py file likely contains SQLAlchemy or ORM query-builder usage.
var sqlalchemyMarkers = []string{
	"sqlalchemy",
	"execute",
	".query(",
	"select(",
	".join(",
	".outerjoin(",
	".filter(",
	".filter_by(",
	".update(",
	".delete(",
}

// Scanner extracts SQL string literals from SQLAlchemy text() and
// .execute() calls in Python source files using Python's ast module.
type Scanner struct{}

// Scan walks the given paths, finds .py files containing sqlalchemy usage,
// and extracts SQL strings by invoking a Python AST walker subprocess.
// The Python subprocess is only started if candidate files are found.
func (s *Scanner) Scan(ctx context.Context, paths []string) iter.Seq2[scanner.SQLStatement, error] {
	return func(yield func(scanner.SQLStatement, error) bool) {
		// Phase 1: collect candidate .py files (Go-side, no subprocess yet).
		candidates, err := pyrunner.CollectPyCandidates(ctx, paths, sqlalchemyMarkers)
		if err != nil {
			_ = yield(scanner.SQLStatement{}, err)
			return
		}

		if len(candidates) == 0 {
			return
		}

		// Phase 2: invoke Python AST extractor on all candidates at once.
		extracted, err := runPythonExtractor(ctx, candidates)
		if err != nil {
			_ = yield(scanner.SQLStatement{}, err)
			return
		}

		// Phase 3: attach directive information from each file.
		yieldWithDirectives(ctx, extracted, yield)
	}
}

// yieldWithDirectives attaches per-file inline-disable directives to each
// extracted SQL result and yields the resulting SQLStatement. A directive
// cache is maintained so each file is read at most once.
func yieldWithDirectives(
	ctx context.Context,
	extracted []pyResult,
	yield func(scanner.SQLStatement, error) bool,
) {
	directiveCache := make(map[string][]scanner.Directive)

	for _, e := range extracted {
		if err := ctx.Err(); err != nil {
			_ = yield(scanner.SQLStatement{}, err)
			return
		}

		directives, ok := directiveCache[e.File]
		if !ok {
			data, readErr := os.ReadFile(e.File) //nolint:gosec // already read above
			if readErr != nil {
				_ = yield(scanner.SQLStatement{}, fmt.Errorf("reading python file %s for directives: %w", e.File, readErr))
				return
			}
			lines := strings.Split(string(data), "\n")
			directives = scanner.ParseDirectives(lines)
			directiveCache[e.File] = directives
		}

		if !yield(scanner.SQLStatement{
			SQL:      e.SQL,
			File:     e.File,
			Line:     e.Line,
			Engine:   scanner.EngineSQLAlchemy,
			Disabled: scanner.DisabledRulesForLine(directives, e.Line),
		}, nil) {
			return
		}
	}
}

// pyResult represents a single SQL extraction from the Python script.
type pyResult struct {
	File string `json:"file"`
	Line int    `json:"line"`
	SQL  string `json:"sql"`
}

// runPythonExtractor invokes the embedded Python script on the given files
// and returns the extracted SQL statements. All files are passed in a single
// subprocess invocation to amortize the ~20ms Python startup cost.
func runPythonExtractor(parent context.Context, files []string) ([]pyResult, error) {
	scriptData, err := extractScript.ReadFile("extract_sql.py")
	if err != nil {
		return nil, err
	}

	scriptPath, cleanup, err := pyrunner.WriteTempScript(scriptData)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	out, err := pyrunner.ExecScript(ctx, scriptPath, files)
	if err != nil {
		return nil, err
	}

	var results []pyResult
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, err
	}

	return results, nil
}
