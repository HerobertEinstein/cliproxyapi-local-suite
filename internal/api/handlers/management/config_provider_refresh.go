package management

import (
	"fmt"
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/modeldiscovery"
)

type configProviderRefreshPlan struct {
	RescanKeys []string
	PruneKeys  []string
}

func geminiConfigProviderRefreshKeys(oldEntries, newEntries []config.GeminiKey) []string {
	return geminiConfigProviderRefreshPlan(oldEntries, newEntries).RescanKeys
}

func claudeConfigProviderRefreshKeys(oldEntries, newEntries []config.ClaudeKey) []string {
	return claudeConfigProviderRefreshPlan(oldEntries, newEntries).RescanKeys
}

func vertexConfigProviderRefreshKeys(oldEntries, newEntries []config.VertexCompatKey) []string {
	return vertexConfigProviderRefreshPlan(oldEntries, newEntries).RescanKeys
}

func codexConfigProviderRefreshKeys(oldEntries, newEntries []config.CodexKey) []string {
	return codexConfigProviderRefreshPlan(oldEntries, newEntries).RescanKeys
}

func openAICompatConfigProviderRefreshKeys(oldEntries, newEntries []config.OpenAICompatibility) []string {
	return openAICompatConfigProviderRefreshPlan(oldEntries, newEntries).RescanKeys
}

func geminiConfigProviderRefreshPlan(oldEntries, newEntries []config.GeminiKey) configProviderRefreshPlan {
	return indexedConfigProviderRefreshPlan(modeldiscovery.ProviderKindGeminiAPIKey, oldEntries, newEntries, sameGeminiDiscoveryInputs)
}

func claudeConfigProviderRefreshPlan(oldEntries, newEntries []config.ClaudeKey) configProviderRefreshPlan {
	return indexedConfigProviderRefreshPlan(modeldiscovery.ProviderKindClaudeAPIKey, oldEntries, newEntries, sameClaudeDiscoveryInputs)
}

func vertexConfigProviderRefreshPlan(oldEntries, newEntries []config.VertexCompatKey) configProviderRefreshPlan {
	return indexedConfigProviderRefreshPlan(modeldiscovery.ProviderKindVertexAPIKey, oldEntries, newEntries, sameVertexDiscoveryInputs)
}

func codexConfigProviderRefreshPlan(oldEntries, newEntries []config.CodexKey) configProviderRefreshPlan {
	return indexedConfigProviderRefreshPlan(modeldiscovery.ProviderKindCodexAPIKey, oldEntries, newEntries, sameCodexDiscoveryInputs)
}

func openAICompatConfigProviderRefreshPlan(oldEntries, newEntries []config.OpenAICompatibility) configProviderRefreshPlan {
	if len(newEntries) == 0 {
		return configProviderRefreshPlan{PruneKeys: openAICompatRemovedKeys(oldEntries, newEntries)}
	}

	oldByName := make(map[string]config.OpenAICompatibility, len(oldEntries))
	for i := range oldEntries {
		key := modeldiscovery.NormalizeOpenAICompatProviderKey(oldEntries[i].Name)
		if key == "" {
			continue
		}
		if _, exists := oldByName[key]; exists {
			continue
		}
		oldByName[key] = oldEntries[i]
	}

	keys := make([]string, 0, len(newEntries))
	seen := make(map[string]struct{}, len(newEntries))
	for i := range newEntries {
		key := modeldiscovery.NormalizeOpenAICompatProviderKey(newEntries[i].Name)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		oldEntry, exists := oldByName[key]
		if !exists || !sameOpenAICompatDiscoveryInputs(oldEntry, newEntries[i]) {
			keys = append(keys, key)
		}
	}
	return configProviderRefreshPlan{
		RescanKeys: keys,
		PruneKeys:  openAICompatRemovedKeys(oldEntries, newEntries),
	}
}

func indexedConfigProviderRefreshKeys[T any](kind string, oldEntries, newEntries []T, same func(T, T) bool) []string {
	return indexedConfigProviderRefreshPlan(kind, oldEntries, newEntries, same).RescanKeys
}

func indexedConfigProviderRefreshPlan[T any](kind string, oldEntries, newEntries []T, same func(T, T) bool) configProviderRefreshPlan {
	if len(newEntries) == 0 {
		return configProviderRefreshPlan{PruneKeys: indexedProviderKeys(kind, 0, len(oldEntries))}
	}

	maxCommon := len(oldEntries)
	if len(newEntries) < maxCommon {
		maxCommon = len(newEntries)
	}

	firstDiff := -1
	for i := 0; i < maxCommon; i++ {
		if same(oldEntries[i], newEntries[i]) {
			continue
		}
		firstDiff = i
		break
	}

	if firstDiff == -1 {
		if len(newEntries) <= len(oldEntries) {
			return configProviderRefreshPlan{}
		}
		firstDiff = len(oldEntries)
	}

	if firstDiff >= len(newEntries) {
		return configProviderRefreshPlan{
			PruneKeys: indexedProviderKeys(kind, len(newEntries), len(oldEntries)),
		}
	}

	keys := make([]string, 0, len(newEntries)-firstDiff)
	for i := firstDiff; i < len(newEntries); i++ {
		keys = append(keys, fmt.Sprintf("%s[%d]", kind, i))
	}
	return configProviderRefreshPlan{
		RescanKeys: keys,
		PruneKeys:  indexedProviderKeys(kind, len(newEntries), len(oldEntries)),
	}
}

