// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// upgrade.go 实现镜像拉取/升级/重建 API。
// 业务逻辑在 service 层，此处仅做 HTTP 适配。
package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// PullImage POST /api/services/:name/pull
func (h *Handler) PullImage(c *gin.Context) {
	name := c.Param("name")
	ps := h.Upgrade.PullImage(name)
	if ps.Status == "failed" {
		c.JSON(http.StatusBadRequest, gin.H{"status": ps.Status, "message": ps.Message})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": ps.Status, "message": ps.Message})
}

// GetPullStatus GET /api/services/:name/pull-status
func (h *Handler) GetPullStatus(c *gin.Context) {
	name := c.Param("name")
	ps := h.Upgrade.GetPullStatus(name)
	c.JSON(http.StatusOK, gin.H{"status": ps.Status, "message": ps.Message})
}

// ApplyUpgrade POST /api/services/:name/upgrade
func (h *Handler) ApplyUpgrade(c *gin.Context) {
	name := c.Param("name")
	if err := h.Upgrade.ApplyUpgrade(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": fmt.Sprintf("升级已启动: %s", name)})
}

// RebuildService POST /api/services/:name/rebuild
func (h *Handler) RebuildService(c *gin.Context) {
	name := c.Param("name")
	if err := h.Upgrade.RebuildService(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("重建已启动: %s", name)})
}
