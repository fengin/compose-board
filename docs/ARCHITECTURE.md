# ComposeBoard 架构设计文档

> **版本**: v2.1  
> **日期**: 2026-04-21

---

## 1. 现有架构问题分析

### 1.1 DeployBoard 代码问题

| 问题 | 文件 | 说明 |
|------|------|------|
| **God File** | `upgrade.go` (734行) | 混合：升级、重建、.env 解析、YAML 解析、版本检测、状态文件、服务映射 |
| **God File** | `client.go` (581行) | 混合：Docker API、YAML 解析、服务分类、项目名检测 |
| **层次不清** | `api/` 包 | handler 中包含大量业务逻辑 |
| **无服务层** | — | handler 直接调用 docker client |
| **全局状态** | `config.C` | 全局变量 |
| **关注混合** | `docker/` 包 | Docker Engine API 与 Compose YAML 解析混在一起 |

### 1.2 目标架构原则

1. **分层清晰**：Handler → Service → Infrastructure
2. **单一职责**：每个文件只做一件事
3. **Compose 与 Docker 分离**：YAML 解析 / CLI 执行 / Engine API 是不同关注点
4. **可测试**：接口依赖注入

---

## 2. 新架构设计

### 2.1 目录结构

```
compose-board/                        # module: github.com/fengin/composeboard
├── main.go                           # 入口：路由注册、依赖注入、启动
├── go.mod / go.sum / Makefile
│
├── internal/
│   ├── api/                          # HTTP Handler 层（薄层）
│   │   ├── handler.go                # Handler 结构体 + 构造函数
│   │   ├── services.go               # 服务列表 + 启停重启
│   │   ├── upgrade.go                # 升级/重建 API（逻辑在 service 层）
│   │   ├── profiles.go               # Profiles 启用/停用 API
│   │   ├── terminal.go               # Web 终端 API（Docker Exec WebSocket 代理）
│   │   ├── deploy.go                 # 部署向导 API
│   │   ├── env.go                    # .env 文件 API
│   │   ├── logs.go                   # 日志 SSE 流（follow 模式用 text/event-stream）
│   │   ├── settings.go               # 项目设置 API（GET /api/settings/project）
│   │   └── host.go                   # 主机信息 API
│   │
│   ├── auth/                         # 认证（不变）
│   │   └── auth.go
│   │
│   ├── compose/                      # Docker Compose 层（新增）
│   │   ├── parser.go                 # 解析 Compose YAML
│   │   │                             #   - 服务声明（image/profiles/depends_on/ports）
│   │   │                             #   - Compose 文件自动发现
│   │   │                             #   - ${VAR} 引用提取
│   │   ├── env.go                    # .env 文件管理
│   │   │                             #   - EnvEntry 行级模型（保留注释/空行/顺序）
│   │   │                             #   - ParseEnvFile() / WriteEnvFile()
│   │   │                             #   - ReadEnvVars()（纯 map，内部用）
│   │   │                             #   - ExpandVars()
│   │   └── executor.go               # Compose CLI 命令执行
│   │                                 #   - 自动检测 v1/v2
│   │                                 #   - Up / Pull / Stop / Rm / Down
│   │                                 #   - --profile / --no-deps 支持
│   │
│   ├── docker/                       # Docker Engine 层（精简）
│   │   ├── client.go                 # Docker API HTTP 客户端
│   │   │                             #   - ListContainers()（标签过滤）
│   │   │                             #   - Stats / Start / Stop / Restart
│   │   │                             #   - GetContainerEnv / Logs / Status
│   │   │                             #   - CreateExec / StartExec / ResizeExec（Web 终端）
│   │   │                             #   - 不再包含 YAML 解析、服务分类
│   │   ├── transport.go              # 本地 Transport 抽象
│   │   │                             #   - Linux: unix:///var/run/docker.sock
│   │   │                             #   - Windows: npipe:////./pipe/docker_engine
│   │   └── cache.go                  # 容器数据缓存（核心逻辑不变）
│   │
│   ├── service/                      # 业务逻辑层（新增）
│   │   ├── manager.go                # 服务视图管理器
│   │   │                             #   - ListServices()：声明 LEFT JOIN 容器
│   │   │                             #   - categorizeService()
│   │   ├── upgrade.go                # 升级/重建编排
│   │   │                             #   - 镜像差异检测（直接对比）
│   │   │                             #   - PullImage / ApplyUpgrade / Rebuild
│   │   ├── profiles.go               # Profiles 管理
│   │   │                             #   - ListProfiles()
│   │   │                             #   - EnableProfile() / DisableProfile()
│   │   ├── deploy.go                 # 部署编排
│   │   │                             #   - 环境检查 / 钩子执行 / 部署流程
│   │   └── state.go                  # 状态文件管理
│   │                                 #   - .composeboard-state.json
│   │
│   ├── config/
│   │   └── config.go                 # 配置加载（扩展字段）
│   │
│   └── host/
│       └── host.go                   # 系统信息 + IP 检测
│
├── web/                              # 前端（go:embed）
│   ├── index.html
│   ├── css/
│   │   ├── style.css
│   │   └── vendor/
│   └── js/
│       ├── app.js                    # Vue Router
│       ├── api.js                    # API 封装
│       ├── i18n.js                   # 轻量 i18n（t() 函数）
│       ├── locales/                  # 语言包
│       │   ├── zh.json               # 中文（默认）
│       │   └── en.json               # English
│       ├── vendor/
│       ├── components/
│       └── pages/
│           ├── login.js
│           ├── dashboard.js          # 增强项目信息
│           ├── services.js           # 服务管理（重构）
│           ├── env.js
│           ├── logs.js               # 增加重连
│           ├── terminal.js           # Web 终端（新增）
│           ├── settings.js           # 新增
│           └── deploy.js             # 新增
│
└── docs/
```

