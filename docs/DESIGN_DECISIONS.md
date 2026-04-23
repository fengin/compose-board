# ComposeBoard 设计决策记录

> **日期**: 2026-04-21
> **背景**: 产品文档 Review 后的决策确认

---

## 1. 产品边界声明

### 决策

ComposeBoard v1 明确定义以下产品边界：

| 维度 | 支持范围 | 不支持（后续开发） |
|------|---------|-------------------|
| **平台** | Linux 单机（主要）、Windows (Docker Desktop) | — |
| **Docker 连接** | 本地 Docker daemon（Linux Unix Socket / Windows Named Pipe） | 远程 Docker Host（后续通过 TCP/TLS 或 SSH 隧道） |
| **项目数量** | 单 Compose 项目 | 多项目管理 |
| **Compose 文件** | 单文件（自动发现优先级见下方） | 多 `-f` 组合、override 文件 |
| **服务副本** | 单副本（一服务一容器） | scale / deploy.replicas 多副本 |
| **镜像来源** | `image:` 预构建镜像 | `build:` 本地构建 |
| **Compose 版本** | version 2.x / 3.x / Compose Spec | — |

### Compose 文件自动发现

按以下优先级在 `project.dir` 中查找，使用第一个找到的文件：

1. `compose.yaml`（Compose Spec 推荐）
2. `compose.yml`
3. `docker-compose.yml`（传统）
4. `docker-compose.yaml`

> **依据**：与 `docker compose` CLI 的默认行为一致。

### 本地 Docker Transport 实现

为兼容 Linux 和 Windows Docker Desktop，v1 采用统一的本地 transport 抽象：

| 平台 | Transport | 地址 |
|------|-----------|------|
| Linux | Unix Socket | `/var/run/docker.sock` |
| Windows (Docker Desktop) | Named Pipe | `//./pipe/docker_engine` |

**实现原则**：
- `docker/client.go` 通过 `docker/transport.go` 建立连接，不在业务代码中写死平台路径
- 上层能力只依赖“本地 Docker daemon 可连接”，不关心底层是 Unix Socket 还是 Named Pipe
- 远程 Docker Host 仍留到 Phase 3，通过 TCP/TLS 或 SSH 单独设计

### 可扩展性预留

**多项目管理**：
- `config.yaml` 中 `project` 设计为单值，后续可扩展为数组
- 路由层预留 `/api/projects/:id/services` 的设计空间
- 但 v1 不实现，不增加架构复杂度

**多副本服务**：
- `ServiceView` 数据模型中 `ContainerID` 等字段设计为可选
- 后续可扩展为 `Containers []ContainerInfo` 数组
- 但 v1 接口语义保持「一服务一容器」

---

## 2. Profiles 交互策略

### 问题

"启用单个可选服务" 还是 "启用整个 Profile" ？

### 分析

Docker Compose 的 profiles 机制特点：
- Profile 是**分组机制**，同一 profile 下的服务通常有功能关联
- 例如 `fdfs` profile = fdfs-tracker + fdfs-storage，两者缺一不可
- 例如 `xiaoxin` profile = xiaoxin-server + xiaoxin-server-ui，前端依赖后端
- **不存在只启用 profile 中部分服务有意义的场景**

技术上可以 `docker-compose --profile fdfs up -d fdfs-tracker`（只启动一个），但实际上没有使用价值。

### 决策：Profile 级别操作

| 操作 | 粒度 | 命令 | UI 位置 |
|------|------|------|---------|
| 启用 | 整个 Profile | `docker-compose --profile <name> up -d` | Profile 分组标题的 `[启用]` 按钮 |
| 停用 | 整个 Profile | `stop + rm` 该 profile 下所有服务 | Profile 分组标题的 `[停用]` 按钮 |

**UI 修正**：
- ❌ ~~每个未部署服务行上有 `[启用]` 按钮~~ — 移除
- ✅ Profile 分组标题有 `[启用整组]` / `[停用整组]` — 保留
- 未部署的单个服务行上不放操作按钮，只显示状态 `⭕ 未部署` + 所属 profile 标签

