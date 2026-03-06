// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package config

// DefaultConfigPaths lists the file names searched for configuration,
// in order of precedence (first match wins).
var DefaultConfigPaths = []string{
	".valk-guard.yaml",
	".valk-guard.yml",
}

// defaultMigrationPaths is the backing data for DefaultMigrationPaths.
var defaultMigrationPaths = []string{
	"migrations",
	"migration",
	"migrate",
}

// DefaultMigrationPaths returns the directory-like path patterns used to
// identify migration SQL files when migration_paths is not configured
// explicitly. Each call returns a fresh copy.
func DefaultMigrationPaths() []string {
	out := make([]string, len(defaultMigrationPaths))
	copy(out, defaultMigrationPaths)
	return out
}

// Output format name constants used for validation and reporter selection.
const (
	FormatTerminal = "terminal"
	FormatJSON     = "json"
	FormatRDJSONL  = "rdjsonl"
	FormatSARIF    = "sarif"
)

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Format:         FormatTerminal,
		Exclude:        nil,
		MigrationPaths: nil,
		Rules:          make(map[string]RuleConfig),
		GoModel: GoModelConfig{
			MappingMode: GoModelMappingStrict,
		},
	}
}
