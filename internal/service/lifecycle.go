// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// lifecycle.go 封装服务启停重启的业务逻辑。
// API Handler 只做 HTTP 适配，不再直接调用 Docker API。
package service

import (
	"context"
	"fmt"

	"github.com/fengin/composeboard/internal/compose"
	"github.com/fengin/composeboard/internal/docker"
)

// LifecycleManager 服务生命周期管理器
type LifecycleManager struct {
	manager   *ServiceManager
	cache     *docker.ContainerCache
	dockerCli *docker.Client
	executor  *compose.Executor
}

// NewLifecycleManager 创建生命周期管理器
func NewLifecycleManager(manager *ServiceManager, cache *docker.ContainerCache, dockerCli *docker.Client, executor *compose.Executor) *LifecycleManager {
	return &LifecycleManager{
		manager:   manager,
		cache:     cache,
		dockerCli: dockerCli,
		executor:  executor,
	}
}

// StartService 启动服务
// 规则：
//   - build 型未部署 → 返回 ErrBuildNotSupported
//   - 有 profiles 且未部署 → 返回 ErrProfileRequired
//   - 已部署且已停止 → Docker Start
//   - 未部署且无 profile 的 registry 型 → compose up
//   - 已在运行 → 返回 nil（幂等）
func (l *LifecycleManager) StartService(ctx context.Context, name string) error {
	// 检查声明态
	project := l.manager.GetProject()
	if project != nil {
		if decl, ok := project.Services[name]; ok {
			// S-4: build 型未部署不支持启动
			status, _, _ := l.dockerCli.FindContainerByServiceName(ctx, name)
			notDeployed := status == nil

			if notDeployed && decl.ImageSource == "build" {
				return &ServiceError{
					Code:    "services.start.build_not_supported",
					Message: fmt.Sprintf("build 类型服务 %s 不支持通过面板启动", name),
				}
			}

			// S-4: 有 profiles 且未部署 → 应通过 Profile API 启用
			if notDeployed && len(decl.Profiles) > 0 {
				return &ServiceError{
					Code:    "services.start.profile_required",
					Message: fmt.Sprintf("可选服务 %s 属于 profile %v，请通过 Profile 管理启用", name, decl.Profiles),
				}
			}
		}
	}

	// 查找容器
	status, containerID, err := l.dockerCli.FindContainerByServiceName(ctx, name)
	if err != nil {
		// 未部署 → compose up
		if err := l.executor.Up(ctx, []string{name}, compose.UpOptions{}); err != nil {
			return fmt.Errorf("启动服务失败: %w", err)
		}
		l.cache.ForceRefresh()
		return nil
	}

	if status.Status == "running" {
		return nil // 幂等
	}

	// 已部署但已停止 → Docker Start
	if err := l.dockerCli.StartContainer(ctx, containerID); err != nil {
		return fmt.Errorf("启动容器失败: %w", err)
	}
	l.cache.UpdateContainerStatus(containerID, name, "running", "Up", "")
	return nil
}

// StopService 停止服务
func (l *LifecycleManager) StopService(ctx context.Context, name string) error {
	_, containerID, err := l.dockerCli.FindContainerByServiceName(ctx, name)
	if err != nil {
		return &ServiceError{Code: "services.not_deployed", Message: "服务未部署"}
	}

	if err := l.dockerCli.StopContainer(ctx, containerID); err != nil {
		return fmt.Errorf("停止容器失败: %w", err)
	}
	l.cache.UpdateContainerStatus(containerID, name, "exited", "Exited", "")
	return nil
}

// RestartService 重启服务
func (l *LifecycleManager) RestartService(ctx context.Context, name string) error {
	_, containerID, err := l.dockerCli.FindContainerByServiceName(ctx, name)
	if err != nil {
		return &ServiceError{Code: "services.not_deployed", Message: "服务未部署"}
	}

	if err := l.dockerCli.RestartContainer(ctx, containerID); err != nil {
		return fmt.Errorf("重启容器失败: %w", err)
	}
	l.cache.UpdateContainerStatus(containerID, name, "running", "Up", "")
	return nil
}

// GetServiceEnv 获取容器运行时环境变量
func (l *LifecycleManager) GetServiceEnv(ctx context.Context, name string) (map[string]string, error) {
	_, containerID, err := l.dockerCli.FindContainerByServiceName(ctx, name)
	if err != nil {
		return nil, &ServiceError{Code: "services.not_deployed", Message: "服务未部署"}
	}
	return l.dockerCli.GetContainerEnv(ctx, containerID)
}

// ServiceError 业务错误（携带错误码供 API 层判断 HTTP 状态码）
type ServiceError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ServiceError) Error() string {
	return e.Message
}
