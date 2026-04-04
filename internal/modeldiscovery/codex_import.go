package modeldiscovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
)

const codexImportHealthFileName = "ccswitch-codex-import-health.json"

type CodexImportHealthSnapshot struct {
	ConfigLocator   string                  `json:"config_locator,omitempty"`
	EffectiveModels []string                `json:"effective_models,omitempty"`
	Health          CodexImportHealthStatus `json:"health"`
}

type CodexImportHealthStatus struct {
	Status string `json:"status,omitempty"`
}

func CodexImportHealthFilePath(configFilePath string) string {
	if base := util.WritablePath(); base != "" {
		return filepath.Join(base, "state", codexImportHealthFileName)
	}
	configFilePath = strings.TrimSpace(configFilePath)
	if configFilePath == "" {
		return ""
	}
	base := filepath.Dir(configFilePath)
	if info, err := os.Stat(configFilePath); err == nil && info.IsDir() {
		base = configFilePath
	}
	parent := filepath.Dir(base)
	if strings.EqualFold(filepath.Base(base), "config") {
		return filepath.Join(parent, "state", codexImportHealthFileName)
	}
	return filepath.Join(base, "state", codexImportHealthFileName)
}

func LoadCodexImportHealthSnapshots(configFilePath string) ([]CodexImportHealthSnapshot, error) {
	path := CodexImportHealthFilePath(configFilePath)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var snapshots []CodexImportHealthSnapshot
	if err := json.Unmarshal(data, &snapshots); err != nil {
		return nil, err
	}
	return snapshots, nil
}

func FindCodexImportHealthSnapshotByLocator(configFilePath, locator string) (CodexImportHealthSnapshot, bool) {
	locator = strings.TrimSpace(locator)
	if locator == "" {
		return CodexImportHealthSnapshot{}, false
	}
	snapshots, err := LoadCodexImportHealthSnapshots(configFilePath)
	if err != nil {
		return CodexImportHealthSnapshot{}, false
	}
	for _, snapshot := range snapshots {
		if strings.EqualFold(strings.TrimSpace(snapshot.ConfigLocator), locator) {
			return snapshot, true
		}
	}
	return CodexImportHealthSnapshot{}, false
}

func CodexImportEffectiveModelsByLocator(configFilePath, locator string) ([]string, bool) {
	snapshot, ok := FindCodexImportHealthSnapshotByLocator(configFilePath, locator)
	if !ok {
		return nil, false
	}
	return normalizeCodexImportModels(snapshot.EffectiveModels), true
}

func IsCodexImportHardFailure(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "auth_failed", "unreachable", "html_frontend":
		return true
	default:
		return false
	}
}

func normalizeCodexImportModels(models []string) []string {
	if len(models) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		trimmed := strings.TrimSpace(model)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
