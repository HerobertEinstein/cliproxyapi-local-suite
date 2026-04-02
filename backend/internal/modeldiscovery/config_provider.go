package modeldiscovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/tidwall/gjson"
)

const (
	ProviderKindOpenAICompat = "openai-compatibility"
	ProviderKindCodexAPIKey  = "codex-api-key"
	ProviderKindClaudeAPIKey = "claude-api-key"
	ProviderKindGeminiAPIKey = "gemini-api-key"
	ProviderKindVertexAPIKey = "vertex-api-key"
	configProviderRescanConcurrency = 6

	discoveryStateFresh = "fresh"
	discoveryStateStale = "stale"
	discoveryStateError = "error"
)

type ConfigProviderStatus struct {
	Key              string    `json:"key"`
	Kind             string    `json:"kind"`
	Name             string    `json:"name"`
	ConfigLocator    string    `json:"config-locator,omitempty"`
	BaseURL          string    `json:"base-url,omitempty"`
	DeclaredModels   []string  `json:"declared-models,omitempty"`
	DiscoveredModels []string  `json:"discovered-models,omitempty"`
	EffectiveModels  []string  `json:"effective-models,omitempty"`
	EffectiveSource  string    `json:"effective-source,omitempty"`
	DiscoveryState   string    `json:"discovery-state,omitempty"`
	LastAttemptAt    time.Time `json:"last-attempt-at,omitempty"`
	RefreshedAt      time.Time `json:"refreshed-at,omitempty"`
	SourceURL        string    `json:"source-url,omitempty"`
	LastError        string    `json:"last-error,omitempty"`
}

type providerDescriptor struct {
	key            string
	kind           string
	name           string
	configLocator  string
	baseURL        string
	proxyURL       string
	apiKey         string
	headers        map[string]string
	declaredModels []config.OpenAICompatibilityModel
	importModels   []string
}

func BuildConfigProviderStatuses(cfg *config.Config) ([]ConfigProviderStatus, error) {
	if cfg == nil {
		return nil, nil
	}
	cache, err := LoadOpenAICompatCache(configPathFromConfig(cfg))
	if err != nil {
		return nil, err
	}
	descriptors := configProviderDescriptors(cfg)
	statuses := make([]ConfigProviderStatus, 0, len(descriptors))
	for _, desc := range descriptors {
		statuses = append(statuses, buildConfigProviderStatus(cache, desc))
	}
	sortConfigProviderStatuses(statuses)
	return statuses, nil
}

func RescanConfigProviders(ctx context.Context, cfg *config.Config, keys []string) ([]ConfigProviderStatus, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	configPath := configPathFromConfig(cfg)
	if configPath == "" {
		return nil, fmt.Errorf("config file path is empty")
	}
	cache, err := LoadOpenAICompatCache(configPath)
	if err != nil {
		return nil, err
	}
	if cache == nil {
		cache = &OpenAICompatCacheFile{}
	}
	if cache.Providers == nil {
		cache.Providers = make(map[string]OpenAICompatCacheEntry)
	}

	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keySet[strings.ToLower(key)] = struct{}{}
	}

	descriptors := configProviderDescriptors(cfg)
	selected := make([]providerDescriptor, 0, len(descriptors))
	for _, desc := range descriptors {
		if len(keySet) > 0 {
			if _, ok := keySet[strings.ToLower(desc.key)]; !ok {
				continue
			}
		}
		selected = append(selected, desc)
	}

	type rescanResult struct {
		desc  providerDescriptor
		entry OpenAICompatCacheEntry
	}

	results := make([]rescanResult, len(selected))
	limit := configProviderRescanConcurrency
	if len(selected) < limit {
		limit = len(selected)
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup

	for i, desc := range selected {
		existing := cache.Providers[desc.key]
		wg.Add(1)
		go func(index int, descriptor providerDescriptor, cached OpenAICompatCacheEntry) {
			defer wg.Done()
			if sem != nil {
				sem <- struct{}{}
				defer func() { <-sem }()
			}
			results[index] = rescanResult{
				desc:  descriptor,
				entry: discoverProviderDescriptor(ctx, descriptor, cached),
			}
		}(i, desc, existing)
	}
	wg.Wait()

	statuses := make([]ConfigProviderStatus, 0, len(selected))
	for _, result := range results {
		cache.Providers[result.desc.key] = result.entry
		statuses = append(statuses, buildConfigProviderStatus(cache, result.desc))
	}
	if err := SaveOpenAICompatCache(configPath, cache); err != nil {
		return nil, err
	}
	sortConfigProviderStatuses(statuses)
	return statuses, nil
}

