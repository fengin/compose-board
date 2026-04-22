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

### 补充：未部署必选服务的启动策略

Profiles 决策只约束“可选服务”的启用方式，不影响必选服务的恢复启动。

统一策略：
- 已部署且已停止的服务：`POST /api/services/:key/start` → Docker Start
- 未部署且 `Profiles` 为空的必选服务：`POST /api/services/:key/start` → 等价执行 `docker compose up -d <service>`
- 未部署且有 `Profiles` 的可选服务：仍通过 `POST /api/profiles/:name/enable` 启用整个 profile，不提供单服务 deploy API

### 补充：Profile 三态语义裁决（D-1）

> **裁决时间**: 2026-04-21 | **状态**: 已确认

三态判定以**运行状态**为准（而非部署状态），因为用户关心的是"这组服务能不能用"：

| 状态 | 条件 | UI 操作 |
|------|------|---------|
| `enabled` | Profile 下**全部**服务处于 `running` | `[全部停用]` |
| `partial` | 至少 1 个已部署，但未全部 running（含 stopped/exited） | `[补齐启用]` + `[全部停用]` |
| `disabled` | Profile 下**全部**服务未部署 | `[启用]` |

individual 服务的操作按钮基于各自 `Status` 独立渲染（running → 停止/重启，exited → 启动，not_deployed → 无按钮）。

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

`POST /api/services/:key/start` 对"未部署且无 profile 的必选服务"的默认语义是 `docker compose up -d <service>`。但对 `ImageSource == "build"` 的服务，该命令会触发本地构建，与"v1 只支持 image: 型"约束冲突。

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
| POST | `/api/services/:key/start` | 启动服务；对未部署且无 profile 的必选服务，等价 `docker compose up -d <service>` |
| POST | `/api/services/:key/stop` | 停止服务 |
| POST | `/api/services/:key/restart` | 重启服务 |
| GET | `/api/services/:key/env` | 服务环境变量 |
| GET | `/api/services/:key/status` | 服务实时状态 |

### 升级与重建（仅 image: 型）
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/services/:key/pull` | 拉取镜像 |
| GET | `/api/services/:key/pull` | 查询拉取状态 |
| POST | `/api/services/:key/upgrade` | 应用升级 |
| POST | `/api/services/:key/rebuild` | 重建容器 |

### Profiles 管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/profiles` | 列出所有 profiles 及其服务和状态 |
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

## 12. 服务操作轮询机制（started_at 时间戳判定）

> **日期**: 2026-04-22
> **背景**: 从旧版 DeployBoard 迁移操作区 UX 设计，修复 loading 状态不一致问题

### 问题

服务操作（stop/start/restart/upgrade/rebuild）完成后，前端需要知道"操作是否已生效"。Docker Compose CLI 的 stop/start/restart 是同步命令（返回即完成），但 upgrade/rebuild 是异步的（后端 fire-and-forget）。

旧版存在两类 Bug：
1. **stop/start/restart 没有 loading 态**：`await API` 阻塞期间操作区按钮可重复点击
2. **upgrade/rebuild loading 被覆盖**：10 秒定时 `fetchServices` 会把 `_loading` 全部重置为 `false`

### 决策

**所有 5 种操作统一走异步轮询模式**，共享一个 `startAsyncPoll()` 方法。

#### 统一流程

```
用户点击确认 → await API（同步等待命令发出）
  → 成功：关闭弹窗 → svc._loading = true → startAsyncPoll()
  → 失败：弹窗内显示错误
```

#### 完成判定表（核心设计）

| 操作 | 完成条件 | 说明 |
|------|---------|------|
| `stop` | `status === 'exited'` | 简单状态判定 |
| `start` | `status === 'running'` | 简单状态判定 |
| `restart` | `status === 'running' && started_at !== startedBefore` | **时间戳变化判定** |
| `upgrade` | 同 restart + `!image_diff` | 时间戳 + 镜像差异消失 |
| `rebuild` | 同 restart + `pending_env.length === 0` | 时间戳 + 环境变更清空 |

