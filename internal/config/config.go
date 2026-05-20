// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/valkdb/valk-guard/internal/rules"
	"github.com/valkdb/valk-guard/internal/scanner"
)

const ruleEngineAll = "all"

// GoModelMappingMode controls how Go model extraction resolves table/column
// mappings when explicit ORM metadata is absent.
type GoModelMappingMode string

// Go model mapping mode values for column/table resolution.
const (
	GoModelMappingStrict     GoModelMappingMode = "strict"
	GoModelMappingBalanced   GoModelMappingMode = "balanced"
	GoModelMappingPermissive GoModelMappingMode = "permissive"
)

// RuleConfig holds per-rule overrides from the config file.
type RuleConfig struct {
	Enabled  *bool          `yaml:"enabled,omitempty"`
	Severity rules.Severity `yaml:"severity,omitempty"`
	Engines  []string       `yaml:"engines,omitempty"`
}

// GoModelConfig controls Go model extraction behavior.
type GoModelConfig struct {
	MappingMode GoModelMappingMode `yaml:"mapping_mode,omitempty"`
}

// Config represents the top-level valk-guard configuration.
type Config struct {
	// Format selects the output reporter: terminal, json, rdjsonl, or sarif.
	Format string `yaml:"format,omitempty"`
	// Exclude lists path globs skipped by scanners and schema/model extractors.
	Exclude []string `yaml:"exclude,omitempty"`
	// MigrationPaths restricts which .sql files build the schema snapshot.
	MigrationPaths []string `yaml:"migration_paths,omitempty"`
	// Sources enables or disables scanner/model sources by engine name. Missing
	// entries default to enabled.
	Sources map[string]bool `yaml:"sources,omitempty"`
	// Rules holds per-rule enablement, severity, and engine overrides.
	Rules map[string]RuleConfig `yaml:"rules,omitempty"`
	// GoModel configures Go model extraction behavior.
	GoModel GoModelConfig `yaml:"go_model,omitempty"`

	// candidateCache memoizes matchCandidates results keyed by file path,
	// avoiding repeated allocations on the ShouldExclude hot path.
	candidateCache sync.Map `yaml:"-"`
}

// Load reads a config file from the given path. If path is empty,
// it searches DefaultConfigPaths in the current directory.
// If no config file is found, it returns Default().
func Load(configPath string) (*Config, error) {
	if configPath != "" {
		return loadFromFile(configPath)
	}

	for _, name := range DefaultConfigPaths {
		if _, err := os.Stat(name); err == nil {
			return loadFromFile(name)
		}
	}

	return Default(), nil
}

