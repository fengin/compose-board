// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// profiles.go 管理 Compose Profiles 的三态（enabled/partial/disabled）。
// Profile 信息来自声明态（parser），运行态来自 docker cache。
package service

import (
	"context"
	"log"
	"time"

	"github.com/fengin/composeboard/internal/compose"
	"github.com/fengin/composeboard/internal/docker"
)

// ProfileInfo Profile 信息
type ProfileInfo struct {
	Name     string        `json:"name"`
	Services []ServiceView `json:"services"` // 该 profile 下的服务视图
	Status   string        `json:"status"`   // "enabled" | "partial" | "disabled"
}

// ProfileManager Profile 管理器
type ProfileManager struct {
	manager  *ServiceManager
	cache    *docker.ContainerCache
	executor *compose.Executor
}

// NewProfileManager 创建 Profile 管理器
func NewProfileManager(manager *ServiceManager, cache *docker.ContainerCache, executor *compose.Executor) *ProfileManager {
	return &ProfileManager{
		manager:  manager,
		cache:    cache,
		executor: executor,
	}
}

// ListProfiles 返回所有 Profiles 及其状态
func (p *ProfileManager) ListProfiles() map[string]*ProfileInfo {
	project := p.manager.GetProject()
	if project == nil {
		return nil
	}

	// 获取 profile → 服务名 映射
	profileMap := project.GetProfiles()
	if len(profileMap) == 0 {
		return nil
	}

	// 获取当前服务视图（含运行态）
	allServices := p.manager.ListServices()
	serviceViewMap := make(map[string]*ServiceView)
	for i := range allServices {
		serviceViewMap[allServices[i].Name] = &allServices[i]
	}

	result := make(map[string]*ProfileInfo)

	for profileName, serviceNames := range profileMap {
		info := &ProfileInfo{
			Name: profileName,
		}

		runningCount := 0
		deployedCount := 0

		for _, svcName := range serviceNames {
			if sv, ok := serviceViewMap[svcName]; ok {
				info.Services = append(info.Services, *sv)
				if sv.Status == "running" {
					runningCount++
					deployedCount++
				} else if sv.Status != "not_deployed" {
					deployedCount++
				}
			}
		}

		// 三态判定
		total := len(serviceNames)
		if runningCount == total {
			info.Status = "enabled"
		} else if deployedCount > 0 {
			info.Status = "partial"
		} else {
			info.Status = "disabled"
		}

		result[profileName] = info
	}

	return result
}

// EnableProfile 启用 Profile（启动全部服务）
func (p *ProfileManager) EnableProfile(name string) error {
	log.Printf("[PROFILE] 启用: %s", name)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := p.executor.Up(ctx, nil, compose.UpOptions{
		Profiles: []string{name},
	})
	if err != nil {
		log.Printf("[PROFILE] 启用失败 %s: %v", name, err)
		return err
	}

	p.cache.RefreshNow(false)
	log.Printf("[PROFILE] 启用完成: %s", name)
	return nil
}

// DisableProfile 停用 Profile（停止并移除服务容器）
func (p *ProfileManager) DisableProfile(name string) error {
	project := p.manager.GetProject()
	if project == nil {
		return nil
	}

	profileMap := project.GetProfiles()
	services, ok := profileMap[name]
	if !ok || len(services) == 0 {
		return nil
	}

	log.Printf("[PROFILE] 停用: %s (服务: %v)", name, services)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// M-13: 携带 --profile 参数，确保不同 Compose 版本兼容
	profiles := []string{name}

	// 先停止
	if err := p.executor.Stop(ctx, services, profiles); err != nil {
		log.Printf("[PROFILE] 停止失败 %s: %v", name, err)
		return err
	}

	// 再移除
	if err := p.executor.Rm(ctx, services, true, profiles); err != nil {
		log.Printf("[PROFILE] 移除失败 %s: %v", name, err)
		return err
	}

	p.cache.RefreshNow(false)
	log.Printf("[PROFILE] 停用完成: %s", name)
	return nil
}
