// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// terminal.go 实现 Web 终端 WebSocket 接入。
package api

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/fengin/composeboard/internal/terminal"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var terminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		return err == nil && u.Host == r.Host
	},
}

// OpenServiceTerminal GET /api/services/:name/terminal
func (h *Handler) OpenServiceTerminal(c *gin.Context) {
	if h.Terminal == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":  "terminal.unavailable",
			"error": "Web 终端未初始化",
		})
		return
	}

	name := c.Param("name")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	status, containerID, err := h.DockerCli.FindContainerByServiceName(ctx, name)
	cancel()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":  "terminal.service_not_found",
			"error": "服务未部署，无法连接终端",
		})
		return
	}
	if status.Status != "running" {
		c.JSON(http.StatusConflict, gin.H{
			"code":  "terminal.service_not_running",
			"error": "仅运行中的服务可连接终端",
		})
		return
	}

	conn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[TERMINAL] WebSocket 升级失败: service=%s err=%v", name, err)
		return
	}

	h.Terminal.Serve(c.Request.Context(), conn, terminal.StartOptions{
		ServiceName: name,
		ContainerID: containerID,
	})
}
