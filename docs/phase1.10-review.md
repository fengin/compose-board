# ComposeBoard Phase 1.2–1.10 后端深度 Code Review 报告

**审查基线**: `PRODUCT_SPEC v2.1` · `ARCHITECTURE v2.1` · `DESIGN_DECISIONS v2026-04-21` · `IMPLEMENTATION_PLAN v2.1` · `DEV_STANDARDS` **审查范围**: `internal/compose`、`internal/docker`、`internal/service`、`internal/api`、`main.go`、`config.go`、`auth.go` **审查结论**: **不合格,需在前端重构(§1.11)前先修复阻塞性与中度问题**。合计发现 **🔴 严重 5 项 / 🟠 中度 14 项 / 🟡 轻度 12 项 / 📝 文档冲突 2 项**。

---

## 一、🔴 严重问题(阻塞性 / 功能性 Bug)

### S-1 Windows Named Pipe 实现完全错误,Windows 平台无法连接 Docker

`internal/docker/client.go:89-96`

client.gointernal/docker

- **问题**: Go 标准库 `net.Dial` 的 `"unix"` network 在 Windows 上无效(Go 对 Windows Unix Socket 有限支持且**不支持 Named Pipe**)。启动时会 dial 失败。
- **违反规范**:
  - `ARCHITECTURE §2.4` 明确写 "Windows 使用 `github.com/Microsoft/go-winio` 拨号 Named Pipe"
  - `IMPLEMENTATION_PLAN §1.5` 同样要求
- **佐证**: `go.mod` 里**没有** `github.com/Microsoft/go-winio` 依赖,`PROGRESS.md §1.5` 虽声称 "跨平台 Transport" 已完成,实际只是加了个 `runtime.GOOS` 分支。
- **修复**:
  1. `go get github.com/Microsoft/go-winio`
  2. 按 `ARCHITECTURE §2.1` 要求新建 `internal/docker/transport.go`,Windows 下用 `winio.DialPipe(\\\\.\\pipe\\docker_engine, &timeout)`,Linux 保留 unix socket

### S-2 PullImage 的 ctx 被显式丢弃,executor 内部也无超时,长拉取永不中断

`internal/service/upgrade.go:73-88`

augml:function_calls

upgrade.gointernal/service

- **问题**: `_ = ctx` 显示作者知道 ctx 没被使用;`executor.go:run()` 用的是 `exec.Command` 非 `exec.CommandContext`,**完全没有超时**。镜像源故障/网络卡住时,goroutine 永久挂起,pullStatus 永远停在 "pulling"。
- **修复**: `Executor` 所有方法签名改为 `(ctx context.Context, ...)`,内部用 `exec.CommandContext(ctx, ...)`,由调用方注入超时。

### S-3 `api/logs.go` 的 timeout 数值严重错误(10s → 约 11 分钟)

`internal/api/logs.go:34`

logs.gointernal/api

- **计算**: `0x1_000_000_000 = 2^36 ≈ 6.87×10^10` ns ≈ **68.7 秒**,再乘 10 ≈ **687 秒 ≈ 11 分 27 秒**。作者误把 `1e9` 当成 `0x1_000_000_000`(实际上 `1e9 = 0x3B9ACA00`)。
- **影响**: 非 follow 模式的日志请求可能挂 11 分钟才释放。
- **修复**: 直接写 `10*time.Second`。

### S-4 StartService 未拦截 build 型服务与 profile 可选服务,违反 DESIGN_DECISIONS §2/§3

`internal/api/services.go:35-50`

services.gointernal/api

- **明确违反**:
  - `DESIGN_DECISIONS §3 补充`: "`ImageSource == "build"` 且未部署 → API 返回 HTTP 409,错误码 `services.start.build_not_supported`"
  - `DESIGN_DECISIONS §2 补充`: "未部署且有 `Profiles` 的可选服务:仍通过 `POST /api/profiles/:name/enable` 启用整个 profile,不提供单服务 deploy API"
  - `PRODUCT_SPEC §3.3.2`: `build:` 型未部署启动返回 409
