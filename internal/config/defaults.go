package config

// DefaultConfigPaths lists the file names searched for configuration,
// in order of precedence (first match wins).
var DefaultConfigPaths = []string{
	".valk-guard.yaml",
	".valk-guard.yml",
}

// Output format name constants used for validation and reporter selection.
const (
	FormatTerminal = "terminal"
	FormatJSON     = "json"
	FormatSARIF    = "sarif"
)

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Format:  FormatTerminal,
		Exclude: nil,
		Rules:   make(map[string]RuleConfig),
	}
}
