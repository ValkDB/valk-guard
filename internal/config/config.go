package config

import (
	"fmt"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/valkdb/valk-guard/internal/rules"
	"github.com/valkdb/valk-guard/internal/scanner"
)

const ruleEngineAll = "all"

// RuleConfig holds per-rule overrides from the config file.
type RuleConfig struct {
	Enabled  *bool          `yaml:"enabled,omitempty"`
	Severity rules.Severity `yaml:"severity,omitempty"`
	Engines  []string       `yaml:"engines,omitempty"`
}

// Config represents the top-level valk-guard configuration.
type Config struct {
	Format  string                `yaml:"format,omitempty"`
	Exclude []string              `yaml:"exclude,omitempty"`
	Rules   map[string]RuleConfig `yaml:"rules,omitempty"`
}

// Load reads a config file from the given path. If path is empty,
// it searches DefaultConfigPaths in the current directory.
// If no config file is found, it returns Default().
func Load(path string) (*Config, error) {
	if path != "" {
		return loadFromFile(path)
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
func loadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // config path is user-provided CLI input
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
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
	if cfg.Format != "" && cfg.Format != FormatTerminal && cfg.Format != FormatJSON && cfg.Format != FormatSARIF {
		return fmt.Errorf("invalid format %q: must be terminal, json, or sarif", cfg.Format)
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
				switch candidate {
				case ruleEngineAll, string(scanner.EngineSQL), string(scanner.EngineGo), string(scanner.EngineGoqu), string(scanner.EngineSQLAlchemy):
					if _, exists := seen[candidate]; exists {
						continue
					}
					seen[candidate] = struct{}{}
					normalized = append(normalized, candidate)
				default:
					return fmt.Errorf("invalid engine %q for rule %s: must be one of all, sql, go, goqu, sqlalchemy", engine, ruleID)
				}
			}
			rc.Engines = normalized
			cfg.Rules[ruleID] = rc
		}
	}
	return nil
}

// normalizeEngine trims whitespace and lowercases an engine string to produce
// a canonical form suitable for comparison against known engine identifiers.
func normalizeEngine(engine string) string {
	return strings.ToLower(strings.TrimSpace(engine))
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

// ShouldExclude returns true if the given file path matches any exclude pattern.
func (c *Config) ShouldExclude(filePath string) bool {
	candidates := matchCandidates(filePath)
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
// For patterns containing "**", it splits on "**" and handles both
// prefix patterns (e.g., "vendor/**") and suffix patterns (e.g.,
// "**/migrations/*.sql") by trying to match against every possible
// subpath of the file path.
// For patterns without "**", it uses path.Match against both the
// full path and the base name.
func matchGlob(pattern, filePath string) bool {
	if strings.Contains(pattern, "**") {
		parts := strings.SplitN(pattern, "**", 2)
		prefix := parts[0]
		suffix := parts[1]

		// Check prefix matches
		if prefix != "" && !strings.HasPrefix(filePath, prefix) {
			return false
		}

		// If no suffix (e.g., "vendor/**"), prefix match is enough
		if suffix == "" || suffix == "/" {
			return prefix == "" || strings.HasPrefix(filePath, prefix)
		}

		// Strip leading / from suffix
		suffix = strings.TrimPrefix(suffix, "/")

		// Try matching suffix against every possible subpath.
		remaining := filePath
		if prefix != "" {
			remaining = strings.TrimPrefix(filePath, prefix)
		}
		remaining = strings.TrimPrefix(remaining, "/")
		// Try matching suffix against the remaining path and all its subdirectories.
		parts2 := strings.Split(remaining, "/")
		for i := range parts2 {
			candidate := strings.Join(parts2[i:], "/")
			if matched, _ := path.Match(suffix, candidate); matched {
				return true
			}
		}
		return false
	}
	if matched, _ := path.Match(pattern, filePath); matched {
		return true
	}
	if matched, _ := path.Match(pattern, path.Base(filePath)); matched {
		return true
	}
	return false
}