// loadFromFile reads and parses a YAML config file at the given path,
// merging its values into a default Config.
func loadFromFile(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath) //nolint:gosec // config path is user-provided CLI input
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", configPath, err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", configPath, err)
	}

	if cfg.Rules == nil {
		cfg.Rules = make(map[string]RuleConfig)
	}

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validateConfig checks that all configured values are valid.
func validateConfig(cfg *Config) error {
	if cfg.Format != "" &&
		cfg.Format != FormatTerminal &&
		cfg.Format != FormatJSON &&
		cfg.Format != FormatRDJSONL &&
		cfg.Format != FormatSARIF {
		return fmt.Errorf("invalid format %q: must be terminal, json, rdjsonl, or sarif", cfg.Format)
	}

	mode := normalizeGoModelMappingMode(cfg.GoModel.MappingMode)
	if mode == "" {
		mode = GoModelMappingStrict
	}
	switch mode {
	case GoModelMappingStrict, GoModelMappingBalanced, GoModelMappingPermissive:
		cfg.GoModel.MappingMode = mode
	default:
		return fmt.Errorf(
			"invalid go_model.mapping_mode %q: must be strict, balanced, or permissive",
			cfg.GoModel.MappingMode,
		)
	}

	if len(cfg.Sources) > 0 {
		normalizedSources := make(map[string]bool, len(cfg.Sources))
		for source, enabled := range cfg.Sources {
			candidate := normalizeSource(source)
			if candidate == "" {
				return fmt.Errorf("invalid source %q: must be one of %s", source, strings.Join(acceptedEngineNames(), ", "))
			}
			if previous, exists := normalizedSources[candidate]; exists && previous != enabled {
				return fmt.Errorf("conflicting source settings for %q", candidate)
			}
			normalizedSources[candidate] = enabled
		}
		cfg.Sources = normalizedSources
	}

	for ruleID, rc := range cfg.Rules {
		if rc.Severity != "" &&
			rc.Severity != rules.SeverityError &&
			rc.Severity != rules.SeverityWarning &&
			rc.Severity != rules.SeverityInfo {
			return fmt.Errorf("invalid severity %q for rule %s: must be error, warning, info, or empty", rc.Severity, ruleID)
		}

		if len(rc.Engines) > 0 {
			normalized := make([]string, 0, len(rc.Engines))
			seen := make(map[string]struct{}, len(rc.Engines))
			for _, engine := range rc.Engines {
				candidate := normalizeEngine(engine)
				if candidate != ruleEngineAll && !scanner.IsKnownEngineName(candidate) {
					return fmt.Errorf("invalid engine %q for rule %s: must be one of %s", engine, ruleID, strings.Join(acceptedEngineNames(ruleEngineAll), ", "))
				}
				if _, exists := seen[candidate]; exists {
					continue
				}
				seen[candidate] = struct{}{}
				normalized = append(normalized, candidate)
			}
			rc.Engines = normalized
			cfg.Rules[ruleID] = rc
		}
	}

	cfg.MigrationPaths = normalizePathPatterns(cfg.MigrationPaths)
	return nil
}

func normalizeGoModelMappingMode(mode GoModelMappingMode) GoModelMappingMode {
	return GoModelMappingMode(strings.ToLower(strings.TrimSpace(string(mode))))
}

var engineAliases = map[string]string{
	"py":     string(scanner.EngineSQLAlchemy),
	"python": string(scanner.EngineSQLAlchemy),
	"cs":     string(scanner.EngineCSharp),
	"c#":     string(scanner.EngineCSharp),
	"dotnet": string(scanner.EngineCSharp),
}

// normalizeAlias trims, lowercases, and maps user-friendly aliases to built-in
// scanner engine identifiers. Unknown aliases return the normalized input.
func normalizeAlias(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if canonical, ok := engineAliases[value]; ok {
		return canonical
	}
	return value
}

// acceptedEngineNames returns known engines plus aliases for validation errors.
func acceptedEngineNames(extra ...string) []string {
	allowed := make([]string, 0, len(scanner.KnownEngines())+len(engineAliases)+len(extra))
	allowed = append(allowed, extra...)
	for _, builtIn := range scanner.KnownEngines() {
		allowed = append(allowed, string(builtIn))
	}
	for alias := range engineAliases {
		allowed = append(allowed, alias)
	}
	slices.Sort(allowed)
	return slices.Compact(allowed)
}

// normalizeEngine trims whitespace, lowercases, and maps engine aliases to the
// canonical engine name used by rule engine filters.
func normalizeEngine(engine string) string {
	return normalizeAlias(engine)
}

// normalizeSource trims whitespace, lowercases, and maps source aliases to the
// canonical built-in engine identifier used by scanner bindings.
func normalizeSource(source string) string {
	source = normalizeAlias(source)
	if scanner.IsKnownEngineName(source) {
		return source
	}
	return ""
}

// normalizePathPatterns trims, canonicalizes, and de-duplicates path patterns.
func normalizePathPatterns(patterns []string) []string {
	if len(patterns) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(patterns))
	seen := make(map[string]struct{}, len(patterns))
	for _, pattern := range patterns {
		candidate := normalizeMatchPath(pattern)
		if candidate == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		normalized = append(normalized, candidate)
	}
	return normalized
}

