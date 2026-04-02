package management

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/modeldiscovery"
)

func (h *Handler) GetProviderModelDiscovery(c *gin.Context) {
	statuses, err := modeldiscovery.BuildConfigProviderStatuses(h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"providers": statuses})
}

func (h *Handler) PostProviderModelDiscoveryRescan(c *gin.Context) {
	var body struct {
		Key  string   `json:"key"`
		Keys []string `json:"keys"`
	}
	_ = c.ShouldBindJSON(&body)

	keys := make([]string, 0, 1+len(body.Keys))
	if trimmed := strings.TrimSpace(body.Key); trimmed != "" {
		keys = append(keys, trimmed)
	}
	keys = append(keys, body.Keys...)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	statuses, err := modeldiscovery.RescanConfigProviders(ctx, h.cfg, keys)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.runtimeRefreshHook != nil {
		h.runtimeRefreshHook(ctx)
	}
	c.JSON(http.StatusOK, gin.H{"providers": statuses})
}
