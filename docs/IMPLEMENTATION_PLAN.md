# ComposeBoard 实施计划

> **版本**: v2.1  
> **日期**: 2026-04-21

---

## 阶段概览

| 阶段 | 目标 | 核心产出 |
|------|------|---------|
| **Phase 1** | 消除耦合、新架构、Profiles | 通用 Compose 管理面板 |
| **Phase 2** | 设置页、日志增强、Web 终端 | 运维体验完整 |
| **Phase 3** | 部署向导、扩展功能、远程连接 | 全生命周期覆盖 |

---

## Phase 1: 核心重构

### 1.1 项目初始化 + 代码骨架

**目标**：建立新项目，迁移可复用代码

- 初始化 Go module：`github.com/fengin/composeboard`
- 创建目录结构（`compose/`、`service/`）
- 所有代码文件添加头部注释（作者：凌封，网址：https://fengin.cn）
- 迁移前审查原 `deployboard-src` 仓库的 LICENSE / 署名 / 版权声明，保留原有开源许可信息
- 迁移：`auth/auth.go`、`docker/cache.go`、`host/host.go`
- 迁移：`web/` 前端目录（确保所有资源本地 vendor，无外部 URL 依赖）
- 更新 import 路径 → `github.com/fengin/composeboard/internal/xxx`
- 品牌名称 → ComposeBoard
- **i18n 基础设施**：
  - 创建 `web/js/i18n.js`（~50 行，提供 `t(key, params)` 函数）
  - 创建 `web/js/locales/zh.json`（中文，默认语言）
  - 创建 `web/js/locales/en.json`（英文，首版完整翻译）
  - Vue 全局注册 `$t` 方法
  - 语言偏好存 localStorage
- `config.go` 扩展字段（Compose/Hooks/Extensions）
- `config.yaml.template` 更新
- 验证编译通过（`go build -ldflags="-s -w"`）
- 参照 `docs/DEV_STANDARDS.md` 执行

---

### 1.2 compose/parser.go — YAML 解析器

**核心函数**：
```go
func FindComposeFile(dir string) (string, error)         // 自动发现
func ParseComposeFile(dir string) (*ComposeProject, error)
func (p *ComposeProject) GetAllServices() []DeclaredService
func (p *ComposeProject) GetProfiles() map[string][]string
func (p *ComposeProject) GetVersion() string
```

**要点**：
- 自动发现：compose.yaml → compose.yml → docker-compose.yml → docker-compose.yaml
- 解析 image / build / profiles / depends_on / ports / environment / labels
- 判断 `ImageSource`："registry"（有 image；即使同时存在 build 也以 image 为准）/"build"（仅 build）/"unknown"
- 从 `labels.com.composeboard.category` 读取 `Category`，缺省为 `"other"`；合法值建议 `base` / `backend` / `frontend` / `init` / `other`（不做强校验，未知值按 `other` 归入兜底分组）
- 提取 `${VAR}` 引用名列表（仅名字，不含值）
- 不做变量展开（由 env.go 负责）
- **职责边界**：parser 只负责"提取引用名"，env.go 负责"按变量表展开"，两边不重复扫描

**旧代码参考**：`client.go:498-521`、`upgrade.go:259-310`、`upgrade.go:615-679`

---

### 1.3 compose/env.go — .env 行级管理

**核心函数**：
```go
func ParseEnvFile(path string) ([]EnvEntry, error)       // 行级解析（保留注释/空行）
func ReadEnvVars(path string) (map[string]string, error)  // 纯变量 map（内部用）
func WriteEnvEntries(path string, entries []EnvEntry) error
func WriteEnvRaw(path string, content string) error
func ExpandVars(template string, vars map[string]string) string
```

**EnvEntry 模型**：
```go
type EnvEntry struct {
    Type  string `json:"type"`            // "variable" | "comment" | "blank"
    Key   string `json:"key,omitempty"`
    Value string `json:"value,omitempty"`
    Raw   string `json:"raw"`
    Line  int    `json:"line"`
}
```

**要点**：
- `ParseEnvFile` 逐行解析，保留注释行（`# xxx`）和空行
- `WriteEnvEntries` 按 EnvEntry 序列写回，保持原始格式
- `ExpandVars` 支持 Compose 标准变量替换语法：
  - `${VAR}` / `$VAR` — 简单替换
  - `${VAR:-default}` — 未设置或为空时用 default
  - `${VAR-default}` — 未设置时用 default
  - `${VAR:+replacement}` — 设置且非空时用 replacement
  - `${VAR+replacement}` — 设置时用 replacement
  - `${VAR:?error}` / `${VAR?error}` — 未设置时返回错误