**API 修正**：
- ❌ ~~`POST /api/services/:key/deploy`~~ — 移除单服务部署
- ✅ `POST /api/profiles/:name/enable` — 启用整个 profile
- ✅ `POST /api/profiles/:name/disable` — 停用整个 profile

**建模修正**：
- `optional` 不再作为 `Category` 取值
- 一个服务是否“可选”，只由 `Profiles` 是否为空决定
- UI 上必选服务继续按 `Category` 分组，可选服务单独按 profile 分组

### 补充：未部署服务的启动策略（2026-04-23 修订）

Profiles 决策只约束“可选服务是否允许单独启动”，不再承担运行态聚合判定。

统一策略：
- 已部署且已停止的服务：`POST /api/services/:name/start` → Docker Start
- 未部署且 `Profiles` 为空的必选服务：`POST /api/services/:name/start` → 等价执行 `docker compose up -d <service>`
- 未部署且有 `Profiles` 的可选服务：
  - 若所属 profile **未启用** → 返回 `services.start.profile_required`
  - 若所属 profile **已启用** → 允许单服务 `start/up`

### 补充：Profile 配置态裁决（D-1，2026-04-23 重定稿）

> **裁决时间**: 2026-04-23 | **状态**: 已确认

Profile 状态改为**配置启用态**，不再从服务运行状态反推，也不再存在 `partial` / `补齐启用`：

| 状态 | 含义 | UI 操作 |
|------|------|---------|
| `enabled` | 该 profile 当前配置为启用 | `[停用]` |
| `disabled` | 该 profile 当前配置为停用 | `[启用]` |

实现要求：
- 服务端在 `.composeboard-state.json` 中持久化 `profiles.<name>.enabled`
- `/api/profiles` 只返回配置态，不返回服务运行态聚合结果
- Profile 下服务行的具体按钮与状态，统一按**单服务实际状态**渲染

### 停用 Profile 时的依赖检查

**实际依赖方向**（基于当前 docker-compose.yml 分析）：

```
可选服务 → 依赖 → 必选服务
rule-engine     → mysql, emqx     (必选)
xiaoxin-server  → mysql           (必选)
fdfs-storage    → fdfs-tracker    (同 profile 内)

必选服务 → 不依赖 → 可选服务  ✅ 安全
```

**交互策略**：

| 场景 | 处理 |
|------|------|
| 停用 profile，无必选服务依赖它 | ✅ 直接允许，弹确认框即可 |
| 停用 profile，有其他 profile 服务依赖它 | ⚠️ 警告提示受影响的服务列表，用户确认后执行 |
| 停用 profile，有必选服务依赖它（理论上不应出现） | 🛑 阻止操作，提示无法停用 |

> v1 实现简化方案：**仅弹确认框**，不做复杂依赖检查。
> 理由：当前实际项目中，可选服务依赖必选服务（单向），停用可选服务不会影响必选服务。

> **注意**: Profile 三态语义已在上方「补充：Profile 三态语义裁决（D-1）」中统一定义，以 **running** 为判定标准。此处不再重复。

---

## 3. build: 型服务的处理

### 决策

ComposeBoard v1 **只覆盖 `image:` 型服务**，不支持 `build:` 本地构建。

**处理方式**：
- 解析 docker-compose.yml 时，如果服务只有 `build:` 没有 `image:`，标记 `imageSource: "build"`
- 服务列表中正常展示，但：
  - 不做镜像差异检测（无目标镜像可对比）
  - 不提供升级/拉取操作
  - 展示标签 `[本地构建]`，提示用户需通过 CLI 操作
- 如果服务同时有 `image:` 和 `build:`（Docker Compose 支持），**以 `image:` 为准**
  - `ImageSource = "registry"`
  - `DeclaredImage` / `ExpandedImage` 读取 `image:` 字段
  - `build:` 仅作为补充构建信息保留，不参与镜像差异检测和升级决策

### 在文档中的体现

API 返回的 `ServiceView` 增加字段：
```go
ImageSource string `json:"image_source"` // "registry" | "build" | "unknown"
```

### 补充：未部署 build 型服务的 start 行为

