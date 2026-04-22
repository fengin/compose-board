// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// compose 包负责 Compose 项目的声明态解析，
// 包括 YAML 文件发现、服务定义提取、变量名提取。
// 不做变量展开（由 env.go 负责）。
package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComposeProject 表示解析后的 Compose 项目
type ComposeProject struct {
	Version  string                      // Compose 文件版本（可能为空）
	Services map[string]*DeclaredService // 服务名 → 服务定义
	FilePath string                      // 解析的文件路径
}

// DeclaredService 声明态的服务定义（从 YAML 中提取）
type DeclaredService struct {
	Name        string            // 服务名（YAML key）
	Image       string            // image 字段原始值（可能含 ${VAR}）
	Build       string            // build 字段（非空表示本地构建）
	Profiles    []string          // profiles 列表
	DependsOn   []string          // 依赖的服务名
	Ports       []string          // 端口映射原始值
	Environment []string          // 环境变量原始值
	Labels      map[string]string // labels 键值对
	Category    string            // 从 com.composeboard.category label 读取，缺省 "other"
	ImageSource string            // "registry" | "build" | "unknown"
	VarRefs     []string          // 服务配置中引用的非 image 变量名列表（用于 pending_env / rebuild 判定）
}

// 变量引用正则：匹配 ${VAR_NAME} 和 ${VAR:-default} 等形式，提取变量名部分
var varRefRegex = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?:[:\-\+\?][^}]*)?\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// Compose 文件自动发现优先级
var composeFileNames = []string{
	"compose.yaml",
	"compose.yml",
	"docker-compose.yml",
	"docker-compose.yaml",
}

// Category label key
const categoryLabelKey = "com.composeboard.category"

// FindComposeFile 在指定目录中按优先级自动发现 Compose 文件
func FindComposeFile(dir string) (string, error) {
	for _, name := range composeFileNames {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("未找到 Compose 文件，已检查: %s", strings.Join(composeFileNames, ", "))
}

// ParseComposeFile 解析 Compose 项目目录，返回结构化的项目定义
func ParseComposeFile(dir string) (*ComposeProject, error) {
	filePath, err := FindComposeFile(dir)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取 Compose 文件失败: %w", err)
	}

	// 解析 YAML
	var raw rawComposeFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析 Compose YAML 失败: %w", err)
	}

	project := &ComposeProject{
		Version:  raw.Version,
		Services: make(map[string]*DeclaredService),
		FilePath: filePath,
	}

	for name, svc := range raw.Services {
		declared := parseDeclaredService(name, svc)
		project.Services[name] = declared
	}

	return project, nil
}

// GetAllServices 返回所有声明服务的有序列表（按名称排序）
func (p *ComposeProject) GetAllServices() []*DeclaredService {
	services := make([]*DeclaredService, 0, len(p.Services))
	for _, svc := range p.Services {
		services = append(services, svc)
	}
	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})
	return services
}

// GetProfiles 返回 profile 名 → 服务名列表的映射
func (p *ComposeProject) GetProfiles() map[string][]string {
	profiles := make(map[string][]string)
	for _, svc := range p.Services {
		for _, profile := range svc.Profiles {
			profiles[profile] = append(profiles[profile], svc.Name)
		}
	}
	return profiles
}

// GetVersion 返回 Compose 文件版本
func (p *ComposeProject) GetVersion() string {
	return p.Version
}

// --- 内部实现 ---

// rawComposeFile YAML 反序列化的中间结构
type rawComposeFile struct {
	Version  string                          `yaml:"version"`
	Services map[string]rawServiceDefinition `yaml:"services"`
}

// rawServiceDefinition 服务定义的 YAML 中间结构
type rawServiceDefinition struct {
	Image       string      `yaml:"image"`
	Build       interface{} `yaml:"build"` // string 或 object
	Profiles    []string    `yaml:"profiles"`
	DependsOn   interface{} `yaml:"depends_on"` // []string 或 map
	Ports       []string    `yaml:"ports"`
	Environment interface{} `yaml:"environment"` // []string 或 map
	Labels      interface{} `yaml:"labels"`      // []string 或 map
}

