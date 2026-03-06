// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"log/slog"

	"github.com/valkdb/valk-guard/internal/config"
	"github.com/valkdb/valk-guard/internal/rules"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

// runSchemaDrift performs schema-aware analysis using a migration snapshot:
// query-schema checks against parsed SQL statements plus model schema-drift
// checks against extracted ORM models.
func runSchemaDrift(
	ctx context.Context,
	sqlStmts []scanner.SQLStatement,
	parsedStmts []parsedStatement,
	inputs scannerInputs,
	modelBindings []modelBinding,
	cfg *config.Config,
	reg *rules.Registry,
	logger *slog.Logger,
) []rules.Finding {
	schemaRules := enabledSchemaRules(reg, cfg, modelBindings)
	querySchemaRules := enabledQuerySchemaRules(reg, cfg)
	if len(schemaRules) == 0 && len(querySchemaRules) == 0 {
		return nil
	}

	migrationSnap := schema.BuildFromStatements(sqlStmts, logger)
	models := extractModels(ctx, inputs, modelBindings, logger)

	modelSnaps := make(map[schema.ModelSource]*schema.Snapshot)
	seenSources := make(map[schema.ModelSource]struct{})
	for _, binding := range modelBindings {
		if _, exists := seenSources[binding.source]; exists {
			continue
		}
		seenSources[binding.source] = struct{}{}
		modelSnaps[binding.source] = schema.BuildModelSnapshotForSource(models, binding.source)
	}
	querySourceMap := sourceQueryEngines(modelBindings)

	var findings []rules.Finding
	queryFindings := runQuerySchemaChecks(ctx, querySchemaRules, parsedStmts, migrationSnap, modelSnaps, querySourceMap, cfg, logger)
	findings = append(findings, queryFindings...)

	if len(schemaRules) == 0 {
		return findings
	}
	if len(migrationSnap.Tables) == 0 {
		logger.Debug("schema-drift skipped: no tables found in schema SQL files")
		return findings
	}
	if len(models) == 0 {
		logger.Debug("schema-drift skipped: no ORM models found")
		return findings
	}

	logger.Debug("schema-drift analysis", "tables", len(migrationSnap.Tables), "models", len(models))

	for _, rule := range schemaRules {
		ruleModels := modelsForRule(models, cfg, rule.ID(), modelBindings)
		if len(ruleModels) == 0 {
			continue
		}
		ruleFindings := rule.CheckSchema(ctx, migrationSnap, ruleModels)
		for i := range ruleFindings {
			ruleFindings[i].Severity = cfg.RuleSeverity(rule.ID(), ruleFindings[i].Severity)
		}
		findings = append(findings, ruleFindings...)
	}

	return findings
}

// extractModels extracts ORM model definitions from Go and Python source
// files, logging warnings on extraction failures.
func extractModels(
	ctx context.Context,
	inputs scannerInputs,
	modelBindings []modelBinding,
	logger *slog.Logger,
) []schema.ModelDef {
	var models []schema.ModelDef

	for _, binding := range modelBindings {
		files := inputs.filesForExtensions(binding.extensions)
		if len(files) == 0 {
			continue
		}
		extractedModels, err := binding.extractor.ExtractModels(ctx, files)
		if err != nil {
			logger.Warn("model extraction failed", "source", string(binding.source), "error", err)
			continue
		}
		models = append(models, extractedModels...)
	}

	return models
}

