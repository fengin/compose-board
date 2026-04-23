// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// services.go 实现服务列表 + 容器启停重启 API。
// 业务逻辑在 service/lifecycle.go，此处仅做 HTTP 适配。
package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/fengin/composeboard/internal/service"
	"github.com/gin-gonic/gin"
)

// ListServices GET /api/services
func (h *Handler) ListServices(c *gin.Context) {
	views := h.Manager.ListServices()
	c.JSON(http.StatusOK, views)
}

// GetServiceStatus GET /api/services/:name/status
// 直查单服务实时状态，并同步回写缓存。
func (h *Handler) GetServiceStatus(c *gin.Context) {
	name := c.Param("name")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	view, err := h.Manager.GetRealtimeServiceStatus(ctx, name)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// StartService POST /api/services/:name/start
func (h *Handler) StartService(c *gin.Context) {
	name := c.Param("name")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	if err := h.Lifecycle.StartService(ctx, name); err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "启动成功"})
}

// StopService POST /api/services/:name/stop
func (h *Handler) StopService(c *gin.Context) {
	name := c.Param("name")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	if err := h.Lifecycle.StopService(ctx, name); err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "停止成功"})
}

// RestartService POST /api/services/:name/restart
func (h *Handler) RestartService(c *gin.Context) {
	name := c.Param("name")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	if err := h.Lifecycle.RestartService(ctx, name); err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "重启成功"})
}

// GetContainerEnv GET /api/services/:name/env
func (h *Handler) GetContainerEnv(c *gin.Context) {
	name := c.Param("name")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	envMap, err := h.Lifecycle.GetServiceEnv(ctx, name)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, envMap)
}

// handleServiceError 根据 ServiceError 错误码返回合适的 HTTP 状态码
func handleServiceError(c *gin.Context, err error) {
	var svcErr *service.ServiceError
	if errors.As(err, &svcErr) {
		switch svcErr.Code {
		case "services.start.build_not_supported":
			c.JSON(http.StatusConflict, gin.H{"code": svcErr.Code, "error": svcErr.Message})
		case "services.start.profile_required":
			c.JSON(http.StatusConflict, gin.H{"code": svcErr.Code, "error": svcErr.Message})
		case "services.not_deployed":
			c.JSON(http.StatusNotFound, gin.H{"code": svcErr.Code, "error": svcErr.Message})
		case "services.not_found":
			c.JSON(http.StatusNotFound, gin.H{"code": svcErr.Code, "error": svcErr.Message})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": svcErr.Code, "error": svcErr.Message})
		}
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
