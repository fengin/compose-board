// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// profiles.go 实现 Profiles 管理 API。
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListProfiles GET /api/profiles
func (h *Handler) ListProfiles(c *gin.Context) {
	profiles := h.Profiles.ListProfiles()
	c.JSON(http.StatusOK, profiles)
}

// EnableProfile POST /api/profiles/:name/enable
func (h *Handler) EnableProfile(c *gin.Context) {
	name := c.Param("name")
	if err := h.Profiles.EnableProfile(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Profile 已启用: " + name})
}

// DisableProfile POST /api/profiles/:name/disable
func (h *Handler) DisableProfile(c *gin.Context) {
	name := c.Param("name")
	if err := h.Profiles.DisableProfile(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Profile 已停用: " + name})
}
