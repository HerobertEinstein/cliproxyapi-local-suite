package cliproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/modeldiscovery"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

func TestRegisterModelsForAuth_APIKeyLogicalModelGroups_RegisterCurrentWhenCompatible(t *testing.T) {
	service := &Service{
		cfg: &config.Config{
			CodexKey: []config.CodexKey{
				{
					APIKey: "k",
					Models: []internalconfig.CodexModel{
						{Name: "gpt-5.4"},
					},
				},
			},
			LogicalModelGroups: config.LogicalModelGroups{
				Current: config.LogicalModelCurrent{Ref: "gpt-5.4"},
				Static: []config.LogicalModelGroup{
					{Alias: "gpt-5.4", Target: "gpt-5.4"},
				},
			},
		},
	}
	service.cfg.SanitizeLogicalModelGroups()

	auth := &coreauth.Auth{
		ID:       "codex-apikey-logical-current",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind": "apikey",
			"api_key":   "k",
		},
	}

	registry := GlobalModelRegistry()
	registry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	if !registry.ClientSupportsModel(auth.ID, "current") {
		t.Fatal("expected api key auth to register logical current alias when target model is supported")
	}

	models := registry.GetAvailableModelsByProvider("codex")
	foundCurrent := false
	for _, model := range models {
		if model == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(model.ID), "current") {
			foundCurrent = true
			break
		}
	}
	if !foundCurrent {
		t.Fatal("expected codex provider model list to include logical current alias")
	}
}

func TestRegisterModelsForAuth_APIKeyLogicalModelGroups_SkipCurrentWhenIncompatible(t *testing.T) {
	service := &Service{
		cfg: &config.Config{
			CodexKey: []config.CodexKey{
				{
					APIKey: "k",
					Models: []internalconfig.CodexModel{
						{Name: "gpt-5.4"},
					},
				},
			},
			LogicalModelGroups: config.LogicalModelGroups{
				Current: config.LogicalModelCurrent{Ref: "gpt-5.2"},
				Static: []config.LogicalModelGroup{
					{Alias: "gpt-5.2", Target: "gpt-5.2"},
				},
			},
		},
	}
	service.cfg.SanitizeLogicalModelGroups()

	auth := &coreauth.Auth{
		ID:       "codex-apikey-logical-skip",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind": "apikey",
			"api_key":   "k",
		},
	}

	registry := GlobalModelRegistry()
	registry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	if registry.ClientSupportsModel(auth.ID, "current") {
		t.Fatal("did not expect api key auth to register logical current alias when target model is unsupported")
	}
}

func TestRefreshAllModelRegistrations_LogicalCurrentSwitchesTargetOnConfigReload(t *testing.T) {
	service := &Service{
		cfg: &config.Config{
			CodexKey: []config.CodexKey{
				{
					APIKey: "k1",
					Models: []internalconfig.CodexModel{
						{Name: "gpt-5.2"},
					},
				},
				{
					APIKey: "k2",
					Models: []internalconfig.CodexModel{
						{Name: "gpt-5.4"},
					},
				},
			},
			LogicalModelGroups: config.LogicalModelGroups{
				Current: config.LogicalModelCurrent{Ref: "gpt-5.2"},
				Static: []config.LogicalModelGroup{
					{Alias: "gpt-5.2", Target: "gpt-5.2"},
					{Alias: "gpt-5.4", Target: "gpt-5.4"},
				},
			},
		},
		coreManager: coreauth.NewManager(nil, nil, nil),
	}
	service.cfg.SanitizeLogicalModelGroups()
	service.coreManager.SetConfig(service.cfg)

	auth52 := &coreauth.Auth{
		ID:       "codex-current-52",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind": "apikey",
			"api_key":   "k1",
		},
	}
	auth54 := &coreauth.Auth{
		ID:       "codex-current-54",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind": "apikey",
			"api_key":   "k2",
		},
	}

	ctx := context.Background()
	if _, err := service.coreManager.Register(ctx, auth52); err != nil {
		t.Fatalf("register auth52: %v", err)
	}
	if _, err := service.coreManager.Register(ctx, auth54); err != nil {
		t.Fatalf("register auth54: %v", err)
	}

	registry := GlobalModelRegistry()
	registry.UnregisterClient(auth52.ID)
	registry.UnregisterClient(auth54.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth52.ID)
		registry.UnregisterClient(auth54.ID)
	})

	service.refreshAllModelRegistrations()

	if !registry.ClientSupportsModel(auth52.ID, "current") {
		t.Fatal("expected current to point at gpt-5.2 auth before config reload")
	}
	if registry.ClientSupportsModel(auth54.ID, "current") {
		t.Fatal("did not expect current to point at gpt-5.4 auth before config reload")
	}

	service.cfg.LogicalModelGroups.Current.Ref = "gpt-5.4"
	service.cfg.SanitizeLogicalModelGroups()
	service.coreManager.SetConfig(service.cfg)
	service.refreshAllModelRegistrations()

	if registry.ClientSupportsModel(auth52.ID, "current") {
		t.Fatal("did not expect current to remain on gpt-5.2 auth after config reload")
	}
	if !registry.ClientSupportsModel(auth54.ID, "current") {
		t.Fatal("expected current to move to gpt-5.4 auth after config reload")
	}
}

