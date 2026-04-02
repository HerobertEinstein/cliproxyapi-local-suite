package antigravity

import "testing"

func TestOAuthClientCredentialsFromEnv(t *testing.T) {
	t.Run("missing credentials", func(t *testing.T) {
		t.Setenv(ClientIDEnv, "")
		t.Setenv(ClientSecretEnv, "")

		_, _, err := OAuthClientCredentials()
		if err == nil {
			t.Fatal("expected error when credentials are missing")
		}
	})

	t.Run("reads trimmed credentials", func(t *testing.T) {
		t.Setenv(ClientIDEnv, "  antigravity-client-id  ")
		t.Setenv(ClientSecretEnv, "  antigravity-client-secret  ")

		clientID, clientSecret, err := OAuthClientCredentials()
		if err != nil {
			t.Fatalf("OAuthClientCredentials returned error: %v", err)
		}
		if clientID != "antigravity-client-id" {
			t.Fatalf("clientID = %q, want %q", clientID, "antigravity-client-id")
		}
		if clientSecret != "antigravity-client-secret" {
			t.Fatalf("clientSecret = %q, want %q", clientSecret, "antigravity-client-secret")
		}
	})
}
