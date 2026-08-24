// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 应用配置
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Project    ProjectConfig    `yaml:"project"`
	Auth       AuthConfig       `yaml:"auth"`
	Compose    ComposeConfig    `yaml:"compose"`
	FileLogs   FileLogsConfig   `yaml:"file_logs"`
	Hooks      HooksConfig      `yaml:"hooks"`
	Extensions ExtensionsConfig `yaml:"extensions"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

// ProjectConfig 项目配置
type ProjectConfig struct {
	Dir  string `yaml:"dir"`  // docker-compose 项目目录路径
	Name string `yaml:"name"` // 项目显示名称（可选，缺省取目录名）
}

// AuthConfig 认证配置
type AuthConfig struct {
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	JWTSecret string `yaml:"jwt_secret"`
}

// ComposeConfig Compose 命令配置
type ComposeConfig struct {
	Command string `yaml:"command"` // "auto" | "docker-compose" | "docker compose"，缺省 auto
}

// FileLogsConfig 宿主机文件日志能力配置。
// 未显式启用时不注册任何可访问根目录，保持旧版本行为不变。
type FileLogsConfig struct {
	Enabled            bool                   `yaml:"enabled"`
	AllowedBases       []FileLogBaseConfig    `yaml:"allowed_bases"`
	FollowExtensions   []string               `yaml:"follow_extensions"`
	DownloadExtensions []string               `yaml:"download_extensions"`
	Discovery          FileLogDiscoveryConfig `yaml:"discovery"`
}

// FileLogBaseConfig 部署管理员授权的宿主机安全基准目录。
// 基准目录本身不会被递归扫描，服务日志只从当前服务挂载或人工相对路径中发现。
type FileLogBaseConfig struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// FileLogDiscoveryConfig 自动发现的性能边界。
type FileLogDiscoveryConfig struct {
	MaxDepth        int `yaml:"max_depth"`
	MaxEntries      int `yaml:"max_entries"`
	TimeoutMS       int `yaml:"timeout_ms"`
	CacheTTLSeconds int `yaml:"cache_ttl_seconds"`
}

// HooksConfig 生命周期钩子
type HooksConfig struct {
	PreDeploy  string `yaml:"pre_deploy"`  // 部署前执行的脚本路径
	PostDeploy string `yaml:"post_deploy"` // 部署后执行的脚本路径
}

// ExtensionsConfig 扩展功能配置
type ExtensionsConfig struct {
	HostIP HostIPExtension `yaml:"host_ip"`
}

// HostIPExtension HOST_IP 自适应扩展
type HostIPExtension struct {
	Enabled         bool   `yaml:"enabled"`           // 是否启用，默认 false
	EnvKey          string `yaml:"env_key"`           // .env 中的变量名，默认 HOST_IP
	DetectOnStartup bool   `yaml:"detect_on_startup"` // 启动时自动检测
}

// 全局配置实例
var C *Config

// Load 从文件加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	cfg := &Config{
		// 默认值
		Server: ServerConfig{
			Port: 9090,
			Host: "0.0.0.0",
		},
		Compose: ComposeConfig{
			Command: "auto",
		},
		Extensions: ExtensionsConfig{
			HostIP: HostIPExtension{
				EnvKey: "HOST_IP",
			},
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 校验必填项
	if cfg.Project.Dir == "" {
		return nil, fmt.Errorf("project.dir 不能为空")
	}
	if cfg.Auth.Username == "" || cfg.Auth.Password == "" {
		return nil, fmt.Errorf("auth.username 和 auth.password 不能为空")
	}
	if cfg.Auth.JWTSecret == "" {
		// 自动生成随机密钥（每次启动不同，重启后旧 token 失效）
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("生成 JWT 密钥失败: %w", err)
		}
		cfg.Auth.JWTSecret = base64.RawURLEncoding.EncodeToString(b)
		log.Println("[CONFIG] jwt_secret 未配置，已自动生成随机密钥（重启后失效，建议在 config.yaml 中配置固定值）")
	}

	if len(cfg.FileLogs.FollowExtensions) == 0 {
		cfg.FileLogs.FollowExtensions = []string{".log"}
	}
	if len(cfg.FileLogs.DownloadExtensions) == 0 {
		cfg.FileLogs.DownloadExtensions = []string{".log", ".gz"}
	}
	if cfg.FileLogs.Discovery.MaxDepth <= 0 {
		cfg.FileLogs.Discovery.MaxDepth = 2
	}
	if cfg.FileLogs.Discovery.MaxEntries <= 0 {
		cfg.FileLogs.Discovery.MaxEntries = 2000
	}
	if cfg.FileLogs.Discovery.TimeoutMS <= 0 {
		cfg.FileLogs.Discovery.TimeoutMS = 300
	}
	if cfg.FileLogs.Discovery.CacheTTLSeconds <= 0 {
		cfg.FileLogs.Discovery.CacheTTLSeconds = 60
	}

	C = cfg
	return cfg, nil
}