func TestServiceRun_LogicalModelGroupMutationRefreshesModelsImmediately(t *testing.T) {
	port := reserveTCPPort(t)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	authDir := filepath.Join(tmpDir, "auth")
	t.Setenv("MANAGEMENT_PASSWORD", "local-password")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("port: 0\n"), 0o600); err != nil {
		t.Fatalf("seed config file: %v", err)
	}
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			APIKeys: []string{"client-key"},
		},
		Host:                   "127.0.0.1",
		Port:                   port,
		AuthDir:                authDir,
		Debug:                  true,
		LoggingToFile:          false,
		UsageStatisticsEnabled: false,
		CodexKey: []config.CodexKey{
			{
				APIKey: "codex-key",
				Models: []internalconfig.CodexModel{
					{Name: "gpt-5.4"},
				},
			},
		},
		LogicalModelGroups: config.LogicalModelGroups{
			Current: config.LogicalModelCurrent{Ref: "gpt-5.4"},
			Static: []config.LogicalModelGroup{
				{Alias: "gpt-5.4", Target: "gpt-5.4"},
			},
		},
	}
	cfg.SanitizeLogicalModelGroups()
	if err := config.SaveConfigPreserveComments(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	service, err := NewBuilder().
		WithConfig(cfg).
		WithConfigPath(configPath).
		WithLocalManagementPassword("local-password").
		Build()
	if err != nil {
		t.Fatalf("build service: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = service.Run(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHTTPStatus(t, baseURL+"/healthz", nil, http.StatusOK)
	waitForModelID(t, baseURL, "client-key", "current")

	alias := "tmp-runtime-refresh-integration"
	t.Cleanup(func() {
		req, err := http.NewRequest(http.MethodDelete, baseURL+"/v0/management/logical-model-groups/static/"+alias, nil)
		if err != nil {
			return
		}
		req.Header.Set("Authorization", "Bearer local-password")
		_, _ = http.DefaultClient.Do(req)
	})

	body := strings.NewReader(`{"alias":"` + alias + `","target":"gpt-5.4","reasoning":{"mode":"request"}}`)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v0/management/logical-model-groups/static", body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer local-password")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post logical group: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from management mutation, got %d", resp.StatusCode)
	}

	models := fetchModelIDs(t, baseURL, "client-key")
	if !containsString(models, alias) {
		t.Fatalf("expected /v1/models to include %q immediately after mutation, got %#v", alias, models)
	}
}

