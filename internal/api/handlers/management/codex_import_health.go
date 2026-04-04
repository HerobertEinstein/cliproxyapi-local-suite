package management

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
)

const codexImportHealthFileName = "ccswitch-codex-import-health.json"

type codexImportHealthStatus struct {
	Provider  string `json:"provider,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	Status    string `json:"status,omitempty"`
	Detail    string `json:"detail,omitempty"`
	CheckedAt string `json:"checked_at,omitempty"`
}

type codexImportHealthProvider struct {
	ConfigLocator    string                  `json:"config_locator,omitempty"`
	Provider         string                  `json:"provider"`
	EndpointURL      string                  `json:"endpoint_url,omitempty"`
	ConfigBaseURL    string                  `json:"config_base_url,omitempty"`
	EffectiveBaseURL string                  `json:"effective_base_url,omitempty"`
	DefaultModel     string                  `json:"default_model,omitempty"`
	TemplateModels   []string                `json:"template_models,omitempty"`
	DiscoveredModels []string                `json:"discovered_models,omitempty"`
	EffectiveModels  []string                `json:"effective_models,omitempty"`
	EffectiveSource  string                  `json:"effective_source,omitempty"`
	Health           codexImportHealthStatus `json:"health"`
}

type codexImportHealthResponse struct {
	Available bool                        `json:"available"`
	Source    string                      `json:"source"`
	Path      string                      `json:"path,omitempty"`
	UpdatedAt string                      `json:"updated_at,omitempty"`
	Providers []codexImportHealthProvider `json:"providers"`
}

func (h *Handler) codexImportHealthPath() string {
	candidates := make([]string, 0, 3)
	seen := make(map[string]struct{})
	addCandidate := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		candidates = append(candidates, path)
	}

	if writable := util.WritablePath(); writable != "" {
		addCandidate(filepath.Join(writable, "state", codexImportHealthFileName))
	}

	configFilePath := strings.TrimSpace(h.configFilePath)
	if configFilePath == "" && h.cfg != nil {
		configFilePath = strings.TrimSpace(h.cfg.ConfigFilePath)
	}
	if configFilePath != "" {
		base := filepath.Dir(configFilePath)
		if info, err := os.Stat(configFilePath); err == nil && info.IsDir() {
			base = configFilePath
		}
		parent := filepath.Dir(base)
		if strings.EqualFold(filepath.Base(base), "config") {
			addCandidate(filepath.Join(parent, "state", codexImportHealthFileName))
			addCandidate(filepath.Join(base, "state", codexImportHealthFileName))
		} else {
			addCandidate(filepath.Join(base, "state", codexImportHealthFileName))
			addCandidate(filepath.Join(parent, "state", codexImportHealthFileName))
		}
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

func loadCodexImportHealth(path string) (*codexImportHealthResponse, error) {
	response := &codexImportHealthResponse{
		Available: false,
		Source:    "cc-switch-codex-import-health",
		Path:      path,
		Providers: []codexImportHealthProvider{},
	}
	if strings.TrimSpace(path) == "" {
		return response, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return response, nil
		}
		return nil, err
	}

	var providers []codexImportHealthProvider
	if err := json.Unmarshal(data, &providers); err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	response.Available = true
	response.UpdatedAt = info.ModTime().Format(time.RFC3339)
	response.Providers = providers
	return response, nil
}

func (h *Handler) GetCodexImportHealth(c *gin.Context) {
	healthPath := h.codexImportHealthPath()
	payload, err := loadCodexImportHealth(healthPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load codex import health"})
		return
	}
	c.JSON(http.StatusOK, payload)
}
