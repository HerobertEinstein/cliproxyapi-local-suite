package iflow

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
		t.Setenv(ClientIDEnv, "  iflow-client-id  ")
		t.Setenv(ClientSecretEnv, "  iflow-client-secret  ")

		clientID, clientSecret, err := OAuthClientCredentials()
		if err != nil {
			t.Fatalf("OAuthClientCredentials returned error: %v", err)
		}
		if clientID != "iflow-client-id" {
			t.Fatalf("clientID = %q, want %q", clientID, "iflow-client-id")
		}
		if clientSecret != "iflow-client-secret" {
			t.Fatalf("clientSecret = %q, want %q", clientSecret, "iflow-client-secret")
		}
	})
}