`POST /api/services/:name/start` 对"未部署且无 profile 的必选服务"的默认语义是 `docker compose up -d <service>`。但对 `ImageSource == "build"` 的服务，该命令会触发本地构建，与"v1 只支持 image: 型"约束冲突。

**统一策略**：
- `ImageSource == "build"` 且未部署 → API 返回 HTTP 409，错误码 `services.start.build_not_supported`，前端提示用户通过 CLI 自行构建。
- 已部署（含已停止）的 build 型服务不受限，Start/Stop/Restart 正常可用（调用 Docker Engine API，不触发构建）。
- Upgrade/Pull 接口对 build 型服务仍然拒绝（沿用原有约束）。

---

## 4. HOST_IP 功能的定位

### 问题

HOST_IP 检测与更新是你司项目的增强需求，而非通用 Compose 核心能力。放在核心设置中会模糊产品边界。

### 决策：作为可选扩展功能

将 HOST_IP 相关能力从「核心设置」降级为「扩展功能」：

**配置驱动**：
```yaml
# config.yaml
extensions:
  host_ip:
    enabled: false          # 默认关闭
    env_key: "HOST_IP"      # 要检测的 .env 变量名（可自定义）
    detect_on_startup: true # 启动时自动检测
```

- `enabled: false` — 开源用户默认看不到此功能
- `enabled: true` — 你司项目启用后，设置页显示 IP 检测区域、Dashboard 显示不一致警告
- `env_key` 可配置 — 不绑定 `HOST_IP` 这个变量名

**从产品规格中移除**：
- Dashboard 的 IP 警告 banner 改为条件渲染（仅 extensions.host_ip.enabled 时显示）
- 设置页的「主机配置」区块改为条件渲染

**实施优先级调整**：
- 从 Phase 2 移到 Phase 3（非开源核心）
- 或作为「你司定制扩展」在 Phase 1 完成后单独实现

---

## 5. 部署向导的实时输出接口

### 问题

部署向导需要"实时显示脚本输出和部署进度"，但 API 只有 REST 接口（check/start/status），无法推送实时流。

### 决策：复用 WebSocket 模式

已有的日志 WebSocket 是好的参考模式。部署向导增加 WebSocket 接口：

| 接口 | 类型 | 说明 |
|------|------|------|
| `GET /api/deploy/check` | REST | 环境检查（同步返回） |
| `POST /api/deploy/start` | REST | 启动部署（返回部署 ID） |
| `GET /api/deploy/:id/stream` | **WebSocket** | 实时推送部署日志和进度 |
| `GET /api/deploy/:id/status` | REST | 查询部署最终状态（WebSocket 断线后的兜底） |

**WebSocket 消息格式**：
```json
{
  "step": "pre_hook",       // pre_hook / pull / up / post_hook
  "type": "log",            // log / progress / error / complete
  "message": "Creating directory /opt/data/mysql...",
  "timestamp": "2026-04-21T15:00:00Z"
}
```

---

## 6. Web 终端

### 需求

用户需要在浏览器中进入容器执行任意 shell 命令，不仅限于查看日志，还包括排查问题、查看文件等。

### 决策：Docker Exec API + xterm.js

**不需要 SSH**。Docker Engine API 原生支持在容器中创建和执行命令：

```
浏览器 (xterm.js) ←→ WebSocket ←→ ComposeBoard 后端 ←→ Docker Exec API
```

| 环节 | 技术 |
|------|------|
| 前端终端 | xterm.js（vendor 引入） |
| 浏览器↔后端 | WebSocket |
| 后端↔Docker | `POST /containers/{id}/exec` + `POST /exec/{id}/start`（connection hijack） |
| 终端窗口同步 | `POST /exec/{id}/resize` |
| 默认 shell | 容器有 bash 用 bash，否则 `/bin/sh` |

**API**：`GET (WS) /api/services/:key/terminal`

**协议约定**：
- 前端 → 后端：
  - `{ "type": "input", "data": "pwd\n" }`
  - `{ "type": "resize", "cols": 120, "rows": 40 }`
  - `{ "type": "close" }`
