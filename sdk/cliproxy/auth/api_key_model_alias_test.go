package auth

import (
	"context"
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"gopkg.in/yaml.v3"
)

func mustLoadAPIKeyLogicalModelGroupsConfig(t *testing.T, source string) *internalconfig.Config {
	t.Helper()

	var cfg internalconfig.Config
	if err := yaml.Unmarshal([]byte(source), &cfg); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	cfg.SanitizeLogicalModelGroups()
	return &cfg
}

func TestLookupAPIKeyUpstreamModel(t *testing.T) {
	cfg := &internalconfig.Config{
		GeminiKey: []internalconfig.GeminiKey{
			{
				APIKey:  "k",
				BaseURL: "https://example.com",
				Models: []internalconfig.GeminiModel{
					{Name: "gemini-2.5-pro-exp-03-25", Alias: "g25p"},
					{Name: "gemini-2.5-flash(low)", Alias: "g25f"},
				},
			},
		},
	}

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(cfg)

	ctx := context.Background()
	_, _ = mgr.Register(ctx, &Auth{ID: "a1", Provider: "gemini", Attributes: map[string]string{"api_key": "k", "base_url": "https://example.com"}})

	tests := []struct {
		name   string
		authID string
		input  string
		want   string
	}{
		// Fast path + suffix preservation
		{"alias with suffix", "a1", "g25p(8192)", "gemini-2.5-pro-exp-03-25(8192)"},
		{"alias without suffix", "a1", "g25p", "gemini-2.5-pro-exp-03-25"},

		// Config suffix takes priority
		{"config suffix priority", "a1", "g25f(high)", "gemini-2.5-flash(low)"},
		{"config suffix no user suffix", "a1", "g25f", "gemini-2.5-flash(low)"},

		// Case insensitive
		{"uppercase alias", "a1", "G25P", "gemini-2.5-pro-exp-03-25"},
		{"mixed case with suffix", "a1", "G25p(4096)", "gemini-2.5-pro-exp-03-25(4096)"},

		// Direct name lookup
		{"upstream name direct", "a1", "gemini-2.5-pro-exp-03-25", "gemini-2.5-pro-exp-03-25"},
		{"upstream name with suffix", "a1", "gemini-2.5-pro-exp-03-25(8192)", "gemini-2.5-pro-exp-03-25(8192)"},

		// Cache miss scenarios
		{"non-existent auth", "non-existent", "g25p", ""},
		{"unknown alias", "a1", "unknown-alias", ""},
		{"empty auth ID", "", "g25p", ""},
		{"empty model", "a1", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved := mgr.lookupAPIKeyUpstreamModel(tt.authID, tt.input)
			if resolved != tt.want {
				t.Errorf("lookupAPIKeyUpstreamModel(%q, %q) = %q, want %q", tt.authID, tt.input, resolved, tt.want)
			}
		})
	}
}

func TestAPIKeyModelAlias_ConfigHotReload(t *testing.T) {
	cfg := &internalconfig.Config{
		GeminiKey: []internalconfig.GeminiKey{
			{
				APIKey: "k",
				Models: []internalconfig.GeminiModel{{Name: "gemini-2.5-pro-exp-03-25", Alias: "g25p"}},
			},
		},
	}

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(cfg)

	ctx := context.Background()
	_, _ = mgr.Register(ctx, &Auth{ID: "a1", Provider: "gemini", Attributes: map[string]string{"api_key": "k"}})

	// Initial alias
	if resolved := mgr.lookupAPIKeyUpstreamModel("a1", "g25p"); resolved != "gemini-2.5-pro-exp-03-25" {
		t.Fatalf("before reload: got %q, want %q", resolved, "gemini-2.5-pro-exp-03-25")
	}

	// Hot reload with new alias
	mgr.SetConfig(&internalconfig.Config{
		GeminiKey: []internalconfig.GeminiKey{
			{
				APIKey: "k",
				Models: []internalconfig.GeminiModel{{Name: "gemini-2.5-flash", Alias: "g25p"}},
			},
		},
	})

	// New alias should take effect
	if resolved := mgr.lookupAPIKeyUpstreamModel("a1", "g25p"); resolved != "gemini-2.5-flash" {
		t.Fatalf("after reload: got %q, want %q", resolved, "gemini-2.5-flash")
	}
}

