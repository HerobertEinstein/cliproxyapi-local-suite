package modeldiscovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestRescanOpenAICompatProviders_PersistsDiscoveryCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("expected bearer auth header, got %q", got)
		}
		if got := r.Header.Get("X-Test"); got != "hello" {
			t.Fatalf("expected custom header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-opus-4-6"},{"id":"gpt-5.4"}]}`))
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := &config.Config{
		ConfigFilePath: configPath,
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    "Pool",
				BaseURL: server.URL,
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "test-key"},
				},
				Headers: map[string]string{"X-Test": "hello"},
			},
		},
	}

	statuses, err := RescanOpenAICompatProviders(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("rescan providers: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].EffectiveSource != "discovered" {
		t.Fatalf("expected discovered effective source, got %q", statuses[0].EffectiveSource)
	}
	if len(statuses[0].EffectiveModels) != 2 {
		t.Fatalf("expected 2 effective models, got %d", len(statuses[0].EffectiveModels))
	}

	cache, err := LoadOpenAICompatCache(configPath)
	if err != nil {
		t.Fatalf("load cache: %v", err)
	}
	entry, ok := cache.Providers["pool"]
	if !ok {
		t.Fatal("expected provider entry in cache")
	}
	if len(entry.Models) != 2 {
		t.Fatalf("expected 2 cached models, got %d", len(entry.Models))
	}
	if !strings.HasSuffix(entry.SourceURL, "/v1/models") {
		t.Fatalf("expected source URL to end with /v1/models, got %q", entry.SourceURL)
	}
}

func TestEffectiveOpenAICompatModels_UsesDiscoveredWhenDeclaredEmpty(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cache := &OpenAICompatCacheFile{
		Providers: map[string]OpenAICompatCacheEntry{
			"pool": {
				Name:        "pool",
				ProviderKey: "pool",
				Models: []config.OpenAICompatibilityModel{
					{Name: "claude-opus-4-6"},
				},
			},
		},
	}
	if err := SaveOpenAICompatCache(configPath, cache); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	cfg := &config.Config{ConfigFilePath: configPath}
	compat := &config.OpenAICompatibility{Name: "pool"}
	models := EffectiveOpenAICompatModels(cfg, compat)
	if len(models) != 1 || models[0].Name != "claude-opus-4-6" {
		t.Fatalf("unexpected effective models: %#v", models)
	}
}

func TestRescanOpenAICompatProviders_PreservesStaleCacheAndUsesAliasOverlay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := SaveOpenAICompatCache(configPath, &OpenAICompatCacheFile{
		Providers: map[string]OpenAICompatCacheEntry{
			"pool": {
				Name:        "pool",
				ProviderKey: "pool",
				Models: []config.OpenAICompatibilityModel{
					{Name: "gpt-5.4"},
				},
				RefreshedAt: time.Now().Add(-5 * time.Minute).UTC(),
			},
		},
	}); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	cfg := &config.Config{
		ConfigFilePath: configPath,
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    "Pool",
				BaseURL: server.URL,
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "test-key"},
				},
				Models: []config.OpenAICompatibilityModel{
					{Name: "gpt-5.4", Alias: "current"},
					{Name: "gpt-5.3"},
				},
			},
		},
	}

	statuses, err := RescanOpenAICompatProviders(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("rescan providers: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].EffectiveSource != "stale-cache" {
		t.Fatalf("expected stale-cache effective source, got %q", statuses[0].EffectiveSource)
	}
	if !strings.Contains(strings.Join(statuses[0].EffectiveModels, ","), "current") {
		t.Fatalf("expected alias overlay in effective models, got %#v", statuses[0].EffectiveModels)
	}
	if strings.Contains(strings.Join(statuses[0].EffectiveModels, ","), "gpt-5.3") {
		t.Fatalf("did not expect unsupported declared-only model in effective models: %#v", statuses[0].EffectiveModels)
	}

	cache, err := LoadOpenAICompatCache(configPath)
	if err != nil {
		t.Fatalf("load cache: %v", err)
	}
	entry := cache.Providers["pool"]
	if len(entry.Models) != 1 || entry.Models[0].Name != "gpt-5.4" {
		t.Fatalf("expected stale cache to preserve discovered models, got %#v", entry.Models)
	}
	if entry.LastError == "" {
		t.Fatal("expected stale cache entry to record the latest discovery error")
	}
}

func TestRescanConfigProviders_DoesNotStarveLaterProvidersWhenOneIsSlow(t *testing.T) {
	var fastHits atomic.Int32

	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"slow-model"}]}`))
	}))
	defer slowServer.Close()

	fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fastHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"fast-model"}]}`))
	}))
	defer fastServer.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := &config.Config{
		ConfigFilePath: configPath,
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    "slow",
				BaseURL: slowServer.URL,
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "slow-key"},
				},
			},
			{
				Name:    "fast",
				BaseURL: fastServer.URL,
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "fast-key"},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	statuses, err := RescanConfigProviders(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("rescan providers: %v", err)
	}

	if fastHits.Load() == 0 {
		t.Fatal("expected fast provider to be scanned before shared timeout expired")
	}

	var fastStatus *ConfigProviderStatus
	for i := range statuses {
		if statuses[i].Key == "fast" {
			fastStatus = &statuses[i]
			break
		}
	}
	if fastStatus == nil {
		t.Fatalf("expected fast provider status, got %#v", statuses)
	}
	if fastStatus.DiscoveryState != "fresh" {
		t.Fatalf("expected fast provider discovery state fresh, got %q", fastStatus.DiscoveryState)
	}
	if len(fastStatus.EffectiveModels) != 1 || fastStatus.EffectiveModels[0] != "fast-model" {
		t.Fatalf("expected fast provider models to be discovered, got %#v", fastStatus.EffectiveModels)
	}
}
