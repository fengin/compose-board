// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// manager.go 实现服务视图层，融合声明态（compose/parser）
// 和运行态（docker/cache），为 API 层提供统一的 ServiceView。
package service

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fengin/composeboard/internal/compose"
	"github.com/fengin/composeboard/internal/docker"
)

const (
	// created 状态持续超过该阈值，视为启动异常。
	startupWarningCreatedThreshold = 30 * time.Second
	// restarting 状态持续超过该阈值，视为启动异常。
	startupWarningRestartingThreshold = 30 * time.Second
)

// ServiceView 前端展示的完整服务视图
type ServiceView struct {
	// 声明态（来自 YAML）
	Name        string   `json:"name"`         // 服务名（YAML key）
	Category    string   `json:"category"`     // 分类（来自 label，非硬编码）
	ImageRef    string   `json:"image_ref"`    // image 字段原始值（含变量）
	ImageSource string   `json:"image_source"` // "registry" | "build" | "unknown"
	Profiles    []string `json:"profiles"`     // Profiles 列表
	DependsOn   []string `json:"depends_on"`   // 依赖服务
	HasBuild    bool     `json:"has_build"`    // 是否有 build 字段

	// 运行态（来自 Docker）
	ContainerID string               `json:"container_id"` // 容器 ID（12 位）
	Status      string               `json:"status"`       // "running" | "exited" | "not_deployed"
	State       string               `json:"state"`        // 人类可读状态文本
	StartedAt   string               `json:"started_at"`   // 启动时间 ISO 时间戳（用于 restart 完成判定）
	Ports       []docker.PortMapping `json:"ports"`
	Health      string               `json:"health"`
	// StartupWarning 表示容器当前处于异常运行态，需要用户关注。
	// 这是运行态派生诊断结果，不参与 loading 判定，只用于列表告警展示。
	StartupWarning bool `json:"startup_warning"`
	CPU         float64              `json:"cpu"`
	MemUsage    uint64               `json:"mem_usage"`
	MemLimit    uint64               `json:"mem_limit"`
	MemPercent  float64              `json:"mem_percent"`

	// 差异检测
	DeclaredImage string   `json:"declared_image"` // .env 展开后的预期镜像
	RunningImage  string   `json:"running_image"`  // 实际运行的镜像
	ImageDiff     bool     `json:"image_diff"`     // 镜像有差异
	EnvDiff       bool     `json:"env_diff"`       // 环境变量有变更（来自 state）
	PendingEnv    []string `json:"pending_env"`    // 具体变更的变量名列表
}

// ServiceManager 服务管理器
type ServiceManager struct {
	projectDir string
	cache      *docker.ContainerCache
	dockerCli  *docker.Client
	executor   *compose.Executor
	stateM     *StateManager // 状态管理器（延迟注入）

	mu           sync.RWMutex
	project      *compose.ComposeProject // 缓存的声明态
	envVars      map[string]string       // 缓存的 .env 变量
	lastParseErr error
}

// NewServiceManager 创建服务管理器
func NewServiceManager(projectDir string, cache *docker.ContainerCache, executor *compose.Executor) *ServiceManager {
	m := &ServiceManager{
		projectDir: projectDir,
		cache:      cache,
		executor:   executor,
	}
	// 初始解析
	m.ReloadCompose()
	return m
}

// ReloadCompose 重新解析 Compose 文件和 .env（配置变更后调用）
func (m *ServiceManager) ReloadCompose() {
	m.mu.Lock()
	defer m.mu.Unlock()

	project, err := compose.ParseComposeFile(m.projectDir)
	if err != nil {
		log.Printf("[SERVICE] 解析 Compose 文件失败: %v", err)
		m.lastParseErr = err
		return
	}
	m.project = project
	m.lastParseErr = nil

	// M-5: 注入 compose 文件路径给 executor，确保 parser 和 CLI 读同一文件
	m.executor.SetComposeFile(project.FilePath)

	envPath := filepath.Join(m.projectDir, ".env")
	vars, err := compose.ReadEnvVars(envPath)
	if err != nil {
		log.Printf("[SERVICE] 读取 .env 失败: %v", err)
		vars = make(map[string]string)
	}
	m.envVars = vars

	log.Printf("[SERVICE] 解析完成: %d 个服务, %d 个变量 (文件: %s)", len(m.project.Services), len(m.envVars), project.FilePath)
}

