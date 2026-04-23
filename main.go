// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/fengin/composeboard/internal/api"
	"github.com/fengin/composeboard/internal/auth"
	"github.com/fengin/composeboard/internal/compose"
	"github.com/fengin/composeboard/internal/config"
	"github.com/fengin/composeboard/internal/docker"
	"github.com/fengin/composeboard/internal/service"
	"github.com/gin-gonic/gin"
)

//go:embed web/*
var webFS embed.FS

// 版本信息（编译时通过 -ldflags 注入）
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	// 命令行参数
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	// Banner
	fmt.Printf(`
╔══════════════════════════════════════╗
║     ComposeBoard v%-18s║
║     Docker Compose 可视化管理面板    ║
╚══════════════════════════════════════╝
`, Version)

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("[CONFIG] 加载配置失败: %v", err)
	}
	log.Printf("[CONFIG] 管理项目: %s", cfg.Project.Dir)
	log.Printf("[CONFIG] 监听地址: %s:%d", cfg.Server.Host, cfg.Server.Port)

	// === 初始化核心组件 ===

	// Docker 客户端
	// Docker 项目名检测链 (用于容器 label 过滤):
	//   COMPOSE_PROJECT_NAME → 目录名 (与 docker-compose CLI 行为一致)
	// 注意: cfg.Project.Name 是 UI 显示名称，不用于 Docker label 匹配
	dockerProjectName := ""
	envVars, _ := compose.ReadEnvVars(filepath.Join(cfg.Project.Dir, ".env"))
	if v, ok := envVars["COMPOSE_PROJECT_NAME"]; ok {
		dockerProjectName = v
	}
	if dockerProjectName == "" {
		// 兜底: 取项目目录名，与 docker-compose v1 行为一致（去掉非字母数字和连字符）
		dirName := filepath.Base(cfg.Project.Dir)
		// docker-compose v1 将目录名中的特殊字符(如点号)去掉
		sanitized := ""
		for _, c := range dirName {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
				sanitized += string(c)
			}
		}
		if sanitized == "" {
			sanitized = dirName
		}
		dockerProjectName = sanitized
	}
	log.Printf("[CONFIG] Docker 项目名: %s", dockerProjectName)
	dockerCli, err := docker.NewClient(dockerProjectName)
	if err != nil {
		log.Fatalf("[DOCKER] 初始化失败: %v", err)
	}

	// Docker 容器缓存
	cache := docker.NewContainerCache(dockerCli)

	// Compose CLI 执行器
	executor := compose.NewExecutor(cfg.Project.Dir, cfg.Compose.Command)

	// 检测 Compose 命令
	cmd, ver, err := executor.DetectCommand()
	if err != nil {
		log.Printf("[COMPOSE] 警告: %v", err)
	} else {
		log.Printf("[COMPOSE] 使用: %s (版本 %s)", cmd, ver)
	}

	// 服务管理器
	manager := service.NewServiceManager(cfg.Project.Dir, cache, executor)

	// 状态管理器
	stateM := service.NewStateManager(cfg.Project.Dir, manager)
	stateM.EnsureState()

	// 延迟注入 stateM 到 manager（解决循环依赖：manager ↔ stateM）
	manager.SetStateManager(stateM)
	manager.SetDockerClient(dockerCli)

	// 升级管理器
	upgradeM := service.NewUpgradeManager(manager, cache, executor, stateM)

	// Profile 管理器
	profileM := service.NewProfileManager(manager, cache, executor, stateM)

	// 生命周期管理器（启停重启业务逻辑）
	lifecycleM := service.NewLifecycleManager(manager, cache, dockerCli, executor)

	// API Handler
	// ProjectName 用于 UI 显示：优先用 config 的显示名称，兜底用 Docker project name
	displayName := cfg.Project.Name
	if displayName == "" {
		displayName = dockerProjectName
	}
	handler := api.NewHandler(displayName, cfg.Project.Dir, manager, lifecycleM, upgradeM, profileM, stateM, cache, dockerCli)

	// === 设置 Gin ===
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// 自定义日志中间件
	r.Use(func(c *gin.Context) {
		c.Next()
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			log.Printf("[API] %s %s → %d", c.Request.Method, c.Request.URL.Path, c.Writer.Status())
		}
	})

	// === API 路由 ===

	// 登录（不需要认证）
	r.POST("/api/auth/login", auth.HandleLogin)

	// 需要认证的 API
	authorized := r.Group("/api")
	authorized.Use(auth.JWTMiddleware())
	{
		// 主机信息
		authorized.GET("/host/info", handler.GetHostInfo)

		// 服务管理
		authorized.GET("/services", handler.ListServices)
		authorized.GET("/services/:name/status", handler.GetServiceStatus)
		authorized.POST("/services/:name/start", handler.StartService)
		authorized.POST("/services/:name/stop", handler.StopService)
		authorized.POST("/services/:name/restart", handler.RestartService)
		authorized.GET("/services/:name/env", handler.GetContainerEnv)

		// 升级与重建
		authorized.POST("/services/:name/pull", handler.PullImage)
		authorized.GET("/services/:name/pull-status", handler.GetPullStatus)
		authorized.POST("/services/:name/upgrade", handler.ApplyUpgrade)
		authorized.POST("/services/:name/rebuild", handler.RebuildService)

		// Profiles 管理
		authorized.GET("/profiles", handler.ListProfiles)
		authorized.POST("/profiles/:name/enable", handler.EnableProfile)
		authorized.POST("/profiles/:name/disable", handler.DisableProfile)

		// .env 配置
		authorized.GET("/env", handler.GetEnvFile)
		authorized.PUT("/env", handler.SaveEnvFile) // B-2: 对齐 DESIGN_DECISIONS §9

		// 日志
		authorized.GET("/services/:name/logs", handler.GetContainerLogs)

		// 设置（B-1: Dashboard 项目信息卡片）
		authorized.GET("/settings/project", handler.GetProjectSettings)
	}

	// === 静态文件（前端）===
	webContent, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("[WEB] 加载前端文件失败: %v", err)
	}

	indexHTML, err := fs.ReadFile(webContent, "index.html")
	if err != nil {
		log.Fatalf("[WEB] 读取 index.html 失败: %v", err)
	}

	r.GET("/css/*filepath", func(c *gin.Context) {
		c.FileFromFS("css"+c.Param("filepath"), http.FS(webContent))
	})
	r.GET("/js/*filepath", func(c *gin.Context) {
		c.FileFromFS("js"+c.Param("filepath"), http.FS(webContent))
	})

	// SPA 回退
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "API not found"})
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})

	// 启动服务器
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("[SERVER] 启动成功: http://%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("[SERVER] 启动失败: %v", err)
	}
}
