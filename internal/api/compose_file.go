// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// compose_file.go 实现 docker-compose.yml 文件读写 API。
package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fengin/composeboard/internal/compose"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// GetComposeFile GET /api/compose-file
func (h *Handler) GetComposeFile(c *gin.Context) {
	filePath, err := compose.FindComposeFile(h.ProjectDir)
	if err != nil {
		// 文件不存在时返回空内容 + 默认路径（支持从零创建）
		defaultPath := filepath.Join(h.ProjectDir, "docker-compose.yml")
		c.JSON(http.StatusOK, gin.H{
			"content":   "",
			"file_path": defaultPath,
		})
		return
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 Compose 文件失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"content":   string(data),
		"file_path": filePath,
	})
}

// DownloadComposeFile GET /api/compose-file/download
func (h *Handler) DownloadComposeFile(c *gin.Context) {
	filePath, err := compose.FindComposeFile(h.ProjectDir)
	if err != nil {
		c.String(http.StatusNotFound, "找不到 Compose 文件")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(filePath)))
	c.Header("Content-Type", "application/octet-stream")
	c.File(filePath)
}

// SaveComposeFile PUT /api/compose-file
// 保存前做 YAML 语法校验 + services 结构校验，自动备份原文件，保存后热重载。
func (h *Handler) SaveComposeFile(c *gin.Context) {
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content 不能为空"})
		return
	}

	// 1. YAML 语法校验
	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(req.Content), &parsed); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "YAML 语法错误: " + err.Error()})
		return
	}

	// 2. 结构校验：必须包含 services 顶级 key
	if _, ok := parsed["services"]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Compose 文件必须包含 services 定义"})
		return
	}

	// 3. 确定 Compose 文件路径
	filePath, err := compose.FindComposeFile(h.ProjectDir)
	if err != nil {
		// 文件不存在，使用默认路径创建
		filePath = filepath.Join(h.ProjectDir, "docker-compose.yml")
	}

	// 4. 记录旧的服务列表（用于差异摘要）
	oldProject := h.Manager.GetProject()
	oldServiceNames := make(map[string]struct{})
	if oldProject != nil {
		for name := range oldProject.Services {
			oldServiceNames[name] = struct{}{}
		}
	}

	// 5. 读取原文件内容备份（文件不存在时跳过备份）
	backupPath := ""
	oldData, err := os.ReadFile(filePath)
	if err == nil && len(oldData) > 0 {
		backupPath = filePath + fmt.Sprintf(".bak.%s", time.Now().Format("20060102-150405"))
		if err := os.WriteFile(backupPath, oldData, 0644); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "备份失败: " + err.Error()})
			return
		}
	}

	// 6. 写入新内容
	if err := os.WriteFile(filePath, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败: " + err.Error()})
		return
	}

	// 7. 热重载声明态
	h.Manager.ReloadCompose()

	// 8. 生成差异摘要
	newProject := h.Manager.GetProject()
	var added, removed []string
	if newProject != nil {
		newServiceNames := make(map[string]struct{})
		for name := range newProject.Services {
			newServiceNames[name] = struct{}{}
		}
		for name := range newServiceNames {
			if _, ok := oldServiceNames[name]; !ok {
				added = append(added, name)
			}
		}
		for name := range oldServiceNames {
			if _, ok := newServiceNames[name]; !ok {
				removed = append(removed, name)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "保存成功",
		"backup":           backupPath,
		"services_added":   added,
		"services_removed": removed,
	})
}
