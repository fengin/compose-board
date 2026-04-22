// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// host.go 主机信息 API。
package api

import (
	"net/http"

	"github.com/fengin/composeboard/internal/host"
	"github.com/gin-gonic/gin"
)

// GetHostInfo GET /api/host
func (h *Handler) GetHostInfo(c *gin.Context) {
	info, err := host.GetHostInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}
