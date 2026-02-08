package rules

import (
	"testing"

	"github.com/valkdb/postgresparser"
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