### 2.2 分层架构图

```
┌────────────────────────────────────────────────────┐
│                    前端 (Vue SPA)                    │
│  pages/ ──→ api.js ──→ REST / WebSocket            │
└────────────────────┬───────────────────────────────┘
                     │
┌────────────────────▼───────────────────────────────┐
│              API 层 (Gin Handlers)                   │
│  services.go | upgrade.go | profiles.go | env.go    │
│  deploy.go   | settings.go | logs.go   | host.go   │
│  ─── 只做参数解析和响应封装 ───                       │
└────────────────────┬───────────────────────────────┘
                     │
┌────────────────────▼───────────────────────────────┐
│             业务逻辑层 (Service)                      │
│  manager.go  | upgrade.go | profiles.go             │
│  deploy.go   | state.go                             │
│  ─── 流程编排、数据聚合、状态管理 ───                  │
└──────┬─────────────┬──────────────┬────────────────┘
       │             │              │
┌──────▼──────┐ ┌────▼─────┐ ┌─────▼─────┐
│  compose/   │ │ docker/  │ │ config/   │
│  parser.go  │ │ client.go│ │ host/     │
│  env.go     │ │ transport│ │           │
│  executor.go│ │ cache.go │ │           │
└─────────────┘ └──────────┘ └───────────┘
   YAML 解析      Engine API    配置/系统
   .env 管理      容器缓存
   CLI 执行
```

### 2.3 模块职责矩阵

| 模块 | 职责 | 不应该做的事 |
|------|------|-------------|
| `api/` | 参数解析、响应封装、路由映射 | 业务逻辑、文件 I/O、YAML 解析 |
| `service/` | 业务流程编排、数据聚合、状态管理 | HTTP 处理、直接 Docker API |
| `compose/` | YAML 解析、.env 读写、CLI 执行 | 容器运行时查询 |
| `docker/` | Docker Engine API、容器缓存 | Compose YAML 解析、服务分类 |
| `config/` | 配置加载、校验 | 业务逻辑 |
| `host/` | 系统信息、IP 检测 | Docker/Compose 操作 |

