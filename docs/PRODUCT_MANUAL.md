# ComposeBoard 产品功能说明书

> 面向使用者、运维人员、项目负责人和产品选型人员。本文说明 ComposeBoard 解决什么问题、适合哪些场景、有哪些功能、与同类产品的差异，以及当前版本的边界。

## 1. 产品概述

ComposeBoard 是一个轻量级 Docker Compose 可视化管理面板。当前 v1.2.0 以已有 Compose 项目目录为管理对象，通过浏览器完成服务状态查看、启停、升级、重建、Profile 分组管理、`.env` 配置编辑、Docker 控制台与宿主机文件日志查看，以及 Web 终端连接。

它的核心设计目标是“保持 Compose 的简单性”。用户不需要把项目迁移到新的平台，也不需要引入数据库、镜像仓库代理、Kubernetes 或额外运维体系。ComposeBoard 只做单机 Compose 项目的日常可视化运维补位。

![系统概览](ui/系统概览.png)

## 2. 解决的问题

很多中小型服务、私有化部署、演示环境、预生产环境、边缘节点和内部工具都采用 Docker Compose 管理。纯命令行方式稳定直接，但在日常维护中常见以下问题：

| 问题                    | ComposeBoard 的处理方式                  |
| --------------------- | ----------------------------------- |
| 不方便查看所有声明服务           | 读取 Compose YAML，展示全部声明服务，包括未部署服务    |
| 容器名不稳定或难以识别           | 使用 Docker Compose 原生 label 识别项目和服务  |
| 可选服务缺少清晰分组            | 识别 Compose Profiles，并按 profile 分组管理 |
| 镜像版本是否需要升级不直观         | 对 `image:` 服务展开 `.env` 后比较声明镜像和运行镜像 |
| `.env` 修改后哪些服务需要重建不明确 | 记录已生效状态，提示受影响变量和服务                  |
| 查看控制台或数据盘日志需要登录服务器    | 浏览器内切换 Docker 控制台和安全基准内文件日志          |
| 排查容器问题需要 SSH 到宿主机     | 浏览器内通过 Docker Exec 打开运行中容器终端        |
| 大平台太重                 | 单文件运行，低内存占用，无数据库                    |

## 3. 适用场景

| 场景                         | 适用性     | 说明                                |
| -------------------------- | ------- | --------------------------------- |
| 单机 Docker Compose 生产或预生产项目 | 适合      | 适合一个实例管理一个 Compose 项目             |
| 私有化部署交付                    | 适合      | 可随项目一起提供给客户或运维人员                  |
| 边缘设备和低配云服务器                | 适合      | 运行时资源占用低                          |
| 多套环境并行                     | 适合      | 每个 Compose 项目部署一个 ComposeBoard 实例 |
| 开发、测试、演示环境                 | 适合      | 服务状态、日志和终端能力可以提升排障效率              |
| 多项目统一平台                    | 不适合当前版本 | 当前配置中 `project.dir` 为单值           |
| Kubernetes / Swarm / 集群管理  | 不适合     | 产品边界是 Docker Compose              |
| 复杂权限、多用户审计                 | 不适合当前版本 | 当前只有配置文件账号密码和 JWT 登录              |

## 4. 产品优势

### 4.1 轻量化

ComposeBoard 是一个 Go 单文件程序，前端资源通过 `go:embed` 内嵌，不依赖数据库、Node.js 运行时、外部 CDN 或独立 Web 服务器。开发测试数据中，休眠状态内存占用约 20 MB，活跃操作约 25~28 MB，CPU占用几乎为零。

### 4.2 声明态优先

传统容器面板通常以“当前容器”为主视图，未部署服务不会出现。ComposeBoard 先解析 Compose 文件，再把 Docker 运行态 LEFT JOIN 到声明服务上，因此可以看到：

- 已运行服务
- 已停止服务
- Compose 中声明但尚未部署的服务
- Profile 下的可选服务
- `image:` 和 `build:` 服务差异

### 4.3 标签驱动而非服务名猜测

ComposeBoard 使用 Docker Compose 自动生成的标签定位容器：

```text
com.docker.compose.project
com.docker.compose.service
```

UI 分类使用可选标签：

```text
com.composeboard.category
```

这避免了“通过服务名包含 mysql / redis / api 之类关键词猜分类”的不稳定做法。

日志页服务下拉按 `backend → base → frontend → 其他` 稳定分组，同组及未打标签服务保持 Compose/API 原始相对顺序。标签只改善 UI 展示，不是文件日志自动发现的前置条件。

