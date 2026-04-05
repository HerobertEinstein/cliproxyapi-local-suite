package config

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
)

const (
	LogicalModelGroupAliasCurrent         = "current"
	LogicalModelGroupReasoningModeRequest = "request"
	LogicalModelGroupReasoningModeGroup   = "group"
)

var logicalModelGroupOAuthChannels = []string{
	"vertex",
	"claude",
	"codex",
	"gemini-cli",
	"aistudio",
	"antigravity",
	"qwen",
	"iflow",
	"kimi",
}

type LogicalModelGroups struct {
	Current LogicalModelCurrent `yaml:"current" json:"current"`
	Static  []LogicalModelGroup `yaml:"static,omitempty" json:"static,omitempty"`
}

type LogicalModelCurrent struct {
	Alias           string                     `yaml:"alias,omitempty" json:"alias,omitempty"`
	Ref             string                     `yaml:"ref,omitempty" json:"ref,omitempty"`
	LegacyTarget    string                     `yaml:"target,omitempty" json:"-"`
	LegacyReasoning LogicalModelGroupReasoning `yaml:"reasoning,omitempty" json:"-"`
}

type LogicalModelGroup struct {
	Alias              string                     `yaml:"alias,omitempty" json:"alias,omitempty"`
	Target             string                     `yaml:"target" json:"target"`
	Reasoning          LogicalModelGroupReasoning `yaml:"reasoning,omitempty" json:"reasoning,omitempty"`
	PreferredProviders []string                   `yaml:"preferred-providers,omitempty" json:"preferred-providers,omitempty"`
}

type LogicalModelGroupReasoning struct {
	Mode   string `yaml:"mode,omitempty" json:"mode,omitempty"`
	Effort string `yaml:"effort,omitempty" json:"effort,omitempty"`
}

func (group LogicalModelGroup) ResolvedTarget() string {
	target := strings.TrimSpace(group.Target)
	if target == "" {
		return ""
	}
	if normalizeLogicalModelGroupReasoning(group.Reasoning).Mode != LogicalModelGroupReasoningModeGroup {
		return target
	}
	if thinking.ParseSuffix(target).HasSuffix {
		return target
	}
	effort := strings.TrimSpace(group.Reasoning.Effort)
	if effort == "" {
		return target
	}
	return target + "(" + effort + ")"
}

func (cfg *Config) SanitizeLogicalModelGroups() {
	if cfg == nil {
		return
	}

	current := sanitizeLogicalModelCurrent(cfg.LogicalModelGroups.Current)

	seen := map[string]struct{}{
		strings.ToLower(LogicalModelGroupAliasCurrent): {},
	}
	static := make([]LogicalModelGroup, 0, len(cfg.LogicalModelGroups.Static)+1)
	for _, group := range cfg.LogicalModelGroups.Static {
		group = sanitizeLogicalModelGroup(group, "", false)
		if group.Alias == "" || group.Target == "" {
			continue
		}
		key := strings.ToLower(group.Alias)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		static = append(static, group)
	}

	if current.Ref == "" && current.LegacyTarget != "" {
		legacyAlias := current.LegacyTarget
		if migrated, ok := findStaticLogicalModelGroup(static, legacyAlias); ok {
			current.Ref = migrated.Alias
		} else {
			migrated := sanitizeLogicalModelGroup(LogicalModelGroup{
				Alias:     legacyAlias,
				Target:    current.LegacyTarget,
				Reasoning: current.LegacyReasoning,
			}, "", false)
			if migrated.Alias != "" && migrated.Target != "" {
				static = append(static, migrated)
				current.Ref = migrated.Alias
			}
		}
	}

	current.LegacyTarget = ""
	current.LegacyReasoning = LogicalModelGroupReasoning{}

	if current.Ref != "" {
		if group, ok := findStaticLogicalModelGroup(static, current.Ref); ok {
			current.Ref = group.Alias
		} else {
			current.Ref = ""
		}
	}

	cfg.LogicalModelGroups.Current = current
	cfg.LogicalModelGroups.Static = static
}

func (cfg *Config) LogicalModelGroupEntries() []LogicalModelGroup {
	if cfg == nil {
		return nil
	}
	out := make([]LogicalModelGroup, 0, 1+len(cfg.LogicalModelGroups.Static))
	if current, ok := cfg.resolveCurrentLogicalModelGroup(); ok {
		out = append(out, current)
	}
	for _, group := range cfg.LogicalModelGroups.Static {
		if strings.TrimSpace(group.Alias) == "" || strings.TrimSpace(group.Target) == "" {
			continue
		}
		out = append(out, group)
	}
	return out
}