---

### 2.4 Docker 本地 Transport 方案

ComposeBoard v1 支持两种本地 Docker 连接方式，均由 `docker/transport.go` 统一抽象：

| 平台 | Transport | 说明 |
|------|-----------|------|
| Linux | Unix Socket | 连接 `/var/run/docker.sock` |
| Windows (Docker Desktop) | Named Pipe | 连接 `//./pipe/docker_engine` |

**约束**：
- `docker/client.go` 不直接写死连接方式，由 `transport.go` 根据运行平台创建连接
- Linux 使用标准库 `net.Dial("unix", ...)`；Windows 使用 `github.com/Microsoft/go-winio` 拨号 Named Pipe
- 远程 Docker Host（TCP/TLS、SSH）仍属于 Phase 3 预留能力，不进入 v1 实现范围
- 上层 `service/`、`api/`、`compose/` 不感知底层使用 Unix Socket 还是 Named Pipe

---

## 3. 核心数据模型

### 3.1 服务视图（ServiceView）— API 返回的核心结构

```go
// ServiceView Compose 声明 LEFT JOIN 实际容器
type ServiceView struct {
    // === 来自 docker-compose.yml ===
    ComposeKey    string   `json:"compose_key"`      // service key
    DeclaredImage string   `json:"declared_image"`   // .env 展开后的目标镜像
    ImageSource   string   `json:"image_source"`     // v1 固定为 "registry"，字段为后续扩展预留
    Profiles      []string `json:"profiles"`         // 所属 profiles（空 = 必选，optional 仅由 profiles 表达）
    DependsOn     []string `json:"depends_on"`
    
    // === 来自运行时（deployed=true 时） ===
    Deployed      bool     `json:"deployed"`
    ContainerID   string   `json:"container_id,omitempty"`
    ContainerName string   `json:"container_name,omitempty"`
    Status        string   `json:"status,omitempty"`       // running / exited
    State         string   `json:"state,omitempty"`        // "Up 42 hours"
    RunningImage  string   `json:"running_image,omitempty"`
    Health        string   `json:"health,omitempty"`
    CPU           float64  `json:"cpu,omitempty"`
    MemUsage      uint64   `json:"mem_usage,omitempty"`
    MemLimit      uint64   `json:"mem_limit,omitempty"`
    MemPercent    float64  `json:"mem_percent,omitempty"`
    Ports         []PortMapping `json:"ports,omitempty"`
    Created       int64    `json:"created,omitempty"`
    
    // === 差异检测（仅 image_source="registry" 且 deployed=true） ===
    ImageDiff     bool     `json:"image_diff"`
    PendingEnv    []string `json:"pending_env,omitempty"`
    
    // === 分类 ===
    Category      string   `json:"category"`   // 来自 labels.com.composeboard.category，未标注默认 "other"
}
```

### 3.2 Compose 服务声明（compose/parser.go 内部）

```go
type DeclaredService struct {
    Key           string
    Category      string            // 从 labels.com.composeboard.category 读取；未标注 = "other"
    Image         string            // 原始 image 字段（含 ${VAR}）
    ExpandedImage string            // .env 展开后
    Build         string            // build 字段（若存在则说明该服务超出 v1 支持范围）
    ContainerName string
    Profiles      []string
    DependsOn     []string
    Ports         []string
    Labels        map[string]string // 原始 labels，供未来扩展读取
    EnvRefs       []string          // 引用的 ${VAR} 变量名（仅引用名，不含值）
}
```

### 3.3 .env 行级模型（compose/env.go）

```go
// EnvEntry .env 文件单行
type EnvEntry struct {
    Type  string `json:"type"`            // "variable" | "comment" | "blank"
    Key   string `json:"key,omitempty"`
    Value string `json:"value,omitempty"`
    Raw   string `json:"raw"`            // 原始行文本
    Line  int    `json:"line"`
}
```

### 3.4 配置文件（config.go）