// IsRuleEnabled returns whether the given rule ID is enabled.
// If the rule is not mentioned in config, it is enabled by default.
func (c *Config) IsRuleEnabled(ruleID string) bool {
	rc, ok := c.Rules[ruleID]
	if !ok {
		return true
	}
	if rc.Enabled == nil {
		return true
	}
	return *rc.Enabled
}

// RuleSeverity returns the configured severity for a rule, or the
// provided default if not overridden.
func (c *Config) RuleSeverity(ruleID string, defaultSev rules.Severity) rules.Severity {
	rc, ok := c.Rules[ruleID]
	if !ok || rc.Severity == "" {
		return defaultSev
	}
	return rc.Severity
}

// IsRuleEnabledForEngine reports whether a rule should run for a given scanner engine.
// If no engines are configured for the rule, it is enabled for all engines.
func (c *Config) IsRuleEnabledForEngine(ruleID string, engine scanner.Engine) bool {
	rc, ok := c.Rules[ruleID]
	if !ok || len(rc.Engines) == 0 {
		return true
	}
	if engine == scanner.EngineUnknown {
		return true
	}

	needle := string(engine)
	for _, allowed := range rc.Engines {
		if allowed == ruleEngineAll || allowed == needle {
			return true
		}
	}
	return false
}

// IsSourceEnabled reports whether a scanner/model source is enabled. Sources
// are enabled by default; configured false values opt a source out entirely.
func (c *Config) IsSourceEnabled(engine scanner.Engine) bool {
	if c == nil || len(c.Sources) == 0 {
		return true
	}
	enabled, ok := c.Sources[string(engine)]
	if !ok {
		return true
	}
	return enabled
}

// ShouldExclude returns true if the given file path matches any exclude pattern.
// It caches matchCandidates results per path to avoid repeated allocations
// when the same path is checked against multiple patterns or across calls.
func (c *Config) ShouldExclude(filePath string) bool {
	candidates := c.matchCandidates(filePath)
	if len(candidates) == 0 {
		return false
	}

	for _, pattern := range c.Exclude {
		normalizedPattern := normalizeMatchPath(pattern)
		if normalizedPattern == "" {
			continue
		}
		for _, candidate := range candidates {
			if matchGlob(normalizedPattern, candidate) {
				return true
			}
		}
	}
	return false
}

// IsMigrationPath reports whether filePath should contribute to schema
// accumulation as migration SQL. Plain patterns such as "db/migrations" match
// any SQL file under that directory; glob patterns are also supported.
func (c *Config) IsMigrationPath(filePath string) bool {
	if strings.ToLower(path.Ext(normalizeMatchPath(filePath))) != ".sql" {
		return false
	}

	candidates := c.matchCandidates(filePath)
	if len(candidates) == 0 {
		return false
	}

	patterns := c.MigrationPaths
	if len(patterns) == 0 {
		patterns = DefaultMigrationPaths()
	}

	for _, rawPattern := range patterns {
		pattern := normalizeMatchPath(rawPattern)
		if pattern == "" {
			continue
		}
		if hasGlobMeta(pattern) {
			for _, candidate := range candidates {
				if matchGlob(pattern, candidate) {
					return true
				}
			}
			continue
		}

		for _, candidate := range candidates {
			if candidate == pattern || strings.HasPrefix(candidate, pattern+"/") {
				return true
			}
		}
	}

	return false
}

// matchCandidates memoizes normalized path variants used for exclude and
// migration-path matching.
func (c *Config) matchCandidates(filePath string) []string {
	if cached, ok := c.candidateCache.Load(filePath); ok {
		return cached.([]string) //nolint:forcetypeassert // always stored as []string
	}

	candidates := matchCandidates(filePath)
	c.candidateCache.Store(filePath, candidates)
	return candidates
}

// normalizeMatchPath converts filesystem paths and patterns into a stable
// slash-delimited form for matching.
func normalizeMatchPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	p = path.Clean(p)
	return strings.TrimPrefix(p, "./")
}

