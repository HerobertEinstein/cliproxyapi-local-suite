package management

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/modeldiscovery"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

const authRuntimeStateFileName = "auth-runtime-state.json"

type providerHealthRuntimeStateFile struct {
	Entries []providerHealthRuntimeStateEntry `json:"entries,omitempty"`
}

type providerHealthRuntimeStateEntry struct {
	AuthID        string                          `json:"auth_id,omitempty"`
	Provider      string                          `json:"provider,omitempty"`
	ConfigLocator string                          `json:"config_locator,omitempty"`
	UpdatedAt     time.Time                       `json:"updated_at,omitempty"`
	ModelStates   map[string]*coreauth.ModelState `json:"model_states,omitempty"`
}

type providerHealthModel struct {
	Model          string `json:"model"`
	Status         string `json:"status,omitempty"`
	StatusMessage  string `json:"status_message,omitempty"`
	RuntimeBlocked bool   `json:"runtime_unavailable"`
	NextRetryAfter string `json:"next_retry_after,omitempty"`
}

type providerHealthProvider struct {
	Key                string                     `json:"key"`
	Kind               string                     `json:"kind,omitempty"`
	Name               string                     `json:"name,omitempty"`
	ConfigLocator      string                     `json:"config_locator,omitempty"`
	AuthIDs            []string                   `json:"auth_ids,omitempty"`
	DiscoveryState     string                     `json:"discovery_state,omitempty"`
	EffectiveModels    []string                   `json:"effective_models,omitempty"`
	RuntimeUnavailable bool                       `json:"runtime_unavailable"`
	ModelHealth        []providerHealthModel      `json:"model_health,omitempty"`
	ImportHealth       *codexImportHealthProvider `json:"import_health,omitempty"`
}

type providerHealthResponse struct {
	Providers []providerHealthProvider `json:"providers"`
}

type providerHealthResetRequest struct {
	Key     string `json:"key"`
	Locator string `json:"locator"`
	AuthID  string `json:"auth_id"`
	Model   string `json:"model"`
}

type providerHealthResetResponse struct {
	MatchedAuths  []string                 `json:"matched_auths"`
	ClearedModels []string                 `json:"cleared_models"`
	Providers     []providerHealthProvider `json:"providers"`
}

type providerHealthRuntimeSource struct {
	key      string
	authID   string
	provider string
	locator  string
	models   []providerHealthModel
	auth     *coreauth.Auth
}

func providerHealthRuntimeStatePath(configFilePath string) string {
	if base := util.WritablePath(); base != "" {
		return filepath.Join(base, "state", authRuntimeStateFileName)
	}
	configFilePath = strings.TrimSpace(configFilePath)
	if configFilePath == "" {
		return ""
	}
	base := filepath.Dir(configFilePath)
	if info, err := os.Stat(configFilePath); err == nil && info.IsDir() {
		base = configFilePath
	}
	parent := filepath.Dir(base)
	if strings.EqualFold(filepath.Base(base), "config") {
		return filepath.Join(parent, "state", authRuntimeStateFileName)
	}
	return filepath.Join(base, "state", authRuntimeStateFileName)
}

func loadProviderHealthRuntimeState(path string) (providerHealthRuntimeStateFile, error) {
	if strings.TrimSpace(path) == "" {
		return providerHealthRuntimeStateFile{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return providerHealthRuntimeStateFile{}, nil
		}
		return providerHealthRuntimeStateFile{}, err
	}
	if len(data) == 0 {
		return providerHealthRuntimeStateFile{}, nil
	}
	var snapshot providerHealthRuntimeStateFile
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return providerHealthRuntimeStateFile{}, err
	}
	return snapshot, nil
}

func saveProviderHealthRuntimeState(path string, snapshot providerHealthRuntimeStateFile) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if len(snapshot.Entries) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func providerHealthBaseModelKey(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if parsed := strings.TrimSpace(thinking.ParseSuffix(model).ModelName); parsed != "" {
		return parsed
	}
	return model
}

func providerHealthCanonicalModelKey(cfg *config.Config, auth *coreauth.Auth, model string) string {
	base := providerHealthBaseModelKey(model)
	if base == "" {
		return ""
	}
	if auth != nil && len(auth.APIKeyModelAliases) > 0 {
		if mapped := strings.TrimSpace(auth.APIKeyModelAliases[strings.ToLower(base)]); mapped != "" {
			base = providerHealthBaseModelKey(mapped)
		}
	}
	if cfg != nil {
		if resolved := strings.TrimSpace(cfg.ResolveLogicalModelGroup(base)); resolved != "" {
			base = providerHealthBaseModelKey(resolved)
		}
	}
	return base
}

