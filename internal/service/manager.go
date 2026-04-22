// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// manager.go 实现服务视图层，融合声明态（compose/parser）
// 和运行态（docker/cache），为 API 层提供统一的 ServiceView。
package service

import (
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fengin/composeboard/internal/compose"
	"github.com/fengin/composeboard/internal/docker"
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

	// 获取运行态容器列表（来自缓存）
	containers := m.cache.Get()

	// 构建 ServiceName → ContainerInfo 映射
	containerMap := make(map[string]*docker.ContainerInfo)
	for i := range containers {
		containerMap[containers[i].ServiceName] = &containers[i]
	}

	var views []ServiceView

	// 遍历声明态服务，做 LEFT JOIN
	for _, decl := range project.Services {
		view := ServiceView{
			// 声明态
			Name:        decl.Name,
			Category:    decl.Category,
			ImageRef:    decl.Image,
			ImageSource: decl.ImageSource,
			Profiles:    decl.Profiles,
			DependsOn:   decl.DependsOn,
			HasBuild:    decl.Build != "",
		}

		// .env 展开后的预期镜像
		if decl.Image != "" {
			view.DeclaredImage = compose.ExpandVars(decl.Image, envVars)
		}

		// LEFT JOIN 运行态
		if ctr, ok := containerMap[decl.Name]; ok {
			view.ContainerID = ctr.ID
			view.Status = ctr.Status
			view.State = ctr.State
			view.StartedAt = ctr.StartedAt
			view.Ports = ctr.Ports
			view.Health = ctr.Health
			view.CPU = ctr.CPU
			view.MemUsage = ctr.MemUsage
			view.MemLimit = ctr.MemLimit
			view.MemPercent = ctr.MemPercent
			view.RunningImage = ctr.Image

			// 镜像差异检测（仅 registry 类型）
			if view.ImageSource == "registry" && view.DeclaredImage != "" && view.RunningImage != "" {
				view.ImageDiff = !imagesMatch(view.DeclaredImage, view.RunningImage)
			}
		} else {
			// 未部署
			view.Status = "not_deployed"
			view.State = "Not Deployed"
			view.Health = "none"
		}

		views = append(views, view)
	}

	// T-11: EnvDiff 合并到 service 层（不再在 API Handler 里做）
	// G-2: 恢复 PendingEnv 具体变量名列表，供前端展示
	if m.stateM != nil {
		pendingEnv := m.stateM.GetPendingEnvChanges()
		for i := range views {
			if vars, ok := pendingEnv[views[i].Name]; ok {
				views[i].EnvDiff = true
				views[i].PendingEnv = vars
			}
		}
	}

	return views
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

// SetStateManager 延迟注入 StateManager（解决循环依赖）
func (m *ServiceManager) SetStateManager(stateM *StateManager) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stateM = stateM
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