func PruneConfigProviderCache(configFilePath string, keys []string) error {
	if strings.TrimSpace(configFilePath) == "" || len(keys) == 0 {
		return nil
	}
	cache, err := LoadOpenAICompatCache(configFilePath)
	if err != nil {
		return err
	}
	if cache == nil || len(cache.Providers) == 0 {
		return nil
	}
	changed := false
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := cache.Providers[key]; exists {
			delete(cache.Providers, key)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return SaveOpenAICompatCache(configFilePath, cache)
}

func EffectiveCodexKeyModels(cfg *config.Config, configLocator string, entry *config.CodexKey) []config.OpenAICompatibilityModel {
	return effectiveModelsForDescriptor(configPathFromConfig(cfg), providerDescriptor{
		key:            strings.TrimSpace(configLocator),
		kind:           ProviderKindCodexAPIKey,
		name:           strings.TrimSpace(configLocator),
		configLocator:  strings.TrimSpace(configLocator),
		baseURL:        strings.TrimSpace(entry.GetBaseURL()),
		proxyURL:       strings.TrimSpace(entry.ProxyURL),
		apiKey:         strings.TrimSpace(entry.APIKey),
		headers:        cloneHeaders(entry.Headers),
		declaredModels: convertDeclaredProviderModels(entry.Models),
		importModels:   codexImportModels(cfg, configLocator),
	})
}

func EffectiveClaudeKeyModels(cfg *config.Config, configLocator string, entry *config.ClaudeKey) []config.OpenAICompatibilityModel {
	return effectiveModelsForDescriptor(configPathFromConfig(cfg), providerDescriptor{
		key:            strings.TrimSpace(configLocator),
		kind:           ProviderKindClaudeAPIKey,
		name:           strings.TrimSpace(configLocator),
		configLocator:  strings.TrimSpace(configLocator),
		baseURL:        strings.TrimSpace(entry.GetBaseURL()),
		proxyURL:       strings.TrimSpace(entry.ProxyURL),
		apiKey:         strings.TrimSpace(entry.APIKey),
		headers:        cloneHeaders(entry.Headers),
		declaredModels: convertDeclaredProviderModels(entry.Models),
	})
}

func EffectiveGeminiKeyModels(cfg *config.Config, configLocator string, entry *config.GeminiKey) []config.OpenAICompatibilityModel {
	return effectiveModelsForDescriptor(configPathFromConfig(cfg), providerDescriptor{
		key:            strings.TrimSpace(configLocator),
		kind:           ProviderKindGeminiAPIKey,
		name:           strings.TrimSpace(configLocator),
		configLocator:  strings.TrimSpace(configLocator),
		baseURL:        strings.TrimSpace(entry.GetBaseURL()),
		proxyURL:       strings.TrimSpace(entry.ProxyURL),
		apiKey:         strings.TrimSpace(entry.APIKey),
		headers:        cloneHeaders(entry.Headers),
		declaredModels: convertDeclaredProviderModels(entry.Models),
	})
}

func EffectiveVertexKeyModels(cfg *config.Config, configLocator string, entry *config.VertexCompatKey) []config.OpenAICompatibilityModel {
	return effectiveModelsForDescriptor(configPathFromConfig(cfg), providerDescriptor{
		key:            strings.TrimSpace(configLocator),
		kind:           ProviderKindVertexAPIKey,
		name:           strings.TrimSpace(configLocator),
		configLocator:  strings.TrimSpace(configLocator),
		baseURL:        strings.TrimSpace(entry.GetBaseURL()),
		proxyURL:       strings.TrimSpace(entry.ProxyURL),
		apiKey:         strings.TrimSpace(entry.APIKey),
		headers:        cloneHeaders(entry.Headers),
		declaredModels: convertDeclaredProviderModels(entry.Models),
	})
}

func configProviderDescriptors(cfg *config.Config) []providerDescriptor {
	if cfg == nil {
		return nil
	}
	out := make([]providerDescriptor, 0, len(cfg.OpenAICompatibility)+len(cfg.CodexKey)+len(cfg.ClaudeKey)+len(cfg.GeminiKey)+len(cfg.VertexCompatAPIKey))
	for i := range cfg.OpenAICompatibility {
		compat := &cfg.OpenAICompatibility[i]
		proxyURL, apiKey, headers := resolveDiscoveryCredentials(compat)
		name := strings.TrimSpace(compat.Name)
		out = append(out, providerDescriptor{
			key:            NormalizeOpenAICompatProviderKey(name),
			kind:           ProviderKindOpenAICompat,
			name:           name,
			baseURL:        strings.TrimSpace(compat.BaseURL),
			proxyURL:       proxyURL,
			apiKey:         apiKey,
			headers:        headers,
			declaredModels: cloneOpenAICompatModels(compat.Models),
		})
	}
	for i := range cfg.CodexKey {
		locator := fmt.Sprintf("%s[%d]", ProviderKindCodexAPIKey, i)
		entry := &cfg.CodexKey[i]
		out = append(out, providerDescriptor{
			key:            locator,
			kind:           ProviderKindCodexAPIKey,
			name:           locator,
			configLocator:  locator,
			baseURL:        strings.TrimSpace(entry.BaseURL),
			proxyURL:       strings.TrimSpace(entry.ProxyURL),
			apiKey:         strings.TrimSpace(entry.APIKey),
			headers:        cloneHeaders(entry.Headers),
			declaredModels: convertDeclaredProviderModels(entry.Models),
			importModels:   codexImportModels(cfg, locator),
		})
	}
	for i := range cfg.ClaudeKey {
		locator := fmt.Sprintf("%s[%d]", ProviderKindClaudeAPIKey, i)
		entry := &cfg.ClaudeKey[i]
		out = append(out, providerDescriptor{
			key:            locator,
			kind:           ProviderKindClaudeAPIKey,
			name:           locator,
			configLocator:  locator,
			baseURL:        strings.TrimSpace(entry.BaseURL),
			proxyURL:       strings.TrimSpace(entry.ProxyURL),
			apiKey:         strings.TrimSpace(entry.APIKey),
			headers:        cloneHeaders(entry.Headers),
			declaredModels: convertDeclaredProviderModels(entry.Models),
		})
	}
	for i := range cfg.GeminiKey {
		locator := fmt.Sprintf("%s[%d]", ProviderKindGeminiAPIKey, i)
		entry := &cfg.GeminiKey[i]
		out = append(out, providerDescriptor{
			key:            locator,
			kind:           ProviderKindGeminiAPIKey,
			name:           locator,
			configLocator:  locator,
			baseURL:        strings.TrimSpace(entry.BaseURL),
			proxyURL:       strings.TrimSpace(entry.ProxyURL),
			apiKey:         strings.TrimSpace(entry.APIKey),
			headers:        cloneHeaders(entry.Headers),
			declaredModels: convertDeclaredProviderModels(entry.Models),
		})
	}
	for i := range cfg.VertexCompatAPIKey {
		locator := fmt.Sprintf("%s[%d]", ProviderKindVertexAPIKey, i)
		entry := &cfg.VertexCompatAPIKey[i]
		out = append(out, providerDescriptor{
			key:            locator,
			kind:           ProviderKindVertexAPIKey,
			name:           locator,
			configLocator:  locator,
			baseURL:        strings.TrimSpace(entry.BaseURL),
			proxyURL:       strings.TrimSpace(entry.ProxyURL),
			apiKey:         strings.TrimSpace(entry.APIKey),
			headers:        cloneHeaders(entry.Headers),
			declaredModels: convertDeclaredProviderModels(entry.Models),
		})
	}
	return out
}

func buildConfigProviderStatus(cache *OpenAICompatCacheFile, desc providerDescriptor) ConfigProviderStatus {
	entry := lookupConfigProviderCacheEntry(cache, desc)
	effectiveModels, effectiveSource := effectiveModelsFromCacheEntry(entry, desc)
	status := ConfigProviderStatus{
		Key:             desc.key,
		Kind:            desc.kind,
		Name:            desc.name,
		ConfigLocator:   desc.configLocator,
		BaseURL:         desc.baseURL,
		DeclaredModels:  modelIDs(desc.declaredModels),
		EffectiveModels: modelIDs(effectiveModels),
		EffectiveSource: effectiveSource,
	}
	if entry != nil {
		status.DiscoveredModels = modelIDs(entry.Models)
		status.DiscoveryState = normalizeDiscoveryState(*entry)
		status.LastAttemptAt = entry.LastAttemptAt
		status.RefreshedAt = entry.RefreshedAt
		status.SourceURL = entry.SourceURL
		status.LastError = entry.LastError
	}
	return status
}

func effectiveModelsForDescriptor(configPath string, desc providerDescriptor) []config.OpenAICompatibilityModel {
	cache, err := LoadOpenAICompatCache(configPath)
	if err != nil {
		cache = &OpenAICompatCacheFile{}
	}
	entry := lookupConfigProviderCacheEntry(cache, desc)
	models, _ := effectiveModelsFromCacheEntry(entry, desc)
	return models
}

func effectiveModelsFromCacheEntry(entry *OpenAICompatCacheEntry, desc providerDescriptor) ([]config.OpenAICompatibilityModel, string) {
	declared := cloneOpenAICompatModels(desc.declaredModels)
	if entry != nil {
		switch normalizeDiscoveryState(*entry) {
		case discoveryStateFresh:
			return overlayDeclaredModels(entry.Models, declared), "discovered"
		case discoveryStateStale:
			return overlayDeclaredModels(entry.Models, declared), "stale-cache"
		}
	}
	if desc.kind == ProviderKindCodexAPIKey && len(desc.importModels) > 0 {
		return overlayDeclaredModels(buildSharedModelsFromNames(desc.importModels), declared), "codex-import"
	}
	if len(declared) > 0 {
		return declared, "declared"
	}
	return nil, "empty"
}

func lookupConfigProviderCacheEntry(cache *OpenAICompatCacheFile, desc providerDescriptor) *OpenAICompatCacheEntry {
	if cache == nil || cache.Providers == nil || desc.key == "" {
		return nil
	}
	entry, ok := cache.Providers[desc.key]
	if !ok {
		return nil
	}
	return &entry
}

func discoverProviderDescriptor(ctx context.Context, desc providerDescriptor, existing OpenAICompatCacheEntry) OpenAICompatCacheEntry {
	now := time.Now().UTC()
	entry := OpenAICompatCacheEntry{
		Name:          desc.name,
		Kind:          desc.kind,
		ProviderKey:   desc.key,
		ConfigLocator: desc.configLocator,
		BaseURL:       desc.baseURL,
		LastAttemptAt: now,
	}
	if desc.baseURL == "" {
		entry.LastError = "base-url is empty"
		return applyStaleFallback(entry, existing)
	}

	httpClient := newDiscoveryHTTPClient(desc.proxyURL)
	var lastErr error
	for _, candidate := range providerModelCandidates(desc.kind, desc.baseURL) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate, nil)
		if err != nil {
			lastErr = err
			continue
		}
		prepareDiscoveryRequest(req, desc)
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", candidate, err)
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, openAICompatResponseLimit))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("%s: read body: %w", candidate, readErr)
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			snippet := strings.TrimSpace(string(body))
			if len(snippet) > 200 {
				snippet = snippet[:200]
			}
			lastErr = fmt.Errorf("%s: status %d: %s", candidate, resp.StatusCode, snippet)
			continue
		}
		models, err := parseProviderModels(desc.kind, body)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", candidate, err)
			continue
		}
		entry.Models = models
		entry.RefreshedAt = now
		entry.SourceURL = candidate
		entry.Status = discoveryStateFresh
		return entry
	}
	if lastErr != nil {
		entry.LastError = lastErr.Error()
	}
	return applyStaleFallback(entry, existing)
}