func providerHealthModelStates(cfg *config.Config, auth *coreauth.Auth, states map[string]*coreauth.ModelState, now time.Time) []providerHealthModel {
	if len(states) == 0 {
		return nil
	}
	dedup := make(map[string]providerHealthModel)
	for modelID, state := range states {
		if state == nil || !state.Unavailable || state.NextRetryAfter.IsZero() || !state.NextRetryAfter.After(now) {
			continue
		}
		canonical := providerHealthCanonicalModelKey(cfg, auth, modelID)
		if canonical == "" {
			continue
		}
		item := providerHealthModel{
			Model:          canonical,
			Status:         string(state.Status),
			StatusMessage:  strings.TrimSpace(state.StatusMessage),
			RuntimeBlocked: true,
		}
		if !state.NextRetryAfter.IsZero() {
			item.NextRetryAfter = state.NextRetryAfter.UTC().Format(time.RFC3339)
		}
		existing, ok := dedup[canonical]
		if !ok || item.NextRetryAfter > existing.NextRetryAfter {
			dedup[canonical] = item
		}
	}
	if len(dedup) == 0 {
		return nil
	}
	out := make([]providerHealthModel, 0, len(dedup))
	for _, item := range dedup {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Model) < strings.ToLower(out[j].Model)
	})
	return out
}

func providerHealthAuthKey(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Attributes != nil {
		if locator := strings.TrimSpace(auth.Attributes["config_locator"]); locator != "" {
			return locator
		}
		if providerKey := strings.TrimSpace(auth.Attributes["provider_key"]); providerKey != "" {
			return strings.ToLower(providerKey)
		}
		if compatName := strings.TrimSpace(auth.Attributes["compat_name"]); compatName != "" {
			return strings.ToLower(compatName)
		}
	}
	return ""
}

func providerHealthEntryKey(entry providerHealthRuntimeStateEntry) string {
	if locator := strings.TrimSpace(entry.ConfigLocator); locator != "" {
		return locator
	}
	return ""
}

func providerHealthConfigBackedAuth(auth *coreauth.Auth) bool {
	if auth == nil {
		return false
	}
	if auth.Attributes != nil {
		if kind := strings.TrimSpace(auth.Attributes["auth_kind"]); kind != "" && !strings.EqualFold(kind, "apikey") {
			return false
		}
		if source := strings.TrimSpace(auth.Attributes["source"]); source != "" && strings.HasPrefix(strings.ToLower(source), "config:") {
			return true
		}
		if providerHealthAuthKey(auth) != "" {
			return true
		}
	}
	kind, _ := auth.AccountInfo()
	return strings.EqualFold(kind, "api_key") && providerHealthAuthKey(auth) != ""
}

func providerHealthKindFromKey(key, provider string) string {
	if prefix, _, ok := strings.Cut(key, "["); ok {
		return prefix
	}
	if strings.EqualFold(provider, "openai-compatibility") {
		return modeldiscovery.ProviderKindOpenAICompat
	}
	return strings.TrimSpace(provider)
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	return out
}

func providerHealthImportMap(payload *codexImportHealthResponse) map[string]codexImportHealthProvider {
	if payload == nil || len(payload.Providers) == 0 {
		return nil
	}
	out := make(map[string]codexImportHealthProvider, len(payload.Providers))
	for _, item := range payload.Providers {
		locator := strings.TrimSpace(item.ConfigLocator)
		if locator == "" {
			continue
		}
		out[locator] = item
	}
	return out
}

func providerHealthRuntimeSources(cfg *config.Config, manager *coreauth.Manager, snapshot providerHealthRuntimeStateFile) map[string]providerHealthRuntimeSource {
	now := time.Now()
	out := make(map[string]providerHealthRuntimeSource)
	if manager != nil {
		for _, auth := range manager.List() {
			if !providerHealthConfigBackedAuth(auth) {
				continue
			}
			key := providerHealthAuthKey(auth)
			if key == "" {
				continue
			}
			out[key] = providerHealthRuntimeSource{
				key:      key,
				authID:   strings.TrimSpace(auth.ID),
				provider: strings.TrimSpace(auth.Provider),
				locator:  strings.TrimSpace(auth.Attributes["config_locator"]),
				models:   providerHealthModelStates(cfg, auth, auth.ModelStates, now),
				auth:     auth,
			}
		}
	}
	for _, entry := range snapshot.Entries {
		key := providerHealthEntryKey(entry)
		if key == "" {
			continue
		}
		auth := out[key].auth
		entrySource := providerHealthRuntimeSource{
			key:      key,
			authID:   strings.TrimSpace(entry.AuthID),
			provider: strings.TrimSpace(entry.Provider),
			locator:  strings.TrimSpace(entry.ConfigLocator),
			models:   providerHealthModelStates(cfg, auth, entry.ModelStates, now),
		}
		if existing, exists := out[key]; exists {
			existing.models = mergeProviderHealthModels(existing.models, entrySource.models)
			if existing.authID == "" {
				existing.authID = entrySource.authID
			}
			if existing.provider == "" {
				existing.provider = entrySource.provider
			}
			if existing.locator == "" {
				existing.locator = entrySource.locator
			}
			out[key] = existing
			continue
		}
		out[key] = entrySource
	}
	return out
}

