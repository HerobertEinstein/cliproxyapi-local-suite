package gemini

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
		t.Setenv(ClientIDEnv, "  gemini-client-id  ")
		t.Setenv(ClientSecretEnv, "  gemini-client-secret  ")

		clientID, clientSecret, err := OAuthClientCredentials()
		if err != nil {
			t.Fatalf("OAuthClientCredentials returned error: %v", err)
		}
		if clientID != "gemini-client-id" {
			t.Fatalf("clientID = %q, want %q", clientID, "gemini-client-id")
		}
		if clientSecret != "gemini-client-secret" {
			t.Fatalf("clientSecret = %q, want %q", clientSecret, "gemini-client-secret")
		}
	})
}
