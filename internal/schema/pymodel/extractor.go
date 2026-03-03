// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

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
	"time"

	"github.com/valkdb/valk-guard/internal/pyrunner"
	"github.com/valkdb/valk-guard/internal/schema"
)

//go:embed extract_models.py
var extractScript []byte

// pymodelMarkers are the quick-reject keywords used to determine whether a
// .py file likely contains SQLAlchemy model definitions.
var pymodelMarkers = []string{
	"__tablename__",
	"Column(",
	"mapped_column(",
}

// Extractor extracts SQLAlchemy model definitions from Python source files.
type Extractor struct{}

// ExtractModels walks the given paths, finds .py files containing SQLAlchemy
// model definitions, and extracts model metadata by invoking a Python AST
// walker subprocess. The Python subprocess is only started if candidate files
// are found.
func (e *Extractor) ExtractModels(ctx context.Context, paths []string) ([]schema.ModelDef, error) {
	// Phase 1: collect candidate .py files (Go-side, no subprocess yet).
	candidates, err := pyrunner.CollectPyCandidates(ctx, paths, pymodelMarkers)
	if err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Phase 2: invoke Python AST extractor on all candidates at once.
	return runPythonExtractor(ctx, candidates)
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
	scriptPath, cleanup, err := pyrunner.WriteTempScript(extractScript)
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

	var raw []pyModelResult
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing python extractor output: %w", err)
	}

	return convertPyResults(raw), nil
}

// convertPyResults converts raw Python extraction results into schema.ModelDef values.
func convertPyResults(raw []pyModelResult) []schema.ModelDef {
	models := make([]schema.ModelDef, len(raw))
	for i, r := range raw {
		cols := make([]schema.ModelColumn, len(r.Columns))
		for j, c := range r.Columns {
			cols[j] = schema.ModelColumn{
				Name:          c.Name,
				Field:         c.Field,
				Type:          c.Type,
				Line:          c.Line,
				MappingKind:   schema.MappingKindExplicit,
				MappingSource: "sqlalchemy_ast",
			}
		}
		models[i] = schema.ModelDef{
			Table:              r.Table,
			TableExplicit:      true,
			TableMappingKind:   schema.MappingKindExplicit,
			TableMappingSource: "sqlalchemy_ast",
			Source:             schema.ModelSourceSQLAlchemy,
			Columns:            cols,
			File:               r.File,
			Line:               r.Line,
		}
	}
	return models
}