// GetComposeInfo 返回 Compose 命令和版本信息
func (m *ServiceManager) GetComposeInfo() (command string, version string) {
	return m.executor.GetCommandInfo()
}

// ListServices 生成前端需要的完整服务视图列表
// 融合声明态 + 运行态，做 LEFT JOIN
func (m *ServiceManager) ListServices() []ServiceView {
	m.mu.RLock()
	project := m.project
	envVars := m.envVars
	m.mu.RUnlock()

	if project == nil {
		return nil
	}

	return m.buildServiceViews(project, envVars, m.cache.Get())
}

// GetProject 获取当前解析的 Compose 项目
func (m *ServiceManager) GetProject() *compose.ComposeProject {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.project
}

// GetEnvVars 获取当前缓存的 .env 变量
func (m *ServiceManager) GetEnvVars() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]string, len(m.envVars))
	for k, v := range m.envVars {
		result[k] = v
	}
	return result
}

// GetExecutor 获取 Compose CLI 执行器
func (m *ServiceManager) GetExecutor() *compose.Executor {
	return m.executor
}

// IsAnyProfileEnabled 返回给定 profile 列表中是否有任一项处于启用配置态。
func (m *ServiceManager) IsAnyProfileEnabled(profileNames []string) bool {
	m.mu.RLock()
	stateM := m.stateM
	m.mu.RUnlock()

	if stateM == nil {
		return false
	}
	for _, profileName := range profileNames {
		if stateM.IsProfileEnabled(profileName) {
			return true
		}
	}
	return false
}

// SetStateManager 延迟注入 StateManager（解决循环依赖）
func (m *ServiceManager) SetStateManager(stateM *StateManager) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stateM = stateM
}

// SetDockerClient 延迟注入 Docker Client（供状态修复等逻辑读取运行态）
func (m *ServiceManager) SetDockerClient(dockerCli *docker.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dockerCli = dockerCli
}

// BuildRuntimeStateEntry 从当前运行中的容器读取已生效基线。
// 仅当服务已部署且可读取容器 env 时返回 true。
func (m *ServiceManager) BuildRuntimeStateEntry(serviceName string) (ServiceStateEntry, bool) {
	m.mu.RLock()
	project := m.project
	envVars := m.envVars
	dockerCli := m.dockerCli
	m.mu.RUnlock()

	if project == nil || dockerCli == nil {
		return ServiceStateEntry{}, false
	}

	decl, ok := project.Services[serviceName]
	if !ok {
		return ServiceStateEntry{}, false
	}

	_, containerID, err := dockerCli.FindContainerByServiceName(context.Background(), serviceName)
	if err != nil || containerID == "" {
		return ServiceStateEntry{}, false
	}

	runtimeEnv, err := dockerCli.GetContainerEnv(context.Background(), containerID)
	if err != nil {
		return ServiceStateEntry{}, false
	}

	entry := m.buildStateEntryFromRuntime(decl, envVars, runtimeEnv)
	return entry, true
}

