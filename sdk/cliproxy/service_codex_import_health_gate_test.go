package cliproxy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

func writeCodexImportHealthSnapshotForServiceTest(t *testing.T, configPath string, body string) string {
	t.Helper()

	stateDir := filepath.Join(filepath.Dir(filepath.Dir(configPath)), "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}

	snapshotPath := filepath.Join(stateDir, "ccswitch-codex-import-health.json")
	if err := os.WriteFile(snapshotPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	return snapshotPath
}

func writeCodexAPIKeyRuntimeStateForServiceTest(t *testing.T, configPath string, body string) string {
	t.Helper()

	stateDir := filepath.Join(filepath.Dir(filepath.Dir(configPath)), "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}

	statePath := filepath.Join(stateDir, "auth-runtime-state.json")
	if err := os.WriteFile(statePath, []byte(body), 0o600); err != nil {
		t.Fatalf("write runtime state: %v", err)
	}
	return statePath
}

func TestRegisterModelsForAuth_CodexImportHealthHardFailureSkipsRegistration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_ = writeCodexImportHealthSnapshotForServiceTest(t, configPath, `[
  {
    "provider": "DeadProvider",
    "config_locator": "codex-api-key[0]",
    "effective_models": ["gpt-5.4"],
    "health": {
      "provider": "DeadProvider",
      "base_url": "https://example.com/v1",
      "status": "auth_failed",
      "detail": "http_401",
      "checked_at": "2026-03-31 12:30:00"
    }
  }
]`)

	service := &Service{
		cfg: &config.Config{
			ConfigFilePath: configPath,
			CodexKey: []config.CodexKey{
				{
					APIKey:  "k",
					BaseURL: "https://example.com/v1",
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
		ID:       "codex-health-hard-fail",
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
		t.Fatal("did not expect hard-failed imported codex auth to register gpt-5.4")
	}
	if registry.ClientSupportsModel(auth.ID, "current") {
		t.Fatal("did not expect hard-failed imported codex auth to register logical current")
	}
}

func TestRegisterModelsForAuth_PreservesModelSupportButHidesActiveCooldownModels(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
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
					Models: []internalconfig.CodexModel{
						{Name: "gpt-5.4"},
						{Name: "gpt-5.2-codex-max"},
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
		ID:       "codex-health-cooldown",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind":      "apikey",
			"api_key":        "k",
			"base_url":       "https://example.com/v1",
			"config_locator": "codex-api-key[0]",
		},
		ModelStates: map[string]*coreauth.ModelState{
			"gpt-5.2-codex-max": {
				Unavailable:    true,
				Status:         coreauth.StatusError,
				StatusMessage:  "model_not_supported",
				NextRetryAfter: time.Now().Add(10 * time.Minute),
			},
		},
	}

	registry := GlobalModelRegistry()
	registry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	if !registry.ClientSupportsModel(auth.ID, "gpt-5.2-codex-max") {
		t.Fatal("expected cooled-down model to stay in client support set")
	}

	models := registry.GetAvailableModels("openai")
	for _, model := range models {
		if model["id"] == "gpt-5.2-codex-max" {
			t.Fatal("did not expect active cooldown model to appear in available models")
		}
	}
}

func TestRegisterModelsForAuth_HidesLogicalAliasWhenTargetModelIsCoolingDown(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
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
					Models: []internalconfig.CodexModel{
						{Name: "gpt-5.2-codex-max"},
					},
				},
			},
			LogicalModelGroups: config.LogicalModelGroups{
				Current: config.LogicalModelCurrent{Ref: "gpt-5.2"},
				Static: []config.LogicalModelGroup{
					{Alias: "gpt-5.2", Target: "gpt-5.2-codex-max"},
				},
			},
		},
	}
	service.cfg.SanitizeLogicalModelGroups()

	auth := &coreauth.Auth{
		ID:       "codex-health-cooldown-current",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind":      "apikey",
			"api_key":        "k",
			"base_url":       "https://example.com/v1",
			"config_locator": "codex-api-key[0]",
		},
		ModelStates: map[string]*coreauth.ModelState{
			"gpt-5.2-codex-max": {
				Unavailable:    true,
				Status:         coreauth.StatusError,
				StatusMessage:  "model_not_supported",
				NextRetryAfter: time.Now().Add(10 * time.Minute),
			},
		},
	}

	registry := GlobalModelRegistry()
	registry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	if !registry.ClientSupportsModel(auth.ID, "current") {
		t.Fatal("expected logical current alias to stay in client support set")
	}

	models := registry.GetAvailableModels("openai")
	for _, model := range models {
		if model["id"] == "current" {
			t.Fatal("did not expect logical current alias to appear while its target model is cooling down")
		}
		if model["id"] == "gpt-5.2" {
			t.Fatal("did not expect static logical alias to appear while its target model is cooling down")
		}
	}
}