// parseDeclaredService 从原始 YAML 定义构造 DeclaredService
func parseDeclaredService(name string, raw rawServiceDefinition) *DeclaredService {
	svc := &DeclaredService{
		Name:     name,
		Image:    raw.Image,
		Profiles: raw.Profiles,
		Labels:   parseLabels(raw.Labels),
	}

	// 解析 build 字段
	svc.Build = parseBuild(raw.Build)

	// 判断 ImageSource：有 image 则为 registry（即使同时有 build）
	if svc.Image != "" {
		svc.ImageSource = "registry"
	} else if svc.Build != "" {
		svc.ImageSource = "build"
	} else {
		svc.ImageSource = "unknown"
	}

	// 从 label 读取 Category，缺省 "other"
	if cat, ok := svc.Labels[categoryLabelKey]; ok && cat != "" {
		svc.Category = cat
	} else {
		svc.Category = "other"
	}

	// 解析 depends_on
	svc.DependsOn = parseDependsOn(raw.DependsOn)

	// 解析 ports
	svc.Ports = raw.Ports

	// 解析 environment
	svc.Environment = parseEnvironment(raw.Environment)

	// 提取服务级变量引用，但排除 image 字段中的变量。
	// image 相关变化统一走 image_diff / upgrade 路径，不落入 pending_env / rebuild。
	imageVarRefs := extractVarNames(svc.Image)
	serviceVarRefs := extractServiceVarRefs(raw)
	svc.VarRefs = subtractVarRefs(serviceVarRefs, imageVarRefs)

	return svc
}

// parseBuild 解析 build 字段（可能是 string 或 object）
func parseBuild(v interface{}) string {
	if v == nil {
		return ""
	}
	switch b := v.(type) {
	case string:
		return b
	case map[string]interface{}:
		// build: { context: "./dir", dockerfile: "Dockerfile" }
		if ctx, ok := b["context"]; ok {
			return fmt.Sprintf("%v", ctx)
		}
		return "."
	}
	return ""
}

// parseLabels 解析 labels（支持 []string 和 map[string]string 两种格式）
func parseLabels(v interface{}) map[string]string {
	labels := make(map[string]string)
	if v == nil {
		return labels
	}
	switch l := v.(type) {
	case []interface{}:
		// labels: ["key=value", "key2=value2"]
		for _, item := range l {
			s := fmt.Sprintf("%v", item)
			// 去除可能的引号包裹
			s = strings.Trim(s, "\"'")
			if idx := strings.Index(s, "="); idx > 0 {
				labels[s[:idx]] = s[idx+1:]
			}
		}
	case map[string]interface{}:
		// labels: { key: value }
		for k, val := range l {
			labels[k] = fmt.Sprintf("%v", val)
		}
	}
	return labels
}

// parseDependsOn 解析 depends_on（支持 []string 和 map 两种格式）
func parseDependsOn(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch d := v.(type) {
	case []interface{}:
		var deps []string
		for _, item := range d {
			deps = append(deps, fmt.Sprintf("%v", item))
		}
		return deps
	case map[string]interface{}:
		// depends_on: { mysql: { condition: service_healthy } }
		var deps []string
		for name := range d {
			deps = append(deps, name)
		}
		return deps
	}
	return nil
}

// parseEnvironment 解析 environment（支持 []string 和 map 两种格式）
func parseEnvironment(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch e := v.(type) {
	case []interface{}:
		var envs []string
		for _, item := range e {
			envs = append(envs, fmt.Sprintf("%v", item))
		}
		return envs
	case map[string]interface{}:
		var envs []string
		for k, val := range e {
			envs = append(envs, fmt.Sprintf("%s=%v", k, val))
		}
		return envs
	}
	return nil
}

// extractVarNames 从模板字符串中提取变量引用名列表（仅名字，不含默认值）
func extractVarNames(template string) []string {
	if template == "" {
		return nil
	}
	matches := varRefRegex.FindAllStringSubmatch(template, -1)
	var names []string
	seen := make(map[string]bool)
	for _, m := range matches {
		// m[1] 是 ${VAR...} 中的变量名，m[2] 是 $VAR 中的变量名
		name := m[1]
		if name == "" {
			name = m[2]
		}
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// extractServiceVarRefs 从整个 service 配置中提取变量引用。
// 这里对 rawServiceDefinition 做 YAML 序列化，统一扫描 ports/environment/labels/build 等字段中的 ${VAR} / $VAR。
func extractServiceVarRefs(raw rawServiceDefinition) []string {
	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil
	}

	var refs []string
	for _, name := range extractVarNames(string(data)) {
		// PROJECT_NAME 主要用于容器命名/项目作用域，不应触发“配置待重建”提示。
		if name == "PROJECT_NAME" {
			continue
		}
		refs = append(refs, name)
	}
	return refs
}

// subtractVarRefs 计算 all - excluded，并保持原有顺序。
func subtractVarRefs(all, excluded []string) []string {
	if len(all) == 0 {
		return nil
	}
	if len(excluded) == 0 {
		return all
	}

	excludedSet := make(map[string]struct{}, len(excluded))
	for _, name := range excluded {
		excludedSet[name] = struct{}{}
	}

	result := make([]string, 0, len(all))
	for _, name := range all {
		if _, ok := excludedSet[name]; ok {
			continue
		}
		result = append(result, name)
	}

	if len(result) == 0 {
		return nil
	}
	return result
}
