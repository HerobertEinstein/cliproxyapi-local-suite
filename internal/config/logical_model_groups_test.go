package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func mustUnmarshalLogicalModelGroupsConfig(t *testing.T, source string) *Config {
	t.Helper()

	var cfg Config
	if err := yaml.Unmarshal([]byte(source), &cfg); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	cfg.SanitizeLogicalModelGroups()
	return &cfg
}

func TestSanitizeLogicalModelGroups_MigratesLegacyCurrentTargetIntoStaticGroup(t *testing.T) {
	cfg := mustUnmarshalLogicalModelGroupsConfig(t, `
logical-model-groups:
  current:
    target: gpt-5.2
    reasoning:
      mode: group
      effort: high
`)

	entries := cfg.LogicalModelGroupEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 logical entries after migration, got %d", len(entries))
	}

	if got := cfg.ResolveLogicalModelGroup("current"); got != "gpt-5.2(high)" {
		t.Fatalf("expected current to resolve via migrated static group, got %q", got)
	}
	if got := cfg.ResolveLogicalModelGroup("gpt-5.2"); got != "gpt-5.2(high)" {
		t.Fatalf("expected migrated static alias to inherit legacy reasoning, got %q", got)
	}
}

func TestResolveLogicalModelGroup_CurrentRefUsesStaticGroupReasoning(t *testing.T) {
	cfg := mustUnmarshalLogicalModelGroupsConfig(t, `
logical-model-groups:
  current:
    ref: gpt-5.4
  static:
    - alias: gpt-5.4
      target: gpt-5.4
      reasoning:
        mode: group
        effort: high
`)

	if got := cfg.ResolveLogicalModelGroup("current(low)"); got != "gpt-5.4(high)" {
		t.Fatalf("expected current to inherit static group suffix priority, got %q", got)
	}
	if got := cfg.ResolveLogicalModelGroup("gpt-5.4(low)"); got != "gpt-5.4(high)" {
		t.Fatalf("expected static alias to keep group-defined suffix priority, got %q", got)
	}
}

func TestLogicalModelGroupResolvedTarget(t *testing.T) {
	t.Run("request mode keeps target", func(t *testing.T) {
		group := LogicalModelGroup{
			Alias:  "current",
			Target: "gpt-5.2",
			Reasoning: LogicalModelGroupReasoning{
				Mode:   LogicalModelGroupReasoningModeRequest,
				Effort: "high",
			},
		}

		if got := group.ResolvedTarget(); got != "gpt-5.2" {
			t.Fatalf("expected request mode to keep target, got %q", got)
		}
	})

	t.Run("group mode appends suffix", func(t *testing.T) {
		group := LogicalModelGroup{
			Alias:  "current",
			Target: "gpt-5.2",
			Reasoning: LogicalModelGroupReasoning{
				Mode:   LogicalModelGroupReasoningModeGroup,
				Effort: "high",
			},
		}

		if got := group.ResolvedTarget(); got != "gpt-5.2(high)" {
			t.Fatalf("expected group mode to append suffix, got %q", got)
		}
	})

	t.Run("existing target suffix wins", func(t *testing.T) {
		group := LogicalModelGroup{
			Alias:  "current",
			Target: "gpt-5.2(low)",
			Reasoning: LogicalModelGroupReasoning{
				Mode:   LogicalModelGroupReasoningModeGroup,
				Effort: "high",
			},
		}

		if got := group.ResolvedTarget(); got != "gpt-5.2(low)" {
			t.Fatalf("expected explicit target suffix to win, got %q", got)
		}
	})
}
