package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func writeCodexImportHealthTestFile(t *testing.T, configPath string, body string) string {
	t.Helper()

	configDir := filepath.Dir(configPath)
	rootDir := filepath.Dir(configDir)
	stateDir := filepath.Join(rootDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}

	healthPath := filepath.Join(stateDir, "ccswitch-codex-import-health.json")
	if err := os.WriteFile(healthPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write health file: %v", err)
	}
	return healthPath
}

func newCodexImportHealthHandler(t *testing.T) (*Handler, string) {
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

	cfg := &config.Config{ConfigFilePath: configPath}
	return NewHandler(cfg, configPath, nil), configPath
}

func TestGetCodexImportHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h, configPath := newCodexImportHealthHandler(t)
	healthPath := writeCodexImportHealthTestFile(t, configPath, `[
  {
    "provider": "SaberRC",
    "config_locator": "codex-api-key[0]",
    "endpoint_url": "https://right.codes/codex/v1",
    "config_base_url": "https://right.codes/codex/v1",
    "effective_base_url": "https://right.codes/codex/v1",
    "default_model": "gpt-5.2",
    "template_models": ["gpt-5.2", "gpt-5.4", "gpt-5.4-mini"],
    "effective_models": ["gpt-5.2"],
    "effective_source": "live",
    "health": {
      "provider": "SaberRC",
      "base_url": "https://right.codes/codex/v1",
      "status": "healthy",
      "detail": "http_200 json_response",
      "checked_at": "2026-03-31 11:38:39"
    }
  }
]`)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/codex/import-health", nil)

	h.GetCodexImportHealth(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["available"] != true {
		t.Fatalf("expected available=true, got %#v", body["available"])
	}
	if body["path"] != healthPath {
		t.Fatalf("expected path %q, got %#v", healthPath, body["path"])
	}

	providers, ok := body["providers"].([]any)
	if !ok || len(providers) != 1 {
		t.Fatalf("expected one provider, got %#v", body["providers"])
	}
	first, ok := providers[0].(map[string]any)
	if !ok {
		t.Fatalf("expected provider object, got %#v", providers[0])
	}
	if first["provider"] != "SaberRC" {
		t.Fatalf("expected provider SaberRC, got %#v", first["provider"])
	}
	if first["config_locator"] != "codex-api-key[0]" {
		t.Fatalf("expected config locator, got %#v", first["config_locator"])
	}
	if first["effective_base_url"] != "https://right.codes/codex/v1" {
		t.Fatalf("expected effective base url, got %#v", first["effective_base_url"])
	}
	if effectiveModels, ok := first["effective_models"].([]any); !ok || len(effectiveModels) != 1 || effectiveModels[0] != "gpt-5.2" {
		t.Fatalf("expected effective models, got %#v", first["effective_models"])
	}
	if first["effective_source"] != "live" {
		t.Fatalf("expected effective source live, got %#v", first["effective_source"])
	}
}

func TestGetCodexImportHealthMissingFile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h, _ := newCodexImportHealthHandler(t)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/codex/import-health", nil)

	h.GetCodexImportHealth(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["available"] != false {
		t.Fatalf("expected available=false, got %#v", body["available"])
	}
	providers, ok := body["providers"].([]any)
	if !ok || len(providers) != 0 {
		t.Fatalf("expected empty providers, got %#v", body["providers"])
	}
}

func TestGetCodexImportHealthInvalidFile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h, configPath := newCodexImportHealthHandler(t)
	_ = writeCodexImportHealthTestFile(t, configPath, `{not-json}`)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/codex/import-health", nil)

	h.GetCodexImportHealth(ctx)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d with body %s", rec.Code, rec.Body.String())
	}
}