- **当前行为**: 对 `build:` 型会触发 compose build + up;对 profile 可选服务会绕过 Profile 粒度管控,单独 up。
- **验收标准 Phase 1 第 4 条直接不过**。
- **修复位置**: 下沉到 `service/manager.go`,新增 `StartService(ctx, name) error`,在里面判断 `ImageSource == "build"` / `len(Profiles) > 0 && !deployed` 并返回规格化错误。

### S-5 项目名检测缺少"目录名"兜底,双重 env 变量都缺失时 label filter 查空串

`main.go:58-67`

main.go

- **违反**: `ARCHITECTURE §3.4` / `DESIGN_DECISIONS` 明确检测链 `config → COMPOSE_PROJECT_NAME → PROJECT_NAME → 目录名`,缺"目录名"这一环。

- **后果**: 用户仅给出 `project.dir` 不配 `name` 且 .env 也没有项目名变量时 → `projectName=""` → Docker label filter `com.docker.compose.project=` → 不匹配任何容器 → 所有服务显示为未部署。

- **验收标准 Phase 1 第 1 条**(无 PROJECT_NAME 的项目可用)**不过**。

- **修复**:

- **顺带**: `cfg.Project.Dir + "/.env"` 应改为 `filepath.Join(cfg.Project.Dir, ".env")`。

---

## 二、🟠 中度问题(设计偏离 / 潜在故障)

### M-1 `ExpandVars` 对 `${VAR:?error}` 语义错误,把错误消息当变量值

`internal/compose/env.go:197-207`

- Compose 官方语义:`${VAR:?msg}` 未设置或空时 **fatal**,compose 命令会拒绝执行。
- 当前实现:把 `msg` 当成替换值返回(`return defaultVal`),会把 `image: ${VERSION:?version not set}` 展开为 `"version not set"`,然后参与 `ImageDiff` 比对,产生荒唐的 `DeclaredImage`。
- **修复**: 记录日志并返回原始 `match`(保留 `${VAR:?msg}` 让上层能识别),或返回空串 + 在 DeclaredService 上标记 `ExpandError`。

### M-2 `ParseEnvFile` 不剥离 value 的引号

`internal/compose/env.go:65-74`

- 用户写 `IMAGE_TAG="v1.0"`,现在解析出的 `Value = "v1.0"`(连双引号),ExpandVars 展开 `${IMAGE_TAG}` 得到 `"v1.0"`,与 Docker 返回的 running image `mysql:"v1.0"` 无法匹配,产生误报。
- Compose CLI 在读取 .env 时会按 shell-like 规则剥离外层引号。
- **修复**: 解析 value 时处理 `"..."` / `'...'` 外包裹。

### M-3 `Executor.run` 没有 ctx/timeout,CLI 调用可能无限阻塞

`internal/compose/executor.go:137-164`

- 所有 `exec.Command` 都未挂 context。`docker compose up` 网络慢/stuck 时整个 HTTP 请求挂死。
- **修复**: 改造 API 为 `Up(ctx, ...)` / `Pull(ctx, ...)`,用 `exec.CommandContext`。

### M-4 `Executor.Up` 把 `--profile` 当子命令选项而非全局选项

`internal/compose/executor.go:79-95`

- Compose 规范:`--profile` 是全局标志,应在子命令前:`docker compose --profile xxx up -d`。
- v2 新版本对 `up --profile` 宽容,但 v1 (`docker-compose`) 强制要求 `--profile` 位于子命令之前,否则报错。
- **修复**: 在 `buildArgs` 里把 profile 注入到子命令之前:

### M-5 `Executor.buildArgs` 没有传 `-f <file>`,parser 与 CLI 可能读到不同文件

`internal/compose/executor.go:169-182`

