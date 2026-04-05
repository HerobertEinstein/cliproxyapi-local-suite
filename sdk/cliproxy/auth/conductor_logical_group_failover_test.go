package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

type logicalGroupFailoverExecutor struct {
	id string
}

func (e logicalGroupFailoverExecutor) Identifier() string { return e.id }

func (logicalGroupFailoverExecutor) Execute(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (logicalGroupFailoverExecutor) ExecuteStream(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, nil
}

func (logicalGroupFailoverExecutor) Refresh(ctx context.Context, auth *Auth) (*Auth, error) {
	return auth, nil
}

func (logicalGroupFailoverExecutor) CountTokens(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (logicalGroupFailoverExecutor) HttpRequest(ctx context.Context, auth *Auth, req *http.Request) (*http.Response, error) {
	return nil, nil
}

func newLogicalGroupFailoverManager(t *testing.T) *Manager {
	t.Helper()

	manager := NewManager(nil, &RoundRobinSelector{}, nil)
	manager.RegisterExecutor(logicalGroupFailoverExecutor{id: "codex"})
	manager.RegisterExecutor(logicalGroupFailoverExecutor{id: "claude"})

	cfg := &internalconfig.Config{
		CodexKey: []internalconfig.CodexKey{
			{APIKey: "codex-key"},
		},
		ClaudeKey: []internalconfig.ClaudeKey{
			{APIKey: "claude-key"},
		},
		LogicalModelGroups: internalconfig.LogicalModelGroups{
			Current: internalconfig.LogicalModelCurrent{Ref: "gpt-5.4"},
			Static: []internalconfig.LogicalModelGroup{
				{
					Alias:              "gpt-5.4",
					Target:             "gpt-5.4",
					PreferredProviders: []string{"codex", "claude"},
				},
			},
		},
	}
	cfg.SanitizeLogicalModelGroups()
	manager.SetConfig(cfg)
	return manager
}

func TestManagerPickNextMixed_LogicalGroupPreferredProvidersBeatProviderOrder(t *testing.T) {
	ctx := context.Background()
	manager := newLogicalGroupFailoverManager(t)

	registerSchedulerModels(t, "codex", "current", "codex-auth")
	registerSchedulerModels(t, "claude", "current", "claude-auth")

	if _, err := manager.Register(ctx, &Auth{
		ID:       "codex-auth",
		Provider: "codex",
		Attributes: map[string]string{
			"auth_kind": "api_key",
			"api_key":   "codex-key",
		},
	}); err != nil {
		t.Fatalf("register codex auth: %v", err)
	}
	if _, err := manager.Register(ctx, &Auth{
		ID:       "claude-auth",
		Provider: "claude",
		Attributes: map[string]string{
			"auth_kind": "api_key",
			"api_key":   "claude-key",
		},
	}); err != nil {
		t.Fatalf("register claude auth: %v", err)
	}

	got, _, provider, err := manager.pickNextMixed(ctx, []string{"claude", "codex"}, "current", cliproxyexecutor.Options{}, nil)
	if err != nil {
		t.Fatalf("pickNextMixed() error = %v", err)
	}
	if got == nil {
		t.Fatal("pickNextMixed() auth = nil")
	}
	if provider != "codex" {
		t.Fatalf("pickNextMixed() provider = %q, want %q", provider, "codex")
	}
	if got.ID != "codex-auth" {
		t.Fatalf("pickNextMixed() auth.ID = %q, want %q", got.ID, "codex-auth")
	}
}

func TestManagerPickNextMixed_LogicalGroupFallsBackWhenPreferredProviderCoolsDown(t *testing.T) {
	ctx := context.Background()
	manager := newLogicalGroupFailoverManager(t)

	registerSchedulerModels(t, "codex", "current", "codex-auth")
	registerSchedulerModels(t, "claude", "current", "claude-auth")

	nextRetry := time.Now().Add(10 * time.Minute)
	if _, err := manager.Register(ctx, &Auth{
		ID:       "codex-auth",
		Provider: "codex",
		Attributes: map[string]string{
			"auth_kind": "api_key",
			"api_key":   "codex-key",
		},
		ModelStates: map[string]*ModelState{
			"gpt-5.4": {
				Status:         StatusError,
				Unavailable:    true,
				NextRetryAfter: nextRetry,
			},
		},
	}); err != nil {
		t.Fatalf("register codex auth: %v", err)
	}
	if _, err := manager.Register(ctx, &Auth{
		ID:       "claude-auth",
		Provider: "claude",
		Attributes: map[string]string{
			"auth_kind": "api_key",
			"api_key":   "claude-key",
		},
	}); err != nil {
		t.Fatalf("register claude auth: %v", err)
	}

	got, _, provider, err := manager.pickNextMixed(ctx, []string{"codex", "claude"}, "current", cliproxyexecutor.Options{}, nil)
	if err != nil {
		t.Fatalf("pickNextMixed() error = %v", err)
	}
	if got == nil {
		t.Fatal("pickNextMixed() auth = nil")
	}
	if provider != "claude" {
		t.Fatalf("pickNextMixed() provider = %q, want %q", provider, "claude")
	}
	if got.ID != "claude-auth" {
		t.Fatalf("pickNextMixed() auth.ID = %q, want %q", got.ID, "claude-auth")
	}
}