func TestAPIKeyModelAlias_MultipleProviders(t *testing.T) {
	cfg := &internalconfig.Config{
		GeminiKey: []internalconfig.GeminiKey{{APIKey: "gemini-key", Models: []internalconfig.GeminiModel{{Name: "gemini-2.5-pro", Alias: "gp"}}}},
		ClaudeKey: []internalconfig.ClaudeKey{{APIKey: "claude-key", Models: []internalconfig.ClaudeModel{{Name: "claude-sonnet-4", Alias: "cs4"}}}},
		CodexKey:  []internalconfig.CodexKey{{APIKey: "codex-key", Models: []internalconfig.CodexModel{{Name: "o3", Alias: "o"}}}},
	}

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(cfg)

	ctx := context.Background()
	_, _ = mgr.Register(ctx, &Auth{ID: "gemini-auth", Provider: "gemini", Attributes: map[string]string{"api_key": "gemini-key"}})
	_, _ = mgr.Register(ctx, &Auth{ID: "claude-auth", Provider: "claude", Attributes: map[string]string{"api_key": "claude-key"}})
	_, _ = mgr.Register(ctx, &Auth{ID: "codex-auth", Provider: "codex", Attributes: map[string]string{"api_key": "codex-key"}})

	tests := []struct {
		authID, input, want string
	}{
		{"gemini-auth", "gp", "gemini-2.5-pro"},
		{"claude-auth", "cs4", "claude-sonnet-4"},
		{"codex-auth", "o", "o3"},
	}

	for _, tt := range tests {
		if resolved := mgr.lookupAPIKeyUpstreamModel(tt.authID, tt.input); resolved != tt.want {
			t.Errorf("lookupAPIKeyUpstreamModel(%q, %q) = %q, want %q", tt.authID, tt.input, resolved, tt.want)
		}
	}
}

func TestApplyAPIKeyModelAlias(t *testing.T) {
	cfg := &internalconfig.Config{
		GeminiKey: []internalconfig.GeminiKey{
			{APIKey: "k", Models: []internalconfig.GeminiModel{{Name: "gemini-2.5-pro-exp-03-25", Alias: "g25p"}}},
		},
	}

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(cfg)

	ctx := context.Background()
	apiKeyAuth := &Auth{ID: "a1", Provider: "gemini", Attributes: map[string]string{"api_key": "k"}}
	oauthAuth := &Auth{ID: "oauth-auth", Provider: "gemini", Attributes: map[string]string{"auth_kind": "oauth"}}
	_, _ = mgr.Register(ctx, apiKeyAuth)

	tests := []struct {
		name       string
		auth       *Auth
		inputModel string
		wantModel  string
	}{
		{
			name:       "api_key auth with alias",
			auth:       apiKeyAuth,
			inputModel: "g25p(8192)",
			wantModel:  "gemini-2.5-pro-exp-03-25(8192)",
		},
		{
			name:       "oauth auth passthrough",
			auth:       oauthAuth,
			inputModel: "some-model",
			wantModel:  "some-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolvedModel := mgr.applyAPIKeyModelAlias(tt.auth, tt.inputModel)

			if resolvedModel != tt.wantModel {
				t.Errorf("model = %q, want %q", resolvedModel, tt.wantModel)
			}
		})
	}
}

func TestLookupAPIKeyUpstreamModel_LogicalModelGroups(t *testing.T) {
	cfg := mustLoadAPIKeyLogicalModelGroupsConfig(t, `
gemini-api-key:
  - api-key: k
    models:
      - name: gemini-2.5-pro
logical-model-groups:
  current:
    ref: gemini-2.5-pro
  static:
    - alias: gemini-2.5-pro
      target: gemini-2.5-pro
      reasoning:
        mode: group
        effort: high
`)

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(cfg)

	ctx := context.Background()
	_, _ = mgr.Register(ctx, &Auth{ID: "a1", Provider: "gemini", Attributes: map[string]string{"api_key": "k"}})

	if resolved := mgr.lookupAPIKeyUpstreamModel("a1", "current"); resolved != "gemini-2.5-pro(high)" {
		t.Fatalf("lookupAPIKeyUpstreamModel() = %q, want %q", resolved, "gemini-2.5-pro(high)")
	}
}

