// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/valkdb/postgresparser"

	"github.com/valkdb/valk-guard/internal/config"
	"github.com/valkdb/valk-guard/internal/output"
	"github.com/valkdb/valk-guard/internal/rules"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

const (
	exitSuccess  = 0
	exitFindings = 1
	exitError    = 2
)

var version = "dev"

// codedError pairs an exit code with an underlying error so that the top-level
// run function can propagate both to the OS and to the user.
type codedError struct {
	code int
	err  error
}

// Error implements the error interface, returning the underlying error message.
func (e *codedError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

// Unwrap returns the underlying error for errors.Is/errors.As support.
func (e *codedError) Unwrap() error {
	return e.err
}

// main is the entry point for the valk-guard binary.
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run builds and executes the root cobra command, returning the appropriate
// OS exit code based on the outcome.
func run(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd(stdout, stderr)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		var coded *codedError
		if errors.As(err, &coded) {
			if coded.err != nil {
				_, _ = fmt.Fprintln(stderr, coded.err.Error())
			}
			return coded.code
		}
		_, _ = fmt.Fprintln(stderr, err.Error())
		return exitError
	}
	return exitSuccess
}

// newRootCmd constructs the top-level cobra command and registers all
// subcommands.
func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "valk-guard",
		Short:         "Static SQL performance linter for CI/CD",
		Long:          "Valk Guard scans SQL across SQL, Go, Goqu, and SQLAlchemy sources and reports performance, safety, and schema-aware findings.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       version,
	}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.AddCommand(newScanCmd(stdout, stderr))
	return cmd
}

// scanOptions holds the CLI flag values parsed for the scan subcommand.
type scanOptions struct {
	configPath string
	format     string
	outputPath string
	logLevel   string
}