func TestRegisterModelsForAuth_APIKeyLogicalModelGroups_PrefersConfigLocatorForDuplicateCodexKeys(t *testing.T) {
	service := &Service{
		cfg: &config.Config{
			CodexKey: []config.CodexKey{
				{
					APIKey:  "shared-key",
					BaseURL: "https://example.com/v1",
					Models: []internalconfig.CodexModel{
						{Name: "gpt-5.2"},
					},
				},
				{
					APIKey:  "shared-key",
					BaseURL: "https://example.com/v1",
					Models: []internalconfig.CodexModel{
						{Name: "gpt-5.4-mini"},
					},
				},
			},
			LogicalModelGroups: config.LogicalModelGroups{
				Current: config.LogicalModelCurrent{Ref: "gpt-5.4-mini"},
				Static: []config.LogicalModelGroup{
					{Alias: "gpt-5.2", Target: "gpt-5.2"},
					{Alias: "gpt-5.4-mini", Target: "gpt-5.4-mini"},
				},
			},
		},
	}
	service.cfg.SanitizeLogicalModelGroups()

	auth := &coreauth.Auth{
		ID:       "codex-duplicate-config-second",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind":      "apikey",
			"api_key":        "shared-key",
			"base_url":       "https://example.com/v1",
			"config_locator": "codex-api-key[1]",
		},
	}

	registry := GlobalModelRegistry()
	registry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	if !registry.ClientSupportsModel(auth.ID, "gpt-5.4-mini") {
		t.Fatal("expected second duplicated auth to register its own concrete model")
	}
	if !registry.ClientSupportsModel(auth.ID, "current") {
		t.Fatal("expected second duplicated auth to register logical current alias")
	}
	if registry.ClientSupportsModel(auth.ID, "gpt-5.2") {
		t.Fatal("did not expect second duplicated auth to inherit the first entry model")
	}
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener addr type %T", listener.Addr())
	}
	return addr.Port
}

func waitForHTTPStatus(t *testing.T, url string, headers map[string]string, expected int) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	var lastStatus int
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			lastStatus = resp.StatusCode
			resp.Body.Close()
			if resp.StatusCode == expected {
				return
			}
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s status %d (last status=%d err=%v)", url, expected, lastStatus, lastErr)
}

