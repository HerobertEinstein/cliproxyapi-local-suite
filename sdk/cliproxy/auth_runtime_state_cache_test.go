package cliproxy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func writeRuntimeStateConfigFixture(t *testing.T) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}

func TestPersistAuthRuntimeState_ConfigBackedClaudeAPIKeyPersistsAndLoads(t *testing.T) {
	configPath := writeRuntimeStateConfigFixture(t)
	nextRetryAfter := time.Now().Add(30 * time.Minute).UTC()

	auth := &coreauth.Auth{
		ID:       "claude-config-auth",
		Provider: "claude",
		Attributes: map[string]string{
			"auth_kind":      "apikey",
			"source":         "config:claude-api-key[test]",
			"config_locator": "claude-api-key[0]",
		},
		ModelStates: map[string]*coreauth.ModelState{
			"claude-sonnet-4": {
				Unavailable:    true,
				Status:         coreauth.StatusError,
				StatusMessage:  "rate_limit",
				NextRetryAfter: nextRetryAfter,
			},
		},
	}

	if err := persistAuthRuntimeState(configPath, auth); err != nil {
		t.Fatalf("persist state: %v", err)
	}

	snapshot, err := loadAuthRuntimeStateFile(configPath)
	if err != nil {
		t.Fatalf("load state file: %v", err)
	}
	if len(snapshot.Entries) != 1 {
		t.Fatalf("expected 1 runtime entry, got %d", len(snapshot.Entries))
	}
	if got := snapshot.Entries[0].ConfigLocator; got != "claude-api-key[0]" {
		t.Fatalf("config locator = %q, want %q", got, "claude-api-key[0]")
	}
	if _, ok := snapshot.Entries[0].ModelStates["claude-sonnet-4"]; !ok {
		t.Fatalf("expected persisted claude model state")
	}

	restored := &coreauth.Auth{
		ID:       "claude-config-auth",
		Provider: "claude",
		Attributes: map[string]string{
			"auth_kind":      "apikey",
			"source":         "config:claude-api-key[test]",
			"config_locator": "claude-api-key[0]",
		},
	}
	if merged := mergePersistedAuthRuntimeState(configPath, restored); !merged {
		t.Fatalf("expected persisted claude runtime state to merge on cold start")
	}
	if state := restored.ModelStates["claude-sonnet-4"]; state == nil || !state.Unavailable {
		t.Fatalf("expected restored claude state to remain unavailable after merge")
	}
}

func TestPersistAuthRuntimeState_CanonicalizesLogicalAliasKeys(t *testing.T) {
	configPath := writeRuntimeStateConfigFixture(t)
	nextRetryAfter := time.Now().Add(45 * time.Minute).UTC()

	auth := &coreauth.Auth{
		ID:       "codex-canonical-write",
		Provider: "codex",
		Attributes: map[string]string{
			"auth_kind":      "apikey",
			"source":         "config:codex-api-key[test]",
			"config_locator": "codex-api-key[0]",
		},
		APIKeyModelAliases: map[string]string{
			"current": "gpt-5.4",
		},
		ModelStates: map[string]*coreauth.ModelState{
			"current": {
				Unavailable:    true,
				Status:         coreauth.StatusError,
				StatusMessage:  "upstream_unavailable",
				NextRetryAfter: nextRetryAfter,
			},
		},
	}

	if err := persistAuthRuntimeState(configPath, auth); err != nil {
		t.Fatalf("persist state: %v", err)
	}

	snapshot, err := loadAuthRuntimeStateFile(configPath)
	if err != nil {
		t.Fatalf("load state file: %v", err)
	}
	if len(snapshot.Entries) != 1 {
		t.Fatalf("expected 1 runtime entry, got %d", len(snapshot.Entries))
	}

	states := snapshot.Entries[0].ModelStates
	if _, ok := states["gpt-5.4"]; !ok {
		t.Fatalf("expected canonical key %q to be persisted", "gpt-5.4")
	}
	if _, ok := states["current"]; ok {
		t.Fatalf("did not expect logical alias key %q to remain in persisted state", "current")
	}
}

func TestMergePersistedAuthRuntimeState_CanonicalizesLegacyLogicalAliasKeys(t *testing.T) {
	configPath := writeRuntimeStateConfigFixture(t)
	nextRetryAfter := time.Now().Add(20 * time.Minute).UTC().Format(time.RFC3339)

	statePath := authRuntimeStateFilePath(configPath)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(statePath, []byte(`{
  "entries": [
    {
      "auth_id": "codex-canonical-read",
      "provider": "codex",
      "config_locator": "codex-api-key[0]",
      "model_states": {
        "current": {
          "status": "error",
          "status_message": "legacy_current_key",
          "unavailable": true,
          "next_retry_after": "`+nextRetryAfter+`"
        }
      }
    }
  ]
}`), 0o600); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	auth := &coreauth.Auth{
		ID:       "codex-canonical-read",
		Provider: "codex",
		Attributes: map[string]string{
			"auth_kind":      "apikey",
			"source":         "config:codex-api-key[test]",
			"config_locator": "codex-api-key[0]",
		},
		APIKeyModelAliases: map[string]string{
			"current": "gpt-5.4",
		},
	}

	if merged := mergePersistedAuthRuntimeState(configPath, auth); !merged {
		t.Fatalf("expected legacy alias state to merge")
	}
	if state := auth.ModelStates["gpt-5.4"]; state == nil || !state.Unavailable {
		t.Fatalf("expected legacy alias state to load under canonical key %q", "gpt-5.4")
	}
	if _, ok := auth.ModelStates["current"]; ok {
		t.Fatalf("did not expect legacy alias key %q to remain after merge", "current")
	}
}
