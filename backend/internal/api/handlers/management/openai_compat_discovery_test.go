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

func TestGetOpenAICompatDiscovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg := &config.Config{
		ConfigFilePath: configPath,
		OpenAICompatibility: []config.OpenAICompatibility{
			{Name: "pool", BaseURL: "https://example.com"},
		},
	}
	if err := modeldiscovery.SaveOpenAICompatCache(configPath, &modeldiscovery.OpenAICompatCacheFile{
		Providers: map[string]modeldiscovery.OpenAICompatCacheEntry{
			"pool": {Name: "pool", ProviderKey: "pool", Models: []config.OpenAICompatibilityModel{{Name: "gpt-5.4"}}},
		},
	}); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	h := NewHandler(cfg, configPath, nil)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/openai-compatibility/discovery", nil)

	h.GetOpenAICompatDiscovery(ctx)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "\"effective-source\":\"discovered\"") {
		t.Fatalf("expected discovered source, got %s", w.Body.String())
	}
}