```go
type Config struct {
    Server     ServerConfig     `yaml:"server"`
    Project    ProjectConfig    `yaml:"project"`
    Auth       AuthConfig       `yaml:"auth"`
    Compose    ComposeConfig    `yaml:"compose"`
    Hooks      HooksConfig      `yaml:"hooks"`
    Extensions ExtensionsConfig `yaml:"extensions"`
}

type ServerConfig struct {
    Addr string `yaml:"addr"` // 监听地址，默认 ":8080"
}

type ProjectConfig struct {
    Dir  string `yaml:"dir"`  // Compose 项目目录，必填
    Name string `yaml:"name"` // 可选；为空时按 COMPOSE_PROJECT_NAME → PROJECT_NAME → 目录名 顺序检测
}

type AuthConfig struct {
    Username  string `yaml:"username"`
    Password  string `yaml:"password"`    // bcrypt 散列
    JWTSecret string `yaml:"jwt_secret"`  // 启动时若为空则自动生成并写回 config.yaml
    TokenTTL  string `yaml:"token_ttl"`   // Go duration 字符串，默认 "24h"
}

type ComposeConfig struct {
    Command string `yaml:"command"` // "auto" | "docker-compose" | "docker compose"
}

type HooksConfig struct {
    PreDeploy  string `yaml:"pre_deploy"`
    PostDeploy string `yaml:"post_deploy"`
}

type ExtensionsConfig struct {
    HostIP HostIPExtension `yaml:"host_ip"`
}

type HostIPExtension struct {
    Enabled         bool   `yaml:"enabled"`           // 默认 false
    EnvKey          string `yaml:"env_key"`           // 默认 "HOST_IP"
    DetectOnStartup bool   `yaml:"detect_on_startup"` // 默认 true
}
```

完整示例见仓库根目录 `config.yaml.template`。

---

## 4. API 设计

> **WebSocket 鉴权**：所有 `GET (WS)` 接口通过 URL query 参数 `?token=<jwt>` 携带 JWT，握手升级前校验，失败返回 401。详见 DESIGN_DECISIONS §13。

### 4.1 认证
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/auth/login` | 登录 |

### 4.2 服务管理（仅 image: 型服务）
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/services` | 服务列表（仅返回 `image:` 型服务，声明 LEFT JOIN 容器） |
| POST | `/api/services/:key/start` | 启动（仅 `image:` 型服务；已停止容器直接 Start；未部署且无 profile 的必选服务等价 `docker compose up -d <service>`） |
| POST | `/api/services/:key/stop` | 停止 |
| POST | `/api/services/:key/restart` | 重启 |
| GET | `/api/services/:key/env` | 服务环境变量 |
| GET | `/api/services/:key/status` | 实时状态 |

### 4.3 升级与重建（仅 image: 型服务）
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/services/:key/pull` | 拉取镜像 |
| GET | `/api/services/:key/pull` | 拉取状态查询 |
| POST | `/api/services/:key/upgrade` | 应用升级 |
| POST | `/api/services/:key/rebuild` | 重建容器 |

### 4.4 Profiles 管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/profiles` | 所有 profiles，每项含 `status`（`enabled` / `partial` / `disabled`）和服务列表 |
| POST | `/api/profiles/:name/enable` | 启用整个 profile |
| POST | `/api/profiles/:name/disable` | 停用整个 profile |

### 4.5 配置管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/env` | .env（返回 EnvEntry 数组） |
| GET | `/api/env?raw=true` | .env 原始文本 |
| PUT | `/api/env` | 保存（支持 entries 或 content） |

### 4.6 日志
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/services/:name/logs?tail=200` | 一次性获取历史日志 |
| GET (SSE) | `/api/services/:name/logs?follow=true` | SSE 实时日志流 |

### 4.7 设置
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/settings/project` | 项目信息（只读） |
| GET | `/api/settings/compose` | Compose 命令信息 |
| PUT | `/api/settings/compose` | 更新命令设置 |
| GET | `/api/settings/docker` | Docker 信息（只读） |

