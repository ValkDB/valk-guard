package config

// DefaultConfigPaths lists the file names searched for configuration,
// in order of precedence (first match wins).
var DefaultConfigPaths = []string{
	".valk-guard.yaml",
	".valk-guard.yml",
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Format:  "terminal",
		Exclude: nil,
		Rules:   make(map[string]RuleConfig),
	}
}