// GetRealtimeServiceStatus 直查单服务实时状态，并同步回写缓存。
// 返回值仍然保持 ServiceView 结构，确保列表接口与实时接口字段语义完全一致。
func (m *ServiceManager) GetRealtimeServiceStatus(ctx context.Context, serviceName string) (ServiceView, error) {
	m.mu.RLock()
	project := m.project
	envVars := m.envVars
	dockerCli := m.dockerCli
	cache := m.cache
	m.mu.RUnlock()

	if project == nil {
		return ServiceView{}, &ServiceError{Code: "services.not_found", Message: "Compose 项目未初始化"}
	}

	decl, ok := project.Services[serviceName]
	if !ok {
		return ServiceView{}, &ServiceError{Code: "services.not_found", Message: "服务不存在"}
	}

	var runtimeInfo *docker.ContainerInfo
	if dockerCli != nil {
		info, err := dockerCli.GetServiceContainerInfo(ctx, serviceName, true)
		if err != nil {
			if errors.Is(err, docker.ErrNotFound) {
				cache.RemoveService(serviceName)
			} else {
				return ServiceView{}, err
			}
		} else {
			runtimeInfo = info
			cache.SyncService(*info)
		}
	}

	pendingEnv := m.getPendingEnvChanges()
	view := m.buildServiceView(decl, envVars, runtimeInfo)
	if vars, ok := pendingEnv[serviceName]; ok {
		view.EnvDiff = true
		view.PendingEnv = vars
	}
	return view, nil
}

// --- 内部实现 ---

// imagesMatch 比较两个镜像名是否匹配
// 处理 tag 缺省为 latest 的场景
func imagesMatch(declared, running string) bool {
	// M-7: Docker prune 后可能返回 sha256 裸 ID，无法比对，跳过
	if strings.HasPrefix(running, "sha256:") {
		return true
	}
	// 剥离 @sha256:... digest 后缀再比对
	if idx := strings.Index(running, "@sha256:"); idx > 0 {
		running = running[:idx]
	}
	d := normalizeImage(declared)
	r := normalizeImage(running)
	return d == r
}

// normalizeImage 规范化镜像名称
// 补充缺省的 :latest tag，移除 docker.io/library/ 前缀
// 正确处理带端口的私有 registry（如 localhost:5000/foo）
func normalizeImage(image string) string {
	// 移除常见的默认 registry 前缀
	image = strings.TrimPrefix(image, "docker.io/library/")
	image = strings.TrimPrefix(image, "docker.io/")

	// 判断是否有 tag：只看最后一个 / 之后的部分是否含 :
	// 这样 localhost:5000/foo 不会被误判为有 tag
	lastSlash := strings.LastIndex(image, "/")
	namePart := image
	if lastSlash >= 0 {
		namePart = image[lastSlash+1:]
	}
	if !strings.Contains(namePart, ":") {
		image += ":latest"
	}

	return image
}

func (m *ServiceManager) buildServiceViews(project *compose.ComposeProject, envVars map[string]string, containers []docker.ContainerInfo) []ServiceView {
	containerMap := make(map[string]*docker.ContainerInfo, len(containers))
	for i := range containers {
		containerMap[containers[i].ServiceName] = &containers[i]
	}

	pendingEnv := m.getPendingEnvChanges()
	views := make([]ServiceView, 0, len(project.Services))

	// 注意：project.Services 是 map，必须统一走声明层稳定顺序。
	for _, decl := range project.GetAllServices() {
		view := m.buildServiceView(decl, envVars, containerMap[decl.Name])
		if vars, ok := pendingEnv[decl.Name]; ok {
			view.EnvDiff = true
			view.PendingEnv = vars
		}
		views = append(views, view)
	}

	return views
}