// runQuerySchemaChecks applies enabled query-schema rules to parsed statements,
// checking against multiple schema snapshots and deduplicating findings.
func runQuerySchemaChecks(
	ctx context.Context,
	querySchemaRules []rules.QuerySchemaRule,
	parsedStmts []parsedStatement,
	migrationSnap *schema.Snapshot,
	modelSnaps map[schema.ModelSource]*schema.Snapshot,
	querySourceMap map[scanner.Engine][]schema.ModelSource,
	cfg *config.Config,
	logger *slog.Logger,
) []rules.Finding {
	if len(querySchemaRules) == 0 {
		return nil
	}

	modelTablesTotal := 0
	for _, snap := range modelSnaps {
		modelTablesTotal += len(snap.Tables)
	}
	logger.Debug(
		"query-schema analysis",
		"migration_tables", len(migrationSnap.Tables),
		"model_sources", len(modelSnaps),
		"model_tables_total", modelTablesTotal,
		"statements", len(parsedStmts),
		"rules", len(querySchemaRules),
	)

	var findings []rules.Finding

	for _, ps := range parsedStmts {
		if ps.parsed == nil {
			continue
		}
		snaps := querySnapshotsForEngine(ps.stmt.Engine, migrationSnap, modelSnaps, querySourceMap)
		if len(snaps) == 0 {
			continue
		}

		for _, rule := range querySchemaRules {
			if !cfg.IsRuleEnabledForEngine(rule.ID(), ps.stmt.Engine) {
				continue
			}
			if scanner.IsDisabled(rule.ID(), ps.stmt.Disabled) {
				continue
			}

			seenByKey := make(map[queryFindingKeyValue]struct{})
			for _, snap := range snaps {
				ruleFindings := rule.CheckQuerySchema(ctx, snap, &ps.stmt, ps.parsed)
				applyStatementRange(ruleFindings, &ps.stmt)
				for i := range ruleFindings {
					ruleFindings[i].Severity = cfg.RuleSeverity(rule.ID(), ruleFindings[i].Severity)
					key := queryFindingKey(&ruleFindings[i])
					if _, ok := seenByKey[key]; ok {
						continue
					}
					seenByKey[key] = struct{}{}
					findings = append(findings, ruleFindings[i])
				}
			}
		}
	}

	return findings
}

// querySnapshotsForEngine returns schema snapshots to check for a statement.
// Migrations always participate when available; model snapshots are added for
// source engines mapped to the statement engine.
func querySnapshotsForEngine(
	engine scanner.Engine,
	migrationSnap *schema.Snapshot,
	modelSnaps map[schema.ModelSource]*schema.Snapshot,
	querySourceMap map[scanner.Engine][]schema.ModelSource,
) []*schema.Snapshot {
	snaps := make([]*schema.Snapshot, 0, 1+len(querySourceMap[engine]))
	if migrationSnap != nil && len(migrationSnap.Tables) > 0 {
		snaps = append(snaps, migrationSnap)
	}

	for _, source := range querySourceMap[engine] {
		snap, ok := modelSnaps[source]
		if !ok || snap == nil || len(snap.Tables) == 0 {
			continue
		}
		snaps = append(snaps, snap)
	}

	return snaps
}

// enabledSchemaRules returns schema rules that are enabled and allowed for at
// least one model source engine.
func enabledSchemaRules(reg *rules.Registry, cfg *config.Config, modelBindings []modelBinding) []rules.SchemaRule {
	sourceEngines := sourceConfigEngines(modelBindings)
	all := reg.AllSchema()
	result := make([]rules.SchemaRule, 0, len(all))
	for _, rule := range all {
		if !cfg.IsRuleEnabled(rule.ID()) {
			continue
		}

		if len(sourceEngines) == 0 {
			result = append(result, rule)
			continue
		}

		allowed := false
		for _, engines := range sourceEngines {
			for _, engine := range engines {
				if cfg.IsRuleEnabledForEngine(rule.ID(), engine) {
					allowed = true
					break
				}
			}
			if allowed {
				break
			}
		}
		if !allowed {
			continue
		}
		result = append(result, rule)
	}
	return result
}

// enabledQuerySchemaRules returns query-schema rules that are enabled.
func enabledQuerySchemaRules(reg *rules.Registry, cfg *config.Config) []rules.QuerySchemaRule {
	all := reg.AllQuerySchema()
	result := make([]rules.QuerySchemaRule, 0, len(all))
	for _, rule := range all {
		if !cfg.IsRuleEnabled(rule.ID()) {
			continue
		}
		result = append(result, rule)
	}
	return result
}

// modelsForRule filters extracted models according to the rule's configured
// engine constraints.
func modelsForRule(models []schema.ModelDef, cfg *config.Config, ruleID string, modelBindings []modelBinding) []schema.ModelDef {
	sourceEngines := sourceConfigEngines(modelBindings)
	filtered := make([]schema.ModelDef, 0, len(models))
	for _, model := range models {
		engines, ok := sourceEngines[model.Source]
		if !ok || len(engines) == 0 {
			// Ignore model sources without an explicit engine binding so newly
			// added sources do not silently opt into every schema rule by default.
			continue
		}

		allowed := false
		for _, engine := range engines {
			if cfg.IsRuleEnabledForEngine(ruleID, engine) {
				allowed = true
				break
			}
		}
		if allowed {
			filtered = append(filtered, model)
		}
	}
	return filtered
}
