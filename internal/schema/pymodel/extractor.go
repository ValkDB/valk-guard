// Package pymodel extracts SQLAlchemy model definitions from Python source
// files by invoking a Python subprocess that walks the Python AST.
//
// The subprocess is only invoked when .py files containing SQLAlchemy model
// keywords are found, keeping the cost at zero when no Python models are
// present.
package pymodel

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/valkdb/valk-guard/internal/schema"
)

//go:embed extract_models.py
var extractScript []byte

// Extractor extracts SQLAlchemy model definitions from Python source files.
type Extractor struct{}

// ExtractModels walks the given paths, finds .py files containing SQLAlchemy
// model definitions, and extracts model metadata by invoking a Python AST
// walker subprocess. The Python subprocess is only started if candidate files
// are found.
func (e *Extractor) ExtractModels(ctx context.Context, paths []string) ([]schema.ModelDef, error) {
	// Phase 1: collect candidate .py files (Go-side, no subprocess yet).
	candidates, err := collectPyCandidates(ctx, paths)
	if err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Phase 2: invoke Python AST extractor on all candidates at once.
	return runPythonExtractor(ctx, candidates)
}

// collectPyCandidates walks the given root paths, finds .py files that contain
// SQLAlchemy model keywords, and returns their paths. Files that do not contain
// any of the quick-reject keywords are skipped to avoid unnecessary subprocess
// invocations.
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

			src := string(data)
			if !strings.Contains(src, "__tablename__") && !strings.Contains(src, "Column(") {
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

// pyModelResult represents a single model extraction from the Python script.
type pyModelResult struct {
	Table   string           `json:"table"`
	Columns []pyColumnResult `json:"columns"`
	File    string           `json:"file"`
	Line    int              `json:"line"`
}

// pyColumnResult represents a column extracted by the Python script.
type pyColumnResult struct {
	Name  string `json:"name"`
	Field string `json:"field"`
	Type  string `json:"type"`
	Line  int    `json:"line"`
}

// runPythonExtractor invokes the embedded Python script on the given files
// and returns the extracted model definitions. All files are passed in a single
// subprocess invocation to amortize the ~20ms Python startup cost.
func runPythonExtractor(parent context.Context, files []string) ([]schema.ModelDef, error) {
	scriptPath, cleanup, err := writeTempScript()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	return execPythonScript(ctx, scriptPath, files)
}

// writeTempScript writes the embedded extract_models.py to a temporary file
// and returns its path together with a cleanup function that removes the file.
func writeTempScript() (scriptPath string, cleanup func(), err error) {
	tmpScript, err := os.CreateTemp("", "valk-guard-models-*.py")
	if err != nil {
		return "", func() {}, err
	}
	removeFn := func() { _ = os.Remove(tmpScript.Name()) } //nolint:errcheck // best-effort cleanup

	if _, err := tmpScript.Write(extractScript); err != nil {
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
// parses its JSON output into a slice of schema.ModelDef.
func execPythonScript(ctx context.Context, scriptPath string, files []string) ([]schema.ModelDef, error) {
	args := append([]string{scriptPath}, files...)
	cmd := exec.CommandContext(ctx, "python3", args...) //nolint:gosec // args are file paths we collected

	var stderr strings.Builder
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("python model extraction timed out after 2m")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return nil, err
		}
		return nil, fmt.Errorf("python model extraction failed: %s", msg)
	}

	var raw []pyModelResult
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}

	models := make([]schema.ModelDef, len(raw))
	for i, r := range raw {
		cols := make([]schema.ModelColumn, len(r.Columns))
		for j, c := range r.Columns {
			cols[j] = schema.ModelColumn{
				Name:  c.Name,
				Field: c.Field,
				Type:  c.Type,
				Line:  c.Line,
			}
		}
		models[i] = schema.ModelDef{
			Table:         r.Table,
			TableExplicit: true,
			Source:        schema.ModelSourceSQLAlchemy,
			Columns:       cols,
			File:          r.File,
			Line:          r.Line,
		}
	}

	return models, nil
}