func TestRegisterModelsForAuth_LoadsPersistedRuntimeStateOnColdStart(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_ = writeCodexAPIKeyRuntimeStateForServiceTest(t, configPath, `{
  "entries": [
    {
      "auth_id": "codex-health-cold-start",
      "provider": "codex",
      "config_locator": "codex-api-key[0]",
      "model_states": {
        "gpt-5.2-codex-max": {
          "status": "error",
          "status_message": "model_not_supported",
          "unavailable": true,
          "next_retry_after": "2099-01-01T00:00:00Z"
        }
      }
    }
  ]
}`)

	service := &Service{
		cfg: &config.Config{
			ConfigFilePath: configPath,
			CodexKey: []config.CodexKey{
				{
					APIKey:  "k",
					BaseURL: "https://example.com/v1",
					Models: []internalconfig.CodexModel{
						{Name: "gpt-5.2-codex-max"},
					},
				},
			},
		},
	}
	service.cfg.SanitizeLogicalModelGroups()

	auth := &coreauth.Auth{
		ID:       "codex-health-cold-start",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind":      "apikey",
			"api_key":        "k",
			"base_url":       "https://example.com/v1",
			"config_locator": "codex-api-key[0]",
			"source":         "config:codex[test]",
		},
	}

	registry := GlobalModelRegistry()
	registry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	if state := auth.ModelStates["gpt-5.2-codex-max"]; state == nil || !state.Unavailable {
		t.Fatal("expected cold-start auth to load persisted model state before registration")
	}

	models := registry.GetAvailableModels("openai")
	for _, model := range models {
		if model["id"] == "gpt-5.2-codex-max" {
			t.Fatal("did not expect persisted cooldown model to appear in available models after cold start")
		}
	}
}

func TestRegisterModelsForAuth_CodexImportSnapshotConstrainsEmptyConfigModels(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_ = writeCodexImportHealthSnapshotForServiceTest(t, configPath, `[
  {
    "provider": "ImportedProvider",
    "config_locator": "codex-api-key[0]",
    "effective_models": ["gpt-5.4"],
    "health": {
      "provider": "ImportedProvider",
      "base_url": "https://example.com/v1",
      "status": "preserved",
      "detail": "retained_from_existing_import",
      "checked_at": "2026-03-31 12:30:00"
    }
  }
]`)

	service := &Service{
		cfg: &config.Config{
			ConfigFilePath: configPath,
			CodexKey: []config.CodexKey{
				{
					APIKey:  "k",
					BaseURL: "https://example.com/v1",
					Models:  nil,
				},
			},
			LogicalModelGroups: config.LogicalModelGroups{
				Current: config.LogicalModelCurrent{Ref: "gpt-5.4"},
				Static: []config.LogicalModelGroup{
					{Alias: "gpt-5.2", Target: "gpt-5.2"},
					{Alias: "gpt-5.4", Target: "gpt-5.4"},
				},
			},
		},
	}
	service.cfg.SanitizeLogicalModelGroups()

	auth := &coreauth.Auth{
		ID:       "codex-import-constrained-models",
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
		t.Fatal("expected snapshot effective models to keep gpt-5.4 available")
	}
	if !registry.ClientSupportsModel(auth.ID, "current") {
		t.Fatal("expected logical current alias to remain available when snapshot supports its target")
	}
	if registry.ClientSupportsModel(auth.ID, "gpt-5.2") {
		t.Fatal("did not expect empty config models to fall back to the global codex catalog")
	}
}

func TestRegisterModelsForAuth_CodexImportSnapshotEmptyModelsSkipsRegistration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_ = writeCodexImportHealthSnapshotForServiceTest(t, configPath, `[
  {
    "provider": "ImportedProvider",
    "config_locator": "codex-api-key[0]",
    "effective_models": [],
    "health": {
      "provider": "ImportedProvider",
      "base_url": "https://example.com/v1",
      "status": "preserved",
      "detail": "retained_from_existing_import",
      "checked_at": "2026-03-31 12:30:00"
    }
  }
]`)

	service := &Service{
		cfg: &config.Config{
			ConfigFilePath: configPath,
			CodexKey: []config.CodexKey{
				{
					APIKey:  "k",
					BaseURL: "https://example.com/v1",
					Models:  nil,
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
		ID:       "codex-import-empty-models",
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
		t.Fatal("did not expect empty snapshot effective models to register gpt-5.4")
	}
	if registry.ClientSupportsModel(auth.ID, "current") {
		t.Fatal("did not expect empty snapshot effective models to register logical aliases")
	}
}
