// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"

	"github.com/valkdb/valk-guard/internal/rules"
)

// sarifVersion is the SARIF specification version emitted by this reporter.
const sarifVersion = "2.1.0"

// sarifSchema is the JSON schema URL for SARIF 2.1.0 validation.
const sarifSchema = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json"

// sarifLog is the top-level SARIF 2.1.0 document structure.
type sarifLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

// sarifRun represents a single analysis run within a SARIF log.
type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

// sarifTool describes the analysis tool that produced the results.
type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

// sarifDriver describes the primary component of the analysis tool.
type sarifDriver struct {
	Name           string                `json:"name"`
	Version        string                `json:"version"`
	InformationURI string                `json:"informationUri"`
	Rules          []sarifRuleDescriptor `json:"rules"`
}

// sarifRulesHelpURI is the generic documentation URL for all built-in rules.
const sarifRulesHelpURI = "https://github.com/ValkDB/valk-guard#built-in-rules"

// sarifRuleDescriptor defines the metadata for a single analysis rule.
type sarifRuleDescriptor struct {
	ID               string          `json:"id"`
	ShortDescription sarifMessage    `json:"shortDescription"`
	FullDescription  sarifMessage    `json:"fullDescription"`
	HelpURI          string          `json:"helpUri"`
	DefaultConfig    sarifRuleConfig `json:"defaultConfiguration"`
}

// sarifRuleConfig holds the default configuration for a rule, such as its severity level.
type sarifRuleConfig struct {
	Level string `json:"level"`
}

// sarifMessage wraps a plain-text message string in the SARIF format.
type sarifMessage struct {
	Text string `json:"text"`
}

// sarifResult represents a single finding produced by the analysis tool.
type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

// sarifLocation wraps the physical location of a finding.
type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

// sarifPhysicalLocation identifies the artifact and region where a finding occurs.
type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

// sarifArtifactLocation identifies the file (artifact) containing the finding.
type sarifArtifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId,omitempty"`
}

// sarifRegion specifies the line and column range of a finding within a file.
type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// SARIFReporter outputs findings in SARIF 2.1.0 format.
// Version is reported in the tool driver; if empty, defaults to "dev".
type SARIFReporter struct {
	Version string
}

type sarifRuleMetadata struct {
	Description     string
	DefaultSeverity rules.Severity
}

type sarifRuleInfo interface {
	ID() string
	Description() string
	DefaultSeverity() rules.Severity
}

var (
	sarifRuleMetadataOnce sync.Once
	sarifRuleMetadataByID map[string]sarifRuleMetadata
)

// Report writes findings as SARIF JSON.
func (r *SARIFReporter) Report(ctx context.Context, w io.Writer, findings []rules.Finding) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	ver := r.Version
	if ver == "" {
		ver = "dev"
	}

	log := sarifLog{
		Version: sarifVersion,
		Schema:  sarifSchema,
		Runs: []sarifRun{
			{
				Tool: sarifTool{
					Driver: sarifDriver{
						Name:           "valk-guard",
						Version:        ver,
						InformationURI: "https://github.com/valkdb/valk-guard",
						Rules:          buildSARIFRuleDescriptors(findings),
					},
				},
				Results: buildSARIFResults(findings),
			},
		},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}

// buildSARIFRuleDescriptors collects unique rule descriptors from findings,
// preserving first-seen order.
func buildSARIFRuleDescriptors(findings []rules.Finding) []sarifRuleDescriptor {
	ruleSet := make(map[string]struct{})
	ruleDescriptors := make([]sarifRuleDescriptor, 0, len(findings))
	ruleMetadata := loadSARIFRuleMetadata()

	for _, f := range findings {
		if _, ok := ruleSet[f.RuleID]; ok {
			continue
		}
		ruleSet[f.RuleID] = struct{}{}

		description := f.Message
		defaultLevel := severityToSARIF(f.Severity)
		if meta, exists := ruleMetadata[f.RuleID]; exists {
			if strings.TrimSpace(meta.Description) != "" {
				description = meta.Description
			}
			defaultLevel = severityToSARIF(meta.DefaultSeverity)
		}

		ruleDescriptors = append(ruleDescriptors, sarifRuleDescriptor{
			ID:               f.RuleID,
			ShortDescription: sarifMessage{Text: description},
			FullDescription:  sarifMessage{Text: description},
			HelpURI:          sarifRulesHelpURI,
			DefaultConfig:    sarifRuleConfig{Level: defaultLevel},
		})
	}

	if ruleDescriptors == nil {
		ruleDescriptors = []sarifRuleDescriptor{}
	}

	return ruleDescriptors
}

func loadSARIFRuleMetadata() map[string]sarifRuleMetadata {
	sarifRuleMetadataOnce.Do(func() {
		reg := rules.DefaultRegistry()
		metadata := make(map[string]sarifRuleMetadata)

		for _, rule := range reg.All() {
			registerSARIFRuleMetadata(metadata, rule)
		}
		for _, rule := range reg.AllSchema() {
			registerSARIFRuleMetadata(metadata, rule)
		}
		for _, rule := range reg.AllQuerySchema() {
			registerSARIFRuleMetadata(metadata, rule)
		}

		sarifRuleMetadataByID = metadata
	})

	return sarifRuleMetadataByID
}

func registerSARIFRuleMetadata(metadata map[string]sarifRuleMetadata, rule sarifRuleInfo) {
	metadata[rule.ID()] = sarifRuleMetadata{
		Description:     rule.Description(),
		DefaultSeverity: rule.DefaultSeverity(),
	}
}

// buildSARIFResults converts findings into SARIF result objects.
func buildSARIFResults(findings []rules.Finding) []sarifResult {
	results := make([]sarifResult, 0, len(findings))
	for _, f := range findings {
		results = append(results, sarifResult{
			RuleID:  f.RuleID,
			Level:   severityToSARIF(f.Severity),
			Message: sarifMessage{Text: f.Message},
			Locations: []sarifLocation{
				{
					PhysicalLocation: sarifPhysicalLocation{
						ArtifactLocation: sarifArtifactLocation{URI: f.File, URIBaseID: "%SRCROOT%"},
						Region: sarifRegion{
							StartLine:   f.Line,
							StartColumn: f.Column,
							EndLine:     f.EndLine,
							EndColumn:   f.EndColumn,
						},
					},
				},
			},
		})
	}
	return results
}

// severityToSARIF converts a valk-guard Severity to the corresponding
// SARIF level string ("error", "warning", "note", or "none").
func severityToSARIF(sev rules.Severity) string {
	switch sev {
	case rules.SeverityError:
		return "error"
	case rules.SeverityWarning:
		return "warning"
	case rules.SeverityInfo:
		return "note"
	default:
		return "none"
	}
}