func waitForModelID(t *testing.T, baseURL, apiKey, want string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	var last []string
	for time.Now().Before(deadline) {
		last = fetchModelIDs(t, baseURL, apiKey)
		if containsString(last, want) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for model %q, last models=%#v", want, last)
}

func fetchModelIDs(t *testing.T, baseURL, apiKey string) []string {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/models", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get models: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get models status=%d", resp.StatusCode)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	ids := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if strings.TrimSpace(item.ID) != "" {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func TestRegisterModelsForAuth_OpenAICompatibilityEmptyModels_UsesLogicalModelGroups(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := modeldiscovery.SaveOpenAICompatCache(configPath, &modeldiscovery.OpenAICompatCacheFile{
		Providers: map[string]modeldiscovery.OpenAICompatCacheEntry{
			"pool": {
				Name:        "pool",
				ProviderKey: "pool",
				Models: []config.OpenAICompatibilityModel{
					{Name: "claude-opus-4-6"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save discovery cache: %v", err)
	}

	service := &Service{
		cfg: &config.Config{
			ConfigFilePath: configPath,
			OpenAICompatibility: []config.OpenAICompatibility{
				{
					Name:    "pool",
					BaseURL: "https://example.com",
				},
			},
			LogicalModelGroups: config.LogicalModelGroups{
				Current: config.LogicalModelCurrent{Ref: "claude-opus-4-6"},
				Static: []config.LogicalModelGroup{
					{Alias: "claude-opus-4-6", Target: "claude-opus-4-6"},
				},
			},
		},
	}
	service.cfg.SanitizeLogicalModelGroups()

	auth := &coreauth.Auth{
		ID:       "pool-apikey-logical-current",
		Provider: "pool",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind":    "apikey",
			"api_key":      "k",
			"compat_name":  "pool",
			"provider_key": "pool",
		},
	}

	registry := GlobalModelRegistry()
	registry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	if !registry.ClientSupportsModel(auth.ID, "current") {
		t.Fatal("expected openai-compat auth with empty models to register logical current alias")
	}
	if !registry.ClientSupportsModel(auth.ID, "claude-opus-4-6") {
		t.Fatal("expected openai-compat auth with empty models to register logical static alias")
	}

	models := registry.GetAvailableModelsByProvider("pool")
	foundCurrent := false
	for _, model := range models {
		if model == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(model.ID), "current") {
			foundCurrent = true
			break
		}
	}
	if !foundCurrent {
		t.Fatal("expected openai-compat provider model list to include logical current alias")
	}
}

func TestRegisterModelsForAuth_CodexDiscoveryOverridesDeclaredModelsAndKeepsAliases(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := modeldiscovery.SaveOpenAICompatCache(configPath, &modeldiscovery.OpenAICompatCacheFile{
		Providers: map[string]modeldiscovery.OpenAICompatCacheEntry{
			"codex-api-key[0]": {
				Name:        "codex-api-key[0]",
				ProviderKey: "codex-api-key[0]",
				Models: []config.OpenAICompatibilityModel{
					{Name: "gpt-5.4"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save discovery cache: %v", err)
	}

	service := &Service{
		cfg: &config.Config{
			ConfigFilePath: configPath,
			CodexKey: []config.CodexKey{
				{
					APIKey:  "k",
					BaseURL: "https://example.com/v1",
					Models: []internalconfig.CodexModel{
						{Name: "gpt-5.3"},
						{Name: "gpt-5.4", Alias: "gpt-5.4-fast"},
					},
				},
			},
			LogicalModelGroups: config.LogicalModelGroups{
				Current: config.LogicalModelCurrent{Ref: "gpt-5.4-fast"},
				Static: []config.LogicalModelGroup{
					{Alias: "gpt-5.4-fast", Target: "gpt-5.4-fast"},
				},
			},
		},
	}
	service.cfg.SanitizeLogicalModelGroups()

	auth := &coreauth.Auth{
		ID:       "codex-discovery-overrides-declared",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind":      "apikey",
			"api_key":        "k",
			"base_url":       "https://example.com/v1",
			"config_locator": "codex-api-key[0]",
		},
	}

	registry := GlobalModelRegistry()
	registry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	if !registry.ClientSupportsModel(auth.ID, "gpt-5.4") {
		t.Fatal("expected discovered base model to stay available")
	}
	if !registry.ClientSupportsModel(auth.ID, "gpt-5.4-fast") {
		t.Fatal("expected declared alias overlay to stay available")
	}
	if !registry.ClientSupportsModel(auth.ID, "current") {
		t.Fatal("expected logical current alias to stay available")
	}
	if registry.ClientSupportsModel(auth.ID, "gpt-5.3") {
		t.Fatal("did not expect declared-only unsupported model to stay registered")
	}
}

func TestRegisterModelsForAuth_CodexWithoutTrustedModelsDoesNotFallbackStaticCatalog(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	service := &Service{
		cfg: &config.Config{
			ConfigFilePath: configPath,
			CodexKey: []config.CodexKey{
				{
					APIKey:  "k",
					BaseURL: "https://example.com/v1",
				},
			},
			LogicalModelGroups: config.LogicalModelGroups{
				Current: config.LogicalModelCurrent{Ref: "gpt-5.4"},
				Static: []config.LogicalModelGroup{
					{Alias: "gpt-5.4", Target: "gpt-5.4"},
				},
			},
		},
	}
	service.cfg.SanitizeLogicalModelGroups()

	auth := &coreauth.Auth{
		ID:       "codex-no-trusted-models",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind":      "apikey",
			"api_key":        "k",
			"base_url":       "https://example.com/v1",
			"config_locator": "codex-api-key[0]",
		},
	}

	registry := GlobalModelRegistry()
	registry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	if registry.ClientSupportsModel(auth.ID, "gpt-5.4") {
		t.Fatal("did not expect static catalog fallback to register gpt-5.4")
	}
	if registry.ClientSupportsModel(auth.ID, "current") {
		t.Fatal("did not expect logical aliases without a trusted model source")
	}
}