func (m *ServiceManager) buildServiceView(decl *compose.DeclaredService, envVars map[string]string, ctr *docker.ContainerInfo) ServiceView {
	view := ServiceView{
		Name:        decl.Name,
		Category:    decl.Category,
		ImageRef:    decl.Image,
		ImageSource: decl.ImageSource,
		Profiles:    decl.Profiles,
		DependsOn:   decl.DependsOn,
		HasBuild:    decl.Build != "",
	}

	if decl.Image != "" {
		view.DeclaredImage = compose.ExpandVars(decl.Image, envVars)
	}

	if ctr == nil {
		view.Status = "not_deployed"
		view.State = "Not Deployed"
		view.Health = "none"
		return view
	}

	view.ContainerID = ctr.ID
	view.Status = ctr.Status
	view.State = ctr.State
	view.StartedAt = ctr.StartedAt
	view.Ports = ctr.Ports
	view.Health = ctr.Health
	view.StartupWarning = isStartupWarning(ctr)
	view.CPU = ctr.CPU
	view.MemUsage = ctr.MemUsage
	view.MemLimit = ctr.MemLimit
	view.MemPercent = ctr.MemPercent
	view.RunningImage = ctr.Image

	if view.ImageSource == "registry" && view.DeclaredImage != "" && view.RunningImage != "" {
		view.ImageDiff = !imagesMatch(view.DeclaredImage, view.RunningImage)
	}

	return view
}

func isStartupWarning(ctr *docker.ContainerInfo) bool {
	if ctr == nil {
		return false
	}
	if strings.EqualFold(ctr.Health, "unhealthy") {
		return true
	}

	switch ctr.Status {
	case "created":
		return containerAgeExceeded(ctr.Created, startupWarningCreatedThreshold)
	case "restarting":
		return stateDurationExceeded(ctr.State, startupWarningRestartingThreshold)
	default:
		return false
	}
}

func containerAgeExceeded(createdUnix int64, threshold time.Duration) bool {
	if createdUnix <= 0 {
		return false
	}
	createdAt := time.Unix(createdUnix, 0)
	return time.Since(createdAt) >= threshold
}

func stateDurationExceeded(state string, threshold time.Duration) bool {
	if state == "" {
		return false
	}
	duration, ok := extractAgoDuration(state)
	if !ok {
		return false
	}
	return duration >= threshold
}

// extractAgoDuration 从 Docker 人类可读状态文本中提取“距今多久”。
// 例如：
// - "Restarting (1) 40 seconds ago" -> 40s
// - "Restarting (2) About a minute ago" -> 1m
func extractAgoDuration(state string) (time.Duration, bool) {
	lower := strings.ToLower(strings.TrimSpace(state))
	switch {
	case strings.Contains(lower, "less than a second ago"):
		return 0, true
	case strings.Contains(lower, "about a minute ago"):
		return time.Minute, true
	case strings.Contains(lower, "about an hour ago"):
		return time.Hour, true
	}

	fields := strings.Fields(lower)
	for i := 1; i < len(fields); i++ {
		value, err := strconv.Atoi(fields[i-1])
		if err != nil {
			continue
		}

		switch strings.Trim(fields[i], " ,.") {
		case "second", "seconds":
			return time.Duration(value) * time.Second, true
		case "minute", "minutes":
			return time.Duration(value) * time.Minute, true
		case "hour", "hours":
			return time.Duration(value) * time.Hour, true
		case "day", "days":
			return time.Duration(value) * 24 * time.Hour, true
		case "week", "weeks":
			return time.Duration(value) * 7 * 24 * time.Hour, true
		}
	}
	return 0, false
}

func (m *ServiceManager) getPendingEnvChanges() map[string][]string {
	if m.stateM == nil {
		return nil
	}
	return m.stateM.GetPendingEnvChanges()
}

func (m *ServiceManager) buildStateEntryFromRuntime(decl *compose.DeclaredService, envVars map[string]string, runtimeEnv map[string]string) ServiceStateEntry {
	entry := ServiceStateEntry{
		Env: make(map[string]string),
	}

	if decl.Image != "" {
		entry.Image = compose.ExpandVars(decl.Image, envVars)
	}

	for _, varName := range decl.VarRefs {
		if value, ok := runtimeEnv[varName]; ok {
			entry.Env[varName] = value
		}
	}

	if len(entry.Env) == 0 {
		entry.Env = nil
	}

	return entry
}
