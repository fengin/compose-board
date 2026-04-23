// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// profiles.go 管理 Compose Profiles 的配置启用态。
// 注意：Profile 状态只表达“配置是否启用”，不再表达下属服务是否全部运行。
package service

import (
	"context"
	"log"
	"sort"
	"time"

	"github.com/fengin/composeboard/internal/compose"
	"github.com/fengin/composeboard/internal/docker"
)

// ProfileInfo Profile 信息
type ProfileInfo struct {
	Name    string `json:"name"`
	Status  string `json:"status"`  // "enabled" | "disabled"
	Enabled bool   `json:"enabled"` // 便于前端直接判断
}

// ProfileManager Profile 管理器
type ProfileManager struct {
	manager  *ServiceManager
	cache    *docker.ContainerCache
	executor *compose.Executor
	stateM   *StateManager
}

// NewProfileManager 创建 Profile 管理器
func NewProfileManager(manager *ServiceManager, cache *docker.ContainerCache, executor *compose.Executor, stateM *StateManager) *ProfileManager {
	return &ProfileManager{
		manager:  manager,
		cache:    cache,
		executor: executor,
		stateM:   stateM,
	}
}

// ListProfiles 返回所有 Profiles 的配置启用态。
func (p *ProfileManager) ListProfiles() []ProfileInfo {
	project := p.manager.GetProject()
	if project == nil {
		return nil
	}

	profileMap := project.GetProfiles()
	if len(profileMap) == 0 {
		return nil
	}

	names := make([]string, 0, len(profileMap))
	for profileName := range profileMap {
		names = append(names, profileName)
	}
	sort.Strings(names)

	result := make([]ProfileInfo, 0, len(names))
	for _, profileName := range names {
		enabled := false
		if p.stateM != nil {
			enabled = p.stateM.IsProfileEnabled(profileName)
		}
		info := ProfileInfo{
			Name: profileName,
			Status: func() string {
				if enabled {
					return "enabled"
				}
				return "disabled"
			}(),
			Enabled: enabled,
		}
		result = append(result, info)
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
	if p.stateM != nil {
		p.stateM.SetProfileEnabled(name, true)
	}
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
	if p.stateM != nil {
		p.stateM.SetProfileEnabled(name, false)
	}
	log.Printf("[PROFILE] 停用完成: %s", name)
	return nil
}
