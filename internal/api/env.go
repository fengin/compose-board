// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// env.go 实现 .env 文件读写 API。
package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fengin/composeboard/internal/compose"
	"github.com/gin-gonic/gin"
)

// GetEnvFile GET /api/env
func (h *Handler) GetEnvFile(c *gin.Context) {
	envPath := filepath.Join(h.ProjectDir, ".env")

	entries, err := compose.ParseEnvFile(envPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 .env 失败: " + err.Error()})
		return
	}

	rawData, _ := os.ReadFile(envPath)

	c.JSON(http.StatusOK, gin.H{
		"entries":   entries,
		"raw_text":  string(rawData),
		"file_path": envPath,
	})
}

// SaveEnvFile POST /api/env
// 支持两种模式（二选一）：
//   - 原始模式：{"content": "RAW_TEXT"} — 前端原始编辑器
//   - 表格模式：{"entries": [EnvEntry...]} — 前端表格编辑器
func (h *Handler) SaveEnvFile(c *gin.Context) {
	var req struct {
		Content string              `json:"content"`
		Entries []compose.EnvEntry  `json:"entries"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	// 至少提供一种模式
	if req.Content == "" && len(req.Entries) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content 或 entries 至少提供一项"})
		return
	}

	envPath := filepath.Join(h.ProjectDir, ".env")

	// 读取旧内容用于备份
	oldData, err := os.ReadFile(envPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取原文件失败: " + err.Error()})
		return
	}

	// 备份
	backupPath := envPath + fmt.Sprintf(".bak.%s", time.Now().Format("20060102-150405"))
	if err := os.WriteFile(backupPath, oldData, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "备份失败: " + err.Error()})
		return
	}

	// 写入新内容：优先 entries 模式，其次 content 模式
	if len(req.Entries) > 0 {
		if err := compose.WriteEnvEntries(envPath, req.Entries); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败: " + err.Error()})
			return
		}
	} else {
		if err := compose.WriteEnvRaw(envPath, req.Content); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败: " + err.Error()})
			return
		}
	}

	// 热重载声明态
	h.Manager.ReloadCompose()

	c.JSON(http.StatusOK, gin.H{
		"message": "保存成功",
		"backup":  backupPath,
	})
}
