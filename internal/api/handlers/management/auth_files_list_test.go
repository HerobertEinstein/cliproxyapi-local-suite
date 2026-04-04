package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	iregistry "github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestListAuthFiles_IncludesConfigAuthWithoutPathAndModelCount(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	manager := coreauth.NewManager(nil, nil, nil)

	activeAuth := &coreauth.Auth{
		ID:       "config-codex-active",
		FileName: "active-codex-config",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"source":  "config:codex[active-token]",
			"api_key": "shared-key",
		},
	}
	disabledAuth := &coreauth.Auth{
		ID:       "config-codex-disabled",
		FileName: "disabled-codex-config",
		Provider: "codex",
		Status:   coreauth.StatusDisabled,
		Disabled: true,
		Attributes: map[string]string{
			"source":  "config:codex[disabled-token]",
			"api_key": "shared-key",
		},
	}
	if _, err := manager.Register(context.Background(), activeAuth); err != nil {
		t.Fatalf("register active auth: %v", err)
	}
	if _, err := manager.Register(context.Background(), disabledAuth); err != nil {
		t.Fatalf("register disabled auth: %v", err)
	}

	reg := iregistry.GetGlobalRegistry()
	reg.UnregisterClient(activeAuth.ID)
	reg.UnregisterClient(disabledAuth.ID)
	t.Cleanup(func() {
		reg.UnregisterClient(activeAuth.ID)
		reg.UnregisterClient(disabledAuth.ID)
	})
	reg.RegisterClient(activeAuth.ID, "codex", []*iregistry.ModelInfo{
		{ID: "current"},
		{ID: "gpt-5.2"},
	})
	reg.RegisterClient(disabledAuth.ID, "codex", []*iregistry.ModelInfo{
		{ID: "current"},
		{ID: "gpt-5.4-mini"},
		{ID: "gpt-5.4"},
	})

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, manager)
	h.tokenStore = &memoryAuthStore{}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/auth-files", nil)
	h.ListAuthFiles(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload struct {
		Files []map[string]any `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list payload: %v", err)
	}
	if len(payload.Files) != 2 {
		t.Fatalf("expected 2 config auth entries, got %d: %#v", len(payload.Files), payload.Files)
	}

	entriesByName := make(map[string]map[string]any, len(payload.Files))
	for _, entry := range payload.Files {
		name, _ := entry["name"].(string)
		entriesByName[name] = entry
	}

	activeEntry := entriesByName["active-codex-config"]
	if activeEntry == nil {
		t.Fatalf("expected active config auth to be listed, got %#v", payload.Files)
	}
	if got, _ := activeEntry["source"].(string); got != "config:codex[active-token]" {
		t.Fatalf("expected active source to preserve config origin, got %#v", activeEntry["source"])
	}
	if got := int(activeEntry["model_count"].(float64)); got != 2 {
		t.Fatalf("expected active model_count 2, got %d", got)
	}

	disabledEntry := entriesByName["disabled-codex-config"]
	if disabledEntry == nil {
		t.Fatalf("expected disabled config auth to be listed, got %#v", payload.Files)
	}
	if got, ok := disabledEntry["disabled"].(bool); !ok || !got {
		t.Fatalf("expected disabled auth to remain marked disabled, got %#v", disabledEntry["disabled"])
	}
	if got, _ := disabledEntry["source"].(string); got != "config:codex[disabled-token]" {
		t.Fatalf("expected disabled source to preserve config origin, got %#v", disabledEntry["source"])
	}
	if got := int(disabledEntry["model_count"].(float64)); got != 3 {
		t.Fatalf("expected disabled model_count 3, got %d", got)
	}
}