> **为什么 restart 必须用 `started_at`**：
> - `docker compose restart` 执行很快（2-3 秒），轮询间隔（3 秒）可能**完全错过"非 running"的瞬间**
> - 单靠 `status === 'running'` 会在**第一次轮询就命中**（容器本来就在 running），立即误判为完成
> - `started_at` 是 Docker 引擎写入的**权威时间戳**，容器重启后必定变化，不受轮询频率影响

#### `_loading` 状态保护

`fetchServices()` 全量刷新时保留正在操作中的 `_loading` 状态：

```js
const loadingSet = new Set(
    this.services.filter(s => s._loading).map(s => s.name)
);
this.services = (services || []).map(s => ({
    ...s, _loading: loadingSet.has(s.name)
}));
```

#### 异常状态快速失败（C-3）

upgrade/rebuild 过程中容器**必然短暂 exited**（旧容器停掉 → 新容器拉起）。为避免误判：

- 仅当 `started_at === startedBefore`（容器还没开始重建）时才计入失败计数
- 连续 3 次判定为异常才触发快速失败

#### 轮询中实时更新

每次轮询都用 `Object.assign()` 同步更新该服务的运行态数据（status/state/cpu/mem/image 等），让用户在 loading 期间也能看到实时变化。

### 新旧版本差异

| 维度 | 旧版 (DeployBoard) | 新版 (ComposeBoard) |
|------|-------------------|-------------------|
| 缓存对象 | 容器列表 `containers[]` | 服务列表 `services[]` |
| 轮询 API | `getContainerStatus(id)` 单容器 | `getServices()` 全量 |
| ID 变化处理 | 升级/重建后容器 ID 变，需 `byName` 回退 | 服务名不变，天然按 name 匹配 |
| `started_at` 来源 | `ContainerStatus.started_at` | `ServiceView.started_at`（从 `ContainerInfo` 透传）|
| 获取方式 | 操作前单独调 `getContainerStatus` | 直接从 `svc.started_at` 取（已在列表数据中）|

### 数据流

```
Docker API
  └─ /containers/{id}/json (inspect)
      └─ State.StartedAt → ContainerInfo.StartedAt
          └─ ServiceView.StartedAt (LEFT JOIN 透传)
              └─ 前端 svc.started_at
```

`ListContainers` 中运行中容器复用已有的 `inspectContainer` 调用（用于 health check），零额外开销获取 `StartedAt`。非运行容器单独调 `getStartedAt`。

---

## 13. Profile 状态变化设计（await API + 轮询校验 + 双源一致性）

> **日期**: 2026-04-22（初版）→ 2026-04-22 修订（await 范式）
> **背景**: Profile 启用/停用的 UX 对齐与时序竞态修复

### 问题

Profile 的 `enable` / `disable` 最初走 `await API + await fetchServices` 同步阻塞模式，存在三个与 §12 单服务操作不一致的行为：

1. **弹窗阻塞**：点击"确认停用"后弹窗持续显示，直到 stop+rm 全部完成（5~10s）才关闭
2. **下属服务无反馈**：用户只看到 Profile 头部 spinner，不知道哪个服务正在操作
3. **定时刷新覆盖**：10s 定时 `fetchServices` 会重建 `this.profiles` 对象树，服务行 `_loading` 字段被新对象覆盖

### 决策

**Profile 操作采用"await API 完成 → 再轮询校验"的两阶段范式**，与 §12 单服务操作共享双源一致性和 `_loading` 保护基础。

> **为什么不走 §12 的 fire-and-forget 模式**：详见本节末「#### 事故记录：为什么从 fire-and-forget 回退为 await」

#### 统一流程

```
用户点击确认
  → 立即关弹窗 + profileLoading=true + 批量下属服务 _loading=true + Toast "enabling/disabling"
  → await API.enableProfile / disableProfile（backend 完成 Up/Stop+Rm 并 ForceRefresh cache）
    │
    ├─ 失败：清 loading + Toast.error + return
    │
    └─ 成功：startProfilePoll() 轮询 GET /api/profiles 校验目标态
        → 命中：清 loading + Toast.success + fetchServices
        → 超时（3 min）：清 loading + Toast.error
        → enable 连续 3 次检测到 allCreated && hasUnhealthy：Toast.error "operation_failed"
```

