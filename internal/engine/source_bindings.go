// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/valkdb/valk-guard/internal/config"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/scanner/csharp"
	"github.com/valkdb/valk-guard/internal/scanner/goqu"
	"github.com/valkdb/valk-guard/internal/scanner/sqlalchemy"
	"github.com/valkdb/valk-guard/internal/schema"
	"github.com/valkdb/valk-guard/internal/schema/gomodel"
	"github.com/valkdb/valk-guard/internal/schema/pymodel"
)

// scannerBinding describes one scanner integration and the file extensions that
// should be fed into it.
type scannerBinding struct {
	name       string
	impl       scanner.Scanner
	extensions []string
}

// modelBinding describes one model extractor integration and how it maps to
// config/query engines.
type modelBinding struct {
	source        schema.ModelSource
	extractor     schema.ModelExtractor
	extensions    []string
	configEngines []scanner.Engine
	queryEngines  []scanner.Engine
}

// scannerInputs stores collected candidate files by lowercase extension.
type scannerInputs struct {
	byExt map[string][]string
}

// newScannerInputs constructs an empty extension-to-files index.
func newScannerInputs() scannerInputs {
	return scannerInputs{byExt: make(map[string][]string)}
}

// normalizeExt canonicalizes file extensions for input bucketing.
func normalizeExt(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}

// filesForExtensions returns a sorted, deduplicated list of files collected
// for the given extensions.
func (in scannerInputs) filesForExtensions(exts []string) []string {
	if len(exts) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string

	for _, rawExt := range exts {
		ext := normalizeExt(rawExt)
		if ext == "" {
			continue
		}
		for _, path := range in.byExt[ext] {
			if _, exists := seen[path]; exists {
				continue
			}
			seen[path] = struct{}{}
			out = append(out, path)
		}
	}

	slices.Sort(out)
	return out
}

// defaultScannerBindings returns the built-in source scanners and their file
// extensions.
func defaultScannerBindings() []scannerBinding {
	return []scannerBinding{
		{name: "sql", impl: &scanner.RawSQLScanner{}, extensions: []string{".sql"}},
		{name: "go", impl: &scanner.GoScanner{}, extensions: []string{".go"}},
		{name: "goqu", impl: &goqu.Scanner{}, extensions: []string{".go"}},
		{name: "sqlalchemy", impl: &sqlalchemy.Scanner{}, extensions: []string{".py"}},
		{name: "csharp", impl: &csharp.Scanner{}, extensions: []string{".cs"}},
	}
}

// defaultModelBindings returns the built-in model extractors and their engine
// mappings for schema-aware analysis.
func defaultModelBindings(cfg *config.Config) []modelBinding {
	return []modelBinding{
		{
			source:        schema.ModelSourceGo,
			extractor:     &gomodel.Extractor{Mode: gomodel.MappingMode(cfg.GoModel.MappingMode)},
			extensions:    []string{".go"},
			configEngines: []scanner.Engine{scanner.EngineGo},
			queryEngines:  []scanner.Engine{scanner.EngineGo, scanner.EngineGoqu},
		},
		{
			source:        schema.ModelSourceSQLAlchemy,
			extractor:     &pymodel.Extractor{},
			extensions:    []string{".py"},
			configEngines: []scanner.Engine{scanner.EngineSQLAlchemy},
			queryEngines:  []scanner.Engine{scanner.EngineSQLAlchemy},
		},
	}
}

// filterScannerBindings removes scanners disabled by the top-level sources
// config before file discovery decides which extensions are needed.
func filterScannerBindings(cfg *config.Config, bindings []scannerBinding) []scannerBinding {
	filtered := make([]scannerBinding, 0, len(bindings))
	for _, binding := range bindings {
		if !cfg.IsSourceEnabled(scanner.Engine(binding.name)) {
			continue
		}
		filtered = append(filtered, binding)
	}
	return filtered
}

// filterModelBindings removes model extractors whose related query/config
// engines are all disabled by the top-level sources config.
func filterModelBindings(cfg *config.Config, bindings []modelBinding) []modelBinding {
	filtered := make([]modelBinding, 0, len(bindings))
	for _, binding := range bindings {
		if modelBindingEnabled(cfg, &binding) {
			filtered = append(filtered, binding)
		}
	}
	return filtered
}

// modelBindingEnabled reports whether any engine that can use a model binding
// is enabled in source config.
func modelBindingEnabled(cfg *config.Config, binding *modelBinding) bool {
	for _, engine := range binding.configEngines {
		if cfg.IsSourceEnabled(engine) {
			return true
		}
	}
	for _, engine := range binding.queryEngines {
		if cfg.IsSourceEnabled(engine) {
			return true
		}
	}
	return false
}

// requiredExtensions returns the set of file extensions needed for all active
// scanners and model extractors.
func requiredExtensions(scannerBindings []scannerBinding, modelBindings []modelBinding) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(ext string) {
		ext = normalizeExt(ext)
		if ext == "" {
			return
		}
		if _, exists := seen[ext]; exists {
			return
		}
		seen[ext] = struct{}{}
		out = append(out, ext)
	}

	for _, b := range scannerBindings {
		for _, ext := range b.extensions {
			add(ext)
		}
	}
	for _, b := range modelBindings {
		for _, ext := range b.extensions {
			add(ext)
		}
	}
	slices.Sort(out)
	return out
}

// sourceConfigEngines maps each model source to the config engines that should
// enable schema rules for it.
func sourceConfigEngines(bindings []modelBinding) map[schema.ModelSource][]scanner.Engine {
	result := make(map[schema.ModelSource][]scanner.Engine)
	seen := make(map[schema.ModelSource]map[scanner.Engine]struct{})

	for _, b := range bindings {
		if _, ok := seen[b.source]; !ok {
			seen[b.source] = make(map[scanner.Engine]struct{})
		}
		for _, engine := range b.configEngines {
			if _, exists := seen[b.source][engine]; exists {
				continue
			}
			seen[b.source][engine] = struct{}{}
			result[b.source] = append(result[b.source], engine)
		}
	}
	return result
}

// sourceQueryEngines maps query engines to the model sources that can augment
// them during query-schema analysis.
func sourceQueryEngines(bindings []modelBinding) map[scanner.Engine][]schema.ModelSource {
	result := make(map[scanner.Engine][]schema.ModelSource)
	seen := make(map[scanner.Engine]map[schema.ModelSource]struct{})

	for _, b := range bindings {
		for _, engine := range b.queryEngines {
			if _, ok := seen[engine]; !ok {
				seen[engine] = make(map[schema.ModelSource]struct{})
			}
			if _, exists := seen[engine][b.source]; exists {
				continue
			}
			seen[engine][b.source] = struct{}{}
			result[engine] = append(result[engine], b.source)
		}
	}
	return result
}

// addInputFile appends path to the extension bucket when unseen.
func addInputFile(inputs scannerInputs, seen map[string]map[string]struct{}, ext, path string) {
	if _, ok := seen[ext]; !ok {
		seen[ext] = make(map[string]struct{})
	}
	if _, exists := seen[ext][path]; exists {
		return
	}
	seen[ext][path] = struct{}{}
	inputs.byExt[ext] = append(inputs.byExt[ext], filepath.Clean(path))
}