- `IMPLEMENTATION_PLAN §1.4` 要求:"项目目录通过 `-f` 和 `--project-directory` 传入"。
- 当前仅传 `--project-directory`,Compose CLI 自己做文件发现,其优先级在不同版本有微妙差异,**可能与 `parser.FindComposeFile` 不一致**。极端情况下:parser 解析 `compose.yaml`,CLI 执行 `docker-compose.yml`,同一项目出现两套声明。
- **修复**: `ServiceManager` 持有 parser 解析出的 `FilePath`,注入 Executor,统一用 `-f`。

### M-6 `normalizeImage` 对带端口 registry 的镜像判 tag 错误

`internal/service/manager.go:203-214`

- 对 `localhost:5000/foo`:含 `:` → 不加 `:latest`;但 Docker 返回的 running 镜像若来自私有 registry 会是 `localhost:5000/foo:latest`,与未加 latest 的 declared 值比对 → **ImageDiff 误报**。
- **修复**:

### M-7 `imagesMatch` 不处理 digest(`@sha256:...`)

- 如果 Docker `PullImage` 之后,inspect 出的 Image 是 `mysql:8.0@sha256:abc...`,与 declared 的 `mysql:8.0` 不等。
- 镜像比对应先剥离 `@sha256:...` 尾缀再归一。

### M-8 `state.go:writeStateLocked` 非真正原子写,Windows 下存在窗口

`internal/service/state.go:256-263`

- 先 `Remove` 再 `Rename`,两步之间若进程崩溃就丢状态文件;注释却说 "原子替换"。
- POSIX 下 `os.Rename` 是原子覆盖,不需 Remove。Windows 下 Rename 不能覆盖目标,需要 Remove 后重试;但最好用 `syscall.MoveFileEx` + `MOVEFILE_REPLACE_EXISTING`(`go-winio`/`renameio` 等有现成方案)。
- **修复**: 引入 `github.com/google/renameio/v2` 或 Linux 分支直接 rename,Windows 分支 RemoveAll+Rename 带重试。

### M-9 `UpgradeManager.RebuildService` 没有校验 build 型服务

`internal/service/upgrade.go:148-166`

- `PullImage` 和 `ApplyUpgrade` 都做了 `ImageSource=="build"` 拦截,`RebuildService` 遗漏。
- `PRODUCT_SPEC §3.3.2`: 重建仅适用于 image: 型。
- 行为:对 build 型调用 rebuild → `compose up --force-recreate --no-deps` → 触发重新 build,偏离 v1 约束。

### M-10 StartService/StopService/RestartService 绕过 service 层,业务逻辑泄漏到 Handler

`internal/api/services.go:35-113`

- Handler 直接调 `h.DockerCli.FindContainerByServiceName` + `h.Manager.GetExecutor().Up(...)`,业务流程都在 Handler 里。
- 违反 `ARCHITECTURE §2.3` 矩阵:"`api/` 不应做业务逻辑、直接 Docker API"。
- 也是 S-4 的根源:业务逻辑下沉不足,导致 build/profile 拦截没地方放。
- **修复**: 新建 `service/lifecycle.go`(或并入 manager.go):`StartService(ctx, name)`、`StopService(ctx, name)`、`RestartService(ctx, name)`,内部做状态判断 + Docker 调用 + cache 更新 + 规格化错误返回。Handler 只负责 HTTP 适配。

### M-11 `api/env.go` 使用全局 `config.C`,与架构目标矛盾

`internal/api/env.go:22,49` + `internal/config/config.go:70` + `internal/auth/auth.go:39,51,96`

- `ARCHITECTURE §1.1` 明确列为旧版问题:"全局状态 | config.C | 全局变量"。
- 新代码依然保留 `var C *Config` 并多处直接读取。
- **修复**: Handler 持有 `*config.Config`(构造时注入),auth 里改为从 `gin.Context` / handler 成员获取。

### M-12 日志接口偏离 §9 规范(路径错 + 不是 WebSocket)

`main.go:151` + `internal/api/logs.go`