#### 目标状态判定（双重判据）

聚合态 `profile.status` 单独不够可靠，必须叠加"逐个服务状态一致"才算完成：

| 操作 | 完成判据 | 说明 |
|------|---------|------|
| `enable` | `profile.status === 'enabled'` **且** 所有下属服务 `status === 'running'` | 排除聚合态已切换但个别服务仍在 restarting/starting 的瞬态 |
| `disable` | `profile.status === 'disabled'` **且** 所有下属服务 `status === 'not_deployed'` | 排除 rm 命令返回但容器还没完全从 docker list 里消失的瞬态 |

**enable 的失败快速判定**：`allCreated && hasUnhealthy`（所有服务都已创建 + 至少一个 restarting/exited）连续 3 次成立 → 判定启动失败，无需等 3 分钟超时。

```js
const allRunning = services.length > 0 && services.every(s => s.status === 'running');
const allRemoved = services.length > 0 && services.every(s => s.status === 'not_deployed');
const allCreated = services.length > 0 && services.every(s => s.status !== 'not_deployed');
const hasUnhealthy = services.some(s => s.status === 'restarting' || s.status === 'exited');
```

`services.length > 0` 作为前置保护：极端情况下后端返回空 services，宁可等超时也不能误 finish。

#### 双源 `_loading` 一致性

Profile 下的服务**同时存在于** `this.services[]` 和 `this.profiles[name].services[]` 两棵树中。任何 _loading 或数据更新必须对两处同步，否则会出现"主列表已转圈但 Profile 行不转"或反之的状态撕裂。

通过两个辅助方法集中处理：

```js
// 按服务名同步更新两个源里的字段
updateServiceRefs(name, fields) {
    const target = this.services.find(s => s.name === name);
    if (target) Object.assign(target, fields);
    for (const pname of Object.keys(this.profiles || {})) {
        const t = (this.profiles[pname].services || []).find(s => s.name === name);
        if (t) Object.assign(t, fields);
    }
}

// 批量设置某 profile 下所有服务的 _loading
setProfileServicesLoading(profileName, loading) {
    for (const s of this.profiles[profileName].services) {
        this.updateServiceRefs(s.name, { _loading: loading });
    }
}
```

`executeAction` / `startAsyncPoll` / `doEnableProfile` / `doDisableProfile` / `startProfilePoll` 全部统一走 `updateServiceRefs`，不再直接 `find + Object.assign` 主列表。

#### `fetchServices` 的 _loading 保护扩展

§12 的保护逻辑只覆盖 `this.services`，在 Profile 场景下需要扩展为双源扫描：

```js
const loadingNames = new Set();
this.services.forEach(s => { if (s._loading) loadingNames.add(s.name); });
for (const pname of Object.keys(this.profiles || {})) {
    (this.profiles[pname].services || []).forEach(s => {
        if (s._loading) loadingNames.add(s.name);
    });
}
// 新 services + 新 profiles.services 均按 loadingNames 重建 _loading
```

#### 轮询参数

| 参数 | 单服务 (§12) | Profile |
|------|------|------|
| 首次延迟 | 1s | **300ms** |
| 轮询间隔 | 3s | 3s |
| 超时 | 2 min（upgrade 5 min） | 3 min |
| 轮询接口 | `GET /api/services` | `GET /api/profiles` |
| API 调用方式 | fire-and-forget（§12 保留历史设计） | **await**（见下文事故记录） |

Profile 首次延迟 300ms 是因为 `await API.enableProfile` 返回时 backend 已完成 `ForceRefresh cache`，无需再等 1s 对齐。Profile 超时 3 min 比单服务长，因为一次 `enable` 可能触发多个容器的串行 `up -d`（含镜像拉取）。`GET /api/profiles` 返回的 `ProfileInfo.Services` 已包含完整 `ServiceView`（运行态指标齐全），无需再并发调用 `/api/services`。