### 4.8 部署向导
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/deploy/check` | 环境检查 |
| POST | `/api/deploy/start` | 启动部署（返回 ID） |
| GET (WS) | `/api/deploy/:id/stream` | WebSocket 实时进度 |
| GET | `/api/deploy/:id/status` | 状态查询（兜底） |

### 4.9 Web 终端
| 方法 | 路径 | 说明 |
|------|------|------|
| GET (WS) | `/api/services/:key/terminal` | WebSocket 交互式终端（exec 进容器，支持输入/输出/resize） |

**WebSocket 协议**：

前端发送：
```json
{ "type": "input", "data": "ls -la\n" }
{ "type": "resize", "cols": 120, "rows": 40 }
{ "type": "close" }
```

后端推送：
```json
{ "type": "output", "data": "total 0\r\n" }
```

**处理语义**：
- `input`：写入 Docker Exec stdin
- `resize`：调用 Docker Exec Resize API，同步终端 `cols/rows`
- `close`：关闭 WebSocket 和对应 exec 会话
- `output`：将 stdout/stderr 原样推送到前端 xterm.js

### 4.10 系统 + 扩展
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/host/info` | 主机 + Docker 版本 |
| GET | `/api/extensions/host-ip/detect` | IP 检测（需启用扩展） |
| POST | `/api/extensions/host-ip/update` | 更新 IP（需启用扩展） |

### 4.11 Dashboard 概览数据

Dashboard 页面不提供独立的聚合接口，由前端并发组合调用：

| 展示区块 | 数据来源 |
|---------|---------|
| 项目信息（名称/目录/Compose 版本/服务统计/Profiles） | `GET /api/settings/project` |
| 主机信息（OS / Docker 版本 / API 版本） | `GET /api/host/info` |
| 资源指标（CPU/内存/磁盘） | `GET /api/host/info` |
| 服务状态统计、分组卡片 | `GET /api/services`（前端按 `status` / `category` 聚合） |

后续如发现前端并发影响首屏体验，可再引入 `GET /api/dashboard/overview` 聚合接口。

---

## 5. 页面交互设计

### 5.1 服务管理页

```
┌──────────────────────────────────────────────────────────┐
│  服务管理                                     [刷新]      │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  ── 基础服务 (7) ──                                       │
│  ┌──────────────────────────────────────────────────────┐│
│  │ ● mysql         运行中   Up 3d   :3306  CPU 2.1%    ││
│  │ ● redis         运行中   Up 3d   :6379  CPU 0.3%    ││
│  │ ● nacos         运行中   Up 3d   :8848  CPU 1.5%    ││
│  │ ● emqx          运行中   Up 3d   :1883  CPU 0.5%    ││
│  │ ○ emqx-init     已退出   Exited (0)                  ││
│  │ ● job-admin     运行中   Up 3d   :8082  CPU 0.2%    ││
│  │ ● kafka         运行中   Up 3d   :9092  CPU 0.8%    ││
│  └──────────────────────────────────────────────────────┘│
│                                                          │
│  ── 后端服务 (4) ──                                       │
│  ┌──────────────────────────────────────────────────────┐│
│  │ ● starter-platform  运行中  :8055   ⬆ 镜像有更新     ││
│  │ ● starter-app       运行中  :8025         CPU 1.2%   ││
│  │ ● starter-link      运行中  :8021  🔄 配置待重建      ││
│  │ ● starter-job       运行中  :8027         CPU 0.4%   ││
│  └──────────────────────────────────────────────────────┘│
│                                                          │
│  ── 可选: fdfs ─────────── 未启用 ⭕ ────── [启用]       │
│  ┌──────────────────────────────────────────────────────┐│
│  │ ⭕ fdfs-tracker    未部署                             ││
│  │ ⭕ fdfs-storage    未部署                             ││
│  └──────────────────────────────────────────────────────┘│
│                                                          │
│  ── 可选: rule ─────────── 未启用 ⭕ ────── [启用]       │
│  ┌──────────────────────────────────────────────────────┐│
│  │ ⭕ rule-engine     未部署                             ││
│  └──────────────────────────────────────────────────────┘│
│                                                          │
│  ── 可选: xiaoxin ──────── 未启用 ⭕ ────── [启用]       │
│  ┌──────────────────────────────────────────────────────┐│
│  │ ⭕ xiaoxin-server   未部署                            ││
│  │ ⭕ xiaoxin-server-ui 未部署                           ││
│  └──────────────────────────────────────────────────────┘│
└──────────────────────────────────────────────────────────┘
```