func (h *Handler) buildProviderHealth(ctx context.Context) ([]providerHealthProvider, error) {
	statuses, err := modeldiscovery.BuildConfigProviderStatuses(h.cfg)
	if err != nil {
		return nil, err
	}
	runtimePath := providerHealthRuntimeStatePath(h.configFilePath)
	runtimeSnapshot, err := loadProviderHealthRuntimeState(runtimePath)
	if err != nil {
		return nil, err
	}
	importPayload, err := loadCodexImportHealth(h.codexImportHealthPath())
	if err != nil {
		return nil, err
	}
	importByLocator := providerHealthImportMap(importPayload)
	runtimeByKey := providerHealthRuntimeSources(h.cfg, h.authManager, runtimeSnapshot)

	providers := make([]providerHealthProvider, 0, len(statuses))
	seen := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		item := providerHealthProvider{
			Key:             status.Key,
			Kind:            status.Kind,
			Name:            status.Name,
			ConfigLocator:   status.ConfigLocator,
			DiscoveryState:  status.DiscoveryState,
			EffectiveModels: cloneStringSlice(status.EffectiveModels),
		}
		if runtime, ok := runtimeByKey[status.Key]; ok {
			if runtime.authID != "" {
				item.AuthIDs = []string{runtime.authID}
			}
			item.ModelHealth = runtime.models
			item.RuntimeUnavailable = len(runtime.models) > 0
		}
		if importItem, ok := importByLocator[status.ConfigLocator]; ok {
			copyImport := importItem
			item.ImportHealth = &copyImport
		}
		providers = append(providers, item)
		seen[strings.ToLower(status.Key)] = struct{}{}
	}

	for key, runtime := range runtimeByKey {
		if _, exists := seen[strings.ToLower(key)]; exists {
			continue
		}
		item := providerHealthProvider{
			Key:                key,
			Kind:               providerHealthKindFromKey(key, runtime.provider),
			Name:               key,
			ConfigLocator:      runtime.locator,
			RuntimeUnavailable: len(runtime.models) > 0,
			ModelHealth:        runtime.models,
		}
		if runtime.authID != "" {
			item.AuthIDs = []string{runtime.authID}
		}
		if importItem, ok := importByLocator[runtime.locator]; ok {
			copyImport := importItem
			item.ImportHealth = &copyImport
		}
		providers = append(providers, item)
	}

	sort.Slice(providers, func(i, j int) bool {
		left := strings.ToLower(providers[i].Key)
		right := strings.ToLower(providers[j].Key)
		if left != right {
			return left < right
		}
		return strings.ToLower(providers[i].Kind) < strings.ToLower(providers[j].Kind)
	})
	return providers, nil
}