// newScanCmd constructs the "scan" subcommand with its flags and wires it to
// runScan.
func newScanCmd(stdout, stderr io.Writer) *cobra.Command {
	var opts scanOptions

	cmd := &cobra.Command{
		Use:   "scan [paths...]",
		Short: "Scan source files for SQL anti-patterns",
		Long: strings.Join([]string{
			"Scan one or more paths for SQL findings.",
			"",
			"Exit codes:",
			"  0: no findings",
			"  1: findings detected",
			"  2: config/runtime error",
		}, "\n"),
		Example: strings.Join([]string{
			"  valk-guard scan .",
			"  valk-guard scan ./queries --format json",
			"  valk-guard scan . --format sarif --output results.sarif",
			"  valk-guard scan . --config .valk-guard.yaml",
		}, "\n"),
		RunE: func(_ *cobra.Command, args []string) error {
			code, err := runScan(opts, args, stdout, stderr)
			if err != nil {
				return &codedError{code: code, err: err}
			}
			if code != exitSuccess {
				return &codedError{code: code}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.configPath, "config", "", "Path to config file (default: .valk-guard.yaml or .valk-guard.yml)")
	cmd.Flags().StringVar(&opts.format, "format", "", "Output format: terminal (default), json, sarif")
	cmd.Flags().StringVar(&opts.outputPath, "output", "", "Write report to file instead of stdout")
	cmd.Flags().StringVar(&opts.logLevel, "log-level", "warn", "Log level: debug, info, warn, error")

	return cmd
}

// runScan loads configuration, resolves the output reporter, collects source
// files from the given paths, runs all enabled rules, and writes the report.
// It returns an exit code and an optional error. Exit code 1 means findings
// were detected (not an error); exit code 2 means a real error occurred.
func runScan(opts scanOptions, args []string, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 {
		args = []string{"."}
	}

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return exitError, err
	}

	format := cfg.Format
	if opts.format != "" {
		format = opts.format
	}
	if format == "" {
		format = config.Default().Format
	}

	reporter, err := buildReporter(format)
	if err != nil {
		return exitError, err
	}

	logger, err := buildLogger(stderr, opts.logLevel)
	if err != nil {
		return exitError, err
	}

	reg := rules.DefaultRegistry()
	warnUnknownConfiguredRules(cfg, reg, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	findings, hadScannableInputs, err := collectAndAnalyze(ctx, args, cfg, reg, logger)
	if err != nil {
		return exitError, err
	}
	if !hadScannableInputs {
		logger.Warn("no .sql, .go, or .py files found in scan paths")
	}

	out := stdout
	var tmpPath string
	if opts.outputPath != "" {
		dir := filepath.Dir(opts.outputPath)
		tmpFile, createErr := os.CreateTemp(dir, ".valk-guard-*.tmp")
		if createErr != nil {
			return exitError, fmt.Errorf("creating output file %s: %w", opts.outputPath, createErr)
		}
		tmpPath = tmpFile.Name()
		out = tmpFile
		defer func() {
			tmpFile.Close()    //nolint:errcheck // best-effort close
			os.Remove(tmpPath) //nolint:errcheck // clean up temp file; no-op after successful rename
		}()
	}

	if err := reporter.Report(ctx, out, findings); err != nil {
		return exitError, err
	}

	// Print "no scannable files" hint after the report for terminal output
	// so it appears below the "0 findings" line.
	if !hadScannableInputs && format == config.FormatTerminal && len(findings) == 0 {
		_, _ = fmt.Fprintln(out, "(no scannable files found)")
	}

	if opts.outputPath != "" {
		if f, ok := out.(*os.File); ok {
			if err := f.Close(); err != nil {
				return exitError, fmt.Errorf("closing output file: %w", err)
			}
			if err := os.Rename(tmpPath, opts.outputPath); err != nil {
				return exitError, fmt.Errorf("writing output file %s: %w", opts.outputPath, err)
			}
		}
	}

	if len(findings) > 0 {
		return exitFindings, nil
	}
	return exitSuccess, nil
}

// namedScanner associates a human-readable scanner name with its implementation
// and the set of input files it should process.
type namedScanner struct {
	name string
	impl scanner.Scanner
	in   []string
}

// plannedRules organises enabled rules into two buckets: those that apply to
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

	var (
		firstErr error
		errMu    sync.Mutex
	)
	setFirstErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		errMu.Unlock()
	}

	results := fanOutScanners(scanCtx, active, logger, setFirstErr)

	checkErr := func() error {
		errMu.Lock()
		defer errMu.Unlock()
		return firstErr
	}

	sr := processStatements(scanCtx, results, cfg, rulePlan, logger, setFirstErr, checkErr)

	if err := checkErr(); err != nil {
		return nil, true, err
	}

	// Schema-aware phase: build migration snapshot once, then run
	// query-schema rules and model schema-drift rules.
	// Prefer files under migration-like paths when present; otherwise fall
	// back to all SQL files to preserve behavior for projects that keep
	// migrations elsewhere.
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
// parsing each SQL statement, applying enabled rules, and categorising
// statements into migration vs regular buckets for the schema-aware phase.
func processStatements(
	ctx context.Context,
	results <-chan scanner.SQLStatement,
	cfg *config.Config,
	rulePlan plannedRules,
	logger *slog.Logger,
	setFirstErr func(error),
	checkErr func() error,
) statementResults {
	var sr statementResults

	for stmt := range results {
		if checkErr() != nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			setFirstErr(err)
			continue
		}

		if cfg.ShouldExclude(stmt.File) {
			logger.Debug("statement excluded by config", "file", stmt.File)
			continue
		}

		// Collect SQL-engine statements for schema-drift analysis.
		if stmt.Engine == scanner.EngineSQL {
			sr.sqlStmts = append(sr.sqlStmts, stmt)
			if isMigrationSQLFile(stmt.File) {
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
			ruleFindings := rule.Check(parsed, stmt.File, stmt.Line, stmt.SQL)
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
// all goroutines finish. Any scanner error is reported via setFirstErr and
// causes the scan context to be cancelled.
func fanOutScanners(
	scanCtx context.Context,
	active []namedScanner,
	logger *slog.Logger,
	setFirstErr func(error),
) <-chan scanner.SQLStatement {
	results := make(chan scanner.SQLStatement, 256)
	var wg sync.WaitGroup

	for _, sc := range active {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					setFirstErr(fmt.Errorf("scanner %s panicked: %v", sc.name, r))
				}
			}()

			emitted := 0
			for stmt, err := range sc.impl.Scan(scanCtx, sc.in) {
				if err != nil {
					setFirstErr(fmt.Errorf("scanner %s: %w", sc.name, err))
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
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

// sortFindings sorts findings deterministically by file, line, column,
// rule ID, and message so that report output is stable across runs.
func sortFindings(findings []rules.Finding) {
	slices.SortFunc(findings, func(a, b rules.Finding) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Line, b.Line); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Column, b.Column); c != 0 {
			return c
		}
		if c := strings.Compare(a.RuleID, b.RuleID); c != 0 {
			return c
		}
		return strings.Compare(a.Message, b.Message)
	})
}

// buildRulePlan iterates over the registry and partitions enabled rules by the
// SQL command types they target, producing a plannedRules ready for fast
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

	var unknown []string
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

// buildReporter constructs the output.Reporter corresponding to the requested
// format string (config.FormatTerminal, config.FormatJSON, or config.FormatSARIF).
func buildReporter(format string) (output.Reporter, error) {
	switch format {
	case config.FormatTerminal:
		return &output.TerminalReporter{}, nil
	case config.FormatJSON:
		return &output.JSONReporter{}, nil
	case config.FormatSARIF:
		return &output.SARIFReporter{Version: version}, nil
	default:
		return nil, fmt.Errorf("invalid format %q: must be terminal, json, or sarif", format)
	}
}

// buildLogger creates a structured slog.Logger that writes text-format log
// records to w at the requested level ("debug", "info", "warn", or "error").
func buildLogger(w io.Writer, level string) (*slog.Logger, error) {
	if level == "" {
		level = "warn"
	}

	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid log level %q: must be debug, info, warn, or error", level)
	}

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slogLevel})
	return slog.New(handler), nil
}

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

	// Build migration snapshot from DDL statements.
	migrationSnap := schema.BuildFromStatements(sqlStmts, logger)

	// Extract models from all configured model sources.
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

	// Run enabled query-schema rules against parsed query statements.
	var findings []rules.Finding
	queryFindings := runQuerySchemaChecks(querySchemaRules, parsedStmts, migrationSnap, modelSnaps, querySourceMap, cfg, logger)
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

	// Run enabled schema rules.
	for _, rule := range schemaRules {
		ruleModels := modelsForRule(models, cfg, rule.ID(), modelBindings)
		if len(ruleModels) == 0 {
			continue
		}
		ruleFindings := rule.CheckSchema(migrationSnap, ruleModels)
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
				ruleFindings := rule.CheckQuerySchema(snap, ps.stmt, ps.parsed)
				for i := range ruleFindings {
					ruleFindings[i].Severity = cfg.RuleSeverity(rule.ID(), ruleFindings[i].Severity)
					key := queryFindingKey(ruleFindings[i])
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
	var snaps []*schema.Snapshot
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

// queryFindingKey builds a deterministic key for de-duplicating query-schema
// findings emitted from multiple schema snapshots.
type queryFindingKeyValue struct {
	RuleID  string
	File    string
	Line    int
	Column  int
	Message string
	SQL     string
}

func queryFindingKey(f rules.Finding) queryFindingKeyValue {
	return queryFindingKeyValue{
		RuleID:  f.RuleID,
		File:    f.File,
		Line:    f.Line,
		Column:  f.Column,
		Message: f.Message,
		SQL:     f.SQL,
	}
}

// isMigrationSQLFile reports whether path looks like a migration SQL file.
func isMigrationSQLFile(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	if !strings.HasSuffix(normalized, ".sql") {
		return false
	}
	// Check for migration directory segments without allocating via Split.
	// Prepend "/" so that a leading segment like "migrations/..." is also matched.
	p := "/" + normalized
	return strings.Contains(p, "/migrations/") ||
		strings.Contains(p, "/migration/") ||
		strings.Contains(p, "/migrate/")
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
		engines, mapped := sourceEngines[model.Source]
		if !mapped || len(engines) == 0 {
			filtered = append(filtered, model)
			continue
		}

		for _, engine := range engines {
			if cfg.IsRuleEnabledForEngine(ruleID, engine) {
				filtered = append(filtered, model)
				break
			}
		}
	}
	return filtered
}
