// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// settings.go 实现项目设置信息 API。
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ProjectSettingsResponse 项目设置信息
type ProjectSettingsResponse struct {
	ProjectName    string   `json:"project_name"`
	ProjectDir     string   `json:"project_dir"`
	ComposeFile    string   `json:"compose_file"`
	ComposeCommand string   `json:"compose_command"`
	ComposeVersion string   `json:"compose_version"`
	ServiceCount   int      `json:"service_count"`
	ProfileNames   []string `json:"profile_names"`
	AppVersion     string   `json:"app_version"`
}

// GetProjectSettings GET /api/settings/project
func (h *Handler) GetProjectSettings(c *gin.Context) {
	resp := ProjectSettingsResponse{
		ProjectName: h.ProjectName,
		ProjectDir:  h.ProjectDir,
		AppVersion:  h.AppVersion,
	}

	project := h.Manager.GetProject()
	if project != nil {
		resp.ComposeFile = project.FilePath
		resp.ServiceCount = len(project.Services)

		// 收集 profile 名称
		profileMap := project.GetProfiles()
		for name := range profileMap {
			resp.ProfileNames = append(resp.ProfileNames, name)
		}
	}

	// Compose 命令信息
	cmd, ver := h.Manager.GetComposeInfo()
	resp.ComposeCommand = cmd
	resp.ComposeVersion = ver

	c.JSON(http.StatusOK, resp)
}
