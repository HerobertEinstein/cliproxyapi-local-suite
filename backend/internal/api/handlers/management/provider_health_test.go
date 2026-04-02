package management

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/modeldiscovery"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func writeProviderHealthConfig(t *testing.T) (*config.Config, string) {
	t.Helper()

	rootDir := t.TempDir()
	configDir := filepath.Join(rootDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := &config.Config{
		ConfigFilePath: configPath,
		CodexKey: []config.CodexKey{
			{
				APIKey:  "k",
				BaseURL: "https://example.com/v1",
				Models: []config.CodexModel{
					{Name: "gpt-5.4"},
				},
			},
		},
	}
	return cfg, configPath
}

func writeProviderHealthRuntimeState(t *testing.T, configPath string, body string) string {
	t.Helper()

	stateDir := filepath.Join(filepath.Dir(filepath.Dir(configPath)), "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	statePath := filepath.Join(stateDir, authRuntimeStateFileName)
	if err := os.WriteFile(statePath, []byte(body), 0o600); err != nil {
		t.Fatalf("write runtime state: %v", err)
	}
	return statePath
}

func TestGetProviderHealth_MergesDiscoveryRuntimeAndImportHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg, configPath := writeProviderHealthConfig(t)
	if err := modeldiscovery.SaveOpenAICompatCache(configPath, &modeldiscovery.OpenAICompatCacheFile{
		Providers: map[string]modeldiscovery.OpenAICompatCacheEntry{
			"codex-api-key[0]": {
				Name:          "codex-api-key[0]",
				Kind:          modeldiscovery.ProviderKindCodexAPIKey,
				ProviderKey:   "codex-api-key[0]",
				ConfigLocator: "codex-api-key[0]",
				Status:        "fresh",
				Models: []config.OpenAICompatibilityModel{
					{Name: "gpt-5.4"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save discovery cache: %v", err)
	}
	_ = writeProviderHealthRuntimeState(t, configPath, `{
  "entries": [
    {
      "auth_id": "codex-provider-health",
      "provider": "codex",
      "config_locator": "codex-api-key[0]",
      "model_states": {
        "gpt-5.4": {
          "status": "error",
          "status_message": "upstream_unavailable",
          "unavailable": true,
          "next_retry_after": "2099-01-01T00:00:00Z"
        }
      }
    }
  ]
}`)
	_ = writeCodexImportHealthTestFile(t, configPath, `[
  {
    "provider": "SaberRC",
    "config_locator": "codex-api-key[0]",
    "effective_models": ["gpt-5.4"],
    "health": {
      "provider": "SaberRC",
      "base_url": "https://example.com/v1",
      "status": "healthy",
      "detail": "http_200",
      "checked_at": "2026-04-01 09:30:00"
    }
  }
]`)

	h := NewHandler(cfg, configPath, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/provider-health", nil)

	h.GetProviderHealth(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, snippet := range []string{
		`"key":"codex-api-key[0]"`,
		`"kind":"codex-api-key"`,
		`"discovery_state":"fresh"`,
		`"runtime_unavailable":true`,
		`"model":"gpt-5.4"`,
		`"status":"healthy"`,
	} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("expected response to contain %s, got %s", snippet, body)
		}
	}
}

func TestGetProviderHealth_MergesSnapshotRuntimeStateWhenAuthExists(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg, configPath := writeProviderHealthConfig(t)
	if err := modeldiscovery.SaveOpenAICompatCache(configPath, &modeldiscovery.OpenAICompatCacheFile{
		Providers: map[string]modeldiscovery.OpenAICompatCacheEntry{
			"codex-api-key[0]": {
				Name:          "codex-api-key[0]",
				Kind:          modeldiscovery.ProviderKindCodexAPIKey,
				ProviderKey:   "codex-api-key[0]",
				ConfigLocator: "codex-api-key[0]",
				Status:        "fresh",
				Models: []config.OpenAICompatibilityModel{
					{Name: "gpt-5.4"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save discovery cache: %v", err)
	}
	_ = writeProviderHealthRuntimeState(t, configPath, `{
  "entries": [
    {
      "auth_id": "codex-provider-health",
      "provider": "codex",
      "config_locator": "codex-api-key[0]",
      "model_states": {
        "gpt-5.4": {
          "status": "error",
          "status_message": "upstream_unavailable",
          "unavailable": true,
          "next_retry_after": "2099-01-01T00:00:00Z"
        }
      }
    }
  ]
}`)

	manager := coreauth.NewManager(nil, &coreauth.RoundRobinSelector{}, nil)
	manager.SetConfig(cfg)
	auth := &coreauth.Auth{
		ID:       "codex-provider-health",
		Provider: "codex",
		Attributes: map[string]string{
			"auth_kind":      "apikey",
			"source":         "config:codex-api-key[test]",
			"config_locator": "codex-api-key[0]",
		},
	}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	h := NewHandler(cfg, configPath, manager)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/provider-health", nil)

	h.GetProviderHealth(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, snippet := range []string{
		`"key":"codex-api-key[0]"`,
		`"runtime_unavailable":true`,
		`"model":"gpt-5.4"`,
		`"status_message":"upstream_unavailable"`,
	} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("expected response to contain %s, got %s", snippet, body)
		}
	}
}

func TestPostProviderHealthReset_ClearsRuntimeStateAndTriggersRefresh(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg, configPath := writeProviderHealthConfig(t)
	statePath := writeProviderHealthRuntimeState(t, configPath, `{
  "entries": [
    {
      "auth_id": "codex-provider-reset",
      "provider": "codex",
      "config_locator": "codex-api-key[0]",
      "model_states": {
        "gpt-5.4": {
          "status": "error",
          "status_message": "upstream_unavailable",
          "unavailable": true,
          "next_retry_after": "2099-01-01T00:00:00Z"
        }
      }
    }
  ]
}`)

	manager := coreauth.NewManager(nil, &coreauth.RoundRobinSelector{}, nil)
	manager.SetConfig(cfg)
	auth := &coreauth.Auth{
		ID:       "codex-provider-reset",
		Provider: "codex",
		Attributes: map[string]string{
			"auth_kind":      "apikey",
			"source":         "config:codex-api-key[test]",
			"config_locator": "codex-api-key[0]",
		},
		ModelStates: map[string]*coreauth.ModelState{
			"gpt-5.4": {
				Unavailable:    true,
				Status:         coreauth.StatusError,
				StatusMessage:  "upstream_unavailable",
				NextRetryAfter: time.Now().Add(30 * time.Minute),
			},
		},
	}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	h := NewHandler(cfg, configPath, manager)
	refreshCalls := 0
	h.SetRuntimeRefreshHook(func(context.Context) {
		refreshCalls++
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/provider-health/reset", bytes.NewBufferString(`{"locator":"codex-api-key[0]","model":"gpt-5.4"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.PostProviderHealthReset(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", rec.Code, rec.Body.String())
	}
	if refreshCalls != 1 {
		t.Fatalf("expected refresh hook to run once, got %d", refreshCalls)
	}
	if !strings.Contains(rec.Body.String(), `"matched_auths":["codex-provider-reset"]`) {
		t.Fatalf("expected matched auth id in response, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"cleared_models":["gpt-5.4"]`) {
		t.Fatalf("expected cleared model in response, got %s", rec.Body.String())
	}

	updated, ok := manager.GetByID("codex-provider-reset")
	if !ok || updated == nil {
		t.Fatalf("expected auth to remain registered")
	}
	if state := updated.ModelStates["gpt-5.4"]; state != nil && state.Unavailable {
		t.Fatalf("expected in-memory runtime state to be cleared")
	}

	data, err := os.ReadFile(statePath)
	if err == nil && strings.Contains(string(data), "gpt-5.4") {
		t.Fatalf("expected runtime state file to drop cleared model, got %s", string(data))
	}
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read runtime state file: %v", err)
	}
}