func applyStaleFallback(entry, existing OpenAICompatCacheEntry) OpenAICompatCacheEntry {
	if len(existing.Models) > 0 {
		entry.Models = cloneOpenAICompatModels(existing.Models)
		entry.RefreshedAt = existing.RefreshedAt
		entry.SourceURL = existing.SourceURL
		entry.Status = discoveryStateStale
		return entry
	}
	entry.Status = discoveryStateError
	return entry
}

func providerModelCandidates(kind, baseURL string) []string {
	switch kind {
	case ProviderKindGeminiAPIKey:
		return geminiModelsCandidates(baseURL)
	default:
		return openAICompatModelsCandidates(baseURL)
	}
}

func geminiModelsCandidates(baseURL string) []string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	candidates := make([]string, 0, 4)
	add := func(url string) {
		url = strings.TrimSpace(url)
		if url == "" {
			return
		}
		for _, existing := range candidates {
			if strings.EqualFold(existing, url) {
				return
			}
		}
		candidates = append(candidates, url)
	}
	lower := strings.ToLower(baseURL)
	if strings.HasSuffix(lower, "/models") {
		add(baseURL)
		return candidates
	}
	if strings.HasSuffix(lower, "/v1beta") || strings.HasSuffix(lower, "/v1") {
		add(baseURL + "/models")
	}
	add(baseURL + "/v1beta/models")
	add(baseURL + "/v1/models")
	add(baseURL + "/models")
	return candidates
}