func providerHealthMatchesFilters(provider providerHealthProvider, key, locator, authID, model string) bool {
	if key != "" && !strings.EqualFold(strings.TrimSpace(provider.Key), key) {
		return false
	}
	if locator != "" && !strings.EqualFold(strings.TrimSpace(provider.ConfigLocator), locator) {
		return false
	}
	if authID != "" {
		found := false
		for _, candidate := range provider.AuthIDs {
			if strings.EqualFold(strings.TrimSpace(candidate), authID) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if model != "" {
		for _, item := range provider.ModelHealth {
			if strings.EqualFold(strings.TrimSpace(item.Model), model) {
				return true
			}
		}
		for _, item := range provider.EffectiveModels {
			if strings.EqualFold(providerHealthBaseModelKey(item), model) {
				return true
			}
		}
		return false
	}
	return true
}

func (h *Handler) GetProviderHealth(c *gin.Context) {
	providers, err := h.buildProviderHealth(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	key := strings.TrimSpace(c.Query("key"))
	locator := strings.TrimSpace(c.Query("locator"))
	authID := strings.TrimSpace(c.Query("auth_id"))
	model := providerHealthBaseModelKey(c.Query("model"))
	filtered := make([]providerHealthProvider, 0, len(providers))
	for _, item := range providers {
		if providerHealthMatchesFilters(item, key, locator, authID, model) {
			filtered = append(filtered, item)
		}
	}
	c.JSON(http.StatusOK, providerHealthResponse{Providers: filtered})
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]string, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = trimmed
	}
	out := make([]string, 0, len(seen))
	for _, value := range seen {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func mergeProviderHealthModels(current, extra []providerHealthModel) []providerHealthModel {
	if len(current) == 0 {
		return extra
	}
	if len(extra) == 0 {
		return current
	}
	merged := make(map[string]providerHealthModel, len(current)+len(extra))
	choose := func(items []providerHealthModel) {
		for _, item := range items {
			key := strings.ToLower(strings.TrimSpace(item.Model))
			if key == "" {
				continue
			}
			existing, ok := merged[key]
			if !ok || item.NextRetryAfter > existing.NextRetryAfter {
				merged[key] = item
			}
		}
	}
	choose(current)
	choose(extra)
	out := make([]providerHealthModel, 0, len(merged))
	for _, item := range merged {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Model) < strings.ToLower(out[j].Model)
	})
	return out
}

func (h *Handler) PostProviderHealthReset(c *gin.Context) {
	var body providerHealthResetRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	key := strings.TrimSpace(body.Key)
	locator := strings.TrimSpace(body.Locator)
	authID := strings.TrimSpace(body.AuthID)
	if key == "" && locator == "" && authID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key, locator, or auth_id is required"})
		return
	}

	targetModel := providerHealthBaseModelKey(body.Model)
	matchedAuths := make([]string, 0)
	clearedModels := make([]string, 0)
	authByID := make(map[string]*coreauth.Auth)
	if h.authManager != nil {
		for _, auth := range h.authManager.List() {
			if !providerHealthConfigBackedAuth(auth) {
				continue
			}
			authKey := providerHealthAuthKey(auth)
			authLocator := strings.TrimSpace(auth.Attributes["config_locator"])
			if key != "" && !strings.EqualFold(authKey, key) {
				continue
			}
			if locator != "" && !strings.EqualFold(authLocator, locator) {
				continue
			}
			if authID != "" && !strings.EqualFold(strings.TrimSpace(auth.ID), authID) {
				continue
			}
			authByID[auth.ID] = auth
			matchedAuths = append(matchedAuths, auth.ID)

			modelsToClear := make([]string, 0)
			if targetModel != "" {
				modelsToClear = append(modelsToClear, providerHealthCanonicalModelKey(h.cfg, auth, targetModel))
			} else {
				for modelID := range auth.ModelStates {
					if canonical := providerHealthCanonicalModelKey(h.cfg, auth, modelID); canonical != "" {
						modelsToClear = append(modelsToClear, canonical)
					}
				}
			}
			modelsToClear = uniqueSortedStrings(modelsToClear)
			for _, modelID := range modelsToClear {
				h.authManager.MarkResult(c.Request.Context(), coreauth.Result{
					AuthID:   auth.ID,
					Provider: auth.Provider,
					Model:    modelID,
					Success:  true,
				})
				clearedModels = append(clearedModels, modelID)
			}
		}
	}

	runtimePath := providerHealthRuntimeStatePath(h.configFilePath)
	snapshot, err := loadProviderHealthRuntimeState(runtimePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filteredEntries := make([]providerHealthRuntimeStateEntry, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		entryLocator := strings.TrimSpace(entry.ConfigLocator)
		entryAuthID := strings.TrimSpace(entry.AuthID)
		matchKey := providerHealthEntryKey(entry)
		if key != "" && !strings.EqualFold(matchKey, key) {
			filteredEntries = append(filteredEntries, entry)
			continue
		}
		if locator != "" && !strings.EqualFold(entryLocator, locator) {
			filteredEntries = append(filteredEntries, entry)
			continue
		}
		if authID != "" && !strings.EqualFold(entryAuthID, authID) {
			filteredEntries = append(filteredEntries, entry)
			continue
		}

		models := entry.ModelStates
		if targetModel == "" {
			for modelID := range models {
				auth := authByID[entryAuthID]
				if canonical := providerHealthCanonicalModelKey(h.cfg, auth, modelID); canonical != "" {
					clearedModels = append(clearedModels, canonical)
				}
			}
			continue
		}

		auth := authByID[entryAuthID]
		nextModels := make(map[string]*coreauth.ModelState, len(models))
		for modelID, state := range models {
			canonical := providerHealthCanonicalModelKey(h.cfg, auth, modelID)
			if strings.EqualFold(canonical, targetModel) {
				clearedModels = append(clearedModels, canonical)
				continue
			}
			nextModels[modelID] = state
		}
		if len(nextModels) == 0 {
			continue
		}
		entry.ModelStates = nextModels
		filteredEntries = append(filteredEntries, entry)
	}
	snapshot.Entries = filteredEntries
	if err := saveProviderHealthRuntimeState(runtimePath, snapshot); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.runtimeRefreshHook != nil {
		h.runtimeRefreshHook(c.Request.Context())
	}

	providers, err := h.buildProviderHealth(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, providerHealthResetResponse{
		MatchedAuths:  uniqueSortedStrings(matchedAuths),
		ClearedModels: uniqueSortedStrings(clearedModels),
		Providers:     providers,
	})
}