func indexedProviderKeys(kind string, start, end int) []string {
	if start < 0 {
		start = 0
	}
	if end <= start {
		return nil
	}
	keys := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		keys = append(keys, fmt.Sprintf("%s[%d]", kind, i))
	}
	return keys
}

func openAICompatRemovedKeys(oldEntries, newEntries []config.OpenAICompatibility) []string {
	if len(oldEntries) == 0 {
		return nil
	}
	newKeys := make(map[string]struct{}, len(newEntries))
	for i := range newEntries {
		key := modeldiscovery.NormalizeOpenAICompatProviderKey(newEntries[i].Name)
		if key == "" {
			continue
		}
		newKeys[key] = struct{}{}
	}
	removed := make([]string, 0, len(oldEntries))
	seen := make(map[string]struct{}, len(oldEntries))
	for i := range oldEntries {
		key := modeldiscovery.NormalizeOpenAICompatProviderKey(oldEntries[i].Name)
		if key == "" {
			continue
		}
		if _, exists := newKeys[key]; exists {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		removed = append(removed, key)
	}
	sort.Strings(removed)
	return removed
}

func sameGeminiDiscoveryInputs(a, b config.GeminiKey) bool {
	return sameAPIKeyProviderDiscoveryInputs(
		strings.TrimSpace(a.APIKey),
		strings.TrimSpace(b.APIKey),
		strings.TrimSpace(a.BaseURL),
		strings.TrimSpace(b.BaseURL),
		strings.TrimSpace(a.ProxyURL),
		strings.TrimSpace(b.ProxyURL),
		a.Headers,
		b.Headers,
	)
}

func sameClaudeDiscoveryInputs(a, b config.ClaudeKey) bool {
	return sameAPIKeyProviderDiscoveryInputs(
		strings.TrimSpace(a.APIKey),
		strings.TrimSpace(b.APIKey),
		strings.TrimSpace(a.BaseURL),
		strings.TrimSpace(b.BaseURL),
		strings.TrimSpace(a.ProxyURL),
		strings.TrimSpace(b.ProxyURL),
		a.Headers,
		b.Headers,
	)
}

func sameVertexDiscoveryInputs(a, b config.VertexCompatKey) bool {
	return sameAPIKeyProviderDiscoveryInputs(
		strings.TrimSpace(a.APIKey),
		strings.TrimSpace(b.APIKey),
		strings.TrimSpace(a.BaseURL),
		strings.TrimSpace(b.BaseURL),
		strings.TrimSpace(a.ProxyURL),
		strings.TrimSpace(b.ProxyURL),
		a.Headers,
		b.Headers,
	)
}

func sameCodexDiscoveryInputs(a, b config.CodexKey) bool {
	return sameAPIKeyProviderDiscoveryInputs(
		strings.TrimSpace(a.APIKey),
		strings.TrimSpace(b.APIKey),
		strings.TrimSpace(a.BaseURL),
		strings.TrimSpace(b.BaseURL),
		strings.TrimSpace(a.ProxyURL),
		strings.TrimSpace(b.ProxyURL),
		a.Headers,
		b.Headers,
	)
}

func sameAPIKeyProviderDiscoveryInputs(apiKeyA, apiKeyB, baseURLA, baseURLB, proxyURLA, proxyURLB string, headersA, headersB map[string]string) bool {
	return apiKeyA == apiKeyB &&
		strings.EqualFold(baseURLA, baseURLB) &&
		strings.EqualFold(proxyURLA, proxyURLB) &&
		equalNormalizedHeaders(headersA, headersB)
}

func sameOpenAICompatDiscoveryInputs(a, b config.OpenAICompatibility) bool {
	return strings.EqualFold(strings.TrimSpace(a.BaseURL), strings.TrimSpace(b.BaseURL)) &&
		equalNormalizedHeaders(a.Headers, b.Headers) &&
		equalOpenAICompatAPIKeyEntries(a.APIKeyEntries, b.APIKeyEntries)
}

func equalNormalizedHeaders(a, b map[string]string) bool {
	normalizedA := normalizeHeadersForComparison(a)
	normalizedB := normalizeHeadersForComparison(b)
	if len(normalizedA) != len(normalizedB) {
		return false
	}
	for key, value := range normalizedA {
		if normalizedB[key] != value {
			return false
		}
	}
	return true
}

func normalizeHeadersForComparison(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		normalized[key] = value
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func equalOpenAICompatAPIKeyEntries(a, b []config.OpenAICompatibilityAPIKey) bool {
	left := normalizeOpenAICompatAPIKeyEntries(a)
	right := normalizeOpenAICompatAPIKeyEntries(b)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func normalizeOpenAICompatAPIKeyEntries(entries []config.OpenAICompatibilityAPIKey) []string {
	if len(entries) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(entries))
	for i := range entries {
		apiKey := strings.TrimSpace(entries[i].APIKey)
		proxyURL := strings.TrimSpace(entries[i].ProxyURL)
		if apiKey == "" && proxyURL == "" {
			continue
		}
		normalized = append(normalized, apiKey+"|"+proxyURL)
	}
	if len(normalized) == 0 {
		return nil
	}
	sort.Strings(normalized)
	return normalized
}