func prepareDiscoveryRequest(req *http.Request, desc providerDescriptor) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", openAICompatUserAgent)
	switch desc.kind {
	case ProviderKindGeminiAPIKey:
		if desc.apiKey != "" {
			req.Header.Set("x-goog-api-key", desc.apiKey)
			req.Header.Del("Authorization")
		}
	case ProviderKindClaudeAPIKey:
		if desc.apiKey != "" && req.URL != nil && strings.EqualFold(req.URL.Host, "api.anthropic.com") {
			req.Header.Set("x-api-key", desc.apiKey)
			req.Header.Del("Authorization")
		} else if desc.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+desc.apiKey)
			req.Header.Del("x-api-key")
		}
	default:
		if desc.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+desc.apiKey)
		}
	}
	for key, value := range desc.headers {
		req.Header.Set(key, value)
	}
}

func parseProviderModels(kind string, data []byte) ([]config.OpenAICompatibilityModel, error) {
	root := gjson.ParseBytes(data)
	var source gjson.Result
	switch {
	case root.IsArray():
		source = root
	case root.Get("data").IsArray():
		source = root.Get("data")
	case root.Get("models").IsArray():
		source = root.Get("models")
	default:
		return nil, fmt.Errorf("models payload missing array")
	}
	seen := make(map[string]struct{})
	out := make([]config.OpenAICompatibilityModel, 0, len(source.Array()))
	for _, item := range source.Array() {
		modelID := strings.TrimSpace(item.Get("id").String())
		if modelID == "" {
			modelID = strings.TrimSpace(item.Get("name").String())
		}
		if modelID == "" && item.Type == gjson.String {
			modelID = strings.TrimSpace(item.String())
		}
		modelID = normalizeDiscoveredModelID(kind, modelID)
		if modelID == "" {
			continue
		}
		key := strings.ToLower(modelID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		model := config.OpenAICompatibilityModel{Name: modelID}
		if static := registry.LookupStaticModelInfo(modelID); static != nil && static.Thinking != nil {
			model.Thinking = static.Thinking
		}
		out = append(out, model)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("models array is empty")
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func normalizeDiscoveredModelID(kind, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "/models/") {
		value = value[strings.LastIndex(value, "/models/")+len("/models/"):]
	}
	if strings.HasPrefix(value, "models/") {
		value = strings.TrimPrefix(value, "models/")
	}
	return strings.TrimSpace(value)
}

func overlayDeclaredModels(base, declared []config.OpenAICompatibilityModel) []config.OpenAICompatibilityModel {
	if len(base) == 0 {
		return cloneOpenAICompatModels(declared)
	}
	out := cloneOpenAICompatModels(base)
	supportedTargets := make(map[string]int, len(out))
	seenIDs := make(map[string]struct{}, len(out))
	for i := range out {
		target := modelTarget(out[i])
		if target != "" {
			if _, exists := supportedTargets[strings.ToLower(target)]; !exists {
				supportedTargets[strings.ToLower(target)] = i
			}
		}
		id := effectiveModelID(out[i])
		if id != "" {
			seenIDs[strings.ToLower(id)] = struct{}{}
		}
	}
	for i := range declared {
		target := modelTarget(declared[i])
		if target == "" {
			continue
		}
		baseIndex, ok := supportedTargets[strings.ToLower(target)]
		if !ok {
			continue
		}
		if declared[i].Thinking != nil && out[baseIndex].Thinking == nil {
			out[baseIndex].Thinking = declared[i].Thinking
		}
		alias := strings.TrimSpace(declared[i].Alias)
		if alias == "" || strings.EqualFold(alias, target) {
			continue
		}
		aliasKey := strings.ToLower(alias)
		if _, exists := seenIDs[aliasKey]; exists {
			continue
		}
		model := config.OpenAICompatibilityModel{
			Name:     target,
			Alias:    alias,
			Thinking: declared[i].Thinking,
		}
		if model.Thinking == nil {
			if static := registry.LookupStaticModelInfo(target); static != nil && static.Thinking != nil {
				model.Thinking = static.Thinking
			}
		}
		out = append(out, model)
		seenIDs[aliasKey] = struct{}{}
	}
	return out
}

func convertDeclaredProviderModels[T interface {
	GetName() string
	GetAlias() string
}](models []T) []config.OpenAICompatibilityModel {
	if len(models) == 0 {
		return nil
	}
	out := make([]config.OpenAICompatibilityModel, 0, len(models))
	for i := range models {
		name := normalizeDiscoveredModelID("", models[i].GetName())
		alias := strings.TrimSpace(models[i].GetAlias())
		if name == "" && alias == "" {
			continue
		}
		model := config.OpenAICompatibilityModel{Name: name, Alias: alias}
		if name != "" {
			if static := registry.LookupStaticModelInfo(name); static != nil && static.Thinking != nil {
				model.Thinking = static.Thinking
			}
		}
		out = append(out, model)
	}
	return out
}

func buildSharedModelsFromNames(models []string) []config.OpenAICompatibilityModel {
	if len(models) == 0 {
		return nil
	}
	out := make([]config.OpenAICompatibilityModel, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		name := normalizeDiscoveredModelID("", model)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		item := config.OpenAICompatibilityModel{Name: name}
		if static := registry.LookupStaticModelInfo(name); static != nil && static.Thinking != nil {
			item.Thinking = static.Thinking
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func modelTarget(model config.OpenAICompatibilityModel) string {
	if name := strings.TrimSpace(model.Name); name != "" {
		return normalizeDiscoveredModelID("", name)
	}
	return strings.TrimSpace(model.Alias)
}

func effectiveModelID(model config.OpenAICompatibilityModel) string {
	if alias := strings.TrimSpace(model.Alias); alias != "" {
		return alias
	}
	return strings.TrimSpace(model.Name)
}

func normalizeDiscoveryState(entry OpenAICompatCacheEntry) string {
	status := strings.ToLower(strings.TrimSpace(entry.Status))
	if status != "" {
		return status
	}
	if len(entry.Models) > 0 {
		if entry.LastError != "" && !entry.LastAttemptAt.IsZero() && (entry.RefreshedAt.IsZero() || entry.LastAttemptAt.After(entry.RefreshedAt)) {
			return discoveryStateStale
		}
		return discoveryStateFresh
	}
	if entry.LastError != "" {
		return discoveryStateError
	}
	return ""
}

func sortConfigProviderStatuses(statuses []ConfigProviderStatus) {
	sort.Slice(statuses, func(i, j int) bool {
		leftKind := strings.ToLower(statuses[i].Kind)
		rightKind := strings.ToLower(statuses[j].Kind)
		if leftKind != rightKind {
			return leftKind < rightKind
		}
		leftName := strings.ToLower(statuses[i].Name)
		rightName := strings.ToLower(statuses[j].Name)
		if leftName != rightName {
			return leftName < rightName
		}
		return strings.ToLower(statuses[i].Key) < strings.ToLower(statuses[j].Key)
	})
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func codexImportModels(cfg *config.Config, locator string) []string {
	if cfg == nil {
		return nil
	}
	models, _ := CodexImportEffectiveModelsByLocator(configPathFromConfig(cfg), locator)
	return models
}
