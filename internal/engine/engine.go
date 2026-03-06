// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/valkdb/postgresparser"

	"github.com/valkdb/valk-guard/internal/config"
	"github.com/valkdb/valk-guard/internal/rules"
	"github.com/valkdb/valk-guard/internal/scanner"
)

// Result captures the engine outputs needed by the CLI layer.
type Result struct {
	// Findings contains all deduplicated findings emitted by the run.
	Findings []rules.Finding
	// HadScannableInputs reports whether any supported source files were found.
	HadScannableInputs bool
}

// namedScanner associates a human-readable scanner name with its implementation
// and the set of input files it should process.
type namedScanner struct {
	name string
	impl scanner.Scanner
	in   []string
}

// plannedRules organizes enabled rules into two buckets: those that apply to
// every SQL command type (any) and those indexed by a specific command type
// (byCommand), enabling fast dispatch during analysis.
type plannedRules struct {
	any       []rules.Rule
	byCommand map[postgresparser.QueryCommand][]rules.Rule
}

// parsedStatement pairs a scanned SQL statement with its parsed form so
// schema-aware query rules can run in a later phase that also has schema state.
type parsedStatement struct {
	stmt   scanner.SQLStatement
	parsed *postgresparser.ParsedQuery
}

// statementResults aggregates the outputs produced by processing every
// statement received from the scanner fan-out channel.
type statementResults struct {
	findings       []rules.Finding
	sqlStmts       []scanner.SQLStatement
	migrationStmts []scanner.SQLStatement
	parsedStmts    []parsedStatement
	stmtCount      int
}

// scanErrorCollector joins scanner failures, cancels the scan on the first
// hard error, and logs later errors so they are not silently dropped.
type scanErrorCollector struct {
	cancel context.CancelFunc
	logger *slog.Logger

	mu   sync.Mutex
	errs []error
}

// Add records a scanner error, canceling the scan on the first one and
// logging later non-context errors for diagnostics.
func (c *scanErrorCollector) Add(err error) {
	if err == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.errs) > 0 && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return
	}

	if len(c.errs) == 0 {
		c.errs = append(c.errs, err)
		c.cancel()
		return
	}

	c.errs = append(c.errs, err)
	c.logger.Warn("additional scanner error", "error", err)
}

// Err returns all collected scanner errors as a single joined error.
func (c *scanErrorCollector) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.errs) == 0 {
		return nil
	}
	return errors.Join(c.errs...)
}

// Run discovers scannable inputs, runs all scanners and enabled rules, and
// returns the aggregated findings plus input metadata for the caller.
func Run(ctx context.Context, paths []string, cfg *config.Config, logger *slog.Logger) (Result, error) {
	reg := rules.DefaultRegistry()
	warnUnknownConfiguredRules(cfg, reg, logger)

	findings, hadScannableInputs, err := collectAndAnalyze(ctx, paths, cfg, reg, logger)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Findings:           findings,
		HadScannableInputs: hadScannableInputs,
	}, nil
}

// collectAndAnalyze walks the given paths to discover source files, fans out
// scanning across all registered scanners concurrently, parses each SQL
// statement, applies enabled rules, and returns the deduplicated, sorted
// findings.
func collectAndAnalyze(ctx context.Context, paths []string, cfg *config.Config, reg *rules.Registry, logger *slog.Logger) ([]rules.Finding, bool, error) {
	scannerBindings := defaultScannerBindings()
	modelBindings := defaultModelBindings(cfg)

	inputs, err := collectScannerInputs(ctx, paths, cfg, requiredExtensions(scannerBindings, modelBindings))
	if err != nil {
		return nil, false, err
	}

	active := activeScanners(inputs, scannerBindings, logger)
	if len(active) == 0 {
		return nil, false, nil
	}

	rulePlan := buildRulePlan(cfg, reg)

	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCollector := &scanErrorCollector{cancel: cancel, logger: logger}

	results := fanOutScanners(scanCtx, active, logger, errCollector.Add)
	checkErr := errCollector.Err

	sr := processStatements(scanCtx, results, cfg, rulePlan, logger, errCollector.Add, checkErr)

	if err := checkErr(); err != nil {
		return nil, true, err
	}

	// Sort statements by (File, Line) so schema accumulation stays deterministic
	// regardless of goroutine scheduling in fanOutScanners.
	sortStatements(sr.sqlStmts)
	sortStatements(sr.migrationStmts)

	// Schema-aware phase: build migration snapshot once, then run query-schema
	// rules and model schema-drift rules. Prefer files under migration-like
	// paths when present; otherwise fall back to all SQL files to preserve
	// behavior for projects that keep migrations elsewhere.
	schemaSQLStatements := sr.sqlStmts
	if len(sr.migrationStmts) > 0 {
		schemaSQLStatements = sr.migrationStmts
	}
	schemaFindings := runSchemaDrift(ctx, schemaSQLStatements, sr.parsedStmts, inputs, modelBindings, cfg, reg, logger)
	sr.findings = append(sr.findings, schemaFindings...)

	sortFindings(sr.findings)

	logger.Debug("analysis complete", "statements", sr.stmtCount, "findings", len(sr.findings))
	return sr.findings, true, nil
}