- **变量来源仅限本地 `.env`**，不读取宿主机 `os.Environ()`，保证镜像差异检测的可复现性（详见 DESIGN_DECISIONS §15）
- 前端表格模式读取 EnvEntry 数组，修改后回传 entries
- 前端文本模式读取 raw content，修改后回传 content

**旧代码参考**：`upgrade.go:312-333`、`api/env.go`

---

### 1.4 compose/executor.go — CLI 执行器

**核心函数**：
```go
func NewExecutor(projectDir, command string) *Executor
func (e *Executor) DetectCommand() (cmd string, version string, err error)
func (e *Executor) Up(services []string, opts UpOptions) error
func (e *Executor) Pull(services []string) (output []byte, err error)
func (e *Executor) Stop(services []string) error
func (e *Executor) Rm(services []string, force bool) error

type UpOptions struct {
    ForceRecreate bool
    NoDeps        bool
    Profiles      []string
}
```

**要点**：
- 自动检测 docker-compose (v1) vs docker compose (v2)
- 所有 CLI 调用统一经过此模块
- `--profile` 参数统一处理
- 项目目录通过 `-f` 和 `--project-directory` 传入

**旧代码参考**：`upgrade.go` 中散落的 `exec.Command`、`deploy.sh:60-90`

---

### 1.5 docker/client.go — 标签过滤改造 + 本地 Transport 抽象

**改造内容**：
- `dockerContainer` 结构增加 `Labels map[string]string`
- `ListContainers()` 用 `com.docker.compose.project` 标签过滤
- `ServiceName` 从 `com.docker.compose.service` 标签获取
- 项目名检测链：config → COMPOSE_PROJECT_NAME → PROJECT_NAME → 目录名
- 新增 `docker/transport.go`：
  - Linux 使用标准库 `net.Dial("unix", "/var/run/docker.sock")`
  - Windows Docker Desktop 使用 `github.com/Microsoft/go-winio` 拨号 Named Pipe `//./pipe/docker_engine`
  - `NewClient()` 根据运行平台选择 transport
- 移除 `parseComposeServices()` → 已在 compose/parser.go
- **移除 `categorizeService()` 硬编码**：原 `client.go:540-562` 基于服务名模糊匹配（`starter` / `openapi` / `xiaoxin-server` 等业务名）写死分类，违反"零适配"原则；新实现改为读取 compose label `com.composeboard.category`
- 移除 `ContainerInfo.Category` 字段：分类属于声明态，不从运行态容器上取
- `getProjectName()` → `detectProjectName()`

---

### 1.6 service/manager.go — 服务视图

**核心函数**：
```go
func (m *ServiceManager) ListServices() ([]ServiceView, error)
```

**逻辑**：
1. `compose/parser.go` 解析全部声明服务（含 `Category`、`Labels`）
2. `compose/env.go` 读取 .env 变量，展开 image 字段（变量来源仅 `.env`）
3. `docker/cache.go` 获取实际容器
4. 用 `com.docker.compose.service` 标签做 LEFT JOIN
5. `ServiceView.Category` 直接取自 `DeclaredService.Category`（来自 compose label），不再做服务名模糊匹配
6. `ImageSource="registry"` 的服务做镜像差异检测
7. `ImageSource="build"` 的服务标记但不做差异检测
8. 环境变量差异检测（复用 state 机制）
9. `Profiles` 非空的服务视为可选服务；`optional` 不再作为 category 建模

---

### 1.7 service/upgrade.go — 升级编排

- PullImage / GetPullStatus / ApplyUpgrade / RebuildService
- 调用 `compose/executor` 执行
- PullStatus 从包级变量迁移到实例变量
- 不再依赖 `VersionVar` / `VersionVal`
- 升级前检查 `ImageSource`，`build` 型服务拒绝升级

---

### 1.8 service/state.go — 状态文件

- 从 `upgrade.go` 中拆分
- 改名 `.composeboard-state.json`
- 移除废弃的 Snapshot 代码
- **首次启动视为基线**：文件不存在时，按当前 `.env` 内容和运行中容器的 `env` / `image` 构造初始快照并落盘，当次请求不产生"配置漂移"告警；从此之后的变更才触发差异检测
- 迁移场景：从 DeployBoard 旧版 `.deployboard-state.json` 升级时，若发现同目录存在旧文件则读取并改名为 `.composeboard-state.json`（只做一次），避免首启把存量配置全部误报为漂移

---

### 1.9 service/profiles.go — Profiles 管理

**核心函数**：
```go
func (p *ProfileManager) ListProfiles() []ProfileInfo
func (p *ProfileManager) EnableProfile(name string) error
func (p *ProfileManager) DisableProfile(name string) error
```

**ProfileInfo**：
```go
type ProfileInfo struct {
    Name    string `json:"name"`
    Status  string `json:"status"`   // "enabled" | "disabled"
    Enabled bool   `json:"enabled"`
}
```

