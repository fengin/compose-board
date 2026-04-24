# ComposeBoard 产品技术参数说明

> 面向部署负责人、运维人员、安全评估人员和二次开发人员。本文列出 ComposeBoard 当前实现的运行环境、配置项、接口、性能数据、资源限制和已知边界。

## 1. 基本信息

| 项         | 参数                                        |
| --------- | ----------------------------------------- |
| 产品名称      | ComposeBoard                              |
| 当前版本默认值   | `1.0.0`，可通过编译 `-ldflags` 注入               |
| 二进制名      | `composeboard`                            |
| Go module | `github.com/fengin/composeboard`          |
| 开源仓库      | `https://github.com/fengin/compose-board` |
| 主要用途      | Docker Compose 单机项目可视化管理                  |
| 部署形态      | 单进程二进制                                    |
| 静态资源      | `go:embed` 内嵌                             |
| 数据库       | 无                                         |
| 外部前端 CDN  | 无                                         |

## 2. 运行环境

| 类型          | 要求                                                |
| ----------- | ------------------------------------------------- |
| 操作系统        | Linux、Windows、macOS                               |
| 架构          | amd64、arm64                                       |
| Docker      | 本机 Docker daemon 可访问                              |
| Compose CLI | `docker compose` 或 `docker-compose` 至少存在一个        |
| 浏览器         | 现代浏览器，需支持 ES module 时代的基础 Web API、SSE 和 WebSocket |
| 网络          | 浏览器可访问 ComposeBoard 监听端口                          |

Docker 连接方式：

| 平台          | 连接方式        | 地址                       |
| ----------- | ----------- | ------------------------ |
| Linux/macOS | Unix Socket | `/var/run/docker.sock`   |
| Windows     | Named Pipe  | `\\.\pipe\docker_engine` |

当前版本不支持远程 Docker TCP/TLS 或 SSH Docker。

## 3. 配置文件

默认配置文件路径为启动目录下的 `config.yaml`，也可通过命令行参数指定：

```powershell
.\composeboard.exe -config .\config.yaml
```

配置模板：

```yaml
server:
  host: "0.0.0.0"
  port: 9090

project:
  dir: "/opt/compose-project"
  name: "我的项目"

auth:
  username: "admin"
  password: "changeme"
  jwt_secret: ""

compose:
  command: "auto"

hooks:
  pre_deploy: ""
  post_deploy: ""

extensions:
  host_ip:
    enabled: false
    env_key: "HOST_IP"
    detect_on_startup: true
```

配置项说明：

| 路径                                     | 类型     | 默认值                     | 必填  | 当前用途                                     |
| -------------------------------------- | ------ | ----------------------- | --- | ---------------------------------------- |
| `server.host`                          | string | `0.0.0.0`               | 否   | HTTP 监听地址                                |
| `server.port`                          | int    | `9090`                  | 否   | HTTP 监听端口                                |
| `project.dir`                          | string | 无                       | 是   | Docker Compose 项目目录                      |
| `project.name`                         | string | 目录名                     | 否   | UI 展示名称，不参与 Docker label 匹配              |
| `auth.username`                        | string | 无                       | 是   | 登录用户名                                    |
| `auth.password`                        | string | 无                       | 是   | 登录密码                                     |
| `auth.jwt_secret`                      | string | 自动生成临时值                 | 否   | JWT 签名密钥                                 |
| `compose.command`                      | string | `auto`                  | 否   | `auto`、`docker-compose`、`docker compose` |
| `hooks.pre_deploy`                     | string | 空                       | 否   | 配置结构预留，当前主流程未执行                          |
| `hooks.post_deploy`                    | string | 空                       | 否   | 配置结构预留，当前主流程未执行                          |
| `extensions.host_ip.enabled`           | bool   | `false`                 | 否   | 配置结构预留                                   |
| `extensions.host_ip.env_key`           | string | `HOST_IP`               | 否   | 配置结构预留                                   |
| `extensions.host_ip.detect_on_startup` | bool   | 代码缺省 `false`，模板值 `true` | 否   | 配置结构预留                                   |

