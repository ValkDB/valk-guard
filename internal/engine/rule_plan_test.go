// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"testing"

	"github.com/valkdb/postgresparser"

	"github.com/valkdb/valk-guard/internal/config"
	"github.com/valkdb/valk-guard/internal/rules"
)

func TestBuildRulePlanDefaultDistribution(t *testing.T) {
	cfg := config.Default()
	plan := buildRulePlan(cfg, rules.DefaultRegistry())

	assertRuleIDs(t, plan.any, nil)
	assertRuleIDs(t, plan.byCommand[postgresparser.QueryCommandSelect], []string{"VG001", "VG004", "VG005", "VG006"})
	assertRuleIDs(t, plan.byCommand[postgresparser.QueryCommandUpdate], []string{"VG002", "VG005"})
	assertRuleIDs(t, plan.byCommand[postgresparser.QueryCommandDelete], []string{"VG003", "VG005"})
	assertRuleIDs(t, plan.byCommand[postgresparser.QueryCommandDDL], []string{"VG007", "VG008"})
}

func TestBuildRulePlanHonorsDisabledRules(t *testing.T) {
	cfg := config.Default()
	disabled := false
	cfg.Rules["VG001"] = config.RuleConfig{Enabled: &disabled}

	plan := buildRulePlan(cfg, rules.DefaultRegistry())

	for _, rule := range plan.byCommand[postgresparser.QueryCommandSelect] {
		if rule.ID() == "VG001" {
			t.Fatalf("expected VG001 to be excluded from SELECT rule plan")
		}
	}
}

func assertRuleIDs(t *testing.T, got []rules.Rule, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %d rules, got %d", len(want), len(got))
	}

	gotSet := make(map[string]struct{}, len(got))
	for _, rule := range got {
		gotSet[rule.ID()] = struct{}{}
	}

	for _, id := range want {
		if _, ok := gotSet[id]; !ok {
			t.Fatalf("expected rule %s in plan, got %#v", id, gotSet)
		}
	}
}
