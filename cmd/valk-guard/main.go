package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/valkdb/postgresparser"

	"github.com/valkdb/valk-guard/internal/config"
	"github.com/valkdb/valk-guard/internal/output"
	"github.com/valkdb/valk-guard/internal/rules"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/scanner/goqu"
	"github.com/valkdb/valk-guard/internal/scanner/sqlalchemy"
	"github.com/valkdb/valk-guard/internal/schema"
	"github.com/valkdb/valk-guard/internal/schema/gomodel"
	"github.com/valkdb/valk-guard/internal/schema/pymodel"
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
			if coded.err != nil && coded.err.Error() != "" {
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
	noColor    bool
}

// newScanCmd constructs the "scan" subcommand with its flags and wires it to
// runScan.
func newScanCmd(stdout, stderr io.Writer) *cobra.Command {
	var opts scanOptions

	cmd := &cobra.Command{
		Use:   "scan [paths...]",
		Short: "Scan source files for SQL anti-patterns",
		RunE: func(_ *cobra.Command, args []string) error {
			return runScan(opts, args, stdout, stderr)
		},
	}

	cmd.Flags().StringVar(&opts.configPath, "config", "", "Path to config file (default: .valk-guard.yaml or .valk-guard.yml)")
	cmd.Flags().StringVar(&opts.format, "format", "", "Output format: terminal, json, sarif")
	cmd.Flags().StringVar(&opts.outputPath, "output", "", "Write report to file instead of stdout")
	cmd.Flags().StringVar(&opts.logLevel, "log-level", "warn", "Log level: debug, info, warn, error")
	cmd.Flags().BoolVar(&opts.noColor, "no-color", false, "Disable terminal colors")

	return cmd
}

// runScan loads configuration, resolves the output reporter, collects source
// files from the given paths, runs all enabled rules, and writes the report.
// It returns a codedError so the caller can propagate the correct exit code.
func runScan(opts scanOptions, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		args = []string{"."}
	}

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return &codedError{code: exitError, err: err}
	}

	format := cfg.Format
	if opts.format != "" {
		format = opts.format
	}
	if format == "" {
		format = config.Default().Format
	}

	reporter, err := buildReporter(format, opts.noColor)
	if err != nil {
		return &codedError{code: exitError, err: err}
	}

	logger, err := buildLogger(stderr, opts.logLevel)
	if err != nil {
		return &codedError{code: exitError, err: err}
	}

	out := stdout
	if opts.outputPath != "" {
		f, createErr := os.Create(opts.outputPath)
		if createErr != nil {
			return &codedError{
				code: exitError,
				err:  fmt.Errorf("creating output file %s: %w", opts.outputPath, createErr),
			}
		}
		defer f.Close() //nolint:errcheck // best-effort close after reporting
		out = f
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	findings, err := collectAndAnalyze(ctx, args, cfg, logger)
	if err != nil {
		return &codedError{code: exitError, err: err}
	}

	if err := reporter.Report(ctx, out, findings); err != nil {
		return &codedError{code: exitError, err: err}
	}

	if len(findings) > 0 {
		return &codedError{code: exitFindings}
	}
	return nil
}

// namedScanner associates a human-readable scanner name with its implementation
// and the set of input files it should process.
type namedScanner struct {
	name string
	impl scanner.Scanner
	in   []string
}

