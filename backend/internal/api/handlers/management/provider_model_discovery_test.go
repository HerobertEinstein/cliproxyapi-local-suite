package management

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/modeldiscovery"
)

func TestGetProviderModelDiscovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
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
					{Name: "gpt-5.4", Alias: "gpt-5.4-fast"},
				},
			},
		},
	}
	if err := modeldiscovery.SaveOpenAICompatCache(configPath, &modeldiscovery.OpenAICompatCacheFile{
		Providers: map[string]modeldiscovery.OpenAICompatCacheEntry{
			"codex-api-key[0]": {
				Name:          "codex-api-key[0]",
				ProviderKey:   "codex-api-key[0]",
				ConfigLocator: "codex-api-key[0]",
				Models: []config.OpenAICompatibilityModel{
					{Name: "gpt-5.4"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	h := NewHandler(cfg, configPath, nil)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/provider-model-discovery", nil)

	h.GetProviderModelDiscovery(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "\"kind\":\"codex-api-key\"") {
		t.Fatalf("expected codex provider kind in body, got %s", body)
	}
	if !strings.Contains(body, "\"key\":\"codex-api-key[0]\"") {
		t.Fatalf("expected config locator key in body, got %s", body)
	}
	if !strings.Contains(body, "\"effective-source\":\"discovered\"") {
		t.Fatalf("expected discovered source in body, got %s", body)
	}
}
