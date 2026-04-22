# ComposeBoard 产品功能规格书

> **版本**: v2.1  
> **日期**: 2026-04-21  
> **定位**: Docker Compose 可视化运维面板 — 开源产品

---

## 1. 产品定位

ComposeBoard 是一个 **Docker Compose 可视化运维管理面板**，面向使用 docker-compose 管理容器化应用的开发者和运维人员。

**核心价值**：将 docker-compose 的命令行操作转化为可视化界面，以 Compose 服务声明为核心视图，提供服务全生命周期管理。

**与同类产品的差异**：
- **Portainer** — 通用 Docker UI，太重，不聚焦 Compose
- **Dockge** — Compose 管理，但侧重 stack 编辑，弱化运行时管理
- **ComposeBoard** — 以 Compose 服务为核心视图，支持 Profiles 管理、变更检测、生命周期钩子

### 1.1 产品边界（v1 明确范围）

| 维度 | ✅ 支持范围 | ❌ 不支持（后续开发） |
|------|------------|---------------------|
| **平台** | Linux 单机（主要）、Windows (Docker Desktop) | — |
| **Docker 连接** | 本地 Docker daemon（Linux Unix Socket / Windows Named Pipe） | 远程 Docker Host（后续通过 Docker TCP/TLS 或 SSH 隧道支持） |
| **项目数量** | 单 Compose 项目 | 多项目管理 |
| **Compose 文件** | 单文件（自动发现） | 多 `-f` 组合、override 文件 |
| **服务副本** | 一服务一容器（单副本） | scale / deploy.replicas |
| **镜像来源** | `image:` 预构建镜像（全功能）、`build:` 本地构建（仅展示 + 已部署容器的启停重启） | `build:` 型服务的未部署启动、镜像升级、pull |
| **Compose 版本** | version 2.x / 3.x / Compose Spec | — |

**Compose 文件自动发现**（按优先级）：
1. `compose.yaml` → 2. `compose.yml` → 3. `docker-compose.yml` → 4. `docker-compose.yaml`

> 与 `docker compose` CLI 默认行为一致。

**可扩展性预留**：
- `config.yaml` 中 `project` 为单值，后续可扩展为数组（多项目）
- `ServiceView` 中容器信息为可选字段，后续可扩展为数组（多副本）
- 但 v1 不实现，不增加架构复杂度

---

## 2. 技术栈

| 层级 | 技术 | 说明 |
|------|------|------|
| 后端 | Go + Gin | REST API + WebSocket + 静态文件服务 |
| 前端 | Vue.js 3 + Vue Router 4 | SPA，无构建步骤，本地 vendored |
| Docker 交互 | 本地 Docker API Transport | Linux 走 Unix Socket，Windows Docker Desktop 走 Named Pipe，均不依赖 Docker SDK |
| Compose 交互 | CLI 调用 | 自动检测 docker-compose (v1) / docker compose (v2) |
| 认证 | JWT | 基于配置文件的账密 |
| 多语言 | 自建轻量 i18n | 默认中文，中/英 locale 完整维护，支持切换 |
| 配置 | YAML | config.yaml 单文件 |
| 分发 | 单文件二进制 | go:embed 嵌入前端资源 |

---

## 3. 功能模块

### 3.1 用户认证

| 功能 | 优先级 | 状态 | 说明 |
|------|--------|------|------|
| 账号密码登录 | — | ✅ 已有 | 从 DeployBoard 迁移 |
| JWT Token | — | ✅ 已有 | 登录后颁发 Token |
| 自动跳转 | — | ✅ 已有 | Token 失效自动跳转登录页 |

### 3.2 Dashboard（系统概览）

| 功能 | 优先级 | 状态 | 说明 |
|------|--------|------|------|
| 项目信息展示 | 🔴 高 | 🆕 新增 | 项目名称、目录、Compose 版本、服务统计（来源：`GET /api/settings/project`） |
| 主机信息 | — | ✅ 已有 | OS、Docker 版本、API 版本（来源：`GET /api/host/info`） |
| 资源指标卡 | — | ✅ 已有 | CPU、内存、磁盘数值卡片 |
| 服务状态统计 | — | ✅ 已有 | 运行/停止/未部署 数量（前端对 `GET /api/services` 聚合） |
| 服务分组卡片 | — | ✅ 已有 | 按 `com.composeboard.category` label 分组展示服务状态 |

> Dashboard 不提供独立聚合接口，由前端并发调用现有 REST API 组装，详见 ARCHITECTURE §4.11。

### 3.3 服务管理（核心页面 — 重构）

> **核心变化**：从「只展示已有容器」升级为「以 Compose 声明为主视图」

#### 3.3.1 服务列表