安全建议：

- 正式部署必须修改 `auth.password`。
- 正式部署建议配置固定、高强度 `auth.jwt_secret`。
- `jwt_secret` 留空时会在每次启动生成随机临时密钥，重启后旧 token 失效。

## 4. Compose 文件参数

自动发现优先级：

| 优先级 | 文件名                   |
| --- | --------------------- |
| 1   | `compose.yaml`        |
| 2   | `compose.yml`         |
| 3   | `docker-compose.yml`  |
| 4   | `docker-compose.yaml` |

服务分类 label：

| Label                       | 值                                          |
| --------------------------- | ------------------------------------------ |
| `com.composeboard.category` | `base`、`backend`、`frontend`、`init`、`other` |

Docker Compose 原生 label 依赖：

| Label                        | 用途                |
| ---------------------------- | ----------------- |
| `com.docker.compose.project` | 过滤当前项目容器          |
| `com.docker.compose.service` | 将容器匹配到 Compose 服务 |

项目名检测：

| 顺序  | 来源                                  |
| --- | ----------------------------------- |
| 1   | 项目 `.env` 中的 `COMPOSE_PROJECT_NAME` |
| 2   | `project.dir` 的目录名                  |

## 5. 服务模型参数

| 字段                | 类型           | 说明                           |
| ----------------- | ------------ | ---------------------------- |
| `name`            | string       | Compose service key          |
| `category`        | string       | UI 分类                        |
| `image_ref`       | string       | 原始 `image:` 字段               |
| `image_source`    | string       | `registry`、`build`、`unknown` |
| `profiles`        | string array | Compose Profiles             |
| `depends_on`      | string array | 依赖服务                         |
| `has_build`       | bool         | 是否含 `build:`                 |
| `container_id`    | string       | 12 位容器 ID                    |
| `status`          | string       | Docker 状态或 `not_deployed`    |
| `state`           | string       | Docker 人类可读状态                |
| `ports`           | array        | 端口映射                         |
| `health`          | string       | Docker health 状态，缺省 `none`   |
| `startup_warning` | bool         | 启动异常派生标记                     |
| `cpu`             | float        | CPU 百分比                      |
| `mem_usage`       | uint64       | 内存使用字节数                      |
| `mem_limit`       | uint64       | 内存限制字节数                      |
| `mem_percent`     | float        | 内存百分比                        |
| `declared_image`  | string       | `.env` 展开后的声明镜像              |
| `running_image`   | string       | Docker 当前运行镜像                |
| `image_diff`      | bool         | 镜像差异                         |
| `env_diff`        | bool         | 环境变量待生效变更                    |
| `pending_env`     | string array | 待生效变量名列表                     |

## 6. API 参数

### 6.1 认证

| 方法   | 路径                | 请求                                         | 响应                                      |
| ---- | ----------------- | ------------------------------------------ | --------------------------------------- |
| POST | `/api/auth/login` | `{ "username": "...", "password": "..." }` | `{ "token": "...", "expires_at": 123 }` |

JWT：

| 项         | 参数                              |
| --------- | ------------------------------- |
| 签名算法      | HMAC                            |
| 默认有效期     | 24 小时                           |
| Header    | `Authorization: Bearer <token>` |
| WebSocket | `?token=<token>`                |

### 6.2 服务与 Profile