- `DESIGN_DECISIONS §9 / ARCHITECTURE §4.6` 清单: `GET (WS) /api/logs/:service` + `GET /api/logs/:service/history`。
- 实际: `GET /api/services/:name/logs?follow=true` + SSE。
- 虽然 SSE 也可工作,但前端将来要加的 "断线重连 + banner"(PRODUCT_SPEC §3.5、PROGRESS §2.2)是基于 WebSocket 设计。路径和协议都要重写。

### M-13 `DisableProfile` 调用 stop/rm 未携带 `--profile`

`internal/service/profiles.go:129-139`

- Compose v2 对带 profile 的服务,没有 `--profile` 参数时,`stop` / `rm` 可能报 "no such service"(具体依 v2 minor 版本)。
- **修复**: Executor 的 Stop/Rm 增加 `opts.Profiles` 字段,profile 场景下补齐。

### M-14 `docker/transport.go` 应独立文件但未建,依赖未加

- 同 S-1,架构上也是分离不彻底的表现。独立文件利于未来远程 transport(Phase 3 TCP/TLS / SSH)扩展。

---

## 三、🟡 轻度问题(规范 / 细节)

| #    | 位置                                     | 问题                                                                                              |
| ---- | -------------------------------------- | ----------------------------------------------------------------------------------------------- |
| T-1  | `parser.go:100-106` `GetAllServices`   | 注释说"按名称排序"但未排序,map 迭代无序导致前端列表抖动                                                                 |
| T-2  | `parser.go:43` `varRefRegex`           | 未处理 Compose 的 `$$` 字面量(两个 `$` 表示真正的 `$`),会把 `$$VAR` 误识别为引用                                      |
| T-3  | `env.go:108-121` `WriteEnvEntries`     | variable 行强制 `Key=Value` 重建,丢失用户原始引号/空格(变量行未保留 Raw)                                             |
| T-4  | `client.go:121-124` `ListContainers`   | filter JSON 直接拼 URL 不做 `url.QueryEscape`,项目名含特殊字符会坏                                             |
| T-5  | `client.go:141-149` 健康状态轮询             | 对每个 running 容器**串行** inspect,zheshang 23 容器一次 ListServices 可能 ≥23 次 HTTP 调用                     |
| T-6  | `client.go:162`                        | `strings.TrimPrefix(ctr.Names[0], ...)` 未检查 `len(ctr.Names)>0`                                  |
| T-7  | `manager.go:99` `ListServices`         | 签名没返 error,与 `IMPLEMENTATION_PLAN §1.6` 的 `([]ServiceView, error)` 不一致;解析失败只打 log,API 返回 `null` |
| T-8  | `api/handler.go:14-21`                 | Handler 所有依赖字段都 exported,违反封装(其他包没有理由直接访问)                                                      |
| T-9  | `state.go:230-234`                     | 版本号写入固定 `stateFileVersion=2`,读入时无 version 校验或迁移钩子                                               |
| T-10 | `config.go:40-44`                      | `AuthConfig` 缺少 `TokenTTL` 字段(`ARCHITECTURE §3.4` 定义了),auth.go 硬编码 24h                          |
| T-11 | `api/services.go:22-29` `ListServices` | `EnvDiff` 合并逻辑在 Handler 里,应归 service 层在组装 ServiceView 时一并赋值                                     |
| T-12 | `executor.go:42-75` `DetectCommand`    | 首次检测对 `e.detected/e.version` 无锁写入;一旦 main.go 之外有并发调用就有 race                                     |

---

## 四、📝 文档冲突(需先人工裁决后再统一代码)

### D-1 Profile `enabled` 语义冲突

- `DESIGN_DECISIONS §2` 补充:**`enabled` = Profile 下所有声明服务都已部署(含已停止)**
- `IMPLEMENTATION_PLAN §1.9`:**`enabled` = profile 下全部服务处于 running**
- **当前代码**(`service/profiles.go:82-88`): 按 IMPLEMENTATION_PLAN,即 `runningCount == total` 才 enabled
- 影响前端 UI 状态展示、验收标准 Phase 1 第 6 条解释，需要你给我一个反馈做决定