### 4.4 离线优先

Vue、Vue Router、xterm.js、字体、图标和页面资源均内置在二进制中。部署到内网、离线环境或客户现场时，不需要访问公网 CDN。

### 4.5 与同类产品的定位差异

| 维度          | ComposeBoard     | Portainer CE | 1Panel  | Rancher       | Dockge               |
| ----------- | ---------------- | ------------ | ------- | ------------- | -------------------- |
| 主要定位        | 单机 Compose 可视化运维 | 通用容器管理       | 服务器运维面板 | Kubernetes 平台 | Compose 堆栈管理         |
| 资源占用        | 很低，单文件           | 中等           | 中等      | 高             | 低到中等                 |
| 部署方式        | 二进制直接运行          | 容器部署         | 安装脚本    | K8s / 容器      | 容器部署                 |
| `.env` 在线编辑 | 支持               | 部分场景支持       | 非核心能力   | 非核心能力         | 支持                   |
| Web 终端      | 支持               | 支持           | 支持      | 支持            | 当前不作为核心能力            |
| 多项目管理       | 当前不支持            | 支持           | 支持      | 支持            | 支持                   |
| 目标用户        | Compose 项目维护者    | 容器平台用户       | 服务器管理员  | 云原生团队         | HomeLab / Compose 用户 |

> 说明：竞品能力会随版本变化。上表用于说明产品定位差异，不作为对第三方产品的完整评测。

## 5. 功能地图

```mermaid
flowchart TB
    Product["ComposeBoard"]

    Product --> Auth["登录认证"]
    Auth --> AuthJWT["JWT"]
    Auth --> AuthConfig["配置文件账号密码"]

    Product --> Dashboard["系统概览"]
    Dashboard --> ProjectInfo["项目信息"]
    Dashboard --> HostInfo["主机信息"]
    Dashboard --> ServiceStats["服务统计"]

    Product --> Services["服务管理"]
    Services --> Declared["Compose 声明服务"]
    Services --> Runtime["运行态状态"]
    Services --> Lifecycle["启停重启"]
    Services --> Upgrade["升级重建"]
    Services --> Profiles["Profile 分组"]

    Product --> Env["环境配置"]
    Env --> TableMode["表格模式"]
    Env --> TextMode["文本模式"]
    Env --> DiffConfirm["差异确认"]
    Env --> Backup["自动备份"]

    Product --> Logs["日志查看"]
    Logs --> ConsoleLogs["Docker 控制台日志"]
    Logs --> FileLogs["宿主机文件日志"]
    FileLogs --> Discovery["服务级发现 / 人工映射"]
    FileLogs --> LiveLogs["SSE 跟随 / 轮转续接"]
    FileLogs --> Download["普通日志 / .gz 下载"]

    Product --> Terminal["Web 终端"]
    Terminal --> DockerExec["Docker Exec"]
    Terminal --> Xterm["xterm.js"]
    Terminal --> Resize["TTY resize"]

    Product --> I18n["国际化"]
    I18n --> Chinese["中文"]
    I18n --> English["English"]
```

## 6. 功能说明

### 6.1 登录认证

![登录](ui/登录.png)

| 功能           | 说明                                                      |
| ------------ | ------------------------------------------------------- |
| 账号密码登录       | 从 `config.yaml` 的 `auth.username` 和 `auth.password` 读取  |
| JWT token    | 登录成功后签发 24 小时 token                                     |
| API 鉴权       | 除 `/api/auth/login` 外，所有 `/api` 路由都需要认证                 |
| WebSocket 鉴权 | Web 终端通过 `?token=<jwt>` 传递 token                        |
| 安全提示         | `jwt_secret` 建议固定配置为高强度随机值；留空时程序会自动生成临时密钥，重启后旧 token 失效 |

当前版本不是多用户系统，不包含角色权限、审计日志和细粒度授权。

### 6.2 系统概览

![系统概览英文](ui/系统概览_en.png)

系统概览页用于快速判断项目和宿主机状态：

| 区域        | 内容                                          |
| --------- | ------------------------------------------- |
| 项目信息      | 项目名称、Compose 文件、Compose 命令、版本、服务数量、Profiles |
| 主机信息      | OS、平台、架构、主机 IP 候选列表、CPU、内存、磁盘               |
| Docker 信息 | Docker 版本和 API 版本                           |
| 服务统计      | 运行、停止、未部署服务数量                               |
| 服务分组      | 按 `com.composeboard.category` 展示服务状态概览      |