// processStatements drains the results channel from the scanner fan-out,
// parsing each SQL statement, applying enabled rules, and categorizing
// statements into migration vs regular buckets for the schema-aware phase.
func processStatements(
	ctx context.Context,
	results <-chan scanner.SQLStatement,
	cfg *config.Config,
	rulePlan plannedRules,
	logger *slog.Logger,
	recordErr func(error),
	checkErr func() error,
) statementResults {
	var sr statementResults

	for stmt := range results {
		if checkErr() != nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			recordErr(err)
			continue
		}

		if cfg.ShouldExclude(stmt.File) {
			logger.Debug("statement excluded by config", "file", stmt.File)
			continue
		}

		// Collect raw SQL statements for schema snapshot accumulation.
		if stmt.Engine == scanner.EngineSQL {
			sr.sqlStmts = append(sr.sqlStmts, stmt)
			if cfg.IsMigrationPath(stmt.File) {
				sr.migrationStmts = append(sr.migrationStmts, stmt)
			}
		}

		sr.stmtCount++
		parsed, err := scanner.ParseStatement(stmt.SQL)
		if err != nil {
			logger.Warn(
				"skipping unparseable SQL statement",
				"file", stmt.File,
				"line", stmt.Line,
				"error", err,
				"hint", "valk-guard uses a PostgreSQL parser; add path to exclude in .valk-guard.yaml if intentional",
			)
			continue
		}
		if parsed == nil {
			continue
		}
		sr.parsedStmts = append(sr.parsedStmts, parsedStatement{
			stmt:   stmt,
			parsed: parsed,
		})

		applyRule := func(rule rules.Rule) {
			if !cfg.IsRuleEnabledForEngine(rule.ID(), stmt.Engine) {
				return
			}
			if scanner.IsDisabled(rule.ID(), stmt.Disabled) {
				return
			}
			ruleFindings := rule.Check(ctx, parsed, stmt.File, stmt.Line, stmt.SQL)
			applyStatementRange(ruleFindings, &stmt)
			for i := range ruleFindings {
				ruleFindings[i].Severity = cfg.RuleSeverity(rule.ID(), ruleFindings[i].Severity)
			}
			sr.findings = append(sr.findings, ruleFindings...)
		}

		for _, rule := range rulePlan.byCommand[parsed.Command] {
			applyRule(rule)
		}
		for _, rule := range rulePlan.any {
			applyRule(rule)
		}
	}

	return sr
}

// activeScanners builds the full list of named scanners from the collected
// inputs, filters out any scanner with no candidate files, and logs the
// outcome for each scanner.
func activeScanners(inputs scannerInputs, bindings []scannerBinding, logger *slog.Logger) []namedScanner {
	active := make([]namedScanner, 0, len(bindings))
	for _, binding := range bindings {
		files := inputs.filesForExtensions(binding.extensions)
		if len(files) == 0 {
			logger.Debug("scanner skipped", "scanner", binding.name, "files", 0)
			continue
		}
		logger.Debug("scanner queued", "scanner", binding.name, "files", len(files))
		active = append(active, namedScanner{
			name: binding.name,
			impl: binding.impl,
			in:   files,
		})
	}
	return active
}

// fanOutScanners launches each scanner in its own goroutine, writing
// discovered statements into a returned channel. The channel is closed once
// all goroutines finish. Scanner failures are recorded through recordErr and
// cancel the scan context.
func fanOutScanners(
	scanCtx context.Context,
	active []namedScanner,
	logger *slog.Logger,
	recordErr func(error),
) <-chan scanner.SQLStatement {
	results := make(chan scanner.SQLStatement, 256)
	var wg sync.WaitGroup

	for _, sc := range active {
		wg.Add(1)
		go func(sc namedScanner) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					recordErr(fmt.Errorf("scanner %s panicked: %v", sc.name, r))
				}
			}()

			emitted := 0
			for stmt, err := range sc.impl.Scan(scanCtx, sc.in) {
				if err != nil {
					recordErr(fmt.Errorf("scanner %s: %w", sc.name, err))
					return
				}

				emitted++
				select {
				case results <- stmt:
				case <-scanCtx.Done():
					return
				}
			}

			logger.Debug("scanner completed", "scanner", sc.name, "statements", emitted)
		}(sc)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