| 方法   | 路径                                | 说明         |
| ---- | --------------------------------- | ---------- |
| GET  | `/api/services`                   | 服务列表       |
| GET  | `/api/services/:name/status`      | 单服务实时状态    |
| POST | `/api/services/:name/start`       | 启动         |
| POST | `/api/services/:name/stop`        | 停止         |
| POST | `/api/services/:name/restart`     | 重启         |
| GET  | `/api/services/:name/env`         | 运行时环境变量    |
| POST | `/api/services/:name/pull`        | 拉取镜像       |
| GET  | `/api/services/:name/pull-status` | 拉取状态       |
| POST | `/api/services/:name/upgrade`     | 应用升级       |
| POST | `/api/services/:name/rebuild`     | 重建         |
| GET  | `/api/profiles`                   | Profile 列表 |
| POST | `/api/profiles/:name/enable`      | 启用 Profile |
| POST | `/api/profiles/:name/disable`     | 停用 Profile |

常见业务错误码：

| code                                 | HTTP | 含义                           |
| ------------------------------------ | ---- | ---------------------------- |
| `services.start.build_not_supported` | 409  | 未部署 `build:` 服务不能由面板直接启动     |
| `services.start.profile_required`    | 409  | 未启用 Profile 下的服务需先启用 Profile |
| `services.not_deployed`              | 404  | 服务未部署                        |
| `services.not_found`                 | 404  | 服务不存在                        |

### 6.3 `.env`

| 方法  | 路径         | 说明                                            |
| --- | ---------- | --------------------------------------------- |
| GET | `/api/env` | 读取 `.env`，返回 `entries`、`raw_text`、`file_path` |
| PUT | `/api/env` | 保存 `.env`                                     |

保存请求二选一：

```json
{ "content": "RAW_TEXT" }
```

```json
{ "entries": [] }
```

### 6.4 日志

| 方法  | 路径                         | 参数                     | 说明       |
| --- | -------------------------- | ---------------------- | -------- |
| GET | `/api/services/:name/logs` | `tail=200`             | 历史日志     |
| GET | `/api/services/:name/logs` | `follow=true&tail=200` | SSE 实时日志 |

SSE 事件：

| event    | data                       | 说明   |
| -------- | -------------------------- | ---- |
| 默认消息     | 日志行文本                      | 单行日志 |
| `status` | `{ "state": "streaming" }` | 流状态  |

`state` 取值：

| 值              | 含义            |
| -------------- | ------------- |
| `streaming`    | 正在跟随日志        |
| `waiting`      | 容器暂不可挂载或等待新容器 |
| `reconnecting` | 日志源异常，准备重连    |

### 6.5 Web 终端

| 方法  | 路径                                         | 协议        |
| --- | ------------------------------------------ | --------- |
| GET | `/api/services/:name/terminal?token=<jwt>` | WebSocket |

控制消息：

| type         | 方向    | 字段                   | 说明        |
| ------------ | ----- | -------------------- | --------- |
| `ready`      | 后端到前端 | `shell`              | exec 已就绪  |
| `input`      | 前端到后端 | `data`               | 写入 stdin  |
| binary frame | 双向    | bytes                | 终端输入输出    |
| `resize`     | 前端到后端 | `cols`、`rows`        | 调整 TTY 尺寸 |
| `close`      | 前端到后端 | 无                    | 主动关闭      |
| `closed`     | 后端到前端 | `reason`、`exit_code` | 会话结束      |
| `error`      | 后端到前端 | `code`、`message`     | 会话错误      |

常见终端错误码：

| code                           | 含义            |
| ------------------------------ | ------------- |
| `terminal.service_not_found`   | 服务未部署         |
| `terminal.service_not_running` | 服务未运行         |
| `terminal.too_many_sessions`   | 活跃终端会话过多      |
| `terminal.no_shell`            | 容器中没有可用 shell |
| `terminal.exec_create_failed`  | 创建 exec 失败    |
| `terminal.exec_start_failed`   | 启动 exec 失败    |
| `terminal.invalid_message`     | 控制消息格式错误      |
| `terminal.unknown_message`     | 未知控制消息类型      |

## 7. 运行时限制参数

