// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/valkdb/postgresparser"
	"github.com/valkdb/valk-guard/internal/scanner"
	"github.com/valkdb/valk-guard/internal/schema"
)

// mockRule implements Rule for testing.
type mockRule struct {
	id       string
	name     string
	desc     string
	severity Severity
}

func (r *mockRule) ID() string                { return r.id }
func (r *mockRule) Name() string              { return r.name }
func (r *mockRule) Description() string       { return r.desc }
func (r *mockRule) DefaultSeverity() Severity { return r.severity }
func (r *mockRule) Check(_ *postgresparser.ParsedQuery, _ string, _ int, _ string) []Finding {
	return nil
}

// mockQuerySchemaRule implements QuerySchemaRule for testing.
type mockQuerySchemaRule struct {
	id string
}

func (r *mockQuerySchemaRule) ID() string                { return r.id }
func (r *mockQuerySchemaRule) Name() string              { return "mock-query-schema" }
func (r *mockQuerySchemaRule) Description() string       { return "mock query schema rule" }
func (r *mockQuerySchemaRule) DefaultSeverity() Severity { return SeverityWarning }
func (r *mockQuerySchemaRule) CheckQuerySchema(_ *schema.Snapshot, _ *scanner.SQLStatement, _ *postgresparser.ParsedQuery) []Finding {
	return nil
}

// mockSchemaRule implements SchemaRule for testing.
type mockSchemaRule struct {
	id string
}

func (r *mockSchemaRule) ID() string                { return r.id }
func (r *mockSchemaRule) Name() string              { return "mock-schema" }
func (r *mockSchemaRule) Description() string       { return "mock schema rule" }
func (r *mockSchemaRule) DefaultSeverity() Severity { return SeverityWarning }
func (r *mockSchemaRule) CheckSchema(_ *schema.Snapshot, _ []schema.ModelDef) []Finding {
	return nil
}

func TestRegisterAndGet(t *testing.T) {
	reg := NewRegistry()

	rule := &mockRule{id: "VG001", name: "Test Rule", severity: SeverityWarning}
	if err := reg.Register(rule); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	got := reg.Get("VG001")
	if got == nil {
		t.Fatal("expected to get VG001")
	}
	if got.ID() != "VG001" {
		t.Errorf("expected ID VG001, got %s", got.ID())
	}
}

func TestRegisterDuplicate(t *testing.T) {
	reg := NewRegistry()

	rule := &mockRule{id: "VG001"}
	if err := reg.Register(rule); err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	if err := reg.Register(rule); err == nil {
		t.Error("expected error on duplicate registration")
	}
}

func TestGetMissing(t *testing.T) {
	reg := NewRegistry()

	if got := reg.Get("VG999"); got != nil {
		t.Error("expected nil for missing rule")
	}
}

func TestAll(t *testing.T) {
	reg := NewRegistry()

	rules := []Rule{
		&mockRule{id: "VG001"},
		&mockRule{id: "VG002"},
		&mockRule{id: "VG003"},
	}

	for _, r := range rules {
		if err := reg.Register(r); err != nil {
			t.Fatalf("register failed: %v", err)
		}
	}

	all := reg.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(all))
	}

	// Verify order.
	for i, r := range all {
		if r.ID() != rules[i].ID() {
			t.Errorf("rule %d: expected %s, got %s", i, rules[i].ID(), r.ID())
		}
	}
}

func TestDefaultRegistryHasBuiltins(t *testing.T) {
	reg := DefaultRegistry()

	all := reg.All()
	if len(all) != 8 {
		t.Fatalf("expected 8 built-in rules, got %d", len(all))
	}

	wantOrder := []string{"VG001", "VG002", "VG003", "VG004", "VG005", "VG006", "VG007", "VG008"}
	for i, want := range wantOrder {
		if all[i].ID() != want {
			t.Errorf("rule %d: expected %s, got %s", i, want, all[i].ID())
		}
	}
}

func TestRegisterQuerySchemaDuplicate(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	rule := &mockQuerySchemaRule{id: "VG105"}

	if err := reg.RegisterQuerySchema(rule); err != nil {
		t.Fatalf("first RegisterQuerySchema failed: %v", err)
	}
	if err := reg.RegisterQuerySchema(rule); err == nil {
		t.Fatal("expected duplicate RegisterQuerySchema to fail")
	}
}

func TestAllQuerySchemaOrder(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	ruleA := &mockQuerySchemaRule{id: "VG105"}
	ruleB := &mockQuerySchemaRule{id: "VG106"}
	if err := reg.RegisterQuerySchema(ruleA); err != nil {
		t.Fatalf("register ruleA failed: %v", err)
	}
	if err := reg.RegisterQuerySchema(ruleB); err != nil {
		t.Fatalf("register ruleB failed: %v", err)
	}

	all := reg.AllQuerySchema()
	if len(all) != 2 {
		t.Fatalf("expected 2 query-schema rules, got %d", len(all))
	}
	if all[0].ID() != "VG105" || all[1].ID() != "VG106" {
		t.Fatalf("unexpected query-schema order: [%s %s]", all[0].ID(), all[1].ID())
	}
}

func TestDefaultRegistryHasBuiltinQuerySchemaRules(t *testing.T) {
	t.Parallel()

	reg := DefaultRegistry()
	all := reg.AllQuerySchema()
	if len(all) != 4 {
		t.Fatalf("expected 4 built-in query-schema rules, got %d", len(all))
	}

	want := []string{"VG105", "VG106", "VG107", "VG108"}
	for i, id := range want {
		if all[i].ID() != id {
			t.Errorf("query-schema rule %d: expected %s, got %s", i, id, all[i].ID())
		}
	}
}

func TestDefaultRegistryHasBuiltinSchemaRules(t *testing.T) {
	t.Parallel()

	reg := DefaultRegistry()
	all := reg.AllSchema()
	if len(all) != 7 {
		t.Fatalf("expected 7 built-in schema rules, got %d", len(all))
	}

	want := []string{"VG101", "VG102", "VG103", "VG104", "VG109", "VG110", "VG111"}
	for i, id := range want {
		if all[i].ID() != id {
			t.Errorf("schema rule %d: expected %s, got %s", i, id, all[i].ID())
		}
	}
}

func TestRegisterSchemaDuplicate(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	rule := &mockSchemaRule{id: "VG101"}

	if err := reg.RegisterSchema(rule); err != nil {
		t.Fatalf("first RegisterSchema failed: %v", err)
	}
	if err := reg.RegisterSchema(rule); err == nil {
		t.Fatal("expected duplicate RegisterSchema to fail")
	}
}

func TestCrossTypeIDCollision(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()

	// Register a normal Rule with ID "VG999".
	rule := &mockRule{id: "VG999"}
	if err := reg.Register(rule); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Attempt to register a SchemaRule with the same ID; should fail.
	schemaRule := &mockSchemaRule{id: "VG999"}
	if err := reg.RegisterSchema(schemaRule); err == nil {
		t.Fatal("expected cross-type ID collision to fail for RegisterSchema")
	}

	// Attempt to register a QuerySchemaRule with the same ID; should also fail.
	qsRule := &mockQuerySchemaRule{id: "VG999"}
	if err := reg.RegisterQuerySchema(qsRule); err == nil {
		t.Fatal("expected cross-type ID collision to fail for RegisterQuerySchema")
	}
}
