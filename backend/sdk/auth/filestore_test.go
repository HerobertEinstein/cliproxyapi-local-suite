package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	geminiauth "github.com/router-for-me/CLIProxyAPI/v6/internal/auth/gemini"
)

func TestExtractAccessToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]any
		expected string
	}{
		{
			"antigravity top-level access_token",
			map[string]any{"access_token": "tok-abc"},
			"tok-abc",
		},
		{
			"gemini nested token.access_token",
			map[string]any{
				"token": map[string]any{"access_token": "tok-nested"},
			},
			"tok-nested",
		},
		{
			"top-level takes precedence over nested",
			map[string]any{
				"access_token": "tok-top",
				"token":        map[string]any{"access_token": "tok-nested"},
			},
			"tok-top",
		},
		{
			"empty metadata",
			map[string]any{},
			"",
		},
		{
			"whitespace-only access_token",
			map[string]any{"access_token": "   "},
			"",
		},
		{
			"wrong type access_token",
			map[string]any{"access_token": 12345},
			"",
		},
		{
			"token is not a map",
			map[string]any{"token": "not-a-map"},
			"",
		},
		{
			"nested whitespace-only",
			map[string]any{
				"token": map[string]any{"access_token": "  "},
			},
			"",
		},
		{
			"fallback to nested when top-level empty",
			map[string]any{
				"access_token": "",
				"token":        map[string]any{"access_token": "tok-fallback"},
			},
			"tok-fallback",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractAccessToken(tt.metadata)
			if got != tt.expected {
				t.Errorf("extractAccessToken() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRefreshGeminiAccessTokenFallsBackToEnvCredentials(t *testing.T) {
	t.Setenv(geminiauth.ClientIDEnv, "env-client-id")
	t.Setenv(geminiauth.ClientSecretEnv, "env-client-secret")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm returned error: %v", err)
		}
		if got := r.Form.Get("client_id"); got != "env-client-id" {
			t.Fatalf("client_id = %q, want %q", got, "env-client-id")
		}
		if got := r.Form.Get("client_secret"); got != "env-client-secret" {
			t.Fatalf("client_secret = %q, want %q", got, "env-client-secret")
		}
		if got := r.Form.Get("refresh_token"); got != "refresh-token" {
			t.Fatalf("refresh_token = %q, want %q", got, "refresh-token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh-access-token"}`))
	}))
	defer server.Close()

	tokenMap := map[string]any{
		"refresh_token": "refresh-token",
		"token_uri":     server.URL,
	}

	accessToken, err := refreshGeminiAccessToken(tokenMap, server.Client())
	if err != nil {
		t.Fatalf("refreshGeminiAccessToken returned error: %v", err)
	}
	if accessToken != "fresh-access-token" {
		t.Fatalf("accessToken = %q, want %q", accessToken, "fresh-access-token")
	}
}
