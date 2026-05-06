// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package scanner

import "strings"

// knownEngines defines all built-in scanner engines accepted in config.
var knownEngines = [...]Engine{
	EngineSQL,
	EngineGo,
	EngineGoqu,
	EngineSQLAlchemy,
	EngineCSharp,
}

// KnownEngines returns a copy of all registered built-in engine identifiers.
func KnownEngines() []Engine {
	out := make([]Engine, len(knownEngines))
	copy(out, knownEngines[:])
	return out
}

// IsKnownEngineName reports whether name matches a built-in engine identifier.
func IsKnownEngineName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, engine := range knownEngines {
		if name == string(engine) {
			return true
		}
	}
	return false
}