- 后端 → 前端：
  - `{ "type": "output", "data": "/app\r\n" }`

`resize` 消息到达后，后端必须调用 Docker Exec Resize API，同步 TTY 尺寸，确保 `vi`、`top` 等全屏交互程序正常工作。

**优势**：未来支持远程 Docker Host 时，Web 终端自动生效（Docker Exec API 不依赖本地连接）。


### 补充：TTY 模式与数据流处理

Docker Exec 在**非 TTY** 模式下，stdout/stderr 经 8 字节帧头多路复用（需 `stdcopy.StdCopy` 拆分）；**TTY** 模式下是原始流。为支持 `vi`、`top` 等全屏交互程序，ComposeBoard 固定使用 TTY。

Exec 创建参数：
```json
{ "Tty": true, "AttachStdin": true, "AttachStdout": true, "AttachStderr": true, "Cmd": ["/bin/sh"] }
```

后端从 hijacked connection 读到的字节流**直接透传**为 WebSocket `output` 帧，无需区分 stdout/stderr 或做任何编解码。

容器内 shell 选择策略：先探测 `/bin/bash` 是否可执行（`docker exec -it <c> sh -c 'command -v bash'`），存在则用 bash，否则回退 `/bin/sh`。

---

## 7. 镜像仓库凭据

### 决策：ComposeBoard 不管理

| 场景 | 凭据管理方 |
|------|----------|
| docker-compose pull | Docker daemon 已存储的凭据（`docker login`） |
| 一键部署 | deploy.sh / pre-deploy 钩子中 `docker login` |
| ComposeBoard 升级拉取 | 调用 `docker-compose pull` → 透明使用 daemon 凭据 |

**结论**：Docker daemon 层透明处理，ComposeBoard 无需介入。如需首次登录，通过 pre-deploy 钩子脚本处理。

---

## 8. .env 编辑的数据模型

### 问题

`ReadEnvFile()` 返回 `map[string]string` 会丢失注释、空行和顺序。但产品要求保留注释+表格模式。

### 决策：EnvEntry 行级模型

```go
// EnvEntry .env 文件的单行表示
type EnvEntry struct {
    Type    string `json:"type"`              // "variable" | "comment" | "blank"
    Key     string `json:"key,omitempty"`     // 变量名（仅 type=variable）
    Value   string `json:"value,omitempty"`   // 变量值（仅 type=variable）
    Raw     string `json:"raw"`              // 原始行文本（保留格式）
    Line    int    `json:"line"`             // 行号
}

// compose/env.go 接口设计
func ParseEnvFile(path string) ([]EnvEntry, error)          // 行级解析
func ReadEnvVars(path string) (map[string]string, error)    // 纯变量 map（内部用）
func WriteEnvFile(path string, entries []EnvEntry) error    // 行级写回
func WriteEnvRaw(path string, content string) error         // 原始文本写回
func ExpandVars(template string, vars map[string]string) string
```

**两种编辑模式的 API 对应**：

| 模式 | 读取 | 写入 |
|------|------|------|
| 表格模式 | `GET /api/env` → `[]EnvEntry` | `PUT /api/env` → `{entries: []EnvEntry}` |
| 原始文本模式 | `GET /api/env?raw=true` → `{content: "..."}` | `PUT /api/env` → `{content: "..."}` |


### 补充：ExpandVars 的变量作用域

`ExpandVars(template, vars)` 的 `vars` **只来自项目目录下的 `.env` 文件**，不合并以下来源：

- ComposeBoard 进程的 shell 环境变量（避免运行环境污染镜像差异检测结果）
- `services.<name>.environment:` 字段的默认值（那是容器内环境变量，不是 Compose 变量）
- `env_file:` 字段、`--env-file` 参数、多份 `.env`（v1 单文件约束）

这与 Compose CLI 默认行为略有差异（CLI 会读 shell env），但保证了"同一 compose + 同一 .env → 同一镜像解析结果"的可重复性。如需让镜像依赖某变量，请写进 .env。

**职责边界**：变量**引用名提取**在 `compose/parser.go`（`DeclaredService.EnvRefs`），变量**值展开**在 `compose/env.go`（`ExpandVars`）。两个文件不重复扫描 `${...}`。

