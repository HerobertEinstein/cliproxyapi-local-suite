package management

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/modeldiscovery"
)

func (h *Handler) GetOpenAICompatDiscovery(c *gin.Context) {
	statuses, err := modeldiscovery.BuildOpenAICompatStatuses(h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"providers": statuses})
}

func (h *Handler) PostOpenAICompatDiscoveryRescan(c *gin.Context) {
	var body struct {
		Name  string   `json:"name"`
		Names []string `json:"names"`
	}
	_ = c.ShouldBindJSON(&body)

	names := make([]string, 0, 1+len(body.Names))
	if trimmed := strings.TrimSpace(body.Name); trimmed != "" {
		names = append(names, trimmed)
	}
	names = append(names, body.Names...)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	statuses, err := modeldiscovery.RescanOpenAICompatProviders(ctx, h.cfg, names)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.runtimeRefreshHook != nil {
		h.runtimeRefreshHook(ctx)
	}
	c.JSON(http.StatusOK, gin.H{"providers": statuses})
}
