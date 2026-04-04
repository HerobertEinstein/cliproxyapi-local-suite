package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"gopkg.in/yaml.v3"
)

func mustLoadManagementLogicalModelGroupsConfig(t *testing.T, source string) *config.Config {
	t.Helper()

	var cfg config.Config
	if err := yaml.Unmarshal([]byte(source), &cfg); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	cfg.SanitizeLogicalModelGroups()
	return &cfg
}

func TestPutLogicalModelGroupCurrent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("logical-model-groups:\n  current:\n    ref: gpt-5.2\n  static:\n    - alias: gpt-5.2\n      target: gpt-5.2\n    - alias: gpt-5.4\n      target: gpt-5.4\n"), 0o600); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	h := NewHandler(mustLoadManagementLogicalModelGroupsConfig(t, `
logical-model-groups:
  current:
    ref: gpt-5.2
  static:
    - alias: gpt-5.2
      target: gpt-5.2
    - alias: gpt-5.4
      target: gpt-5.4
`), configPath, nil)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/v0/management/logical-model-groups/current", strings.NewReader(`{"ref":"gpt-5.4"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.PutLogicalModelGroupCurrent(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read persisted config: %v", err)
	}
	if !strings.Contains(string(data), "logical-model-groups:") || !strings.Contains(string(data), "ref: gpt-5.4") {
		t.Fatalf("expected persisted config to contain updated logical model group, got %s", string(data))
	}
	if strings.Contains(string(data), "current:\n    target:") {
		t.Fatalf("expected current to persist ref instead of target, got %s", string(data))
	}
}

func TestPostAndDeleteLogicalModelGroupStatic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("logical-model-groups:\n  current:\n    ref: gpt-5.2\n  static:\n    - alias: gpt-5.2\n      target: gpt-5.2\n"), 0o600); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	h := NewHandler(mustLoadManagementLogicalModelGroupsConfig(t, `
logical-model-groups:
  current:
    ref: gpt-5.2
  static:
    - alias: gpt-5.2
      target: gpt-5.2
`), configPath, nil)

	postRec := httptest.NewRecorder()
	postCtx, _ := gin.CreateTestContext(postRec)
	postCtx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/logical-model-groups/static", strings.NewReader(`{"alias":"gpt-5.4","target":"gpt-5.4","reasoning":{"mode":"request"}}`))
	postCtx.Request.Header.Set("Content-Type", "application/json")

	h.PostLogicalModelGroupStatic(postCtx)

	if postRec.Code != http.StatusOK {
		t.Fatalf("expected post status %d, got %d with body %s", http.StatusOK, postRec.Code, postRec.Body.String())
	}
	if len(h.cfg.LogicalModelGroups.Static) != 2 {
		t.Fatalf("expected 2 static groups after create, got %d", len(h.cfg.LogicalModelGroups.Static))
	}

	deleteRec := httptest.NewRecorder()
	deleteCtx, r := gin.CreateTestContext(deleteRec)
	deleteCtx.Params = append(deleteCtx.Params, gin.Param{Key: "alias", Value: "gpt-5.4"})
	deleteCtx.Request = httptest.NewRequest(http.MethodDelete, "/v0/management/logical-model-groups/static/gpt-5.4", nil)
	_ = r

	h.DeleteLogicalModelGroupStatic(deleteCtx)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete status %d, got %d with body %s", http.StatusOK, deleteRec.Code, deleteRec.Body.String())
	}
	if len(h.cfg.LogicalModelGroups.Static) != 1 {
		t.Fatalf("expected only referenced static group to remain, got %d", len(h.cfg.LogicalModelGroups.Static))
	}
}

func TestDeleteLogicalModelGroupStatic_RejectsCurrent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutConfigFilePath(mustLoadManagementLogicalModelGroupsConfig(t, `
logical-model-groups:
  current:
    ref: gpt-5.2
  static:
    - alias: gpt-5.2
      target: gpt-5.2
`), nil)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Params = append(ctx.Params, gin.Param{Key: "alias", Value: "current"})
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/v0/management/logical-model-groups/static/current", nil)

	h.DeleteLogicalModelGroupStatic(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestDeleteLogicalModelGroupStatic_RejectsCurrentReference(t *testing.T) {
	gin.SetMode(gin.TestMode)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("logical-model-groups:\n  current:\n    ref: gpt-5.4\n  static:\n    - alias: gpt-5.4\n      target: gpt-5.4\n"), 0o600); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	h := NewHandler(mustLoadManagementLogicalModelGroupsConfig(t, `
logical-model-groups:
  current:
    ref: gpt-5.4
  static:
    - alias: gpt-5.4
      target: gpt-5.4
`), configPath, nil)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Params = append(ctx.Params, gin.Param{Key: "alias", Value: "gpt-5.4"})
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/v0/management/logical-model-groups/static/gpt-5.4", nil)

	h.DeleteLogicalModelGroupStatic(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestGetLogicalModelGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutConfigFilePath(mustLoadManagementLogicalModelGroupsConfig(t, `
logical-model-groups:
  current:
    ref: gpt-5.4
  static:
    - alias: gpt-5.4
      target: gpt-5.4
`), nil)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/logical-model-groups", nil)

	h.GetLogicalModelGroups(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	payload, ok := body["logical-model-groups"].(map[string]any)
	if !ok {
		t.Fatalf("expected logical-model-groups payload, got %#v", body)
	}
	current, ok := payload["current"].(map[string]any)
	if !ok {
		t.Fatalf("expected current payload, got %#v", payload)
	}
	if current["ref"] != "gpt-5.4" {
		t.Fatalf("expected current.ref to be returned, got %#v", current)
	}
	if _, exists := current["target"]; exists {
		t.Fatalf("expected current target to stay hidden in API payload, got %#v", current)
	}
}