IP 展示采用候选列表策略，优先展示物理局域网或公网 IPv4，Docker、WSL、Hyper-V、VPN 等虚拟网络地址会降权但不直接丢弃。

### 6.3 服务管理

![服务管理](ui/服务管理.png)

服务管理页是 ComposeBoard 的核心页面。它展示 Compose 文件中的全部服务，并融合 Docker 运行态。

| 字段       | 说明                                  |
| -------- | ----------------------------------- |
| 服务名      | Compose YAML 中的 service key         |
| 镜像版本     | `image:` 展开 `.env` 后的目标镜像，或本地构建标识   |
| 状态       | `running`、`exited`、`not_deployed` 等 |
| 端口       | Docker API 返回的端口映射                  |
| CPU / 内存 | 运行中容器的实时资源数据                        |
| 运行时间     | Docker 容器启动时间                       |
| 操作       | 启动、停止、重启、升级、重建、查看 ENV、查看日志、打开终端     |

服务状态说明：

| 状态                       | 含义                              |
| ------------------------ | ------------------------------- |
| `running`                | 容器正在运行                          |
| `exited`                 | 容器已存在但停止                        |
| `not_deployed`           | Compose 声明存在，但 Docker 中没有对应服务容器 |
| `created` / `restarting` | Docker 当前状态，持续超过阈值时会展示启动异常提示    |

### 6.4 Profile 分组管理

Compose Profiles 用于描述可选服务组。ComposeBoard 按 profile 分组展示可选服务，并提供整组操作。

```mermaid
flowchart LR
    Disabled["Profile disabled"] -->|启用 Profile| Enabling["docker compose --profile <name> up -d"]
    Enabling --> Enabled["Profile enabled"]
    Enabled -->|停用 Profile| Disabling["stop + rm profile 服务"]
    Disabling --> Disabled
```

规则：

| 规则          | 说明                                        |
| ----------- | ----------------------------------------- |
| 未启用 profile | 未部署服务不显示单服务启动按钮，需要先启用整个 Profile           |
| 已启用 profile | 组内服务与固定服务共用启停、重启、日志、终端等操作                 |
| 停用 profile  | 会停止并移除该 profile 下的服务容器，操作前弹出确认            |
| 状态来源        | Profile 状态表达“配置启用态”，服务是否运行仍以 Docker 运行态为准 |

### 6.5 镜像升级

![服务升级](ui/服务升级.png)

已部署且镜像来源为仓库的 `image:` 服务始终显示升级操作。声明镜像与运行镜像不一致时，列表继续显示当前版本到目标版本的变化，升级按钮使用黄色强调样式；版本号一致时，升级按钮使用与 ENV 等普通操作相同的样式，也可以重新拉取同标签镜像并重建容器，适用于镜像标签未变但内容重新发布的场景。

升级弹窗规则：

| 场景 | 目标版本 | 拉取行为 |
| ---- | -------- | -------- |
| 声明镜像与运行镜像不同 | 显示声明镜像的新版本号 | 拉取声明的新版本镜像 |
| 镜像版本无变化 | 显示“无变更” | 重新拉取当前声明的同标签镜像 |

`build:` 服务和未部署服务不显示升级操作。本地升级仍只使用服务器已有的目标镜像，不访问镜像仓库。

升级流程：

```mermaid
sequenceDiagram
    participant User as 用户
    participant UI as 前端
    participant API as API
    participant Compose as Compose CLI
    participant Docker as Docker

    User->>UI: 点击升级
    UI->>API: POST /api/services/:name/pull
    API->>Compose: docker compose pull <service>
    UI->>API: GET /api/services/:name/pull-status
    API-->>UI: pulling / success / failed
    User->>UI: 确认应用升级
    UI->>API: POST /api/services/:name/upgrade
    API->>Compose: docker compose up -d --force-recreate --no-deps <service>
    API->>Docker: 刷新服务状态
    API-->>UI: 升级完成
```

目标镜像已通过 `docker load` 等方式导入服务器时，可选择“本地升级”。ComposeBoard 会先确认完整目标镜像已存在，再执行 `docker compose up -d --pull never --no-deps <service>`；镜像不存在时立即提示，不会重建当前容器。

限制：

- 仅 `image:` 服务支持升级检测和拉取。
- `build:` 型服务不做镜像差异检测。
- 镜像仓库凭据由 Docker 自身管理，首次登录请使用 `docker login` 或部署脚本完成。