---

## 9. 修正后的 API 完整清单

### 认证
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/auth/login` | 登录 |

### 服务管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/services` | 服务列表（声明 LEFT JOIN 容器） |
| POST | `/api/services/:name/start` | 启动服务；对未部署且无 profile 的必选服务，等价 `docker compose up -d <service>` |
| POST | `/api/services/:name/stop` | 停止服务 |
| POST | `/api/services/:name/restart` | 重启服务 |
| GET | `/api/services/:name/env` | 服务环境变量 |
| GET | `/api/services/:name/status` | 服务实时状态 |

### 升级与重建（仅 image: 型）
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/services/:name/pull` | 拉取镜像 |
| GET | `/api/services/:name/pull` | 查询拉取状态 |
| POST | `/api/services/:name/upgrade` | 应用升级 |
| POST | `/api/services/:name/rebuild` | 重建容器 |

### Profiles 管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/profiles` | 列出所有 profiles 的配置启用态 |
| POST | `/api/profiles/:name/enable` | 启用整个 profile |
| POST | `/api/profiles/:name/disable` | 停用整个 profile |

### 配置管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/env` | 读取 .env（返回 EnvEntry 数组） |
| GET | `/api/env?raw=true` | 读取 .env 原始文本 |
| PUT | `/api/env` | 保存 .env（支持 entries 或 content） |

### 日志
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/services/:name/logs?tail=200` | 一次性获取历史日志 |
| GET (SSE) | `/api/services/:name/logs?follow=true` | SSE 实时日志流 |

### Web 终端
| 方法 | 路径 | 说明 |
|------|------|------|
| GET (WS) | `/api/services/:key/terminal` | WebSocket 交互式终端（exec 进容器，支持 resize） |

### 设置
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/settings/project` | 项目信息（只读） |
| GET | `/api/settings/compose` | Compose 命令信息 |
| PUT | `/api/settings/compose` | 更新 Compose 命令设置 |
| GET | `/api/settings/docker` | Docker 配置信息（只读） |

### 部署向导
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/deploy/check` | 环境检查 |
| POST | `/api/deploy/start` | 启动部署（返回部署 ID） |
| GET (WS) | `/api/deploy/:id/stream` | WebSocket 部署实时日志 |
| GET | `/api/deploy/:id/status` | 部署状态查询（兜底） |

### 系统 + 扩展（可选）
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/host/info` | 主机 + Docker 版本 |
| GET | `/api/extensions/host-ip/detect` | 检测主机 IP（需启用扩展） |
| POST | `/api/extensions/host-ip/update` | 更新 .env 中的 IP（需启用扩展） |

---

## 10. 多语言完整性

### 决策

- 首版即完整维护 `zh.json` 和 `en.json`
- 默认语言为中文
- 所有新增用户可见文案，提交时必须同步补齐中英文 key，不允许“中文完整、英文后补”
- 前端切换语言后不应出现缺失 key、中文/英文混杂或回退到硬编码文本


---

## 11. 服务分类：通过 Docker Label 标识

### 背景

DeployBoard 旧实现中 `categorizeService()` 基于服务名关键词（`mysql`/`redis`/`starter`/`xiaoxin-server` 等）硬编码分类。这违反"零适配"原则——一个通用开源产品不能内置使用者的业务语义。

### 决策：标签驱动

服务分类通过 Docker Compose 的 `labels` 字段声明：

```yaml
services:
  mysql:
    image: ...
    labels:
      com.composeboard.category: base
  starter-platform:
    image: ...
    labels:
      com.composeboard.category: backend
```

**标签规范**：

| Label Key | 取值 | 说明 |
|-----------|------|------|
| `com.composeboard.category` | `base` / `backend` / `frontend` / `init` / `other` | 服务分类；缺省视为 `other` |

**数据流**：

1. `compose/parser.go` 解析 YAML 时读取 `services.<name>.labels.com.composeboard.category`
2. `service/manager.go` 将该值填入 `ServiceView.Category`
3. 代码中不再保留任何形式的关键词匹配分类函数