| 参数                   | 当前值       | 来源             |
| -------------------- | --------- | -------------- |
| HTTP Client 超时       | 30 秒      | Docker Client  |
| Docker socket 连接超时   | 5 秒       | Transport      |
| Compose CLI 默认超时     | 10 分钟     | Executor       |
| 容器停止超时               | 10 秒      | Docker stop    |
| 容器重启停止超时             | 10 秒      | Docker restart |
| 缓存刷新间隔               | 15 秒      | ContainerCache |
| 缓存空闲暂停               | 60 秒      | ContainerCache |
| stats 采集并发           | 5         | ContainerCache |
| 日志重试延迟               | 1200 ms   | logs.go        |
| 日志源检查间隔              | 800 ms    | logs.go        |
| 日志单行最大扫描             | 1 MiB     | logs.go        |
| 最大终端会话               | 8         | terminal       |
| 终端 ping 间隔           | 25 秒      | terminal       |
| 终端 pong 超时           | 60 秒      | terminal       |
| Exec create/start 超时 | 10 秒      | terminal       |
| shell 探测超时           | 5 秒       | terminal       |
| 终端控制消息最大大小           | 64 KiB    | terminal       |
| 终端输出缓冲               | 32 KiB    | terminal       |
| 终端列范围                | 10 到 1000 | terminal       |
| 终端行范围                | 3 到 500   | terminal       |

## 8. 状态和文件参数

| 文件                         | 位置                                | 用途                      |
| -------------------------- | --------------------------------- | ----------------------- |
| `config.yaml`              | ComposeBoard 启动目录或 `-config` 指定路径 | 应用配置                    |
| `.env`                     | `project.dir`                     | Compose 环境变量            |
| `.env.bak.YYYYMMDD-HHMMSS` | `project.dir`                     | `.env` 保存前自动备份          |
| `.composeboard-state.json` | `project.dir`                     | 已生效镜像、变量和 Profile 配置态基线 |
| Compose YAML               | `project.dir`                     | 服务声明                    |

当前不会读取或迁移旧版 `.deployboard-state.json`。

## 9. 性能测试参数

以下数据来自开发过程性能测试报告，测试日期为 2026-04-24，版本为 v1.0.0，测试人为凌封。数据用于评估 ComposeBoard 在典型单机 Compose 项目中的资源占用和响应能力，实际结果会受硬件、Docker 服务数量、容器状态、磁盘性能和网络环境影响。

### 9.1 测试环境

| 项目         | Windows 环境                       | Linux 环境      |
| ---------- | -------------------------------- | ------------- |
| 操作系统       | Windows 11                       | Anolis OS 8.8 |
| CPU        | AMD Ryzen / Apple M 系列           | Intel Xeon    |
| 内存         | 32 GB                            | 16 GB         |
| Docker 服务数 | 5 个，测试项目                         | 24 个，生产项目     |
| 编译参数       | `CGO_ENABLED=0 -ldflags "-s -w"` | 同左            |

### 9.2 资源占用

测试场景：

| 场景   | 说明                              |
| ---- | ------------------------------- |
| 休眠状态 | 启动后无用户访问，后台仅保留必要的按需机制           |
| 活跃状态 | 浏览器持续操作约 30 秒，包括页面切换、日志查看、终端连接等 |

测试结果：

| 指标       | Windows 休眠 | Windows 活跃 | Linux 休眠 | Linux 活跃 |
| -------- |:----------:|:----------:|:--------:|:--------:|
| 内存 RSS   | 17.5 MB    | 28.6 MB    | 22.4 MB  | 24.1 MB  |
| 私有内存     | 18.5 MB    | 22.7 MB    | 无记录      | 无记录      |
| CPU 累计耗时 | 0.05 s     | 0.55 s     | 无记录      | 无记录      |
| CPU 使用率  | 约 0%       | 约 0.3%     | 0.1%     | 0.1%     |
| Go 线程数   | 无记录        | 无记录        | 12       | 12       |
| 虚拟内存     | 无记录        | 无记录        | 1.2 GB   | 1.2 GB   |

结论：