**交互要点**：
- v1 服务管理页只覆盖 `image:` 型服务
- 必选服务按分类分组（base/backend/frontend/init/other），分类来自 Compose label `com.composeboard.category`，缺省为 `other`
- 可选服务按 profile 分组，标题含三态状态与操作按钮：
  - `enabled` → `已启用 ✅ ── [停用]`
  - `partial` → `部分启用 ⚠ ── [补齐启用] [全部停用]`
  - `disabled` → `未启用 ⭕ ── [启用]`
- `[启用]` / `[补齐启用]` 按钮 → 确认弹窗 → `POST /api/profiles/:name/enable`
- 未部署且无 profile 的必选服务行显示 `[启动]`，语义等价 `docker compose up -d <service>`
- 未部署的可选服务行上**无单服务操作按钮**
- Profile 启用/停用操作走 **await API + 轮询校验** 两阶段：立即关弹窗 + 头部 `profileLoading` + 下属服务行批量 `_loading` → `await POST /api/profiles/:name/enable|disable`（backend 完成 Up/Stop+Rm 并刷新 cache）→ 轮询 `GET /api/profiles` 直到满足双重判据（聚合态 `profile.status` 命中目标态 **且** 所有下属服务个体态一致）。详见 `DESIGN_DECISIONS.md §13`
- 单服务操作（stop/start/restart/upgrade/rebuild）的轮询机制见 `DESIGN_DECISIONS.md §12`

---

## 6. 迁移策略

### 6.1 直接复用

| 模块 | 处理 |
|------|------|
| `auth/auth.go` | 原样迁移 |
| `docker/cache.go` | 原样迁移 |
| `host/host.go` | 迁移 + 扩展 IP 检测 |
| `web/js/vendor/` + `web/css/vendor/` | 原样迁移 |
| `web/js/components/` | 原样迁移 |
| 登录页、.env 编辑页 | 基本不变 |

### 6.2 重写/拆分

| 旧 → 新 | 说明 |
|---------|------|
| `client.go` YAML 解析 → `compose/parser.go` | 抽离 |
| `client.go` 容器过滤 → `com.docker.compose.project` 标签过滤 | 重写 |
| `client.go` `categorizeService()` → 读取 `com.composeboard.category` 标签 | 删除硬编码 |
| `client.go` `ContainerInfo.Category` 字段 → 移除 | 分类由 `service/manager.go` 在组装 ServiceView 时填入 |
| `upgrade.go` 版本检测 → `service/upgrade.go` | 镜像对比 |
| `upgrade.go` 状态文件 → `service/state.go` | 独立 |
| `upgrade.go` .env 读写 → `compose/env.go`（EnvEntry 模型） | 重写 |
| `upgrade.go` CLI 调用 → `compose/executor.go` | 统一 |
| `containers.go` → `api/services.go` + `service/manager.go` | 重构 |
| `containers.js` → `services.js` | 重写 |

### 6.3 全新

| 模块 |
|------|
| `compose/executor.go` |
| `docker/transport.go` |
| `service/manager.go` / `profiles.go` / `deploy.go` |
| `api/profiles.go` / `terminal.go` / `deploy.go` / `settings.go` |
| `web/js/pages/settings.js` / `terminal.js` / `deploy.js` |
| `web/js/vendor/xterm.js`（终端模拟器库） |
