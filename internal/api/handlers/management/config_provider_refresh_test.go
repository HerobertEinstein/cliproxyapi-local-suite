package management

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/modeldiscovery"
)

func newOpenAIModelsServer(t *testing.T, model string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models", "/models":
			_, _ = w.Write([]byte(fmt.Sprintf(`{"data":[{"id":"%s"}]}`, model)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestPutConfigProviderListRefreshesDiscoveryAndRuntime(t *testing.T) {
	gin.SetMode(gin.TestMode)

	discoveryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models", "/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.4-mini"}]}`))
		case "/v1beta/models":
			_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-2.5-pro"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer discoveryServer.Close()

	tests := []struct {
		name         string
		body         string
		expectedKey  string
		expectedName string
		invoke       func(*Handler, *gin.Context)
	}{
		{
			name:         "codex",
			body:         fmt.Sprintf(`[{"api-key":"k","base-url":"%s/v1"}]`, discoveryServer.URL),
			expectedKey:  "codex-api-key[0]",
			expectedName: "gpt-5.4-mini",
			invoke:       (*Handler).PutCodexKeys,
		},
		{
			name:         "claude",
			body:         fmt.Sprintf(`[{"api-key":"k","base-url":"%s/v1"}]`, discoveryServer.URL),
			expectedKey:  "claude-api-key[0]",
			expectedName: "gpt-5.4-mini",
			invoke:       (*Handler).PutClaudeKeys,
		},
		{
			name:         "gemini",
			body:         fmt.Sprintf(`[{"api-key":"k","base-url":"%s"}]`, discoveryServer.URL),
			expectedKey:  "gemini-api-key[0]",
			expectedName: "gemini-2.5-pro",
			invoke:       (*Handler).PutGeminiKeys,
		},
		{
			name:         "vertex",
			body:         fmt.Sprintf(`[{"api-key":"k","base-url":"%s/v1"}]`, discoveryServer.URL),
			expectedKey:  "vertex-api-key[0]",
			expectedName: "gpt-5.4-mini",
			invoke:       (*Handler).PutVertexCompatKeys,
		},
		{
			name:         "openai-compatibility",
			body:         fmt.Sprintf(`[{"name":"rightcode","base-url":"%s/v1","api-key-entries":[{"api-key":"k"}]}]`, discoveryServer.URL),
			expectedKey:  "rightcode",
			expectedName: "gpt-5.4-mini",
			invoke:       (*Handler).PutOpenAICompat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writableDir := t.TempDir()
			t.Setenv("WRITABLE_PATH", writableDir)

			configPath := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg := &config.Config{ConfigFilePath: configPath}
			handler := NewHandler(cfg, configPath, nil)

			runtimeRefreshCalls := 0
			handler.SetRuntimeRefreshHook(func(context.Context) {
				runtimeRefreshCalls++
			})

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/v0/management/provider", strings.NewReader(tt.body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			tt.invoke(handler, ctx)

			if recorder.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
			}
			if runtimeRefreshCalls != 1 {
				t.Fatalf("expected runtime refresh hook once, got %d", runtimeRefreshCalls)
			}

			cache, err := modeldiscovery.LoadOpenAICompatCache(configPath)
			if err != nil {
				t.Fatalf("load discovery cache: %v", err)
			}
			entry, ok := cache.Providers[tt.expectedKey]
			if !ok {
				t.Fatalf("expected discovery cache entry %q, got %#v", tt.expectedKey, cache.Providers)
			}
			if !strings.EqualFold(entry.Status, "fresh") {
				t.Fatalf("expected fresh discovery status, got %q", entry.Status)
			}
			if len(entry.Models) != 1 || entry.Models[0].Name != tt.expectedName {
				t.Fatalf("expected discovered model %q, got %#v", tt.expectedName, entry.Models)
			}
		})
	}
}

func TestPutCodexKeysRefreshesOnlyChangedProviders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var untouchedHits atomic.Int32
	var changedHits atomic.Int32
	untouchedServer := newOpenAIModelsServer(t, "gpt-5.4-mini", &untouchedHits)
	changedServer := newOpenAIModelsServer(t, "gpt-5.4", &changedHits)

	writableDir := t.TempDir()
	t.Setenv("WRITABLE_PATH", writableDir)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := &config.Config{
		ConfigFilePath: configPath,
		CodexKey: []config.CodexKey{
			{APIKey: "old-a", BaseURL: untouchedServer.URL + "/v1"},
			{APIKey: "old-b", BaseURL: changedServer.URL + "/v1"},
		},
	}
	handler := NewHandler(cfg, configPath, nil)

	body := fmt.Sprintf(
		`[{"api-key":"old-a","base-url":"%s/v1"},{"api-key":"new-b","base-url":"%s/v1"}]`,
		untouchedServer.URL,
		changedServer.URL,
	)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/v0/management/provider/codex-api-key", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.PutCodexKeys(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := untouchedHits.Load(); got != 0 {
		t.Fatalf("expected untouched provider discovery hits 0, got %d", got)
	}
	if got := changedHits.Load(); got != 1 {
		t.Fatalf("expected changed provider discovery hits 1, got %d", got)
	}
}