func (cfg *Config) ProjectedOAuthModelAliases(channel string) []OAuthModelAlias {
	if cfg == nil {
		return nil
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	base := cfg.OAuthModelAlias[channel]
	out := make([]OAuthModelAlias, 0, len(base)+len(cfg.LogicalModelGroups.Static)+1)
	out = append(out, base...)
	for _, group := range cfg.LogicalModelGroupEntries() {
		out = append(out, OAuthModelAlias{
			Name:  group.ResolvedTarget(),
			Alias: group.Alias,
			Fork:  true,
		})
	}
	return out
}

func (cfg *Config) ProjectedOAuthModelAliasTable() map[string][]OAuthModelAlias {
	if cfg == nil {
		return nil
	}
	out := make(map[string][]OAuthModelAlias, len(cfg.OAuthModelAlias)+len(logicalModelGroupOAuthChannels))
	for channel := range cfg.OAuthModelAlias {
		projected := cfg.ProjectedOAuthModelAliases(channel)
		if len(projected) > 0 {
			out[channel] = projected
		}
	}
	for _, channel := range logicalModelGroupOAuthChannels {
		if _, exists := out[channel]; exists {
			continue
		}
		projected := cfg.ProjectedOAuthModelAliases(channel)
		if len(projected) > 0 {
			out[channel] = projected
		}
	}
	return out
}

func (cfg *Config) ResolveLogicalModelGroup(requestedModel string) string {
	group, requestResult, ok := cfg.resolveLogicalModelGroupEntry(requestedModel)
	if !ok {
		return ""
	}
	return preserveLogicalModelGroupSuffix(group.ResolvedTarget(), requestResult)
}

func (cfg *Config) ResolveLogicalModelGroupEntry(requestedModel string) (LogicalModelGroup, bool) {
	group, _, ok := cfg.resolveLogicalModelGroupEntry(requestedModel)
	return group, ok
}

func (cfg *Config) resolveCurrentLogicalModelGroup() (LogicalModelGroup, bool) {
	if cfg == nil {
		return LogicalModelGroup{}, false
	}
	group, ok := findStaticLogicalModelGroup(cfg.LogicalModelGroups.Static, cfg.LogicalModelGroups.Current.Ref)
	if !ok {
		return LogicalModelGroup{}, false
	}
	return LogicalModelGroup{
		Alias:              LogicalModelGroupAliasCurrent,
		Target:             group.Target,
		Reasoning:          group.Reasoning,
		PreferredProviders: append([]string(nil), group.PreferredProviders...),
	}, true
}

func (cfg *Config) findLogicalModelGroup(alias string) (LogicalModelGroup, bool) {
	alias = strings.TrimSpace(alias)
	if cfg == nil || alias == "" {
		return LogicalModelGroup{}, false
	}
	if strings.EqualFold(alias, LogicalModelGroupAliasCurrent) {
		return cfg.resolveCurrentLogicalModelGroup()
	}
	return findStaticLogicalModelGroup(cfg.LogicalModelGroups.Static, alias)
}

func findStaticLogicalModelGroup(groups []LogicalModelGroup, alias string) (LogicalModelGroup, bool) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return LogicalModelGroup{}, false
	}
	for _, group := range groups {
		if strings.EqualFold(strings.TrimSpace(group.Alias), alias) {
			return group, true
		}
	}
	return LogicalModelGroup{}, false
}

func sanitizeLogicalModelCurrent(current LogicalModelCurrent) LogicalModelCurrent {
	current.Alias = LogicalModelGroupAliasCurrent
	current.Ref = strings.TrimSpace(current.Ref)
	current.LegacyTarget = strings.TrimSpace(current.LegacyTarget)
	current.LegacyReasoning = normalizeLogicalModelGroupReasoning(current.LegacyReasoning)
	if current.Ref != "" {
		current.LegacyTarget = ""
		current.LegacyReasoning = LogicalModelGroupReasoning{}
	}
	return current
}

func sanitizeLogicalModelGroup(group LogicalModelGroup, forcedAlias string, forceAlias bool) LogicalModelGroup {
	group.Alias = strings.TrimSpace(group.Alias)
	group.Target = strings.TrimSpace(group.Target)
	group.Reasoning = normalizeLogicalModelGroupReasoning(group.Reasoning)
	group.PreferredProviders = normalizeLogicalModelGroupPreferredProviders(group.PreferredProviders)
	if forceAlias {
		group.Alias = forcedAlias
	}
	return group
}

func normalizeLogicalModelGroupPreferredProviders(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeLogicalModelGroupReasoning(reasoning LogicalModelGroupReasoning) LogicalModelGroupReasoning {
	reasoning.Mode = strings.ToLower(strings.TrimSpace(reasoning.Mode))
	reasoning.Effort = strings.TrimSpace(reasoning.Effort)
	if reasoning.Mode != LogicalModelGroupReasoningModeGroup {
		reasoning.Mode = LogicalModelGroupReasoningModeRequest
		reasoning.Effort = ""
	}
	return reasoning
}

func preserveLogicalModelGroupSuffix(resolved string, requestResult thinking.SuffixResult) string {
	resolved = strings.TrimSpace(resolved)
	if resolved == "" {
		return ""
	}
	if thinking.ParseSuffix(resolved).HasSuffix {
		return resolved
	}
	if requestResult.HasSuffix && requestResult.RawSuffix != "" {
		return resolved + "(" + requestResult.RawSuffix + ")"
	}
	return resolved
}

func (cfg *Config) resolveLogicalModelGroupEntry(requestedModel string) (LogicalModelGroup, thinking.SuffixResult, bool) {
	requestedModel = strings.TrimSpace(requestedModel)
	if cfg == nil || requestedModel == "" {
		return LogicalModelGroup{}, thinking.SuffixResult{}, false
	}
	requestResult := thinking.ParseSuffix(requestedModel)
	base := requestResult.ModelName
	if base == "" {
		base = requestedModel
	}
	candidates := []string{base}
	if base != requestedModel {
		candidates = append(candidates, requestedModel)
	}
	for _, candidate := range candidates {
		group, ok := cfg.findLogicalModelGroup(candidate)
		if ok {
			return group, requestResult, true
		}
	}
	return LogicalModelGroup{}, requestResult, false
}