// matchCandidates returns normalized path variants used for exclude matching:
// full path plus each suffix subpath. This allows relative patterns (e.g.
// "vendor/**") to match absolute scanner paths.
func matchCandidates(filePath string) []string {
	normalized := normalizeMatchPath(filePath)
	if normalized == "" {
		return nil
	}

	seen := make(map[string]struct{})
	var candidates []string
	add := func(value string) {
		if value == "" || value == "." {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, value)
	}

	add(normalized)
	trimmed := strings.TrimPrefix(normalized, "/")
	add(trimmed)

	parts := strings.Split(trimmed, "/")
	for i := range parts {
		add(strings.Join(parts[i:], "/"))
	}

	return candidates
}

// matchGlob checks whether filePath matches the given glob pattern.
// For patterns containing "**", it splits on all "**" occurrences and
// verifies each segment between them appears in the file path in order.
// The first segment must be a prefix, the last segment is matched with
// path.Match against every possible suffix, and middle segments must
// appear in order between them.
// For patterns without "**", it uses path.Match against both the
// full path and the base name.
func matchGlob(pattern, filePath string) bool {
	if strings.Contains(pattern, "**") {
		return matchDoubleStarGlob(pattern, filePath)
	}
	if matched, _ := path.Match(pattern, filePath); matched {
		return true
	}
	if matched, _ := path.Match(pattern, path.Base(filePath)); matched {
		return true
	}
	return false
}

// hasGlobMeta reports whether pattern contains glob metacharacters.
func hasGlobMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

// matchDoubleStarGlob handles glob patterns containing one or more "**"
// segments. It splits the pattern on "**", then verifies each segment
// appears in the file path in order: the first segment must be a prefix,
// and subsequent segments are matched at any position in the remaining path.
func matchDoubleStarGlob(pattern, filePath string) bool {
	segments := strings.Split(pattern, "**")

	// Clean up separators: trim slashes at ** boundaries.
	for i := range segments {
		if i > 0 {
			segments[i] = strings.TrimPrefix(segments[i], "/")
		}
		if i < len(segments)-1 {
			segments[i] = strings.TrimSuffix(segments[i], "/")
		}
	}

	// The first segment (before the first **) must be a prefix.
	first := segments[0]
	remaining := filePath
	if first != "" {
		if !strings.HasPrefix(filePath, first) {
			return false
		}
		remaining = strings.TrimPrefix(filePath, first)
		remaining = strings.TrimPrefix(remaining, "/")
	}

	// Match remaining segments (each separated by ** in the original pattern).
	return matchRemainingSegments(segments[1:], remaining)
}

// matchRemainingSegments tries to match each segment against the remaining
// file path in order. Each segment can match at any position (since it was
// separated by ** which matches zero or more directories). The last segment
// must match at the end of the path.
func matchRemainingSegments(segments []string, filePath string) bool {
	if len(segments) == 0 {
		return true
	}

	seg := segments[0]
	rest := segments[1:]

	// Empty segment (trailing ** or adjacent **/**) matches anything.
	if seg == "" {
		if len(rest) == 0 {
			return true
		}
		// Try every starting position for the next non-empty segment.
		pathParts := strings.Split(filePath, "/")
		for i := range pathParts {
			candidate := strings.Join(pathParts[i:], "/")
			if matchRemainingSegments(rest, candidate) {
				return true
			}
		}
		return false
	}

	// Non-empty segment: determine how many path components the segment spans.
	segParts := strings.Split(seg, "/")
	segDepth := len(segParts)
	pathParts := strings.Split(filePath, "/")

	for i := 0; i <= len(pathParts)-segDepth; i++ {
		candidate := strings.Join(pathParts[i:i+segDepth], "/")
		matched, _ := path.Match(seg, candidate)
		if matched {
			after := ""
			if i+segDepth < len(pathParts) {
				after = strings.Join(pathParts[i+segDepth:], "/")
			}
			if matchRemainingSegments(rest, after) {
				return true
			}
		}
	}
	return false
}