**未打标签的项目**：

- 所有服务显示为 `other` 分类
- 前端继续按 `other` 分组正常展示，功能不受影响
- 在设置页 → 项目信息中提示用户可按需添加分类标签

**兼容现有项目**：从 DeployBoard 迁移过来的 compose 文件需要在服务上补齐一次分类标签。本仓库 `docker-compose/projects/*/docker-compose.yml` 已完成标签补齐。

---

## 12. 状态文件：首次启动视为基线

### 背景

`.deployboard-state.json` 更名为 `.composeboard-state.json`。如果 ComposeBoard 启动时既读不到新文件，也不处理旧文件，每个服务都会立刻被判定为"环境变量有变更、待重建"——误报。

### 决策：首次启动即基线，不迁移旧文件

1. ComposeBoard 启动时若项目目录下**不存在** `.composeboard-state.json`，立即以当前 `.env` 内容写入一份新文件，作为"基线"。
2. **不读取、不迁移、不删除** 旧的 `.deployboard-state.json`。ComposeBoard 对其视而不见，由用户自行清理。
3. 基线建立后，后续的"配置待重建"判定才开始生效。

**理由**：

- 迁移旧 state 文件语义风险高（旧 state 可能记录了已被删除的变量，或记录的时间点与当前 `.env` 不对应），与其承担错误判定的风险，不如让用户在升级点显式接受"当前状态即新基线"。
- 用户升级 ComposeBoard 的前提本就是"当前环境已稳定运行"，以此为基线是合理假设。

**UI 提示**：设置页 → 项目信息中展示"基线创建时间"（Baseline created at ...），方便用户确认。

---

## 13. WebSocket 鉴权

### 背景

所有 WebSocket 接口（日志、部署流、Web 终端）都需要鉴权。但浏览器 `new WebSocket()` 不支持自定义 HTTP header，常规 `Authorization: Bearer <jwt>` 方案不适用。

### 决策：Query String Token

统一通过 URL query 参数传递 JWT：

```
sse: /api/services/mysql/logs?follow=true&token=<jwt>
ws://host/api/services/mysql/terminal?token=<jwt>
ws://host/api/deploy/<id>/stream?token=<jwt>
```

**实现要点**：

