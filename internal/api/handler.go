// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// handler.go 定义 API Handler，持有全部 service 层管理器引用。
package api

import (
	"github.com/fengin/composeboard/internal/docker"
	"github.com/fengin/composeboard/internal/service"
	"github.com/fengin/composeboard/internal/terminal"
)

// Handler API 请求处理器
type Handler struct {
	ProjectName string // 项目名
	ProjectDir  string // 项目目录
	AppVersion  string // 应用版本号
	Manager     *service.ServiceManager
	Lifecycle   *service.LifecycleManager
	Upgrade     *service.UpgradeManager
	Profiles    *service.ProfileManager
	State       *service.StateManager
	Cache       *docker.ContainerCache
	DockerCli   *docker.Client
	Terminal    *terminal.SessionManager
}

// NewHandler 创建 API Handler
func NewHandler(
	projectName string,
	projectDir string,
	appVersion string,
	manager *service.ServiceManager,
	lifecycle *service.LifecycleManager,
	upgrade *service.UpgradeManager,
	profiles *service.ProfileManager,
	state *service.StateManager,
	cache *docker.ContainerCache,
	dockerCli *docker.Client,
	terminalM *terminal.SessionManager,
) *Handler {
	return &Handler{
		ProjectName: projectName,
		ProjectDir:  projectDir,
		AppVersion:  appVersion,
		Manager:     manager,
		Lifecycle:   lifecycle,
		Upgrade:     upgrade,
		Profiles:    profiles,
		State:       state,
		Cache:       cache,
		DockerCli:   dockerCli,
		Terminal:    terminalM,
	}
}
