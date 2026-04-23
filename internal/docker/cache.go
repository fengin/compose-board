// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// cache.go 实现容器数据的按需缓存机制。
// 有前端请求时启动后台刷新，空闲 60 秒自动暂停。
package docker

import (
	"context"
	"log"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// ContainerCache 容器数据按需缓存
// 有前端请求时才启动后台刷新，空闲 60 秒自动暂停
type ContainerCache struct {
	client *Client

	mu         sync.RWMutex
	containers []ContainerInfo
	lastUpdate time.Time
	updating   bool

	// 按服务名保存最新覆写：解决 refresh 整体替换 slice 导致增量更新丢失的竞态
	overrideMu sync.Mutex
	overrides  map[string]statusOverride

	lastAccess atomic.Int64 // 最后一次 API 访问时间戳
	active     atomic.Bool  // 后台刷新是否活跃
	wakeCh     chan struct{}
}

// statusOverride 单服务状态覆写记录
type statusOverride struct {
	serviceName string
	info        ContainerInfo
	removed     bool
	at          time.Time
}

const (
	refreshInterval = 15 * time.Second // 刷新间隔
	idleTimeout     = 60 * time.Second // 无访问多久后暂停
)

// NewContainerCache 创建缓存
func NewContainerCache(client *Client) *ContainerCache {
	cache := &ContainerCache{
		client:    client,
		wakeCh:    make(chan struct{}, 1),
		overrides: make(map[string]statusOverride),
	}
	go cache.backgroundLoop()
	return cache
}

// Get 获取缓存的容器列表
func (cc *ContainerCache) Get() []ContainerInfo {
	// 记录访问时间
	cc.lastAccess.Store(time.Now().Unix())

	// 如果后台未活跃，唤醒它
	if !cc.active.Load() {
		select {
		case cc.wakeCh <- struct{}{}:
		default:
		}
	}

	cc.mu.RLock()
	empty := len(cc.containers) == 0
	cc.mu.RUnlock()

	// 缓存为空：同步快速加载容器列表（不含 stats，~0.5s）
	if empty {
		cc.refresh(false)
	}

	cc.mu.RLock()
	defer cc.mu.RUnlock()
	result := make([]ContainerInfo, len(cc.containers))
	copy(result, cc.containers)
	return result
}

// LastUpdate 上次更新时间
func (cc *ContainerCache) LastUpdate() time.Time {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.lastUpdate
}

// ForceRefresh 异步强制刷新（操作后调用，含资源数据）
func (cc *ContainerCache) ForceRefresh() {
	go cc.refresh(true)
}

// RefreshNow 同步立即刷新（操作完成后调用，默认可关闭 stats 以更快拿到最新状态）
func (cc *ContainerCache) RefreshNow(withStats bool) {
	cc.lastAccess.Store(time.Now().Unix())
	cc.refresh(withStats)
}

// SyncService 用单服务实时状态同步缓存。
// 该方法既更新当前内存 slice，也登记 override，确保正在进行的 refresh 不会把新状态回刷掉。
func (cc *ContainerCache) SyncService(info ContainerInfo) {
	if info.ServiceName == "" {
		return
	}
	if info.Status != "running" {
		info.CPU = 0
		info.MemUsage = 0
		info.MemLimit = 0
		info.MemPercent = 0
	}

	cc.mu.Lock()
	replaced := false
	for i := range cc.containers {
		if cc.containers[i].ServiceName == info.ServiceName {
			cc.containers[i] = info
			replaced = true
			break
		}
	}
	if !replaced {
		cc.containers = append(cc.containers, info)
	}
	cc.lastUpdate = time.Now()
	cc.mu.Unlock()

	cc.overrideMu.Lock()
	cc.overrides[info.ServiceName] = statusOverride{
		serviceName: info.ServiceName,
		info:        info,
		removed:     false,
		at:          time.Now(),
	}
	cc.overrideMu.Unlock()
}

// RemoveService 将服务从缓存中移除，并登记 override，防止旧 refresh 把已删除的容器重新写回缓存。
func (cc *ContainerCache) RemoveService(serviceName string) {
	if serviceName == "" {
		return
	}

	cc.mu.Lock()
	filtered := cc.containers[:0]
	for _, ctr := range cc.containers {
		if ctr.ServiceName != serviceName {
			filtered = append(filtered, ctr)
		}
	}
	cc.containers = filtered
	cc.lastUpdate = time.Now()
	cc.mu.Unlock()

	cc.overrideMu.Lock()
	cc.overrides[serviceName] = statusOverride{
		serviceName: serviceName,
		removed:     true,
		at:          time.Now(),
	}
	cc.overrideMu.Unlock()
}

// UpdateContainerStatus 保留给轻量生命周期操作使用。
// 新逻辑统一转换成按服务维度同步缓存，兼容旧调用点。
func (cc *ContainerCache) UpdateContainerStatus(containerID, serviceName, status, state, image string) {
	if serviceName == "" {
		return
	}

	cc.mu.RLock()
	var base *ContainerInfo
	for i := range cc.containers {
		if cc.containers[i].ServiceName == serviceName {
			clone := cc.containers[i]
			base = &clone
			break
		}
	}
	cc.mu.RUnlock()

	if status == "not_deployed" {
		cc.RemoveService(serviceName)
		return
	}

	info := ContainerInfo{
		ID:          containerID,
		ServiceName: serviceName,
		Status:      status,
		State:       state,
		Image:       image,
	}
	if base != nil {
		info = *base
		if containerID != "" {
			info.ID = containerID
		}
		if state != "" {
			info.State = state
		}
		if status != "" {
			info.Status = status
		}
		if image != "" {
			info.Image = image
		}
	}

	cc.SyncService(info)
}

// backgroundLoop 后台刷新主循环
func (cc *ContainerCache) backgroundLoop() {
	for {
		// 等待唤醒信号（有人访问了）
		<-cc.wakeCh
		cc.active.Store(true)
		log.Println("[CACHE] 检测到访问，启动后台刷新")

		// 首次快速加载（不含 stats）
		cc.refresh(false)

		// 延迟 3 秒后开始含 stats 的全量刷新
		time.Sleep(3 * time.Second)
		cc.refresh(true)

		// 定时刷新循环
		ticker := time.NewTicker(refreshInterval)
		for {
			select {
			case <-ticker.C:
				// 检查是否空闲
				lastAccess := time.Unix(cc.lastAccess.Load(), 0)
				if time.Since(lastAccess) > idleTimeout {
					ticker.Stop()
					cc.active.Store(false)
					// 清空缓存：避免唤醒后返回过期数据 + 释放内存
					cc.mu.Lock()
					cc.containers = nil
					cc.mu.Unlock()
					debug.FreeOSMemory() // 强制归还内存给 OS
					log.Println("[CACHE] 无访问超过 60s，暂停后台刷新，已清空缓存")
					break
				}
				cc.refresh(true)
			}
			if !cc.active.Load() {
				break
			}
		}
	}
}

// refresh 刷新容器数据
func (cc *ContainerCache) refresh(withStats bool) {
	cc.mu.Lock()
	if cc.updating {
		cc.mu.Unlock()
		return
	}
	cc.updating = true
	cc.mu.Unlock()

	refreshStart := time.Now() // 记录 refresh 开始时间

	defer func() {
		cc.mu.Lock()
		cc.updating = false
		cc.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	containers, err := cc.client.ListContainers(ctx)
	if err != nil {
		log.Printf("[CACHE] 刷新容器列表失败: %v", err)
		return
	}

	if withStats && len(containers) > 0 {
		cc.fetchStatsParallel(ctx, containers)
	}

	// 合并 overrides：将 refresh 期间产生的服务级覆写应用到新 slice
	cc.overrideMu.Lock()
	for serviceName, ov := range cc.overrides {
		if ov.at.After(refreshStart) {
			containers = cc.applyOverride(containers, ov)
		} else {
			// 此 override 比本次 refresh 早，Docker 查询的数据更新，清除
			delete(cc.overrides, serviceName)
		}
	}
	cc.overrideMu.Unlock()

	cc.mu.Lock()
	cc.containers = containers
	cc.lastUpdate = time.Now()
	cc.mu.Unlock()

	if withStats {
		log.Printf("[CACHE] 刷新完成: %d 个容器 (含资源数据)", len(containers))
	}
}

// applyOverride 将服务级覆写应用到容器列表。
func (cc *ContainerCache) applyOverride(containers []ContainerInfo, ov statusOverride) []ContainerInfo {
	filtered := containers[:0]
	for _, ctr := range containers {
		if ctr.ServiceName != ov.serviceName {
			filtered = append(filtered, ctr)
		}
	}
	if ov.removed {
		return filtered
	}
	return append(filtered, ov.info)
}

// fetchStatsParallel 并发采集所有容器的资源使用数据
func (cc *ContainerCache) fetchStatsParallel(ctx context.Context, containers []ContainerInfo) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // 限制 5 并发

	for i := range containers {
		if containers[i].Status != "running" {
			continue
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			statsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			stats, err := cc.client.GetContainerStats(statsCtx, containers[idx].ID)
			if err != nil {
				return
			}
			containers[idx].CPU = stats.CPU
			containers[idx].MemUsage = stats.MemUsage
			containers[idx].MemLimit = stats.MemLimit
			containers[idx].MemPercent = stats.MemPercent
		}(i)
	}
	wg.Wait()
}
