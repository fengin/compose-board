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

	// 状态覆写：解决 refresh 整体替换 slice 导致增量更新丢失的竞态
	overrideMu sync.Mutex
	overrides  map[string]statusOverride

	lastAccess atomic.Int64 // 最后一次 API 访问时间戳
	active     atomic.Bool  // 后台刷新是否活跃
	wakeCh     chan struct{}
}

// statusOverride 单容器状态覆写记录
type statusOverride struct {
	status      string
	state       string
	image       string // 升级场景：新镜像
	serviceName string // 升级/重建场景：用于 ID 变更时的回退匹配
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

// UpdateContainerStatus 更新缓存中单个容器的状态（轮询时同步写回）
// serviceName 非空时，若按 ID 找不到则回退为按 ServiceName 匹配（升级/重建后 ID 变化场景）
// image 非空时同步更新缓存中的 Image 字段
func (cc *ContainerCache) UpdateContainerStatus(containerID, serviceName, status, state, image string) {
	// 1. 立即更新当前 slice（供即时读取）
	cc.mu.Lock()
	found := false
	var oldID string
	for i := range cc.containers {
		if cc.containers[i].ID == containerID {
			found = true
			cc.containers[i].Status = status
			cc.containers[i].State = state
			if image != "" {
				cc.containers[i].Image = image
			}
			if status != "running" {
				cc.containers[i].CPU = 0
				cc.containers[i].MemUsage = 0
				cc.containers[i].MemPercent = 0
			}
			break
		}
	}
	// ID 未匹配到（升级/重建后容器 ID 已变），回退按 ServiceName 查找
	if !found && serviceName != "" {
		for i := range cc.containers {
			if cc.containers[i].ServiceName == serviceName {
				oldID = cc.containers[i].ID // 记录旧 ID 用于清理 overrides
				log.Printf("[CACHE] 容器 ID 变更: %s → %s (服务: %s)", oldID, containerID, serviceName)
				cc.containers[i].ID = containerID // 同步新 ID
				cc.containers[i].Status = status
				cc.containers[i].State = state
				if image != "" {
					cc.containers[i].Image = image
				}
				if status != "running" {
					cc.containers[i].CPU = 0
					cc.containers[i].MemUsage = 0
					cc.containers[i].MemPercent = 0
				}
				break
			}
		}
	}
	cc.mu.Unlock()

	// 2. 写入 override（防止 refresh 整体替换时丢失）
	cc.overrideMu.Lock()
	cc.overrides[containerID] = statusOverride{
		status:      status,
		state:       state,
		image:       image,
		serviceName: serviceName,
		at:          time.Now(),
	}
	// 清理旧 ID 的 override 条目（容器已不存在）
	if oldID != "" {
		delete(cc.overrides, oldID)
	}
	cc.overrideMu.Unlock()
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

	// 合并 overrides：将 refresh 期间产生的状态覆写应用到新 slice
	cc.overrideMu.Lock()
	for id, ov := range cc.overrides {
		if ov.at.After(refreshStart) {
			// 此 override 比本次 refresh 的 Docker 查询更新，以 override 为准
			matched := false
			// 先按 ID 匹配
			for i := range containers {
				if containers[i].ID == id {
					cc.applyOverride(&containers[i], ov)
					matched = true
					break
				}
			}
			// ID 未匹配（升级/重建后容器 ID 已变），回退按 ServiceName
			if !matched && ov.serviceName != "" {
				for i := range containers {
					if containers[i].ServiceName == ov.serviceName {
						containers[i].ID = id // 同步新 ID
						cc.applyOverride(&containers[i], ov)
						break
					}
				}
			}
		} else {
			// 此 override 比本次 refresh 早，Docker 查询的数据更新，清除
			delete(cc.overrides, id)
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

// applyOverride 将覆写数据应用到容器条目
func (cc *ContainerCache) applyOverride(ctr *ContainerInfo, ov statusOverride) {
	ctr.Status = ov.status
	ctr.State = ov.state
	if ov.image != "" {
		ctr.Image = ov.image
	}
	if ov.status != "running" {
		ctr.CPU = 0
		ctr.MemUsage = 0
		ctr.MemPercent = 0
	}
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