| 功能 | 优先级 | 状态 | 说明 |
|------|--------|------|------|
| 服务声明视图 | 🔴 高 | 🆕 重构 | 展示 Compose YAML 中所有声明的服务，包括未部署的 |
| 三种状态 | 🔴 高 | 🆕 新增 | 运行中 ✅ / 已停止 ⏹ / 未部署 ⭕ |
| Profiles 标识 | 🔴 高 | 🆕 新增 | 可选服务标注所属 profile |
| 镜像差异检测 | 🔴 高 | 🆕 重构 | 对比 YAML 声明镜像 vs 运行镜像（仅 `image:` 型） |
| build 服务标识 | 🔴 高 | 🆕 新增 | `build:` 型服务标注 `[本地构建]`，不做差异检测 |
| 环境变量差异检测 | — | ✅ 已有 | .env 变更提示重建/升级 |
| 端口映射 | — | ✅ 已有 | 显示主机端口→容器端口 |
| 资源监控 | — | ✅ 已有 | CPU/内存用量（仅运行中容器） |
| 分类标签 | 🔴 高 | 🆕 重构 | 读取 Compose label `com.composeboard.category`（合法值 `base` / `backend` / `frontend` / `init` / `other`），**不再基于服务名硬编码**；未打标签的服务归入 `other` 兜底分组；可选服务通过 profile 分组表达，不再作为 category |

#### 3.3.2 服务操作

| 操作 | 适用状态 | 说明 |
|------|---------|------|
| 启动 | 已停止 / 未部署（仅必选服务） | ✅ 已有；对未部署且无 profile 的必选服务，等价 `docker-compose up -d <service>` 创建并启动；**`build:` 型未部署服务**点击启动返回 409 `services.start.build_not_supported`，需使用部署向导或 `docker compose up` |
| 停止 | 运行中 | ✅ 已有，对 `image:` / `build:` 型服务均适用 |
| 重启 | 运行中 | ✅ 已有，对 `image:` / `build:` 型服务均适用 |
| 重建 | 有变更（仅 image: 型） | ✅ 已有，force-recreate + --no-deps |
| 升级 | 镜像不同（仅 image: 型） | ✅ 已有，两阶段（pull → apply） |
| 查看环境变量 | 所有已部署 | ✅ 已有 |

#### 3.3.3 Profiles 管理

> **操作粒度**：Profile 级别（整组启用/停用），不支持单个可选服务独立操作。
> **原因**：同一 profile 下的服务通常有功能关联（如 fdfs-tracker + fdfs-storage），单独启用无意义。

| 功能 | 优先级 | 说明 |
|------|--------|------|
| Profile 分组展示 | 🔴 高 | 可选服务按 profile 分组，标注三态状态（`enabled` / `partial` / `disabled`） |
| 启用 Profile | 🔴 高 | 整组启用：`docker-compose --profile <name> up -d` |
| 停用 Profile | 🔴 高 | 整组停用：stop + rm 该 profile 下所有服务 |
| 停用确认 | 🔴 高 | 弹出确认框，列出将被停止的服务 |
| `partial` 态处理 | 🔴 高 | 同时展示 `[补齐启用]` 与 `[全部停用]`：补齐执行 `up -d --profile <name>` 幂等拉齐，停用执行整组 stop+rm |

> **未部署的可选服务**（有 profile）不显示单服务操作按钮，只显示 `⭕ 未部署` 状态 + profile 标签。
> 操作入口在 profile 分组标题的 `[启用]` / `[停用]` 按钮。

### 3.4 环境配置（.env 编辑）

| 功能 | 优先级 | 状态 | 说明 |
|------|--------|------|------|
| 表格模式查看 | — | ✅ 已有 | KEY = VALUE 表格（基于行级 EnvEntry 模型） |
| 原始文本编辑 | — | ✅ 已有 | 切换为文本框编辑 |
| 保存配置 | — | ✅ 已有 | 保存后自动检测变更 |
| 注释保留 | — | ✅ 已有 | 行级模型保留注释、空行、顺序 |

### 3.5 日志查看

| 功能 | 优先级 | 状态 | 说明 |
|------|--------|------|------|
| 实时日志流 | — | ✅ 已有 | WebSocket 推送 |
| 历史日志 | — | ✅ 已有 | 最近 N 行 |
| 服务选择 | — | ✅ 已有 | 下拉选择服务 |
| 日志级别着色 | — | ✅ 已有 | ERROR/WARN/INFO/DEBUG |
| **断线自动重连** | 🟡 中 | 🆕 新增 | 指数退避重连 |
| **连接状态提示** | 🟡 中 | 🆕 新增 | 断线时 banner 提示 |

### 3.6 Web 终端（全新）

> 在浏览器中 exec 进入容器执行任意命令。基于 Docker Exec API，不需要 SSH。

| 功能 | 优先级 | 说明 |
|------|--------|------|
| 容器终端 | 🟡 中 | 选择服务 → 打开 Web 终端（xterm.js） → 执行任意 shell 命令 |
| 交互式会话 | 🟡 中 | WebSocket 双向代理 Docker Exec stdin/stdout，**TTY 固定开启** |
| 终端窗口自适应 | 🟡 中 | 浏览器 resize 事件同步到 Docker Exec TTY，保障 `vi` / `top` 等全屏应用可用 |
| 多 Tab 支持 | 🟢 低 | 同时打开多个容器终端 |

