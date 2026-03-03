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
	"io/fs"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/valkdb/valk-guard/internal/scanner"
)

//go:embed extract_sql.py
var extractScript embed.FS

// Scanner extracts SQL string literals from SQLAlchemy text() and
// .execute() calls in Python source files using Python's ast module.
type Scanner struct{}

// Scan walks the given paths, finds .py files containing sqlalchemy usage,
// and extracts SQL strings by invoking a Python AST walker subprocess.
// The Python subprocess is only started if candidate files are found.
func (s *Scanner) Scan(ctx context.Context, paths []string) iter.Seq2[scanner.SQLStatement, error] {
	return func(yield func(scanner.SQLStatement, error) bool) {
		// Phase 1: collect candidate .py files (Go-side, no subprocess yet).
		candidates, err := collectPyCandidates(ctx, paths)
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

// collectPyCandidates walks the given root paths, finds .py files that contain
// SQLAlchemy or ORM query-builder usage patterns, and returns their paths.
// Files that do not contain any of the quick-reject keywords are skipped to
// avoid unnecessary subprocess invocations.
func collectPyCandidates(ctx context.Context, paths []string) ([]string, error) {
	var candidates []string

	for _, root := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".py" {
				return nil
			}

			data, readErr := os.ReadFile(path) //nolint:gosec // scanning user-provided source paths
			if readErr != nil {
				return fmt.Errorf("reading python file %s: %w", path, readErr)
			}

			// Quick-reject: skip files that don't look like SQLAlchemy or ORM query-builder usage.
			if !containsAny(string(data),
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
			) {
				return nil
			}

			candidates = append(candidates, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return candidates, nil
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

// containsAny reports whether s contains any of the provided needle substrings.
func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
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
	scriptPath, cleanup, err := writeTempScript()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	return execPythonScript(ctx, scriptPath, files)
}

// writeTempScript writes the embedded extract_sql.py to a temporary file and
// returns its path together with a cleanup function that removes the file.
func writeTempScript() (scriptPath string, cleanup func(), err error) {
	scriptData, err := extractScript.ReadFile("extract_sql.py")
	if err != nil {
		return "", func() {}, err
	}

	tmpScript, err := os.CreateTemp("", "valk-guard-extract-*.py")
	if err != nil {
		return "", func() {}, err
	}
	removeFn := func() { _ = os.Remove(tmpScript.Name()) } //nolint:errcheck // best-effort cleanup

	if _, err := tmpScript.Write(scriptData); err != nil {
		if closeErr := tmpScript.Close(); closeErr != nil {
			return "", removeFn, fmt.Errorf("closing temp extractor script after write failure: %w", closeErr)
		}
		return "", removeFn, err
	}
	if err := tmpScript.Close(); err != nil {
		return "", removeFn, err
	}

	return tmpScript.Name(), removeFn, nil
}

// execPythonScript runs the Python script at scriptPath against files and
// parses its JSON output into a slice of pyResult. It converts subprocess
// errors into descriptive messages.
func execPythonScript(ctx context.Context, scriptPath string, files []string) ([]pyResult, error) {
	// Build command: python3 <script> file1.py file2.py ...
	args := append([]string{scriptPath}, files...)
	cmd := exec.CommandContext(ctx, "python3", args...) //nolint:gosec // args are file paths we collected

	var stderr strings.Builder
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("python ast extraction timed out after 2m")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return nil, err
		}
		return nil, fmt.Errorf("python ast extraction failed: %s", msg)
	}

	var results []pyResult
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, err
	}

	return results, nil
}