func TestPutCodexKeysRefreshesFromFirstChangedIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var firstHits atomic.Int32
	var shiftedHits atomic.Int32
	firstServer := newOpenAIModelsServer(t, "gpt-5.4-mini", &firstHits)
	shiftedServer := newOpenAIModelsServer(t, "gpt-5.4", &shiftedHits)
	removedServer := newOpenAIModelsServer(t, "gpt-4.1", new(atomic.Int32))

	writableDir := t.TempDir()
	t.Setenv("WRITABLE_PATH", writableDir)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := &config.Config{
		ConfigFilePath: configPath,
		CodexKey: []config.CodexKey{
			{APIKey: "old-a", BaseURL: firstServer.URL + "/v1"},
			{APIKey: "old-b", BaseURL: removedServer.URL + "/v1"},
			{APIKey: "old-c", BaseURL: shiftedServer.URL + "/v1"},
		},
	}
	handler := NewHandler(cfg, configPath, nil)

	body := fmt.Sprintf(
		`[{"api-key":"old-a","base-url":"%s/v1"},{"api-key":"old-c","base-url":"%s/v1"}]`,
		firstServer.URL,
		shiftedServer.URL,
	)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/v0/management/provider/codex-api-key", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.PutCodexKeys(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := firstHits.Load(); got != 0 {
		t.Fatalf("expected unchanged provider before first diff to stay unscanned, got %d hits", got)
	}
	if got := shiftedHits.Load(); got != 1 {
		t.Fatalf("expected shifted provider discovery hits 1, got %d", got)
	}
}

func TestPutOpenAICompatRefreshesOnlyChangedProviders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var untouchedHits atomic.Int32
	var changedHits atomic.Int32
	untouchedServer := newOpenAIModelsServer(t, "gpt-5.4-mini", &untouchedHits)
	changedServer := newOpenAIModelsServer(t, "claude-opus-4-6", &changedHits)

	writableDir := t.TempDir()
	t.Setenv("WRITABLE_PATH", writableDir)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := &config.Config{
		ConfigFilePath: configPath,
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:          "rightcode",
				BaseURL:       untouchedServer.URL + "/v1",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{{APIKey: "right-old"}},
			},
			{
				Name:          "saberrc",
				BaseURL:       changedServer.URL + "/v1",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{{APIKey: "saber-old"}},
			},
		},
	}
	handler := NewHandler(cfg, configPath, nil)

	body := fmt.Sprintf(
		`[
			{"name":"rightcode","base-url":"%s/v1","api-key-entries":[{"api-key":"right-old"}]},
			{"name":"saberrc","base-url":"%s/v1","api-key-entries":[{"api-key":"saber-new"}]}
		]`,
		untouchedServer.URL,
		changedServer.URL,
	)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/v0/management/provider/openai-compatibility", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.PutOpenAICompat(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := untouchedHits.Load(); got != 0 {
		t.Fatalf("expected untouched openai-compat provider discovery hits 0, got %d", got)
	}
	if got := changedHits.Load(); got != 1 {
		t.Fatalf("expected changed openai-compat provider discovery hits 1, got %d", got)
	}
}