| 结论           | 说明                                        |
| ------------ | ----------------------------------------- |
| 低内存占用        | 休眠约 17 到 22 MB，活跃峰值不超过 30 MB              |
| 低 CPU 占用     | 休眠时趋近 0%，活跃操作约 0.1% 到 0.3%                |
| Linux 虚拟内存正常 | 1.2 GB 虚拟内存为 Go runtime 预留地址空间，不等同于实际物理占用 |
| 线程稳定         | Linux 测试中活跃和休眠均为 12 个系统线程                 |

### 9.3 启动速度

| 测试项          | 结果      | 说明                           |
| ------------ |:-------:| ---------------------------- |
| 冷启动到 HTTP 就绪 | 约 1.1 秒 | 包含解析 24 个服务、131 个变量和首次容器缓存构建 |

启动后即可通过浏览器访问，无需数据库初始化、前端构建或额外预热。

### 9.4 API 响应延迟

测试方法：本机使用 curl 循环请求各接口 10 次取平均值，排除外部网络延迟。

| 接口                          | 平均延迟    | 说明             |
| --------------------------- |:-------:| -------------- |
| `GET /api/settings/project` | 小于 1 ms | Dashboard 项目信息 |
| `GET /api/host/info`        | 小于 1 ms | 主机信息，含 IP 检测   |
| `GET /api/services`         | 小于 1 ms | 24 个服务的完整状态列表  |
| `GET /`                     | 小于 1 ms | 首屏 HTML，静态资源内嵌 |

核心接口达到亚毫秒级响应，主要得益于内存缓存、按需资源刷新和 `go:embed` 内嵌静态资源。

### 9.5 并发吞吐量

测试工具：Apache Bench，在 Linux 服务器本机执行。

| 场景     | 并发数 | 总请求  | QPS  | 平均延迟    | 失败数 |
| ------ |:---:|:----:|:----:|:-------:|:---:|
| 项目设置接口 | 100 | 1000 | 5425 | 18.4 ms | 0   |
| 服务列表接口 | 50  | 500  | 5099 | 9.8 ms  | 0   |

压测后资源占用：

| 指标     | 数值          |
| ------ | ----------- |
| CPU    | 6.0%，压测瞬时峰值 |
| 内存 RSS | 30.8 MB     |
| 失败请求   | 0           |

在 100 并发持续请求下，测试结果为零失败、5000+ QPS，内存增长到约 30 MB，压测结束后 CPU 迅速回落。

### 9.6 竞品资源对比

以下对比用于描述产品定位和资源量级差异。Portainer、1Panel、Rancher 数据来自官方推荐配置及社区公开基准测试，Dockge 数据来自社区用户报告；数据截至 2026 年 4 月，实际表现会随版本、部署方式和环境变化。

产品定位：

| 维度       | ComposeBoard             | Portainer CE | 1Panel        | Rancher         | Dockge            |
| -------- | ------------------------ | ------------ | ------------- | --------------- | ----------------- |
| **定位**   | **Docker Compose 可视化面板** | 通用容器管理平台     | Linux 服务器运维面板 | Kubernetes 集群管理 | Docker Compose 管理 |
| **目标用户** | **运维、开发者**               | 企业、团队        | 站长、运维         | DevOps、云原生团队    | 个人、HomeLab        |
| **核心场景** | **单机 Compose 项目管理**      | 多环境容器编排      | 服务器全栈管理       | 多集群 K8s 管理      | Compose 堆栈管理      |

资源占用：

| 指标         | ComposeBoard   | Portainer CE | 1Panel       | Rancher      | Dockge      |
| ---------- | -------------- | ------------ | ------------ | ------------ | ----------- |
| **内存，休眠**  | **约 20 MB**    | 100 到 200 MB | 60 到 100 MB  | 2 到 4 GB     | 50 到 80 MB  |
| **内存，活跃**  | **约 30 MB**    | 200 到 300 MB | 100 到 150 MB | 4 到 8 GB     | 80 到 120 MB |
| **CPU，休眠** | **小于 0.1%**    | 0.5% 到 1%    | 0.3% 到 0.5%  | 5% 到 10%     | 0.2% 到 0.5% |
| **最低配置要求** | **512 MB RAM** | 1 到 2 GB RAM | 1 GB RAM     | 4 GB RAM，单节点 | 512 MB RAM  |