### 6.6 环境变量配置

![环境变量配置](ui/环境变量配置.png)

`.env` 编辑支持两种模式：

| 模式   | 说明                       |
| ---- | ------------------------ |
| 表格模式 | 按变量行编辑 key/value，未编辑行保留原始引号、注释、空白和顺序 |
| 文本模式 | 直接编辑原始 `.env` 文本         |

![环境变量文本模式](ui/环境变量配置-文本模式.png)

Compose 配置修改模式：

支持直接查看和修改 `docker-compose.yml` 内容，同样使用带语法高亮和行号的代码编辑器：

![Compose配置修改](ui/docker-compose修改.png)

保存行为：

1. 保存前展示差异确认。
2. 自动生成备份文件，格式为 `.env.bak.YYYYMMDD-HHMMSS`。
3. 保存后重新解析 Compose 文件和 `.env`。
4. 对受 `.env` 变更影响的服务展示“配置已变更”提示。
5. 用户可在服务页点击“重建”让配置生效。

表格与文本模式切换时以当前文本建立编辑基线。表格模式只重建实际修改的变量行，未修改的带引号值不会出现在差异预览中，也不会被改写格式。

实例环境变量查看：

![实例环境变量查看](ui/实例环境变量查看.png)

敏感变量会按变量名脱敏，包含 `PASSWORD`、`SECRET`、`TOKEN`、`PASS` 的 key 不展示完整值。

### 6.7 日志查看

![日志查看](ui/日志查看.png)

日志页支持：

| 功能   | 说明                                      |
| ---- | --------------------------------------- |
| 服务选择 | 两种来源共用标签分组顺序；同组和未标记服务保持原始相对顺序 |
| 历史日志 | 获取最近 N 行日志，页面默认 100 行                   |
| 实时日志 | 通过 SSE 持续推送 Docker logs                 |
| 自动滚动 | 可开关                                     |
| 重连跟随 | 服务重建、容器 ID 变化或短暂不可用时，前端展示状态，后端尝试重新挂载日志源 |
| 日志清理 | 清空当前前端显示内容，不影响容器日志                      |
文件日志是控制台日志的补充能力。部署管理员在 `config.yaml > file_logs.allowed_bases` 中声明宿主机安全基准目录后，页面才显示日志来源下拉；来源、服务、目录、文件和操作按钮共用一行，默认仍为“容器控制台”。

文件日志模式支持：

| 功能 | 说明 |
| --- | --- |
| 服务级自动发现 | 只检查当前服务位于安全基准目录内的实际 Docker Mounts 和 Compose volumes，不要求额外 Labels，也不识别具体产品名称 |
| 日志树归并 | 互为父子的主日志目录和 Nacos 等子挂载归为同一日志树，避免嵌套挂载造成错误冲突 |
| 空目录候选 | 明确的日志语义挂载即使暂时为空也可被识别，等待应用首次生成文件 |
| 有界探测 | 最深2层、最多2000项、最长300ms；达到上限立即返回，不扫描整个 `DATA_ROOT` |
| 未匹配处理 | Redis、无文件日志的前端等服务不展示其他应用目录，也不默认选择第一个目录 |
| 人工映射 | 在安全基准目录下按层浏览或输入相对路径，检测通过后保存到 `.composeboard-file-logs.json` |
| 跨项目复制 | 映射只保存稳定的 `base_id`、Compose service name 和相对路径，可复制给相同目录结构的项目 |
| 实时跟随 | 从文件尾部读取最近 N 行，通过 SSE 持续推送追加内容 |
| 轮转续接 | 检测文件替换、截断和重新创建并自动续接同名文件 |
| 文件下载 | 普通日志和 `.gz` 归档均可流式下载，支持 HTTP Range |
| 安全边界 | Web 不能新增绝对基准目录；服务端拒绝绝对相对路径、`..`、符号链接和非普通文件 |

自动发现只在唯一高可信日志树或候选明显更强时选中目录；不相关的多候选或无候选由用户选择或人工设置。`.gz` 只支持下载，不支持实时跟随或在线解压预览。文件名还必须命中扩展名白名单，例如默认不会列出以 `.1` 结尾的 Nacos 轮转文件。“清空”只清理浏览器显示内容。

英文界面：

![日志查看英文](ui/日志查看_en.png)

### 6.8 Web 容器直连终端

![Web 容器直连终端](ui/Web容器直连终端.png)

Web 终端基于 Docker Exec API，不需要 SSH 到宿主机。

