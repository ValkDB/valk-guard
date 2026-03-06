// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package config

// DefaultConfigPaths lists the file names searched for configuration,
// in order of precedence (first match wins).
var DefaultConfigPaths = []string{
	".valk-guard.yaml",
	".valk-guard.yml",
}

// DefaultMigrationPaths lists directory-like path patterns used to identify
// migration SQL files when migration_paths is not configured explicitly.
var DefaultMigrationPaths = []string{
	"migrations",
	"migration",
	"migrate",
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