部署方式：

| 指标           | ComposeBoard | Portainer CE    | 1Panel         | Rancher         | Dockge      |
| ------------ | ------------ | --------------- | -------------- | --------------- | ----------- |
| **部署方式**     | **单文件直接运行**  | Docker 容器       | 安装脚本 + Docker  | K8s / Docker 容器 | Docker 容器   |
| **镜像/安装包体积** | **~22 MB**   | ~300 MB         | ~500 MB (含依赖)  | ~1 GB+ (含 K8s)  | ~150 MB     |
| **外部依赖**     | **无**        | BoltDB (内嵌)     | MySQL + Docker | etcd + K8s      | Node.js     |
| **跨平台**      | ✅ 6 平台       | ✅ Linux/Win/Mac | ❌ 仅 Linux      | ✅ Linux/Mac     | ✅ Linux/Mac |
| **启动时间**     | **~1 秒**     | 3~10 秒          | 5~15 秒         | 30~120 秒        | 2~5 秒       |
| **离线部署**     | ✅ 零依赖        | ⚠️ 需 Docker     | ⚠️ 需网络安装       | ❌ 需 K8s         | ⚠️ 需 Docker |

功能范围：

| 功能           | ComposeBoard | Portainer CE | 1Panel | Rancher | Dockge |
| ------------ | ------------ | ------------ | ------ | ------- | ------ |
| Compose 堆栈管理 | ✅            | ✅            | ✅      | ✅       | ✅      |
| 容器生命周期控制     | ✅            | ✅            | ✅      | ✅       | ✅      |
| 实时日志查看       | ✅            | ✅            | ✅      | ✅       | ✅      |
| Web 终端       | ✅            | ✅            | ✅      | ✅       | ❌      |
| 镜像升级检测       | ✅            | ✅            | ✅      | ✅       | ❌      |
| 环境变量在线编辑     | ✅            | ⚠️           | ❌      | ❌       | ✅      |
| 多项目管理        | ❌ (单项目)      | ✅            | ✅      | ✅       | ✅      |
| K8s 集群管理     | ❌            | ✅ (BE)       | ❌      | ✅       | ❌      |
| 文件管理         | ❌            | ❌            | ✅      | ❌       | ❌      |
| 数据库管理        | ❌            | ❌            | ✅      | ❌       | ❌      |
| 防火墙/安全       | ❌            | ❌            | ✅      | ✅       | ❌      |

### 9.7 ComposeBoard 核心优势

| 优势    | 参数或说明                                         |
| ----- | --------------------------------------------- |
| 极致轻量  | 休眠约 20 MB，活跃约 30 MB；约为 Portainer 常见内存占用的 1/10 |
| 零依赖部署 | 无需数据库、Node.js 运行时、K8s 或外部静态资源服务器              |
| 秒级就绪  | 冷启动约 1.1 秒，启动后可直接访问                           |
| 高吞吐   | 100 并发场景达到 5000+ QPS，测试失败数为 0                 |
| 跨平台   | 支持 Linux、Windows、macOS，支持 amd64 和 arm64       |
| 专注场景  | 聚焦单机 Docker Compose 运维，不做服务器面板或集群平台式功能堆叠      |
| 离线优先  | 前端资源、字体、终端组件随二进制内嵌                            |

### 9.8 适用场景评估

| 场景              | 是否适用 | 说明                                       |
| --------------- |:----:| ---------------------------------------- |
| 低配云服务器，1C1G     | ✅    | 内存占用小于 30 MB，对 1 GB 机器压力较低               |
| 边缘设备 / ARM 开发板  | ✅    | 提供 ARM64 编译产物，资源占用低                      |
| 多项目并行部署         | ✅    | 每个项目运行一个实例，单实例资源占用低                      |
| 大规模服务集群，100+ 容器 | ⚠️   | 当前未做 100+ 容器规模测试，理论上轮询和 Docker API 延迟会增加 |