func TestPatchConfigProviderRefreshesDiscoveryAndRuntime(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		cfg          *config.Config
		body         string
		invoke       func(*Handler, *gin.Context)
		expectedKey  string
		expectedName string
	}{
		{
			name: "codex",
			cfg: &config.Config{
				CodexKey: []config.CodexKey{
					{APIKey: "old-codex", BaseURL: "https://old.example.com/v1"},
				},
			},
			body:         `{"index":0,"value":{"api-key":"new-codex","base-url":"%s/v1"}}`,
			invoke:       (*Handler).PatchCodexKey,
			expectedKey:  "codex-api-key[0]",
			expectedName: "gpt-5.4-mini",
		},
		{
			name: "claude",
			cfg: &config.Config{
				ClaudeKey: []config.ClaudeKey{
					{APIKey: "old-claude", BaseURL: "https://old.example.com/v1"},
				},
			},
			body:         `{"index":0,"value":{"api-key":"new-claude","base-url":"%s/v1"}}`,
			invoke:       (*Handler).PatchClaudeKey,
			expectedKey:  "claude-api-key[0]",
			expectedName: "gpt-5.4-mini",
		},
		{
			name: "gemini",
			cfg: &config.Config{
				GeminiKey: []config.GeminiKey{
					{APIKey: "old-gemini", BaseURL: "https://old.example.com"},
				},
			},
			body:         `{"index":0,"value":{"api-key":"new-gemini","base-url":"%s"}}`,
			invoke:       (*Handler).PatchGeminiKey,
			expectedKey:  "gemini-api-key[0]",
			expectedName: "gemini-2.5-pro",
		},
		{
			name: "vertex",
			cfg: &config.Config{
				VertexCompatAPIKey: []config.VertexCompatKey{
					{APIKey: "old-vertex", BaseURL: "https://old.example.com/v1"},
				},
			},
			body:         `{"index":0,"value":{"api-key":"new-vertex","base-url":"%s/v1"}}`,
			invoke:       (*Handler).PatchVertexCompatKey,
			expectedKey:  "vertex-api-key[0]",
			expectedName: "gpt-5.4-mini",
		},
		{
			name: "openai-compatibility",
			cfg: &config.Config{
				OpenAICompatibility: []config.OpenAICompatibility{
					{
						Name:          "rightcode",
						BaseURL:       "https://old.example.com/v1",
						APIKeyEntries: []config.OpenAICompatibilityAPIKey{{APIKey: "old-rightcode"}},
					},
				},
			},
			body:         `{"index":0,"value":{"base-url":"%s/v1","api-key-entries":[{"api-key":"new-rightcode"}]}}`,
			invoke:       (*Handler).PatchOpenAICompat,
			expectedKey:  "rightcode",
			expectedName: "gpt-5.4-mini",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			discoveryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/models", "/models":
					_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.4-mini"}]}`))
				case "/v1beta/models":
					_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-2.5-pro"}]}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer discoveryServer.Close()

			writableDir := t.TempDir()
			t.Setenv("WRITABLE_PATH", writableDir)

			configPath := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg := tt.cfg
			cfg.ConfigFilePath = configPath
			handler := NewHandler(cfg, configPath, nil)

			runtimeRefreshCalls := 0
			handler.SetRuntimeRefreshHook(func(context.Context) {
				runtimeRefreshCalls++
			})

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(
				http.MethodPatch,
				"/v0/management/provider",
				strings.NewReader(fmt.Sprintf(tt.body, discoveryServer.URL)),
			)
			ctx.Request.Header.Set("Content-Type", "application/json")

			tt.invoke(handler, ctx)

			if recorder.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
			}
			if runtimeRefreshCalls != 1 {
				t.Fatalf("expected runtime refresh hook once, got %d", runtimeRefreshCalls)
			}

			cache, err := modeldiscovery.LoadOpenAICompatCache(configPath)
			if err != nil {
				t.Fatalf("load discovery cache: %v", err)
			}
			entry, ok := cache.Providers[tt.expectedKey]
			if !ok {
				t.Fatalf("expected discovery cache entry %q, got %#v", tt.expectedKey, cache.Providers)
			}
			if !strings.EqualFold(entry.Status, "fresh") {
				t.Fatalf("expected fresh discovery status, got %q", entry.Status)
			}
			if len(entry.Models) != 1 || entry.Models[0].Name != tt.expectedName {
				t.Fatalf("expected discovered model %q, got %#v", tt.expectedName, entry.Models)
			}
		})
	}
}

