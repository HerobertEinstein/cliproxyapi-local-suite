package oauthcreds

import "testing"

func TestResolve_PrefersDirectMetadata(t *testing.T) {
	t.Setenv("TEST_OAUTH_CLIENT_ID", "env-id")
	t.Setenv("TEST_OAUTH_CLIENT_SECRET", "env-secret")

	creds, err := Resolve(map[string]any{
		"client_id":     "meta-id",
		"client_secret": "meta-secret",
	}, "TEST_OAUTH_CLIENT_ID", "TEST_OAUTH_CLIENT_SECRET")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if creds.ClientID != "meta-id" {
		t.Fatalf("expected direct metadata client id, got %q", creds.ClientID)
	}
	if creds.ClientSecret != "meta-secret" {
		t.Fatalf("expected direct metadata client secret, got %q", creds.ClientSecret)
	}
}

func TestResolve_UsesNestedTokenMetadata(t *testing.T) {
	creds, err := Resolve(map[string]any{
		"token": map[string]any{
			"client_id":     "token-id",
			"client_secret": "token-secret",
		},
	}, "TEST_OAUTH_CLIENT_ID", "TEST_OAUTH_CLIENT_SECRET")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if creds.ClientID != "token-id" {
		t.Fatalf("expected token metadata client id, got %q", creds.ClientID)
	}
	if creds.ClientSecret != "token-secret" {
		t.Fatalf("expected token metadata client secret, got %q", creds.ClientSecret)
	}
}

func TestResolve_FallsBackToEnvironment(t *testing.T) {
	t.Setenv("TEST_OAUTH_CLIENT_ID", "env-id")
	t.Setenv("TEST_OAUTH_CLIENT_SECRET", "env-secret")

	creds, err := Resolve(nil, "TEST_OAUTH_CLIENT_ID", "TEST_OAUTH_CLIENT_SECRET")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if creds.ClientID != "env-id" {
		t.Fatalf("expected environment client id, got %q", creds.ClientID)
	}
	if creds.ClientSecret != "env-secret" {
		t.Fatalf("expected environment client secret, got %q", creds.ClientSecret)
	}
}

func TestResolve_ReturnsErrorWhenMissing(t *testing.T) {
	_, err := Resolve(nil, "TEST_OAUTH_CLIENT_ID", "TEST_OAUTH_CLIENT_SECRET")
	if err == nil {
		t.Fatal("expected error when credentials are missing")
	}
	if got := err.Error(); got != "oauth client credentials are not configured; set TEST_OAUTH_CLIENT_ID and TEST_OAUTH_CLIENT_SECRET or persist client_id/client_secret in auth metadata" {
		t.Fatalf("unexpected error message: %q", got)
	}
}

func TestApply_PersistsCredentialsIntoMap(t *testing.T) {
	metadata := map[string]any{
		"type": "antigravity",
	}

	updated := Apply(metadata, Credentials{
		ClientID:     "saved-id",
		ClientSecret: "saved-secret",
	})

	if got := updated["client_id"]; got != "saved-id" {
		t.Fatalf("expected client_id to be persisted, got %#v", got)
	}
	if got := updated["client_secret"]; got != "saved-secret" {
		t.Fatalf("expected client_secret to be persisted, got %#v", got)
	}
	if got := updated["type"]; got != "antigravity" {
		t.Fatalf("expected original fields to be preserved, got %#v", got)
	}
}
