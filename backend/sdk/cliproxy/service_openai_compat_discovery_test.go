package cliproxy

import (
	"reflect"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

func TestPendingOpenAICompatDiscoveryNames_InitialLoadIncludesOnlyUndeclaredProviders(t *testing.T) {
	cfg := &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{Name: "pool-a", BaseURL: "https://a.example.com"},
			{Name: "pool-b", BaseURL: "https://b.example.com", Models: []config.OpenAICompatibilityModel{{Name: "gpt-5.4"}}},
			{Name: "pool-c", BaseURL: "https://c.example.com"},
		},
	}

	got := pendingOpenAICompatDiscoveryNames(nil, cfg)
	want := []string{"pool-a", "pool-c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pendingOpenAICompatDiscoveryNames(nil, cfg) = %#v, want %#v", got, want)
	}
}

func TestPendingOpenAICompatDiscoveryNames_ReloadIncludesOnlyChangedUndeclaredProviders(t *testing.T) {
	previous := &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    "pool-a",
				BaseURL: "https://a.example.com",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "k1"},
				},
			},
			{
				Name:    "pool-b",
				BaseURL: "https://b.example.com",
				Models:  []config.OpenAICompatibilityModel{{Name: "gpt-5.4"}},
			},
			{
				Name:    "pool-c",
				BaseURL: "https://c.example.com",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "same"},
				},
			},
		},
	}
	current := &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    "pool-a",
				BaseURL: "https://a.example.com",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "k2"},
				},
			},
			{
				Name:    "pool-b",
				BaseURL: "https://b.example.com",
			},
			{
				Name:    "pool-c",
				BaseURL: "https://c.example.com",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{
					{APIKey: "same"},
				},
			},
			{
				Name:    "pool-d",
				BaseURL: "https://d.example.com",
			},
		},
	}

	got := pendingOpenAICompatDiscoveryNames(previous, current)
	want := []string{"pool-a", "pool-b", "pool-d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pendingOpenAICompatDiscoveryNames(previous, current) = %#v, want %#v", got, want)
	}
}