### D-2 旧版 `.deployboard-state.json` 是否迁移——明确不考虑，不考虑旧deployboard运行的环境升级

- `DESIGN_DECISIONS §12`:**不读取、不迁移、不删除**旧文件,首启即基线
- `IMPLEMENTATION_PLAN §1.8`:**若发现同目录存在旧文件则读取并改名**——这个不需要，去除
- **当前代码**(`service/state.go:71-79`): 按 IMPLEMENTATION_PLAN 做 rename 迁移——这些代码是不是可以去掉？
- 影响从 DeployBoard 升级用户的首次体验——不考虑

建议以 `DESIGN_DECISIONS` 为准(它有更详细的理由分析),然后回头修正 `IMPLEMENTATION_PLAN` 与代码。

---

## 五、✅ 合规与亮点

- **头部注释**: 所有 `.go` 文件均带 `// ComposeBoard - Docker Compose 可视化管理面板 / 作者:凌封 / 网址:https://fengin.cn` ✅
- **硬编码根除**: `categorizeService()` / 服务名模糊匹配 / `ContainerInfo.Category` 字段等旧债均已删除 ✅
- **容器识别**: 完全基于 `com.docker.compose.project` / `com.docker.compose.service` label ✅
- **Category 通路**: parser 读 label → DeclaredService.Category → ServiceView.Category,零硬编码 ✅
- **LEFT JOIN 结构正确**: 以声明态为主、运行态为辅,未部署服务可正确呈现 ✅
- **ExpandVars 8 语法**: 6 种形式实现到位(除 M-1 的 `:?` / `?` 语义偏差),变量来源严格限于 .env ✅(`DESIGN_DECISIONS §8 补充`合规)
- **State 基线机制**: 首启无状态文件时即刻写入基线,避免误报 ✅
- **JWT 类型校验**: `SigningMethodHMAC` 检查防算法混淆攻击 ✅
- **cache.go override 机制**: 操作后立即写 override + refresh 合并 + 容器 ID 变更回退 by ServiceName,这部分**设计相当扎实** ✅
- **gofmt / i18n-check / 编译通过**: PROGRESS 有明确记录 ✅

---

## 六、修复优先级建议(给下一步行动用)

**必须在启动 §1.11 前端重构之前修复的**(前端会基于这些 API 写代码):

1. **S-1** Windows Named Pipe — 否则你司 Windows 开发机无法本地调试
2. **S-4** StartService 的 build/profile 拦截 — 前端按钮行为依赖 409 错误码
3. **S-5** 项目名目录名兜底 — 影响最小可用闭环
4. **M-10** Handler 业务下沉 — 会影响所有 `/services/:name/*` 路径的返回格式
5. **M-12** 日志路径 + WebSocket 化 — 前端日志页需要稳定路径
6. **D-1 / D-2** 文档冲突裁决 — 影响前端 profile 三态 UI 语义

**P1(本阶段收尾同步修)**:

7. S-2 / M-3 `Executor` 加 ctx
8. S-3 timeout 数值
9. M-1 / M-2 变量展开与 env 引号
10. M-4 / M-5 `--profile` 位置与 `-f` 显式传入
11. M-6 / M-7 镜像归一
12. M-11 `config.C` 全局依赖 DI 化

**P2(Phase 2 开始前再补)**:

13. M-8 state 原子写
14. M-9 Rebuild 对 build 型拦截
15. M-13 Profile stop/rm 带 `--profile`
16. 所有轻度 T-* 项
17. M-14 独立 `docker/transport.go` + `go-winio` 依赖

---

## 七、附:建议的跟进动作

1. **立即修复阻塞性的 S-1 / S-3 / S-5**(最小代价,约 30 行代码)?
2. 起草一份 `docs/phase1.10-review.md`,把上述问题按同 `phase1.1-review.md` 的格式沉淀,便于你团队评审后勾选处置方案?
3. 按 M-10 提供 `service/lifecycle.go` 的完整拆分方案(调用点、API 返回格式、错误码)?