// sortStatements sorts scanned statements deterministically by file and line.
func sortStatements(stmts []scanner.SQLStatement) {
	slices.SortFunc(stmts, func(a, b scanner.SQLStatement) int {
		if a.File != b.File {
			if a.File < b.File {
				return -1
			}
			return 1
		}
		return a.Line - b.Line
	})
}

// collectScannerInputs recursively walks the provided paths and classifies
// files by extension while respecting config exclusions and context
// cancellation.
func collectScannerInputs(ctx context.Context, paths []string, cfg *config.Config, extensions []string) (scannerInputs, error) {
	inputs := newScannerInputs()
	seenByExt := make(map[string]map[string]struct{})
	allowedExt := make(map[string]struct{}, len(extensions))
	for _, rawExt := range extensions {
		ext := normalizeExt(rawExt)
		if ext == "" {
			continue
		}
		allowedExt[ext] = struct{}{}
	}

	for _, root := range paths {
		if err := ctx.Err(); err != nil {
			return scannerInputs{}, err
		}

		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "node_modules" || name == ".hg" || name == ".svn" {
					return filepath.SkipDir
				}
				return nil
			}
			if cfg.ShouldExclude(path) {
				return nil
			}

			ext := normalizeExt(filepath.Ext(path))
			if _, ok := allowedExt[ext]; !ok {
				return nil
			}

			addInputFile(inputs, seenByExt, ext, path)
			return nil
		})
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return scannerInputs{}, fmt.Errorf("scan path %q does not exist; check the path argument", root)
			}
			return scannerInputs{}, fmt.Errorf("walking %s: %w", root, err)
		}
	}

	for ext := range inputs.byExt {
		slices.Sort(inputs.byExt[ext])
	}
	return inputs, nil
}

// buildRulePlan partitions enabled rules by SQL command for efficient
// per-statement dispatch.
func buildRulePlan(cfg *config.Config, reg *rules.Registry) plannedRules {
	plan := plannedRules{
		byCommand: make(map[postgresparser.QueryCommand][]rules.Rule),
	}

	for _, rule := range reg.All() {
		if !cfg.IsRuleEnabled(rule.ID()) {
			continue
		}

		var targets []postgresparser.QueryCommand
		if targetedRule, ok := rule.(rules.CommandTargetedRule); ok {
			targets = targetedRule.CommandTargets()
		}
		if len(targets) == 0 {
			plan.any = append(plan.any, rule)
			continue
		}
		for _, command := range targets {
			plan.byCommand[command] = append(plan.byCommand[command], rule)
		}
	}

	return plan
}

// warnUnknownConfiguredRules logs config rule IDs that do not exist in the
// active registry so typos are visible to users.
func warnUnknownConfiguredRules(cfg *config.Config, reg *rules.Registry, logger *slog.Logger) {
	if cfg == nil || reg == nil || logger == nil || len(cfg.Rules) == 0 {
		return
	}

	knownSet := make(map[string]struct{})
	knownList := make([]string, 0, len(reg.All())+len(reg.AllSchema())+len(reg.AllQuerySchema()))
	for _, rule := range reg.All() {
		id := rule.ID()
		knownSet[id] = struct{}{}
		knownList = append(knownList, id)
	}
	for _, rule := range reg.AllSchema() {
		id := rule.ID()
		knownSet[id] = struct{}{}
		knownList = append(knownList, id)
	}
	for _, rule := range reg.AllQuerySchema() {
		id := rule.ID()
		knownSet[id] = struct{}{}
		knownList = append(knownList, id)
	}
	slices.Sort(knownList)
	available := strings.Join(knownList, ", ")

	unknown := make([]string, 0, len(cfg.Rules))
	for id := range cfg.Rules {
		if _, ok := knownSet[id]; ok {
			continue
		}
		unknown = append(unknown, id)
	}
	slices.Sort(unknown)

	for _, id := range unknown {
		logger.Warn("unknown rule in config", "rule", id, "available_rules", available)
	}
}
