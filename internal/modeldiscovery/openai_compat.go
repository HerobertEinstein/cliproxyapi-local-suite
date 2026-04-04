package modeldiscovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	"github.com/tidwall/gjson"
)

const (
	openAICompatCacheFileName = "openai-compat-model-cache.json"
	openAICompatUserAgent     = "CLIProxyAPI-openai-compat-discovery"
	openAICompatResponseLimit = 8 << 20
)

var errModelsArrayEmpty = fmt.Errorf("models array is empty")

type OpenAICompatCacheFile struct {
	Providers map[string]OpenAICompatCacheEntry `json:"providers,omitempty"`
}

type OpenAICompatCacheEntry struct {
	Name          string                            `json:"name,omitempty"`
	Kind          string                            `json:"kind,omitempty"`
	ProviderKey   string                            `json:"provider-key,omitempty"`
	ConfigLocator string                            `json:"config-locator,omitempty"`
	BaseURL       string                            `json:"base-url,omitempty"`
	Models        []config.OpenAICompatibilityModel `json:"models,omitempty"`
	Status        string                            `json:"status,omitempty"`
	LastAttemptAt time.Time                         `json:"last-attempt-at,omitempty"`
	RefreshedAt   time.Time                         `json:"refreshed-at,omitempty"`
	SourceURL     string                            `json:"source-url,omitempty"`
	LastError     string                            `json:"last-error,omitempty"`
}

type OpenAICompatStatus struct {
	Name             string    `json:"name"`
	ProviderKey      string    `json:"provider-key"`
	BaseURL          string    `json:"base-url,omitempty"`
	DeclaredModels   []string  `json:"declared-models,omitempty"`
	DiscoveredModels []string  `json:"discovered-models,omitempty"`
	EffectiveModels  []string  `json:"effective-models,omitempty"`
	EffectiveSource  string    `json:"effective-source,omitempty"`
	LastAttemptAt    time.Time `json:"last-attempt-at,omitempty"`
	RefreshedAt      time.Time `json:"refreshed-at,omitempty"`
	SourceURL        string    `json:"source-url,omitempty"`
	LastError        string    `json:"last-error,omitempty"`
}

func NormalizeOpenAICompatProviderKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "openai-compatibility"
	}
	return name
}

func OpenAICompatCacheFilePath(configFilePath string) string {
	dir := openAICompatStateDir(configFilePath)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, openAICompatCacheFileName)
}