1. 后端 WebSocket handler 在 `upgrader.Upgrade()` 前从 query `token` 取 JWT，校验通过才允许握手升级；失败返回 HTTP 401。
2. 前端 `api.js` 统一封装：
   ```js
   function wsURL(path) {
     const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
     const token = localStorage.getItem('composeboard_token');
     return `${scheme}://${location.host}${path}?token=${encodeURIComponent(token)}`;
   }
   ```
3. 服务端日志打印 URL 时必须屏蔽 `token` 参数（日志中间件在记录前做 query mask）。

**安全注意**：Query token 会出现在 Referer、反向代理 access log 中。在可控的单机运维场景（v1 范围）可接受；未来引入反向代理/远程部署时应升级为短时签名或 cookie 方案。

---

## 12. 服务操作与 Profile 启停状态机（实时单服务轮询）

> **日期**: 2026-04-23
> **背景**: services.js 拆分后，列表缓存轮询与 Profile 聚合判定持续引发 loading/状态/按钮不一致问题，决定回到“单服务实时状态为唯一运行态真相源”的模型。

### 目标

1. 批量列表接口继续保留，用于页面基线展示和 15 秒自动刷新。
2. 正在操作的服务，完成判定只能基于**单服务实时状态接口**。
3. 每次实时状态查询都必须**同步回写缓存**，保证自动刷新不会把刚操作完的服务刷回旧状态。
4. Profile 只表示“配置是否启用”，不再与服务运行态做聚合耦合。

### 真相源划分

| 数据 | 接口 | 用途 |
|------|------|------|
| 服务列表基线 | `GET /api/services` | 页面初始渲染、15 秒自动刷新、CPU/内存全量补齐 |
| 单服务实时状态 | `GET /api/services/:name/status` | 所有单服务操作的 loading 判定、实时行内更新 |
| Profile 配置态 | `GET /api/profiles` | Profile 头部“启用 / 停用”按钮与状态展示 |

### Profile 设计裁决

- `Profile.status` 仅有 `enabled / disabled`
- `enabled` 表示该 profile 当前配置为启用
- `disabled` 表示该 profile 当前配置为停用
- 服务端将其持久化到 `.composeboard-state.json > profiles`
- 不再存在 `partial / 补齐启用`

### 后端接口契约

#### 1. `GET /api/services/:name/status`

返回值沿用 `ServiceView` 结构的关键字段：

- `name`
- `container_id`
- `status`
- `state`
- `started_at`
- `running_image`
- `health`
- `startup_warning`
- `ports`
- `cpu`
- `mem_usage`
- `mem_limit`
- `mem_percent`
- `image_diff`
- `pending_env`

接口行为：
- 按 **service name** 直查 Docker，不走列表缓存
- 查到实时状态后立即同步回写 `ContainerCache`
- 若当前服务不存在容器，则同步把缓存中的该服务移除，列表层自然显示为 `not_deployed`

#### 1.1 `startup_warning` 运行态告警

`startup_warning` 是服务当前运行态的派生诊断结果，不参与 loading 完成判定，只用于列表告警展示。

判定规则：

- `running + health=unhealthy`
  - 直接判 `startup_warning = true`
- `created`
  - 若 `当前时间 - 容器创建时间 >= 30s`
  - 判 `startup_warning = true`
- `restarting`
  - 不直接使用累计 `RestartCount`，避免人工频繁重启时误报
  - 改为解析 Docker `state` 文本中的持续时长（如 `Restarting (1) 40 seconds ago`）
  - 若该持续时长 `>= 30s`
  - 判 `startup_warning = true`
- 其它状态
  - 默认 `startup_warning = false`

设计说明：

- 该字段不是“某次操作失败记忆”，而是**当前服务是否处于异常运行态**的统一判断
- 因此：
  - `GET /api/services` 的 15 秒自动刷新会带出该值
  - `GET /api/services/:name/status` 的单服务轮询也会带出该值
- 前端只显示通用告警“启动异常”，不在 v1 解释具体失败原因

#### 2. `GET /api/profiles`

仅返回配置启用态：

```json
[
  { "name": "monitoring", "status": "enabled", "enabled": true },
  { "name": "debug", "status": "disabled", "enabled": false }
]
```

#### 3. `POST /api/profiles/:name/enable|disable`

- `enable`：执行整组 `up`
- `disable`：执行整组 `stop + rm`
- API 成功后立即写入 profile 配置态
- 服务是否真的全部起来，由前端继续对组内服务逐个轮询 `GET /api/services/:name/status`

### 单服务操作规则

适用于固定服务与 Profile 下服务，统一处理：

- `start`
- `stop`
- `restart`
- `upgrade`
- `rebuild`

#### 流程

```text
用户确认操作
  → 行内进入 loading
  → 调用对应 POST API
  → 启动 3s/次轮询：GET /api/services/:name/status
    → 用返回结果即时更新这一行显示
    → 用同一份结果判断是否完成