### 9.9 性能亮点汇总

| 亮点      | 数据                          |
| ------- | --------------------------- |
| 🚀秒级启动  | 冷启动约 1 秒，含 24 个服务解析         |
| 💾低资源占用 | 休眠约 20 MB，活跃约 30 MB         |
| ⚡亚毫秒响应  | 核心 API 本机测试小于 1 ms          |
| 🔥高并发   | 100 并发、5000+ QPS、0 失败       |
| 📦单文件部署 | 二进制约 21 到 23 MB，含完整前端       |
| 🏆资源优势  | 内存占用约为 Portainer 常见占用的 1/10 |

## 10. 构建产物参数

当前工作副本中已有构建产物参考。二进制文件已包含完整前端资源，包括 HTML、CSS、JavaScript、图片、字体和终端组件，无需额外部署静态文件。

| 平台      | 架构                  | 文件名                              | 体积      |
| ------- | ------------------- | -------------------------------- | ------- |
| Linux   | x86_64，amd64        | `composeboard-linux-amd64`       | 22.2 MB |
| Linux   | ARM64               | `composeboard-linux-arm64`       | 20.8 MB |
| Windows | x86_64，amd64        | `composeboard-windows-amd64.exe` | 22.7 MB |
| Windows | ARM64               | `composeboard-windows-arm64.exe` | 20.9 MB |
| macOS   | Intel，amd64         | `composeboard-darwin-amd64`      | 22.5 MB |
| macOS   | Apple Silicon，arm64 | `composeboard-darwin-arm64`      | 21.2 MB |

构建说明：

| 项    | 说明                               |
| ---- | -------------------------------- |
| 静态资源 | 通过 `go:embed` 内嵌到二进制             |
| 编译参数 | `CGO_ENABLED=0 -ldflags "-s -w"` |
| 体积优化 | `-s -w` 去除符号表和 DWARF 调试信息        |
| 运行依赖 | 本机 Docker daemon 和 Compose CLI   |

## 11. 安全参数

| 项               | 当前实现                                           |
| --------------- | ---------------------------------------------- |
| 用户存储            | `config.yaml` 明文账号密码                           |
| Token           | JWT                                            |
| Token 有效期       | 24 小时                                          |
| WebSocket token | query string                                   |
| 终端审计            | 当前无                                            |
| 运行时 env 脱敏      | 按 key 包含 `PASSWORD`、`SECRET`、`TOKEN`、`PASS` 判断 |
| HTTPS           | 应由外部反向代理提供                                     |

建议：

- 不要把 ComposeBoard 直接裸露到公网。
- 生产环境使用 HTTPS。
- 配合防火墙、VPN、反向代理 Basic Auth 或 SSO 入口使用。
- 只给可信人员开放 Web 终端。

## 12. 当前边界

| 能力                 | 当前状态                 |
| ------------------ | -------------------- |
| 多用户、多角色            | 未实现                  |
| 操作审计               | 未实现                  |
| 多项目管理              | 未实现                  |
| 远程 Docker Host     | 未实现                  |
| Kubernetes / Swarm | 不支持                  |
| Compose 多副本管理      | 未实现                  |
| 部署向导               | 当前代码未实现              |
| 完整设置页              | 当前代码未实现              |
| HOST_IP 自动写回       | 配置结构存在，当前主流程未实现      |
| 镜像仓库凭据管理           | 不管理，依赖 Docker daemon |

## 作者信息

作者：凌封  
作者主页：[https://fengin.cn](https://fengin.cn)  
AI 全书：[https://aibook.ren](https://aibook.ren) 
GitHub：[https://github.com/fengin/compose-board