| 功能       | 说明                                                                       |
| -------- | ------------------------------------------------------------------------ |
| 连接对象     | 仅运行中服务                                                                   |
| Shell 探测 | Linux 容器优先 `bash`，回退 `/bin/sh`；Windows 容器尝试 `cmd.exe` 和 `powershell.exe` |
| TTY      | 固定开启 TTY                                                                 |
| 尺寸同步     | 前端 resize 后同步到 Docker Exec                                               |
| 并发限制     | 默认全局最多 8 个活跃终端会话                                                         |
| 会话生命周期   | 一个 WebSocket 对应一个 Docker Exec 会话，断开后重新连接会创建新 shell                       |

安全注意：

- Web 终端等价于进入容器内部执行命令，应只暴露给可信用户。
- 当前版本不记录命令输入和终端输出审计日志。
- 建议在反向代理层启用 HTTPS。

### 6.9 关于信息

![关于](ui/关于.png)

关于弹窗展示产品名称、版本、作者主页、AI 全书和 GitHub 地址，便于开源传播和问题反馈。

## 7. 操作规则汇总

| 服务状态           | 服务类型                | Profile 状态       | 可用操作                                        |
| -------------- | ------------------- | ---------------- | ------------------------------------------- |
| `running`      | `image:`            | 任意               | 停止、重启、查看 ENV、日志、终端；存在镜像差异时可升级；存在 env 变更时可重建 |
| `running`      | `build:`            | 任意               | 停止、重启、查看 ENV、日志、终端；不支持镜像升级                  |
| `exited`       | `image:` / `build:` | 任意               | 启动、查看 ENV、日志                                |
| `not_deployed` | `image:`            | 无 profile        | 启动                                          |
| `not_deployed` | `image:`            | profile disabled | 先启用 Profile                                 |
| `not_deployed` | `build:`            | 任意               | 不支持面板直接启动，需使用 Compose 命令或后续部署向导能力           |

## 8. 局限和注意事项

| 类别          | 当前边界                                        |
| ----------- | ------------------------------------------- |
| 项目范围        | 一个 ComposeBoard 实例管理一个 Compose 项目目录         |
| Docker 连接   | 仅本地 Docker daemon，不支持远程 Docker Host         |
| 副本模型        | 按一服务一容器视图处理，不管理 scale / replicas            |
| 权限模型        | 单账号密码，不支持多用户、角色和审计                          |
| `build:` 服务 | 已部署后可启停、重启、日志、终端；未部署构建启动不由当前面板处理            |
| 部署向导        | 开发期有规划，当前代码未实现对外页面和接口                       |
| 设置页         | 当前只有 Dashboard 使用的项目设置只读 API，无完整设置页面        |
| 凭据管理        | 不保存镜像仓库凭据，依赖 Docker daemon 的 `docker login` |
| 文件日志授权      | 只能访问 `allowed_bases`，不能从 Web 新增任意绝对目录    |
| 归档预览        | `.gz` 可下载但不在线解压；非白名单后缀不列出              |

## 9. 推荐使用方式

1. 每个 Compose 项目部署一个 ComposeBoard 实例。
2. 给项目设置稳定的 `COMPOSE_PROJECT_NAME`，避免目录名变化影响标签匹配。
3. 给服务增加 `com.composeboard.category` 标签，改善服务分组和日志下拉顺序。
4. 使用 Compose Profiles 描述可选服务组。
5. 需要文件日志时，由部署管理员配置非根 `allowed_bases`，不要开放 `/`、`/etc` 等宽泛目录。
6. 确保 `DATA_ROOT` 实际位于目标数据盘；路径名称本身不代表独立分区。
7. 修改 `.env` 后通过服务页的重建提示逐项确认。
8. 对公网访问启用 HTTPS 和额外访问控制。

## 10. 相关文档

- [产品技术说明](TECHNICAL_OVERVIEW.md)
- [产品技术参数说明](TECHNICAL_PARAMETERS.md)
- [产品编译、部署和使用手册](BUILD_DEPLOY_USAGE.md)
- [开发规范文档](DEVELOPMENT_STANDARDS.md)
- [产品精简介绍](INTRODUCTION.md)
- [v1.2.0 变更日志](../CHANGELOG.md)



## 作者信息

作者：凌封  
作者主页：[https://fengin.cn](https://fengin.cn)  
AI 全书：[https://aibook.ren](https://aibook.ren)  
GitHub：[https://github.com/fengin/compose-board](https://github.com/fengin/compose-board)
