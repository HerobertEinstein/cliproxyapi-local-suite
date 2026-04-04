package cliproxy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

const authRuntimeStateFileName = "auth-runtime-state.json"

type authRuntimeStateFile struct {
	Entries []authRuntimeStateEntry `json:"entries,omitempty"`
}

type authRuntimeStateEntry struct {
	AuthID        string                          `json:"auth_id,omitempty"`
	Provider      string                          `json:"provider,omitempty"`
	ConfigLocator string                          `json:"config_locator,omitempty"`
	UpdatedAt     time.Time                       `json:"updated_at,omitempty"`
	ModelStates   map[string]*coreauth.ModelState `json:"model_states,omitempty"`
}

type authRuntimeStateHook struct {
	configPath string
	manager    *coreauth.Manager
}

var authRuntimeStateFileMu sync.Mutex

func authRuntimeStateFilePath(configFilePath string) string {
	if base := util.WritablePath(); base != "" {
		return filepath.Join(base, "state", authRuntimeStateFileName)
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
		return filepath.Join(parent, "state", authRuntimeStateFileName)
	}
	return filepath.Join(base, "state", authRuntimeStateFileName)
}

func shouldPersistAuthRuntimeState(auth *coreauth.Auth) bool {
	if auth == nil {
		return false
	}
	if auth.Attributes == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(auth.Attributes["auth_kind"]), "apikey") {
		return false
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(auth.Attributes["source"])), "config:")
}

func authRuntimeCanonicalModelKey(auth *coreauth.Auth, model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}

	base := strings.TrimSpace(thinking.ParseSuffix(model).ModelName)
	if base == "" {
		base = model
	}
	if auth != nil && len(auth.APIKeyModelAliases) > 0 {
		aliasKey := strings.ToLower(base)
		if mapped := strings.TrimSpace(auth.APIKeyModelAliases[aliasKey]); mapped != "" {
			if resolved := strings.TrimSpace(thinking.ParseSuffix(mapped).ModelName); resolved != "" {
				return resolved
			}
			return mapped
		}
	}
	return base
}

func selectPreferredRuntimeState(current, candidate *coreauth.ModelState) *coreauth.ModelState {
	if candidate == nil {
		return current
	}
	if current == nil {
		return candidate.Clone()
	}
	switch {
	case candidate.NextRetryAfter.After(current.NextRetryAfter):
		return candidate.Clone()
	case current.NextRetryAfter.After(candidate.NextRetryAfter):
		return current
	case candidate.UpdatedAt.After(current.UpdatedAt):
		return candidate.Clone()
	default:
		return current
	}
}

