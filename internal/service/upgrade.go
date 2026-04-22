// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// upgrade.go 实现镜像升级和服务重建编排。
// 通过 compose/executor 执行 CLI 命令，
// PullStatus 使用实例变量（非包级全局）。
package service

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/fengin/composeboard/internal/compose"
	"github.com/fengin/composeboard/internal/docker"
)

// PullStatus 镜像拉取状态
type PullStatus struct {
	Status  string    `json:"status"` // "pulling" | "success" | "failed" | "none"
	Message string    `json:"message"`
	Time    time.Time `json:"-"`
}

// UpgradeManager 升级管理器
type UpgradeManager struct {
	manager  *ServiceManager
	cache    *docker.ContainerCache
	executor *compose.Executor
	stateM   *StateManager

	pullStatusMu sync.Mutex
	pullStatuses map[string]*PullStatus
}

// NewUpgradeManager 创建升级管理器
func NewUpgradeManager(manager *ServiceManager, cache *docker.ContainerCache, executor *compose.Executor, stateM *StateManager) *UpgradeManager {
	return &UpgradeManager{
		manager:      manager,
		cache:        cache,
		executor:     executor,
		stateM:       stateM,
		pullStatuses: make(map[string]*PullStatus),
	}
}

// PullImage 异步拉取镜像
func (u *UpgradeManager) PullImage(serviceName string) *PullStatus {
	// 检查 ImageSource
	project := u.manager.GetProject()
	if project != nil {
		if decl, ok := project.Services[serviceName]; ok {
			if decl.ImageSource == "build" {
				return &PullStatus{Status: "failed", Message: "build 类型服务不支持拉取镜像"}
			}
		}
	}

	// 检查是否已在拉取
	u.pullStatusMu.Lock()
	if ps, ok := u.pullStatuses[serviceName]; ok && ps.Status == "pulling" {
		u.pullStatusMu.Unlock()
		return ps
	}
	ps := &PullStatus{Status: "pulling", Message: "正在拉取镜像...", Time: time.Now()}
	u.pullStatuses[serviceName] = ps
	u.pullStatusMu.Unlock()

	log.Printf("[PULL] %s: 开始拉取镜像", serviceName)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
		defer cancel()

		_, err := u.executor.Pull(ctx, []string{serviceName})

		u.pullStatusMu.Lock()
		defer u.pullStatusMu.Unlock()
		if err != nil {
			log.Printf("[PULL] 失败 %s: %v", serviceName, err)
			u.pullStatuses[serviceName] = &PullStatus{Status: "failed", Message: err.Error(), Time: time.Now()}
		} else {
			log.Printf("[PULL] 完成 %s", serviceName)
			u.pullStatuses[serviceName] = &PullStatus{Status: "success", Message: "镜像拉取完成", Time: time.Now()}
		}
	}()

	return ps
}

// GetPullStatus 查询镜像拉取状态
func (u *UpgradeManager) GetPullStatus(serviceName string) *PullStatus {
	u.pullStatusMu.Lock()
	defer u.pullStatusMu.Unlock()

	// 清理超过 2 小时的旧条目
	for k, v := range u.pullStatuses {
		if time.Since(v.Time) > 2*time.Hour {
			delete(u.pullStatuses, k)
		}
	}

	if ps, ok := u.pullStatuses[serviceName]; ok {
		return ps
	}
	return &PullStatus{Status: "none", Message: "未开始拉取"}
}

// ApplyUpgrade 应用升级（镜像已就绪后，重建单个服务容器）
func (u *UpgradeManager) ApplyUpgrade(serviceName string) error {
	// 检查 ImageSource
	project := u.manager.GetProject()
	if project != nil {
		if decl, ok := project.Services[serviceName]; ok {
			if decl.ImageSource == "build" {
				return &UpgradeError{Service: serviceName, Message: "build 类型服务不支持升级"}
			}
		}
	}

	log.Printf("[UPGRADE] %s: 应用升级", serviceName)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		err := u.executor.Up(ctx, []string{serviceName}, compose.UpOptions{
			NoDeps: true, // --no-deps：不拉起依赖
		})
		if err != nil {
			log.Printf("[UPGRADE] 失败 %s: %v", serviceName, err)
			return
		}
		log.Printf("[UPGRADE] 完成: %s", serviceName)
		u.cache.ForceRefresh()
		u.stateM.UpdateServiceState(serviceName)

		// 清除 pull 状态
		u.pullStatusMu.Lock()
		delete(u.pullStatuses, serviceName)
		u.pullStatusMu.Unlock()
	}()

	return nil
}

// RebuildService 重建容器（应用 .env 变更）
func (u *UpgradeManager) RebuildService(serviceName string) error {
	// M-9: build 型服务警告（不拦截，但记录日志提示风险）
	project := u.manager.GetProject()
	if project != nil {
		if decl, ok := project.Services[serviceName]; ok && decl.ImageSource == "build" {
			log.Printf("[REBUILD] 警告: %s 为 build 类型服务，若本地镜像不存在将触发重新构建", serviceName)
		}
	}

	log.Printf("[REBUILD] %s: 重建容器", serviceName)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		err := u.executor.Up(ctx, []string{serviceName}, compose.UpOptions{
			ForceRecreate: true, // --force-recreate
			NoDeps:        true, // --no-deps
		})
		if err != nil {
			log.Printf("[REBUILD] 失败 %s: %v", serviceName, err)
			return
		}
		log.Printf("[REBUILD] 完成: %s", serviceName)
		u.cache.ForceRefresh()
		u.stateM.UpdateServiceState(serviceName)
	}()

	return nil
}

// UpgradeError 升级错误
type UpgradeError struct {
	Service string
	Message string
}

func (e *UpgradeError) Error() string {
	return e.Message
}