// scannerInputs groups the file paths collected during directory walking by
// their language / file type.
type scannerInputs struct {
	sqlFiles []string
	goFiles  []string
	pyFiles  []string
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

// collectAndAnalyze walks the given paths to discover source files, fans out
// scanning across all registered scanners concurrently, parses each SQL
// statement, applies enabled rules, and returns the deduplicated, sorted
// findings.
func collectAndAnalyze(ctx context.Context, paths []string, cfg *config.Config, logger *slog.Logger) ([]rules.Finding, error) {
	inputs, err := collectScannerInputs(ctx, paths, cfg)
	if err != nil {
		return nil, err
	}

	active := activeScanners(inputs, logger)
	if len(active) == 0 {
		logger.Debug("no candidate files found")
		return nil, nil
	}

	reg := rules.DefaultRegistry()
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

	var findings []rules.Finding
	var sqlStatements []scanner.SQLStatement
	var migrationSQLStatements []scanner.SQLStatement
	var parsedStatements []parsedStatement
	statements := 0

	for stmt := range results {
		errMu.Lock()
		activeErr := firstErr
		errMu.Unlock()
		if activeErr != nil {
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
			sqlStatements = append(sqlStatements, stmt)
			if isMigrationSQLFile(stmt.File) {
				migrationSQLStatements = append(migrationSQLStatements, stmt)
			}
		}

		statements++
		parsed, err := scanner.ParseStatement(stmt.SQL)
		if err != nil {
			setFirstErr(fmt.Errorf("parse error at %s:%d: %w", stmt.File, stmt.Line, err))
			continue
		}
		if parsed == nil {
			continue
		}
		parsedStatements = append(parsedStatements, parsedStatement{
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
			findings = append(findings, ruleFindings...)
		}

		for _, rule := range rulePlan.byCommand[parsed.Command] {
			applyRule(rule)
		}
		for _, rule := range rulePlan.any {
			applyRule(rule)
		}
	}

	errMu.Lock()
	defer errMu.Unlock()
	if firstErr != nil {
		return nil, firstErr
	}

	// Schema-aware phase: build migration snapshot once, then run
	// query-schema rules and model schema-drift rules.
	// Prefer files under migration-like paths when present; otherwise fall
	// back to all SQL files to preserve behavior for projects that keep
	// migrations elsewhere.
	schemaSQLStatements := sqlStatements
	if len(migrationSQLStatements) > 0 {
		schemaSQLStatements = migrationSQLStatements
	}
	schemaFindings := runSchemaDrift(ctx, schemaSQLStatements, parsedStatements, inputs, cfg, reg, logger)
	findings = append(findings, schemaFindings...)

	sortFindings(findings)

	logger.Debug("analysis complete", "statements", statements, "findings", len(findings))
	return findings, nil
}

// activeScanners builds the full list of named scanners from the collected
// inputs, filters out any scanner with no candidate files, and logs the
// outcome for each scanner.
func activeScanners(inputs scannerInputs, logger *slog.Logger) []namedScanner {
	all := []namedScanner{
		{name: "sql", impl: &scanner.RawSQLScanner{}, in: inputs.sqlFiles},
		{name: "go", impl: &scanner.GoScanner{}, in: inputs.goFiles},
		{name: "goqu", impl: &goqu.Scanner{}, in: inputs.goFiles},
		{name: "sqlalchemy", impl: &sqlalchemy.Scanner{}, in: inputs.pyFiles},
	}

	active := make([]namedScanner, 0, len(all))
	for _, sc := range all {
		if len(sc.in) == 0 {
			logger.Debug("scanner skipped", "scanner", sc.name, "files", 0)
			continue
		}
		logger.Debug("scanner queued", "scanner", sc.name, "files", len(sc.in))
		active = append(active, sc)
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
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		if findings[i].Column != findings[j].Column {
			return findings[i].Column < findings[j].Column
		}
		if findings[i].RuleID != findings[j].RuleID {
			return findings[i].RuleID < findings[j].RuleID
		}
		return findings[i].Message < findings[j].Message
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
		targets := ruleCommandTargets(rule.ID())
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

// ruleCommandTargets returns the SQL command types that a given rule targets.
// Rules that apply to all command types return nil.
func ruleCommandTargets(ruleID string) []postgresparser.QueryCommand {
	switch ruleID {
	case "VG001", "VG004", "VG006":
		return []postgresparser.QueryCommand{postgresparser.QueryCommandSelect}
	case "VG002":
		return []postgresparser.QueryCommand{postgresparser.QueryCommandUpdate}
	case "VG003":
		return []postgresparser.QueryCommand{postgresparser.QueryCommandDelete}
	case "VG007", "VG008":
		return []postgresparser.QueryCommand{postgresparser.QueryCommandDDL}
	default:
		// Cross-cutting rules (for example VG005) run for all command types.
		return nil
	}
}

// collectScannerInputs recursively walks the provided paths, classifying each
// file by extension into SQL, Go, or Python buckets while respecting config
// exclusion patterns and context cancellation.
func collectScannerInputs(ctx context.Context, paths []string, cfg *config.Config) (scannerInputs, error) {
	var inputs scannerInputs
	seenSQL := make(map[string]struct{})
	seenGo := make(map[string]struct{})
	seenPy := make(map[string]struct{})

	add := func(dst *[]string, seen map[string]struct{}, path string) {
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		*dst = append(*dst, path)
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
				return nil
			}
			if cfg.ShouldExclude(path) {
				return nil
			}

			switch strings.ToLower(filepath.Ext(path)) {
			case ".sql":
				add(&inputs.sqlFiles, seenSQL, path)
			case ".go":
				add(&inputs.goFiles, seenGo, path)
			case ".py":
				add(&inputs.pyFiles, seenPy, path)
			}
			return nil
		})
		if err != nil {
			return scannerInputs{}, fmt.Errorf("walking %s: %w", root, err)
		}
	}

	sort.Strings(inputs.sqlFiles)
	sort.Strings(inputs.goFiles)
	sort.Strings(inputs.pyFiles)
	return inputs, nil
}

// buildReporter constructs the output.Reporter corresponding to the requested
// format string (config.FormatTerminal, config.FormatJSON, or config.FormatSARIF).
func buildReporter(format string, noColor bool) (output.Reporter, error) {
	switch format {
	case config.FormatTerminal:
		return &output.TerminalReporter{NoColor: noColor}, nil
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
	cfg *config.Config,
	reg *rules.Registry,
	logger *slog.Logger,
) []rules.Finding {
	schemaRules := enabledSchemaRules(reg, cfg)
	querySchemaRules := enabledQuerySchemaRules(reg, cfg)
	if len(schemaRules) == 0 && len(querySchemaRules) == 0 {
		return nil
	}

	// Build migration snapshot from DDL statements.
	migrationSnap := schema.BuildFromStatements(sqlStmts)

	// Extract models from Go and Python source files. Models are needed for:
	// 1) model schema-drift rules (VG101-VG104)
	// 2) query-schema rules (VG105-VG106) for go/goqu/sqlalchemy engines.
	var models []schema.ModelDef

	if len(inputs.goFiles) > 0 {
		goModels, err := (&gomodel.Extractor{}).ExtractModels(ctx, inputs.goFiles)
		if err != nil {
			logger.Warn("go model extraction failed", "error", err)
		} else {
			models = append(models, goModels...)
		}
	}

	if len(inputs.pyFiles) > 0 {
		pyModels, err := (&pymodel.Extractor{}).ExtractModels(ctx, inputs.pyFiles)
		if err != nil {
			logger.Warn("python model extraction failed", "error", err)
		} else {
			models = append(models, pyModels...)
		}
	}

	goModelSnap := buildModelSnapshotBySource(models, schema.ModelSourceGo)
	pyModelSnap := buildModelSnapshotBySource(models, schema.ModelSourceSQLAlchemy)

	var findings []rules.Finding

	// Run enabled query-schema rules against parsed query statements.
	if len(querySchemaRules) > 0 {
		logger.Debug(
			"query-schema analysis",
			"migration_tables", len(migrationSnap.Tables),
			"go_model_tables", len(goModelSnap.Tables),
			"python_model_tables", len(pyModelSnap.Tables),
			"statements", len(parsedStmts),
			"rules", len(querySchemaRules),
		)

		for _, ps := range parsedStmts {
			if ps.parsed == nil {
				continue
			}
			snaps := querySnapshotsForEngine(ps.stmt.Engine, migrationSnap, goModelSnap, pyModelSnap)
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

				seen := make(map[string]bool)
				for _, snap := range snaps {
					ruleFindings := rule.CheckQuerySchema(snap, ps.stmt, ps.parsed)
					for i := range ruleFindings {
						ruleFindings[i].Severity = cfg.RuleSeverity(rule.ID(), ruleFindings[i].Severity)
						key := queryFindingKey(ruleFindings[i])
						if seen[key] {
							continue
						}
						seen[key] = true
						findings = append(findings, ruleFindings[i])
					}
				}
			}
		}
	}

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
		ruleModels := modelsForRule(models, cfg, rule.ID())
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

// buildModelSnapshotBySource converts extracted models for one source engine
// into a lightweight table/column snapshot used by query-schema rules.
func buildModelSnapshotBySource(models []schema.ModelDef, source schema.ModelSource) *schema.Snapshot {
	snap := schema.NewSnapshot()

	for _, model := range models {
		if model.Source != source {
			continue
		}
		tableName := strings.TrimSpace(model.Table)
		if tableName == "" {
			continue
		}

		tableKey := strings.ToLower(tableName)
		td, ok := snap.Tables[tableKey]
		if !ok {
			td = &schema.TableDef{
				Name:    tableName,
				Columns: make(map[string]schema.ColumnDef),
				File:    model.File,
				Line:    model.Line,
			}
			snap.Tables[tableKey] = td
		}

		for _, col := range model.Columns {
			colName := strings.TrimSpace(col.Name)
			if colName == "" {
				continue
			}
			td.Columns[strings.ToLower(colName)] = schema.ColumnDef{
				Name: colName,
				Type: col.Type,
			}
		}
	}

	return snap
}

// querySnapshotsForEngine returns schema snapshots to check for a statement.
// Migrations always participate when available; model snapshots are added for
// engine-matched sources (go/goqu -> Go models, sqlalchemy -> SQLAlchemy models).
func querySnapshotsForEngine(
	engine scanner.Engine,
	migrationSnap, goModelSnap, pyModelSnap *schema.Snapshot,
) []*schema.Snapshot {
	var snaps []*schema.Snapshot
	if migrationSnap != nil && len(migrationSnap.Tables) > 0 {
		snaps = append(snaps, migrationSnap)
	}

	switch engine {
	case scanner.EngineGo, scanner.EngineGoqu:
		if goModelSnap != nil && len(goModelSnap.Tables) > 0 {
			snaps = append(snaps, goModelSnap)
		}
	case scanner.EngineSQLAlchemy:
		if pyModelSnap != nil && len(pyModelSnap.Tables) > 0 {
			snaps = append(snaps, pyModelSnap)
		}
	}

	return snaps
}

// queryFindingKey builds a deterministic key for de-duplicating query-schema
// findings emitted from multiple schema snapshots.
func queryFindingKey(f rules.Finding) string {
	return fmt.Sprintf("%s|%s|%d|%d|%s|%s", f.RuleID, f.File, f.Line, f.Column, f.Message, f.SQL)
}

// isMigrationSQLFile reports whether path looks like a migration SQL file.
func isMigrationSQLFile(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	if !strings.HasSuffix(normalized, ".sql") {
		return false
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == "migrations" || part == "migration" || part == "migrate" {
			return true
		}
	}
	return false
}

// enabledSchemaRules returns schema rules that are enabled and allowed for at
// least one model source engine.
func enabledSchemaRules(reg *rules.Registry, cfg *config.Config) []rules.SchemaRule {
	all := reg.AllSchema()
	result := make([]rules.SchemaRule, 0, len(all))
	for _, rule := range all {
		if !cfg.IsRuleEnabled(rule.ID()) {
			continue
		}
		goAllowed := cfg.IsRuleEnabledForEngine(rule.ID(), scanner.EngineGo)
		pyAllowed := cfg.IsRuleEnabledForEngine(rule.ID(), scanner.EngineSQLAlchemy)
		if !goAllowed && !pyAllowed {
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
func modelsForRule(models []schema.ModelDef, cfg *config.Config, ruleID string) []schema.ModelDef {
	goAllowed := cfg.IsRuleEnabledForEngine(ruleID, scanner.EngineGo)
	pyAllowed := cfg.IsRuleEnabledForEngine(ruleID, scanner.EngineSQLAlchemy)
	if !goAllowed && !pyAllowed {
		return nil
	}

	filtered := make([]schema.ModelDef, 0, len(models))
	for _, model := range models {
		switch model.Source {
		case schema.ModelSourceGo:
			if goAllowed {
				filtered = append(filtered, model)
			}
		case schema.ModelSourceSQLAlchemy:
			if pyAllowed {
				filtered = append(filtered, model)
			}
		default:
			filtered = append(filtered, model)
		}
	}
	return filtered
}