func TestDeleteConfigProviderPrunesDiscoveryCacheAndRefreshesRuntime(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		cfg         *config.Config
		expectedKey string
		requestURL  string
		invoke      func(*Handler, *gin.Context)
	}{
		{
			name: "codex",
			cfg: &config.Config{
				CodexKey: []config.CodexKey{{APIKey: "delete-codex", BaseURL: "https://example.com/v1"}},
			},
			expectedKey: "codex-api-key[0]",
			requestURL:  "/v0/management/codex-api-key?api-key=delete-codex",
			invoke:      (*Handler).DeleteCodexKey,
		},
		{
			name: "claude",
			cfg: &config.Config{
				ClaudeKey: []config.ClaudeKey{{APIKey: "delete-claude", BaseURL: "https://example.com/v1"}},
			},
			expectedKey: "claude-api-key[0]",
			requestURL:  "/v0/management/claude-api-key?api-key=delete-claude",
			invoke:      (*Handler).DeleteClaudeKey,
		},
		{
			name: "gemini",
			cfg: &config.Config{
				GeminiKey: []config.GeminiKey{{APIKey: "delete-gemini", BaseURL: "https://example.com"}},
			},
			expectedKey: "gemini-api-key[0]",
			requestURL:  "/v0/management/gemini-api-key?api-key=delete-gemini",
			invoke:      (*Handler).DeleteGeminiKey,
		},
		{
			name: "vertex",
			cfg: &config.Config{
				VertexCompatAPIKey: []config.VertexCompatKey{{APIKey: "delete-vertex", BaseURL: "https://example.com/v1"}},
			},
			expectedKey: "vertex-api-key[0]",
			requestURL:  "/v0/management/vertex-api-key?api-key=delete-vertex",
			invoke:      (*Handler).DeleteVertexCompatKey,
		},
		{
			name: "openai-compatibility",
			cfg: &config.Config{
				OpenAICompatibility: []config.OpenAICompatibility{
					{Name: "rightcode", BaseURL: "https://example.com/v1", APIKeyEntries: []config.OpenAICompatibilityAPIKey{{APIKey: "delete-openai"}}},
				},
			},
			expectedKey: "rightcode",
			requestURL:  "/v0/management/openai-compatibility?name=rightcode",
			invoke:      (*Handler).DeleteOpenAICompat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writableDir := t.TempDir()
			t.Setenv("WRITABLE_PATH", writableDir)

			configPath := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg := tt.cfg
			cfg.ConfigFilePath = configPath
			if err := modeldiscovery.SaveOpenAICompatCache(configPath, &modeldiscovery.OpenAICompatCacheFile{
				Providers: map[string]modeldiscovery.OpenAICompatCacheEntry{
					tt.expectedKey: {
						Name:        tt.expectedKey,
						Kind:        strings.Split(tt.expectedKey, "[")[0],
						ProviderKey: tt.expectedKey,
						BaseURL:     "https://example.com/v1",
						Models: []config.OpenAICompatibilityModel{
							{Name: "stale-model"},
						},
						Status: "fresh",
					},
				},
			}); err != nil {
				t.Fatalf("save cache: %v", err)
			}

			handler := NewHandler(cfg, configPath, nil)
			runtimeRefreshCalls := 0
			handler.SetRuntimeRefreshHook(func(context.Context) {
				runtimeRefreshCalls++
			})

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodDelete, tt.requestURL, nil)

			tt.invoke(handler, ctx)

			if recorder.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
			}
			if runtimeRefreshCalls != 1 {
				t.Fatalf("expected runtime refresh hook once, got %d", runtimeRefreshCalls)
			}

			cache, err := modeldiscovery.LoadOpenAICompatCache(configPath)
			if err != nil {
				t.Fatalf("load cache: %v", err)
			}
			if _, ok := cache.Providers[tt.expectedKey]; ok {
				t.Fatalf("expected cache entry %q to be pruned, got %#v", tt.expectedKey, cache.Providers)
			}
		})
	}
}
