package cliproxy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