func normalizedAuthRuntimeModelStates(auth *coreauth.Auth, states map[string]*coreauth.ModelState, now time.Time) map[string]*coreauth.ModelState {
	if len(states) == 0 {
		return nil
	}
	out := make(map[string]*coreauth.ModelState)
	for modelID, state := range states {
		if state == nil || !state.Unavailable || state.NextRetryAfter.IsZero() || !state.NextRetryAfter.After(now) {
			continue
		}
		canonical := authRuntimeCanonicalModelKey(auth, modelID)
		if canonical == "" {
			continue
		}
		out[canonical] = selectPreferredRuntimeState(out[canonical], state)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func activeAuthRuntimeModelStates(auth *coreauth.Auth, now time.Time) map[string]*coreauth.ModelState {
	if auth == nil || len(auth.ModelStates) == 0 {
		return nil
	}
	return normalizedAuthRuntimeModelStates(auth, auth.ModelStates, now)
}

func loadAuthRuntimeStateFile(configFilePath string) (authRuntimeStateFile, error) {
	path := authRuntimeStateFilePath(configFilePath)
	if path == "" {
		return authRuntimeStateFile{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return authRuntimeStateFile{}, nil
		}
		return authRuntimeStateFile{}, err
	}
	if len(data) == 0 {
		return authRuntimeStateFile{}, nil
	}
	var snapshot authRuntimeStateFile
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return authRuntimeStateFile{}, err
	}
	return snapshot, nil
}

func saveAuthRuntimeStateFile(configFilePath string, snapshot authRuntimeStateFile) error {
	path := authRuntimeStateFilePath(configFilePath)
	if path == "" {
		return nil
	}
	if len(snapshot.Entries) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func findAuthRuntimeStateEntry(snapshot authRuntimeStateFile, auth *coreauth.Auth) (int, authRuntimeStateEntry, bool) {
	if auth == nil {
		return -1, authRuntimeStateEntry{}, false
	}
	authID := strings.TrimSpace(auth.ID)
	configLocator := strings.TrimSpace(auth.Attributes["config_locator"])
	for i, entry := range snapshot.Entries {
		if authID != "" && strings.EqualFold(strings.TrimSpace(entry.AuthID), authID) {
			return i, entry, true
		}
		if configLocator != "" && strings.EqualFold(strings.TrimSpace(entry.ConfigLocator), configLocator) {
			return i, entry, true
		}
	}
	return -1, authRuntimeStateEntry{}, false
}

func mergePersistedAuthRuntimeState(configFilePath string, auth *coreauth.Auth) bool {
	if !shouldPersistAuthRuntimeState(auth) {
		return false
	}

	authRuntimeStateFileMu.Lock()
	defer authRuntimeStateFileMu.Unlock()

	snapshot, err := loadAuthRuntimeStateFile(configFilePath)
	if err != nil {
		log.Warnf("failed to load auth runtime state cache: %v", err)
		return false
	}

	_, entry, ok := findAuthRuntimeStateEntry(snapshot, auth)
	if !ok || len(entry.ModelStates) == 0 {
		return false
	}
	if auth.ModelStates == nil {
		auth.ModelStates = make(map[string]*coreauth.ModelState, len(entry.ModelStates))
	}

	now := time.Now()
	merged := false
	for modelID, state := range normalizedAuthRuntimeModelStates(auth, entry.ModelStates, now) {
		if existing, exists := auth.ModelStates[modelID]; exists && existing != nil {
			auth.ModelStates[modelID] = selectPreferredRuntimeState(existing, state)
			continue
		}
		auth.ModelStates[modelID] = state.Clone()
		merged = true
	}
	return merged
}

func persistAuthRuntimeState(configFilePath string, auth *coreauth.Auth) error {
	if !shouldPersistAuthRuntimeState(auth) {
		return nil
	}

	authRuntimeStateFileMu.Lock()
	defer authRuntimeStateFileMu.Unlock()

	snapshot, err := loadAuthRuntimeStateFile(configFilePath)
	if err != nil {
		return err
	}

	activeStates := activeAuthRuntimeModelStates(auth, time.Now())
	index, _, exists := findAuthRuntimeStateEntry(snapshot, auth)
	if len(activeStates) == 0 {
		if exists {
			snapshot.Entries = append(snapshot.Entries[:index], snapshot.Entries[index+1:]...)
		}
		return saveAuthRuntimeStateFile(configFilePath, snapshot)
	}

	entry := authRuntimeStateEntry{
		AuthID:        strings.TrimSpace(auth.ID),
		Provider:      strings.TrimSpace(auth.Provider),
		ConfigLocator: strings.TrimSpace(auth.Attributes["config_locator"]),
		UpdatedAt:     time.Now().UTC(),
		ModelStates:   activeStates,
	}
	if exists {
		snapshot.Entries[index] = entry
	} else {
		snapshot.Entries = append(snapshot.Entries, entry)
	}
	return saveAuthRuntimeStateFile(configFilePath, snapshot)
}

func (h *authRuntimeStateHook) OnAuthRegistered(context.Context, *coreauth.Auth) {}

func (h *authRuntimeStateHook) OnAuthUpdated(context.Context, *coreauth.Auth) {}

func (h *authRuntimeStateHook) OnResult(ctx context.Context, result coreauth.Result) {
	if h == nil || h.manager == nil || strings.TrimSpace(result.AuthID) == "" {
		return
	}
	auth, ok := h.manager.GetByID(result.AuthID)
	if !ok || auth == nil {
		return
	}
	if err := persistAuthRuntimeState(h.configPath, auth); err != nil {
		log.Warnf("failed to persist auth runtime state cache for %s: %v", result.AuthID, err)
	}
}