**技术方案**：
- 前端：xterm.js 5.x（终端模拟器，vendor 引入，具体版本见 IMPLEMENTATION_PLAN §2.4）
- 后端：`POST /containers/{id}/exec` + `POST /exec/{id}/start` → WebSocket 代理（`Tty: true` 固定开启）
- 协议：WebSocket 消息区分 `input` / `resize` / `close`，`resize` 时同步 cols/rows 到 Docker Exec TTY
- 鉴权：WebSocket 握手时通过 URL `?token=<jwt>` 传递 Token
- 默认命令：`/bin/sh`（容器内有 bash 则用 bash）

### 3.7 设置页（全新）

**通用设置**：

| 功能 | 优先级 | 说明 |
|------|--------|------|
| 项目信息展示 | 🔴 高 | 项目名/目录/Compose 文件/版本/服务统计/Profiles |
| Compose 命令设置 | 🔴 高 | docker-compose(v1) / docker compose(v2) / 自动检测 |
| Docker 信息展示 | 🟡 中 | Docker 版本、insecure-registry 列表（只读） |
| 生命周期钩子 | 🟢 低 | pre_deploy / post_deploy 脚本路径 |

**主机设置**（可选，通过 config.yaml `extensions` 启用，默认关闭）：

> 属于特定场景的增强需求，非通用 Compose 核心能力。

| 功能 | 优先级 | 说明 |
|------|--------|------|
| HOST_IP 自适应 | 🟢 低 | 检测主机 IP vs .env 配置 IP，不一致时警告 + 一键更新 |
| 开机自启动 | 🟢 低 | systemd service 状态展示 + 启停 |

> **关于镜像仓库凭据**：Docker daemon 层通过 `docker login` 管理凭据，`docker-compose pull` 自动使用已存储的凭据，ComposeBoard 无需介入。如需首次登录，通过 pre-deploy 钩子脚本处理。

### 3.8 部署向导（全新）

| 功能 | 优先级 | 说明 |
|------|--------|------|
| 环境检查 | 🟡 中 | Docker/Compose 是否可用、.env 完整性 |
| 服务选择 | 🟡 中 | 勾选要部署的 profiles |
| Pre-Deploy 钩子 | 🟡 中 | 执行外部脚本，WebSocket 实时显示输出 |
| 部署执行 | 🟡 中 | docker-compose pull + up -d，WebSocket 实时进度 |
| Post-Deploy 钩子 | 🟡 中 | 执行外部脚本 |
| 结果展示 | 🟡 中 | 服务启动状态汇总 |

---

## 4. 页面导航

```
登录页 → Dashboard (概览)
          ├── 服务管理 (核心)
          ├── 环境配置 (.env)
          ├── 日志查看
          ├── Web 终端
          ├── 部署向导
          └── 设置
```

---

## 5. 通用性设计原则

| 原则 | 说明 |
|------|------|
| 零适配 | 用户只需指定 Compose 项目目录，无需修改 docker-compose.yml 或 .env 即可完成展示和基础运维；UI 分组为可选优化，不强制 |
| 容器识别标签 | 用 Docker Compose 自动标签 `com.docker.compose.project` / `com.docker.compose.service` 识别容器，不依赖 PROJECT_NAME 等约定 |
| UI 分组标签 | UI 分组通过可选的 `com.composeboard.category` 标签驱动（合法值 `base` / `backend` / `frontend` / `init` / `other`），未打标签的服务归入 `other` 兜底分组，**不做基于服务名的模糊匹配** |
| 镜像对比 | 版本差异基于 .env 展开后的镜像 vs 实际运行镜像，不依赖变量命名；变量来源仅限本地 `.env`，不读取宿主机环境 |
| Compose 文件自动发现 | 支持 compose.yaml / compose.yml / docker-compose.yml / docker-compose.yaml |
| 核心与扩展分离 | 通用能力为核心，特定场景需求通过 extensions 配置启用 |
| 可选服务建模 | optional 不再作为 category，而是由 profile 维度表达和分组；profile 状态区分 `enabled` / `partial` / `disabled` 三态 |
| 镜像源差异处理 | `image:` 型服务享受全量能力（升级、重建、未部署启动）；`build:` 型服务只读展示 + 已部署容器的启停重启，未部署启动阻断并提示使用部署向导 |

---

## 6. 实施优先级总览

### Phase 1: 核心重构（开源基础）
1. 代码架构重组 + 模块拆分
2. Docker Compose 标签识别
3. 服务声明视图（LEFT JOIN）
4. 镜像直接对比
5. Profiles 管理（整组启用/停用）
6. Dashboard 项目信息

### Phase 2: 增值功能
7. 设置页
8. WebSocket 断线重连
9. .env 行级模型完善
10. Web 终端（Docker Exec + xterm.js）

### Phase 3: 高级功能
11. 部署向导 + 生命周期钩子 + WebSocket 实时输出
12. 主机设置扩展（HOST_IP、开机自启动）
13. Dashboard 图表增强
14. 远程 Docker Host 支持（Docker TCP/TLS 或 SSH 隧道）
