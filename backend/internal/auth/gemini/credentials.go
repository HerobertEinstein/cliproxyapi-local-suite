package gemini

import (
	"fmt"
	"os"
	"strings"
)

const (
	ClientIDEnv     = "GEMINI_OAUTH_CLIENT_ID"
	ClientSecretEnv = "GEMINI_OAUTH_CLIENT_SECRET"
)

// OAuthClientCredentials returns the Gemini OAuth client credentials from env.
func OAuthClientCredentials() (string, string, error) {
	clientID := strings.TrimSpace(os.Getenv(ClientIDEnv))
	clientSecret := strings.TrimSpace(os.Getenv(ClientSecretEnv))
	if clientID == "" || clientSecret == "" {
		return "", "", fmt.Errorf("gemini oauth credentials not configured: set %s and %s", ClientIDEnv, ClientSecretEnv)
	}
	return clientID, clientSecret, nil
}