func TestLookupAPIKeyUpstreamModel_LogicalModelGroupsWithoutConfigModels(t *testing.T) {
	cfg := mustLoadAPIKeyLogicalModelGroupsConfig(t, `
codex-api-key:
  - api-key: k
    base-url: https://example.com
logical-model-groups:
  current:
    ref: gpt-5.4
  static:
    - alias: gpt-5.4
      target: gpt-5.4
`)

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(cfg)

	ctx := context.Background()
	_, _ = mgr.Register(ctx, &Auth{ID: "a1", Provider: "codex", Attributes: map[string]string{"api_key": "k", "base_url": "https://example.com"}})

	if resolved := mgr.lookupAPIKeyUpstreamModel("a1", "current"); resolved != "gpt-5.4" {
		t.Fatalf("lookupAPIKeyUpstreamModel() = %q, want %q", resolved, "gpt-5.4")
	}
}

func TestLookupAPIKeyUpstreamModel_PrefersConfigLocatorForDuplicateCodexKeys(t *testing.T) {
	cfg := mustLoadAPIKeyLogicalModelGroupsConfig(t, `
codex-api-key:
  - api-key: shared-key
    base-url: https://example.com/v1
    models:
      - name: gpt-5.2
  - api-key: shared-key
    base-url: https://example.com/v1
    models:
      - name: gpt-5.4-mini
logical-model-groups:
  current:
    ref: gpt-5.4-mini
  static:
    - alias: gpt-5.2
      target: gpt-5.2
    - alias: gpt-5.4-mini
      target: gpt-5.4-mini
`)

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(cfg)

	ctx := context.Background()
	if _, err := mgr.Register(ctx, &Auth{
		ID:       "codex-auth-1",
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":        "shared-key",
			"base_url":       "https://example.com/v1",
			"source":         "config:codex[token-1]",
			"config_locator": "codex-api-key[0]",
		},
	}); err != nil {
		t.Fatalf("register first auth: %v", err)
	}
	if _, err := mgr.Register(ctx, &Auth{
		ID:       "codex-auth-2",
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":        "shared-key",
			"base_url":       "https://example.com/v1",
			"source":         "config:codex[token-2]",
			"config_locator": "codex-api-key[1]",
		},
	}); err != nil {
		t.Fatalf("register second auth: %v", err)
	}

	if resolved := mgr.lookupAPIKeyUpstreamModel("codex-auth-2", "current"); resolved != "gpt-5.4-mini" {
		t.Fatalf("lookupAPIKeyUpstreamModel() = %q, want %q", resolved, "gpt-5.4-mini")
	}
}

func TestLookupAPIKeyUpstreamModel_OpenAICompatibilityLogicalModelGroupsWithoutConfigModels(t *testing.T) {
	cfg := mustLoadAPIKeyLogicalModelGroupsConfig(t, `
openai-compatibility:
  - name: pool
    base-url: https://example.com
logical-model-groups:
  current:
    ref: claude-opus-4-6
  static:
    - alias: claude-opus-4-6
      target: claude-opus-4-6
`)

	mgr := NewManager(nil, nil, nil)
	mgr.SetConfig(cfg)

	ctx := context.Background()
	_, _ = mgr.Register(ctx, &Auth{
		ID:       "pool-auth",
		Provider: "pool",
		Attributes: map[string]string{
			"api_key":      "k",
			"compat_name":  "pool",
			"provider_key": "pool",
		},
	})

	if resolved := mgr.lookupAPIKeyUpstreamModel("pool-auth", "current"); resolved != "claude-opus-4-6" {
		t.Fatalf("lookupAPIKeyUpstreamModel() = %q, want %q", resolved, "claude-opus-4-6")
	}
}