```

#### 完成判据

| 操作 | 完成条件 |
|------|----------|
| `stop` | `status === 'exited'` 或 `status === 'not_deployed'` |
| `start` | `status === 'running'` |
| `restart` | `status === 'running'` 且 `started_at` 或 `container_id` 相比操作前已变化 |
| `upgrade` | 同 `restart`，且 `image_diff === false` |
| `rebuild` | 同 `restart`，且 `pending_env.length === 0` |

> `started_at` 是主判据；`container_id` 作为重建/升级时的补充锚点，避免容器替换后只看 `running` 误判成功。

#### 失败收敛

- `start`
  - 连续 3 次拿到 `exited / restarting`
  - 且还没有出现新的 `started_at / container_id`
  - 判定启动失败，清 loading
- `restart / upgrade / rebuild`
  - 一旦出现新的 `started_at / container_id`
  - 后续连续 3 次仍为 `exited / restarting`
  - 判定本次新实例启动失败，清 loading
- 统一超时：
  - 普通操作 2 分钟
  - `upgrade` 5 分钟

#### 服务行按钮显示矩阵

> 原则：`loading` 优先级最高。只要该行处于 loading，中间状态只显示 spinner，不显示按钮；loading 结束后再按下面矩阵渲染。

| 当前状态 | 显示按钮 |
|------|----------|
| `running` | `重启`、`停止`、`查看环境变量`、`查看日志`；若 `image_diff=true` 追加 `升级`；若 `pending_env.length>0 && !image_diff` 追加 `重建` |
| `exited` | `启动`、`查看环境变量`、`查看日志`；若 `image_diff=true` 追加 `升级`；若 `pending_env.length>0 && !image_diff` 追加 `重建` |
| `created` | `启动`、`重启`、`查看环境变量`、`查看日志`；若 `image_diff=true` 追加 `升级`；若 `pending_env.length>0 && !image_diff` 追加 `重建` |
| `restarting` | `重启`、`停止`、`查看环境变量`、`查看日志`；若 `image_diff=true` 追加 `升级`；若 `pending_env.length>0 && !image_diff` 追加 `重建` |
| `not_deployed` | 固定服务显示 `启动`；已启用 Profile 下的可选服务显示 `启动`；未启用 Profile 下的可选服务不显示单服务启动按钮 |

设计说明：

- `startup_warning` 只影响状态列告警展示，不参与按钮隐藏
- `created / restarting` 视为“已部署但异常”的运行态，需继续暴露恢复与排查入口
- `created` 不显示 `停止`：该状态下容器本就未进入运行态，Docker `stop` 不会把它转换成 `exited`，继续显示 `停止` 只会制造“操作无效”的误解
- `查看日志` 对所有 `status != not_deployed` 的服务都应可见，便于排查异常
- `升级 / 重建` 继续只由 `image_diff / pending_env` 决定，不额外受异常态抑制

### Profile 启用 / 停用规则

Profile 本身不再拥有独立的运行态判据，组内操作也统一回到“逐服务轮询”：

#### `enable`

```text
点击启用
  → 头部 loading + 组内服务行 loading
  → await POST /api/profiles/:name/enable
  → 本地切换 profile.status = enabled
  → 轮询组内每个服务的 GET /api/services/:name/status
  → 全部 running 才结束 loading
```

#### `disable`

```text
点击停用
  → 头部 loading + 组内服务行 loading
  → await POST /api/profiles/:name/disable
  → 本地切换 profile.status = disabled
  → 轮询组内每个服务的 GET /api/services/:name/status
  → 全部 not_deployed 才结束 loading
```

#### 关键原则

- Profile 头部状态来自 `/api/profiles`
- 服务行状态来自 `/api/services` / `/api/services/:name/status`
- Profile 的 loading 完成判定只看**组内所有服务的目标状态是否达成**
- 不再引入 `partial / 补齐启用` 这种运行态聚合中间态

### 自动刷新与操作轮询的职责边界

- `15s` 自动刷新保留，只做页面基线刷新
- 正在操作的服务，其 loading 结束与否**完全不依赖**自动刷新
- 自动刷新拿到的数据若比实时轮询旧，也不会影响正在操作服务的完成判定
- 因为实时接口每次都会回写缓存，所以自动刷新最终会和实时轮询收敛到同一状态
- `startup_warning` 由列表基线与实时接口统一派生，因此既能覆盖当前操作中的异常态，也能在重新打开页面时继续反映当前不正常服务

### 代码边界

- `service/manager.go`
  - `ListServices()`：批量列表
  - `GetRealtimeServiceStatus()`：单服务实时状态 + 缓存回写
- `docker/cache.go`
  - `SyncService()` / `RemoveService()`：服务级缓存同步
- `service/profiles.go`
  - 管理 Profile 配置态，不再聚合运行态
- `web/js/pages/services.js`
  - 页面编排
  - 单服务 / Profile 轮询状态机
- `web/js/pages/services-rules.js`
  - 按钮规则与完成判据
- `web/js/pages/services-ops.js`
  - 排序、Profile 配置态映射、loading 判定