**状态语义**：
- `enabled`：该 profile 在配置层已启用
- `disabled`：该 profile 在配置层未启用
- 不再从服务运行状态反推 `partial`

**交互**：
- EnableProfile → `executor.Up(nil, UpOptions{Profiles: []string{name}})` → `state.SetProfileEnabled(name, true)`
- DisableProfile → 获取 profile 下的服务 keys → `executor.Stop(keys)` → `executor.Rm(keys, true)` → `state.SetProfileEnabled(name, false)`
- Profile 头部状态只反映配置是否启用；服务运行态由 `/api/services` / `/api/services/:name/status` 单独呈现

---

### 1.10 API 层重构

- `api/handler.go`：持有 ServiceManager / UpgradeManager / ProfileManager
- `api/services.go`：`GET /api/services` + 单服务实时状态 + 启停重启
  - `POST /api/services/:name/start`：
    - 已部署且已停止 → Docker Start
    - 未部署且无 profile 的必选服务 → `executor.Up([]string{key}, UpOptions{})`
    - 已启用 profile 下的未部署可选服务 → `executor.Up([]string{key}, UpOptions{Profiles: decl.Profiles})`
- `GET /api/services/:name/status`：直查 Docker 当前服务状态，并同步回写服务缓存
- `api/upgrade.go`：精简版（逻辑在 service 层）
- `api/profiles.go`：Profiles API（新增）
  - `GET /api/profiles`：返回 profile 配置启用态
- 更新 `main.go` 路由

---

### 1.11 前端重构

> **重要**：所有前端页面的用户可见文本均使用 `$t('key')` 而非硬编码字符串。

- `services.js`（重构自 `containers.js`）：
  - ServiceView 适配（三种状态、ImageSource 标识）
  - Profile 分组展示 + `[启用]`/`[停用]` 按钮在分组标题
  - 未部署且无 profile 的必选服务行显示 `[启动]`
  - 未部署的 profile 服务行无单服务操作按钮
- `api.js`：路径 `/api/containers` → `/api/services`，新增 profiles API
- `app.js`：路由更新，启动时加载 i18n locale
- `dashboard.js`：增加项目信息卡片
- `locales/zh.json` + `locales/en.json`：完整填充所有页面文本并保持 key 一致

---

### 1.12 配置 + 品牌更新

- `main.go` banner → ComposeBoard
- `index.html` 标题、meta
- `api.js` token key → `composeboard_token`
- `config.yaml.template` 完整示例

---

## Phase 2: 增值功能

### 2.1 设置页

- `api/settings.go`：通用设置（项目信息、Compose 命令、Docker 信息）
- 主机设置（条件渲染，仅 `extensions` 启用时显示）
- **语言切换**：中文/English 下拉选择，切换后重新加载 locale
- `web/js/pages/settings.js`：设置页 UI
- 路由 + 侧边栏更新

### 2.2 WebSocket 日志增强

- `logs.js`：自动重连（指数退避，最多 5 次）
- 连接状态 banner
- 重连后恢复日志流

### 2.3 .env 行级模型完善

- 确保前端表格模式正确使用 EnvEntry 数组读写
- 验证注释/空行/顺序完整保留

### 2.4 Web 终端

> 在浏览器中 exec 进入容器执行任意 shell 命令。基于 Docker Exec API，不需要 SSH。

**工作内容**：
- `docker/client.go` 增加：
  - `CreateExec(containerID, cmd)` — `POST /containers/{id}/exec`
  - `StartExec(execID)` — `POST /exec/{id}/start`（连接升级为双向流）
  - `ResizeExec(execID, cols, rows)` — `POST /exec/{id}/resize`
- `api/terminal.go`：
  - `GET (WS) /api/services/:key/terminal` — WebSocket 接入
  - 根据 service key 查找 container ID
  - 创建 exec 会话（默认 `/bin/sh`）
  - 双向代理 stdin/stdout
  - 解析 WebSocket 消息：
    - `{type:\"input\", data:string}`
    - `{type:\"resize\", cols:number, rows:number}`
    - `{type:\"close\"}`
  - 收到 `resize` 时调用 `ResizeExec()`
- `web/js/vendor/` 引入 xterm.js 5.x：
  - `xterm.js` + `xterm.css`（主库，锁定 `@xterm/xterm@5.5.0` 的 UMD build）
  - `xterm-addon-fit.js`（终端尺寸自适应，`@xterm/addon-fit@0.10.0`）
  - 直接下载 UMD 包放入 vendor 目录，不使用 npm / CDN
- `web/js/pages/terminal.js`：
  - xterm.js 终端组件 + FitAddon
  - 服务选择下拉框
  - 连接状态指示
  - 终端尺寸变化 → 调用 `fitAddon.fit()` → 通过 WebSocket 发送 `resize` 消息同步 cols/rows