func LoadOpenAICompatCache(configFilePath string) (*OpenAICompatCacheFile, error) {
	cachePath := OpenAICompatCacheFilePath(configFilePath)
	if cachePath == "" {
		return &OpenAICompatCacheFile{}, nil
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &OpenAICompatCacheFile{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return &OpenAICompatCacheFile{}, nil
	}
	var cache OpenAICompatCacheFile
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	if cache.Providers == nil {
		cache.Providers = make(map[string]OpenAICompatCacheEntry)
	}
	return &cache, nil
}

func SaveOpenAICompatCache(configFilePath string, cache *OpenAICompatCacheFile) error {
	cachePath := OpenAICompatCacheFilePath(configFilePath)
	if cachePath == "" {
		return fmt.Errorf("empty config file path")
	}
	if cache == nil {
		cache = &OpenAICompatCacheFile{}
	}
	if cache.Providers == nil {
		cache.Providers = make(map[string]OpenAICompatCacheEntry)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(cachePath, data)
}

func EffectiveOpenAICompatModels(cfg *config.Config, compat *config.OpenAICompatibility) []config.OpenAICompatibilityModel {
	if compat == nil {
		return nil
	}
	proxyURL, apiKey, headers := resolveDiscoveryCredentials(compat)
	_ = apiKey
	return effectiveModelsForDescriptor(configPathFromConfig(cfg), providerDescriptor{
		key:            NormalizeOpenAICompatProviderKey(compat.Name),
		kind:           ProviderKindOpenAICompat,
		name:           strings.TrimSpace(compat.Name),
		baseURL:        strings.TrimSpace(compat.BaseURL),
		proxyURL:       proxyURL,
		headers:        headers,
		declaredModels: cloneOpenAICompatModels(compat.Models),
	})
}

func BuildOpenAICompatStatuses(cfg *config.Config) ([]OpenAICompatStatus, error) {
	statuses, err := BuildConfigProviderStatuses(cfg)
	if err != nil {
		return nil, err
	}
	out := make([]OpenAICompatStatus, 0, len(statuses))
	for _, status := range statuses {
		if status.Kind != ProviderKindOpenAICompat {
			continue
		}
		out = append(out, OpenAICompatStatus{
			Name:             status.Name,
			ProviderKey:      status.Key,
			BaseURL:          status.BaseURL,
			DeclaredModels:   append([]string(nil), status.DeclaredModels...),
			DiscoveredModels: append([]string(nil), status.DiscoveredModels...),
			EffectiveModels:  append([]string(nil), status.EffectiveModels...),
			EffectiveSource:  status.EffectiveSource,
			LastAttemptAt:    status.LastAttemptAt,
			RefreshedAt:      status.RefreshedAt,
			SourceURL:        status.SourceURL,
			LastError:        status.LastError,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func RescanOpenAICompatProviders(ctx context.Context, cfg *config.Config, names []string) ([]OpenAICompatStatus, error) {
	keys := make([]string, 0, len(names))
	for _, name := range names {
		if key := NormalizeOpenAICompatProviderKey(name); key != "" {
			keys = append(keys, key)
		}
	}
	all, err := RescanConfigProviders(ctx, cfg, keys)
	if err != nil {
		return nil, err
	}
	out := make([]OpenAICompatStatus, 0, len(all))
	for _, status := range all {
		if status.Kind != ProviderKindOpenAICompat {
			continue
		}
		out = append(out, OpenAICompatStatus{
			Name:             status.Name,
			ProviderKey:      status.Key,
			BaseURL:          status.BaseURL,
			DeclaredModels:   append([]string(nil), status.DeclaredModels...),
			DiscoveredModels: append([]string(nil), status.DiscoveredModels...),
			EffectiveModels:  append([]string(nil), status.EffectiveModels...),
			EffectiveSource:  status.EffectiveSource,
			LastAttemptAt:    status.LastAttemptAt,
			RefreshedAt:      status.RefreshedAt,
			SourceURL:        status.SourceURL,
			LastError:        status.LastError,
		})
	}
	return out, nil
}

func discoverOpenAICompatProvider(ctx context.Context, cfg *config.Config, compat *config.OpenAICompatibility) OpenAICompatCacheEntry {
	now := time.Now().UTC()
	if compat == nil {
		return OpenAICompatCacheEntry{
			LastAttemptAt: now,
			LastError:     "provider config is nil",
		}
	}
	entry := OpenAICompatCacheEntry{
		Name:          strings.TrimSpace(compat.Name),
		ProviderKey:   NormalizeOpenAICompatProviderKey(compat.Name),
		BaseURL:       strings.TrimSpace(compat.BaseURL),
		LastAttemptAt: now,
	}
	if entry.BaseURL == "" {
		entry.LastError = "base-url is empty"
		return entry
	}

	proxyURL, apiKey, headers := resolveDiscoveryCredentials(compat)
	httpClient := newDiscoveryHTTPClient(proxyURL)
	var lastErr error
	for _, candidate := range openAICompatModelsCandidates(entry.BaseURL) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", openAICompatUserAgent)
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
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
			if shouldStopDiscoveryAfterStatus(resp.StatusCode) {
				break
			}
			continue
		}
		models, err := parseOpenAICompatModels(body)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", candidate, err)
			if shouldStopDiscoveryAfterParseError(err) {
				break
			}
			continue
		}
		entry.Models = models
		entry.RefreshedAt = now
		entry.SourceURL = candidate
		entry.LastError = ""
		return entry
	}
	if lastErr != nil {
		entry.LastError = lastErr.Error()
	}
	return entry
}

func parseOpenAICompatModels(data []byte) ([]config.OpenAICompatibilityModel, error) {
	root := gjson.ParseBytes(data)
	var source gjson.Result
	switch {
	case root.IsArray():
		source = root
	case root.Get("data").IsArray():
		source = root.Get("data")
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
		return nil, errModelsArrayEmpty
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func shouldStopDiscoveryAfterStatus(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
}

func shouldStopDiscoveryAfterParseError(err error) bool {
	return errors.Is(err, errModelsArrayEmpty)
}

func openAICompatModelsCandidates(baseURL string) []string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	candidates := make([]string, 0, 3)
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
	if strings.HasSuffix(lower, "/v1") {
		add(baseURL + "/models")
		add(strings.TrimSuffix(baseURL, "/v1") + "/models")
		return candidates
	}
	add(baseURL + "/v1/models")
	add(baseURL + "/models")
	return candidates
}

func resolveDiscoveryCredentials(compat *config.OpenAICompatibility) (proxyURL, apiKey string, headers map[string]string) {
	if compat == nil {
		return "", "", nil
	}
	for i := range compat.APIKeyEntries {
		key := strings.TrimSpace(compat.APIKeyEntries[i].APIKey)
		if key == "" {
			continue
		}
		apiKey = key
		proxyURL = strings.TrimSpace(compat.APIKeyEntries[i].ProxyURL)
		break
	}
	headers = make(map[string]string, len(compat.Headers))
	for key, value := range compat.Headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		headers[key] = value
	}
	return proxyURL, apiKey, headers
}

func modelIDs(models []config.OpenAICompatibilityModel) []string {
	if len(models) == 0 {
		return nil
	}
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for i := range models {
		modelID := strings.TrimSpace(models[i].Alias)
		if modelID == "" {
			modelID = strings.TrimSpace(models[i].Name)
		}
		if modelID == "" {
			continue
		}
		key := strings.ToLower(modelID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, modelID)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func cloneOpenAICompatModels(models []config.OpenAICompatibilityModel) []config.OpenAICompatibilityModel {
	if len(models) == 0 {
		return nil
	}
	out := make([]config.OpenAICompatibilityModel, len(models))
	copy(out, models)
	return out
}

func configPathFromConfig(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.ConfigFilePath)
}

func openAICompatStateDir(configFilePath string) string {
	if base := util.WritablePath(); base != "" {
		return filepath.Join(base, "state")
	}
	configFilePath = strings.TrimSpace(configFilePath)
	if configFilePath == "" {
		return ""
	}
	base := filepath.Dir(configFilePath)
	if info, err := os.Stat(configFilePath); err == nil && info.IsDir() {
		base = configFilePath
	}
	return filepath.Join(base, "state")
}

func atomicWriteFile(path string, data []byte) error {
	tmpFile, err := os.CreateTemp(filepath.Dir(path), "openai-compat-models-*.json")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()
	if _, err := tmpFile.Write(data); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func newDiscoveryHTTPClient(proxyURL string) *http.Client {
	client := &http.Client{Timeout: 20 * time.Second}
	util.SetProxy(&sdkconfig.SDKConfig{ProxyURL: strings.TrimSpace(proxyURL)}, client)
	return client
}
