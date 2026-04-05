package management

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func (h *Handler) GetLogicalModelGroups(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusOK, gin.H{"logical-model-groups": config.LogicalModelGroups{
			Current: config.LogicalModelCurrent{Alias: config.LogicalModelGroupAliasCurrent},
		}})
		return
	}
	h.cfg.SanitizeLogicalModelGroups()
	c.JSON(http.StatusOK, gin.H{"logical-model-groups": h.cfg.LogicalModelGroups})
}

func (h *Handler) PutLogicalModelGroupCurrent(c *gin.Context) {
	var body struct {
		Ref *string `json:"ref"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Ref == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	ensureLogicalModelGroupsConfig(h)
	ref := strings.TrimSpace(*body.Ref)
	if ref == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ref is required"})
		return
	}
	if strings.EqualFold(ref, config.LogicalModelGroupAliasCurrent) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current cannot reference itself"})
		return
	}
	if !hasLogicalStaticGroup(h.cfg, ref) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "referenced static group not found"})
		return
	}
	h.cfg.LogicalModelGroups.Current.Alias = config.LogicalModelGroupAliasCurrent
	h.cfg.LogicalModelGroups.Current.Ref = ref
	h.cfg.SanitizeLogicalModelGroups()
	h.persistWithRuntimeRefresh(c)
}

func (h *Handler) PostLogicalModelGroupStatic(c *gin.Context) {
	var body config.LogicalModelGroup
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	ensureLogicalModelGroupsConfig(h)
	alias := strings.TrimSpace(body.Alias)
	if alias == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alias is required"})
		return
	}
	if strings.EqualFold(alias, config.LogicalModelGroupAliasCurrent) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current is reserved"})
		return
	}
	replaced := false
	for index := range h.cfg.LogicalModelGroups.Static {
		if strings.EqualFold(strings.TrimSpace(h.cfg.LogicalModelGroups.Static[index].Alias), alias) {
			h.cfg.LogicalModelGroups.Static[index] = body
			replaced = true
			break
		}
	}
	if !replaced {
		h.cfg.LogicalModelGroups.Static = append(h.cfg.LogicalModelGroups.Static, body)
	}
	h.cfg.SanitizeLogicalModelGroups()
	h.persistWithRuntimeRefresh(c)
}

func (h *Handler) DeleteLogicalModelGroupStatic(c *gin.Context) {
	alias := strings.TrimSpace(c.Param("alias"))
	if alias == "" {
		alias = strings.TrimSpace(c.Query("alias"))
	}
	if alias == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing alias"})
		return
	}
	if strings.EqualFold(alias, config.LogicalModelGroupAliasCurrent) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current cannot be deleted"})
		return
	}
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
		return
	}
	filtered := make([]config.LogicalModelGroup, 0, len(h.cfg.LogicalModelGroups.Static))
	removed := false
	for _, group := range h.cfg.LogicalModelGroups.Static {
		if strings.EqualFold(strings.TrimSpace(group.Alias), alias) {
			removed = true
			continue
		}
		filtered = append(filtered, group)
	}
	if !removed {
		c.JSON(http.StatusNotFound, gin.H{"error": "item not found"})
		return
	}
	if strings.EqualFold(strings.TrimSpace(h.cfg.LogicalModelGroups.Current.Ref), alias) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current references this static group"})
		return
	}
	h.cfg.LogicalModelGroups.Static = filtered
	h.cfg.SanitizeLogicalModelGroups()
	h.persistWithRuntimeRefresh(c)
}

func ensureLogicalModelGroupsConfig(h *Handler) {
	if h.cfg == nil {
		h.cfg = &config.Config{}
	}
	h.cfg.LogicalModelGroups.Current.Alias = config.LogicalModelGroupAliasCurrent
}

func hasLogicalStaticGroup(cfg *config.Config, alias string) bool {
	if cfg == nil {
		return false
	}
	for _, group := range cfg.LogicalModelGroups.Static {
		if strings.EqualFold(strings.TrimSpace(group.Alias), strings.TrimSpace(alias)) {
			return true
		}
	}
	return false
}