#### 事故记录：为什么从 fire-and-forget 回退为 await

> **触发场景**：停用 Profile 完成后 1~3 秒内立即点启用，loading 状态瞬间消失，按钮恢复为"启用"，但 Toast 仍显示"正在启用中"——与用户预期的"loading 持续到启动完成"严重不符。

**初版设计（fire-and-forget）**：

```js
// 旧版：立即开轮询，不等 API 完成
async doEnableProfile(name) {
    this.profileLoading[name] = true;
    this.setProfileServicesLoading(name, true);
    API.enableProfile(name).catch(e => { /* 清 loading */ });
    this.startProfilePoll(name, 'enable');  // ← T=0 就开始轮询
}
```

**根因**：`GET /api/profiles` 是只读、非阻塞的。当 backend 的 `executor.Up` 还在执行中间阶段（容器刚创建还没 running、或 `cache.ForceRefresh` 还没触发），轮询读到的是**上一次 disable 完成的残留快照**或**启动中间瞬态**。旧版判据过于宽松——仅 `freshProfile.status === 'enabled'` 就 finish——只要 Docker daemon 在某一瞬间让所有容器都处于 running（哪怕下一秒就会因为健康检查失败而 exit），轮询就误判完成，清 loading 还原按钮。紧接着下一次 `fetchServices` 把 status 拉回 `partial`，用户看到的就是"loading 闪一下就消失、按钮回到启用"。

**修复方案（await + 双重判据 + 失败抗抖）**：

1. **`await API.enableProfile / disableProfile`**：确保 backend 的 `Up/Stop+Rm` 命令 **和** 随后的 `cache.ForceRefresh` 都已完成，再开始轮询。彻底消除"轮询早于 backend 写入 cache"的时序窗口。
2. **双重判据**：`profile.status` 聚合态必须匹配，**且** 所有下属服务逐个满足目标单态（`running` / `not_deployed`）。排除聚合瞬态误命中。
3. **失败抗抖**：enable 场景下 `allCreated && hasUnhealthy` 连续 3 次才算失败，避免启动过程中短暂的 `restarting` 被误判。
4. **首次延迟 300ms**：await 已对齐 backend 状态，无需再等 1s。

**吸取的教训**：

- 异步轮询的"完成判据"必须覆盖**聚合态 + 个体态**，单看其中之一都会被中间瞬态欺骗
- `fire-and-forget + 轮询`适合那种**可观测性强、无中间瞬态**的操作（单服务 start/stop 的 `started_at` 变化就是不可逆锚点）；多容器串行编排的操作中间瞬态复杂，必须 await 锚定 cache 写入时点
- 设计对称不等于代码对称。§12 和 §13 共享 UX 模式、双源保护、轮询框架，但 API 调用时机因后端契约不同而有差异，这是合理的"具体问题具体分析"

### 与 §12 的关系

§13 不替换 §12，而是**以 §12 为基础扩展到群组场景**：

| 维度 | §12 单服务操作 | §13 Profile 操作 |
|------|------|------|
| 操作发起点 | 主列表服务行 or Profile 服务行 | Profile 分组头部 |
| Loading 粒度 | 单服务 `svc._loading` | Profile 头部 `profileLoading[name]` + 批量 `svc._loading` |
| API 调用方式 | fire-and-forget | **await**（锚定 backend cache 刷新时点） |
| 目标状态 | `started_at` 变化或 `status` 切换 | `profile.status` 聚合态 + 全部下属服务个体态 |
| 失败抗抖 | 单服务 status 异常即可判定 | enable 需 `allCreated && hasUnhealthy` 连续 3 次 |
| 数据同步函数 | `updateServiceRefs`（双源） | `updateServiceRefs` + `setProfileServicesLoading` |
| 共享基础 | `fetchServices` 双源 _loading 保护、轮询节奏、Toast 反馈风格 | 完全复用 |

