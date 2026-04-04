package oauthcreds

import (
	"fmt"
	"os"
	"strings"
)

type ProviderConfig struct {
	ClientIDEnv     string
	ClientSecretEnv string
}

type Credentials struct {
	ClientID     string
	ClientSecret string
}

var (
	Gemini = ProviderConfig{
		ClientIDEnv:     "CPA_GEMINI_OAUTH_CLIENT_ID",
		ClientSecretEnv: "CPA_GEMINI_OAUTH_CLIENT_SECRET",
	}
	Antigravity = ProviderConfig{
		ClientIDEnv:     "CPA_ANTIGRAVITY_OAUTH_CLIENT_ID",
		ClientSecretEnv: "CPA_ANTIGRAVITY_OAUTH_CLIENT_SECRET",
	}
	IFlow = ProviderConfig{
		ClientIDEnv:     "CPA_IFLOW_OAUTH_CLIENT_ID",
		ClientSecretEnv: "CPA_IFLOW_OAUTH_CLIENT_SECRET",
	}
)

func (p ProviderConfig) Resolve(metadata map[string]any) (Credentials, error) {
	return Resolve(metadata, p.ClientIDEnv, p.ClientSecretEnv)
}

func Resolve(metadata map[string]any, clientIDEnv, clientSecretEnv string) (Credentials, error) {
	clientID := firstString(
		valueFromMap(metadata, "client_id"),
		valueFromNestedToken(metadata, "client_id"),
		os.Getenv(clientIDEnv),
	)
	clientSecret := firstString(
		valueFromMap(metadata, "client_secret"),
		valueFromNestedToken(metadata, "client_secret"),
		os.Getenv(clientSecretEnv),
	)
	if clientID == "" || clientSecret == "" {
		return Credentials{}, fmt.Errorf(
			"oauth client credentials are not configured; set %s and %s or persist client_id/client_secret in auth metadata",
			clientIDEnv,
			clientSecretEnv,
		)
	}
	return Credentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}, nil
}

func Apply(metadata map[string]any, creds Credentials) map[string]any {
	updated := cloneMap(metadata)
	if updated == nil {
		updated = make(map[string]any, 2)
	}
	updated["client_id"] = strings.TrimSpace(creds.ClientID)
	updated["client_secret"] = strings.TrimSpace(creds.ClientSecret)
	return updated
}

func valueFromMap(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	raw, ok := metadata[key]
	if !ok {
		return ""
	}
	str, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(str)
}

func valueFromNestedToken(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	raw, ok := metadata["token"]
	if !ok || raw == nil {
		return ""
	}
	tokenMap, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	return valueFromMap(tokenMap, key)
}

func firstString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