**技术要点**：
- Docker Exec API 的交互模式需要 HTTP connection hijack（Go `net/http` Hijacker 接口）
- WebSocket 两侧：浏览器 ↔ ComposeBoard ↔ Docker Socket
- 容器内有 bash 则用 bash，否则回退 sh
- **TTY 固定为 `true`**：后端创建 exec 时始终 `Tty: true`，保证 `vi` / `top` 等全屏程序可用；不提供非 TTY 模式

---

## Phase 3: 高级功能

### 3.1 部署向导

- `service/deploy.go`：环境检查 + 钩子执行 + 部署编排
- `api/deploy.go`：REST + WebSocket 接口
  - `GET /api/deploy/check` — 环境检查
  - `POST /api/deploy/start` — 启动部署
  - `GET (WS) /api/deploy/:id/stream` — 实时日志推送
  - `GET /api/deploy/:id/status` — 兜底查询
- `web/js/pages/deploy.js`：Step 流程 UI

### 3.2 主机设置扩展

- HOST_IP 自适应（`extensions.host_ip.enabled` 启用）
  - 设置页条件渲染 IP 检测区块
- 开机自启动管理（systemd）

> **关于镜像仓库凭据**：Docker daemon 层通过 `docker login` 管理，ComposeBoard 无需介入。

### 3.3 Dashboard 图表

- 引入轻量图表库（vendor 方式）
- CPU/内存趋势图

### 3.4 远程 Docker Host 支持（后续开发）

> 支持连接远程服务器上的 Docker daemon，实现跨机器管理。

**技术方案（待定）**：
- 方案 A：Docker TCP/TLS（`DOCKER_HOST=tcp://host:2376`）
  - 复杂度低，Docker API 调用改为 TCP 连接
  - 但 docker-compose CLI 和文件访问需额外处理
- 方案 B：SSH 隧道（`DOCKER_HOST=ssh://user@host`）
  - Docker 原生支持，Go `golang.org/x/crypto/ssh` 库
  - 可远程执行 docker-compose、访问文件、exec 进容器
  - 复杂度中等
- Web 终端在远程模式下自动生效（Docker Exec API 不依赖本地）

---

## 验收标准

### Phase 1

| 验收项 | 方法 |
|--------|------|
| 无 PROJECT_NAME 的项目可用 | 用简单的 nginx + redis compose 测试 |
| 服务列表含未部署服务 | 声明服务均可见，含未部署的必选服务和 profile 服务 |
| 未部署必选服务可启动 | 删除某个必选服务容器后，点击启动 → 等价 `up -d <service>` |
| build 型服务标注 `[本地构建]` | 不显示升级按钮；未部署时点击启动返回 409 `services.start.build_not_supported` |
| 分类由 label 驱动 | 未打 `com.composeboard.category` 标签的服务全部归入 `other`，不出现基于服务名的分类 |
| Profile 两态展示 | fdfs 默认显示 `disabled`；启用后显示 `enabled`；不再出现 `partial` |
| Profile 整组启用/停用 | fdfs → 点击启用后两服务都进入启动轮询；点击停用后两服务都进入停止轮询 |
| 单服务实时状态接口 | 操作中的服务通过 `/api/services/:name/status` 更新行内状态与 loading 判定，不依赖全量列表缓存 |
| 已启用 Profile 的单服务操作 | 启用 Profile 后，组内服务与固定服务使用同一套单服务按钮和 loading 规则 |
| 镜像差异检测不依赖 *_VERSION | 修改任意变量名后仍可检测 |
| 状态文件基线 | 首启无 `.composeboard-state.json` 时不产生配置漂移告警；从旧 `.deployboard-state.json` 迁移无误报 |
| 向后兼容 zheshang 项目 | 23 个服务全部可见；base/backend/frontend/init 四组服务按 label 正确分组；3 个 profile（fdfs/rule/xiaoxin）状态识别正确；`${HOST_IP}`、`${*_VERSION}` 展开与旧版一致；镜像差异检测命中 ≥1 个服务即通过 |

### Phase 2

| 验收项 | 方法 |
|--------|------|
| 设置页展示项目信息 | 服务数、profiles、compose 版本正确 |
| 中英文切换完整可用 | 所有页面切换语言后无缺失 key、无中英混杂 |
| 日志断线重连 | 断开 WebSocket → 自动恢复 |
| .env 保存保留注释 | 编辑保存后注释和空行不丢失 |
| Web 终端可用 | 选择服务 → 打开终端 → 执行 `ls`、`cat` 等命令正常 |
| 终端交互正常 | vi/top 等全屏应用可用（TTY 模式） |
