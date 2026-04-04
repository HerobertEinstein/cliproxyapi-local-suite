package auth

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func registerCanonicalStateKeyTestAuth(t *testing.T, cfgSource, authID string) (*Manager, *Auth) {
	t.Helper()

	cfg := mustLoadAPIKeyLogicalModelGroupsConfig(t, cfgSource)
	manager := NewManager(nil, &RoundRobinSelector{}, nil)
	manager.SetConfig(cfg)

	auth := &Auth{
		ID:       authID,
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":        "k",
			"base_url":       "https://example.com/v1",
			"config_locator": "codex-api-key[0]",
		},
	}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	return manager, auth
}

func TestManager_MarkResult_LogicalAliasFailureSharesCooldownWithResolvedStaticModel(t *testing.T) {
	manager, _ := registerCanonicalStateKeyTestAuth(t, `
codex-api-key:
  - api-key: k
    base-url: https://example.com/v1
    models:
      - name: gpt-5.4
logical-model-groups:
  current:
    ref: gpt-5.4
  static:
    - alias: gpt-5.4
      target: gpt-5.4
`, "canonical-share")

	manager.MarkResult(context.Background(), Result{
		AuthID:   "canonical-share",
		Provider: "codex",
		Model:    "current",
		Success:  false,
		Error:    &Error{HTTPStatus: http.StatusServiceUnavailable, Message: "upstream unavailable"},
	})

	updated, ok := manager.GetByID("canonical-share")
	if !ok || updated == nil {
		t.Fatalf("expected auth to exist")
	}

	if state := updated.ModelStates["gpt-5.4"]; state == nil {
		t.Fatalf("expected canonical state key %q to exist", "gpt-5.4")
	}
	if _, ok := updated.ModelStates["current"]; ok {
		t.Fatalf("did not expect legacy alias key %q to be written", "current")
	}

	blocked, reason, next := isAuthBlockedForModel(updated, "gpt-5.4", time.Now())
	if !blocked {
		t.Fatalf("expected resolved model to be blocked by alias failure")
	}
	if reason != blockReasonOther {
		t.Fatalf("blocked reason = %v, want %v", reason, blockReasonOther)
	}
	if next.IsZero() {
		t.Fatalf("expected cooldown deadline to be set")
	}
}

func TestManager_MarkResult_CurrentPointerSwitchDoesNotStickOldTargetState(t *testing.T) {
	manager, _ := registerCanonicalStateKeyTestAuth(t, `
codex-api-key:
  - api-key: k
    base-url: https://example.com/v1
    models:
      - name: gpt-5.2
      - name: gpt-5.4
logical-model-groups:
  current:
    ref: gpt-5.2
  static:
    - alias: gpt-5.2
      target: gpt-5.2
    - alias: gpt-5.4
      target: gpt-5.4
`, "canonical-pointer")

	manager.MarkResult(context.Background(), Result{
		AuthID:   "canonical-pointer",
		Provider: "codex",
		Model:    "current",
		Success:  false,
		Error:    &Error{HTTPStatus: http.StatusServiceUnavailable, Message: "upstream unavailable"},
	})

	manager.SetConfig(mustLoadAPIKeyLogicalModelGroupsConfig(t, `
codex-api-key:
  - api-key: k
    base-url: https://example.com/v1
    models:
      - name: gpt-5.2
      - name: gpt-5.4
logical-model-groups:
  current:
    ref: gpt-5.4
  static:
    - alias: gpt-5.2
      target: gpt-5.2
    - alias: gpt-5.4
      target: gpt-5.4
`))

	updated, ok := manager.GetByID("canonical-pointer")
	if !ok || updated == nil {
		t.Fatalf("expected auth to exist")
	}

	blocked, reason, next := isAuthBlockedForModel(updated, "current", time.Now())
	if blocked {
		t.Fatalf("expected current to stop inheriting old target cooldown, got blocked reason=%v next=%v", reason, next)
	}
	if state := updated.ModelStates["gpt-5.2"]; state == nil {
		t.Fatalf("expected old target state key %q to be preserved", "gpt-5.2")
	}
	if _, ok := updated.ModelStates["current"]; ok {
		t.Fatalf("did not expect sticky alias key %q after pointer switch", "current")
	}
}
