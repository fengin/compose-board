# ComposeBoard 开发进度

> **开始时间**: 2026-04-21
> **当前阶段**: Phase 1 ✅ 全部完成 → 准备 Phase 2

---

## Phase 1: 核心重构

### 1.1 项目初始化 + 代码骨架
- [x] 完成
- ✅ 初始化 Go module：`github.com/fengin/composeboard`
- ✅ 创建目录结构（api/auth/compose/config/docker/host/service）
- ✅ 所有代码文件添加头部注释（作者：凌封，网址：https://fengin.cn）
- ✅ 迁移 `auth/auth.go`（import 路径 + token key 更新）
- ✅ 迁移 `host/info.go`（Windows 磁盘路径兼容 `C:\\`）
- ✅ 迁移 `web/` 前端目录（所有资源本地 vendor）
- ✅ 更新 import 路径 → `github.com/fengin/composeboard/internal/xxx`
- ✅ 品牌名称 → ComposeBoard（main.go / index.html / login.js / header.js / style.css / api.js）
- ✅ i18n 基础设施：i18n.js + zh.json(135 key) + en.json(135 key) 完整对称
- ✅ i18n 接入 Vue：`app.config.globalProperties.$t = I18n.t.bind(I18n)` + 启动时预加载语言包
- ✅ i18n 校验脚本：scripts/check-i18n-keys.js
- ✅ config.go 扩展字段（Compose/Hooks/Extensions）
- ✅ config.yaml.template 完整示例
- ✅ Makefile（多平台编译 + i18n 校验）
- ✅ main.go 路由骨架（预留所有 Phase 1/2 API TODO 注释）
- ✅ 编译通过、gofmt 通过、i18n 校验通过
- ⚠️ docker/cache.go 待 1.5 任务中改造后迁移（标签过滤 + transport 抽象）
- ⚠️ 前端组件内硬编码中文文本待 §1.11 前端重构时统一迁移到 `$t()`

#### Review 修复记录（phase1.1-review.md）

| # | 问题 | 处置 |
|---|------|------|
| #1 | JWT 未验算法 | ✅ 已修：添加 `SigningMethodHMAC` 类型检查 |
| #2 | 密码 bcrypt | ❌ 不修：轻量本地工具，config.yaml 与 Docker socket 同机，过度设计 |
| #3 | JWTSecret 固定字符串 | ✅ 已修：空值时 `crypto/rand` 生成 32 字节随机密钥 + 日志提醒 |
| #4 | TokenTTL 配置化 | ❌ 不修：24h 对单用户本地工具足够 |
| #5 | NoRoute 吞 /api/* | ✅ 已修：`/api/` 前缀返回 JSON 404 |
| #6 | i18n 未接入 Vue | ✅ 已修：app.js 预加载 + 全局注册 `$t` |
| #7 | 品牌残留 | ✅ 已修：login.js/header.js/app.js/style.css 全部更新 |
| #8 | 前端硬编码中文 | ❌ §1.11 统一处理：现有页面将整体重构 |
| #9 | gofmt 不合规 | ✅ 已修：`gofmt -w .` 全部通过 |
| #10 | config 字段名不一致 | ✅ 已修：`DetectOnStart` → `DetectOnStartup` |
| #11 | sidebar 旧路由 | ❌ §1.11 处理 |
| #12 | dashboard optional | ✅ 已修：删除 `optional` category |
| #13 | Windows 磁盘路径 | ✅ 已修：`"C:"` → `"C:\\"` |
| #14 | i18n 失败 throw | ❌ 不修：静默降级到 key 是 i18n 标准做法 |

### 1.2 compose/parser.go — YAML 解析器
- [x] 完成
- ✅ 自动发现 Compose 文件（compose.yaml > compose.yml > docker-compose.yml > docker-compose.yaml）
- ✅ 解析 image/build/profiles/depends_on/ports/environment/labels
- ✅ 从 `com.composeboard.category` label 读取 Category，缺省 "other"
- ✅ ImageSource 判断（registry / build / unknown）
- ✅ VarRefs 提取变量名（支持 `${VAR}` 和 `${VAR:-default}` 等语法）
- ✅ GetProfiles() 返回 profile → 服务名映射
- ✅ 已用 zheshang 实际 YAML 验证：23 服务全部正确解析

### 1.3 compose/env.go — .env 行级管理
- [x] 完成
- ✅ EnvEntry 行级模型：variable / comment / blank 三类型，保留原始行内容和行号
- ✅ ParseEnvFile / ReadEnvVars / WriteEnvEntries / WriteEnvRaw 完整读写API
- ✅ ExpandVars 支持 Compose 完整变量语法（8 种形式）
  - `${VAR}` / `$VAR` / `${VAR:-default}` / `${VAR-default}` / `${VAR:+rep}` / `${VAR+rep}` / `${VAR:?err}` / `${VAR?err}`
- ✅ 变量来源仅限 .env，不读 os.Environ()
- ✅ 已用 zheshang .env 验证：229 行正确解析，镜像展开完全正确

### 1.4 compose/executor.go — CLI 执行器
- [x] 完成
- ✅ 自动检测 docker compose (v2) / docker-compose (v1)
- ✅ 支持 "auto" / 手动指定命令模式
- ✅ Up / Pull / Stop / Rm 统一封装
- ✅ UpOptions 支持 ForceRecreate / NoDeps / Profiles

### 1.5 docker/client.go — 标签过滤改造 + Transport
- [x] 完成
- ✅ 基于 `com.docker.compose.project` 标签原生过滤（API 侧 label filter）
- ✅ 从 `com.docker.compose.service` 读取 ServiceName（不再 parse 容器名）
- ✅ 从 `com.composeboard.category` 读取 Category（不再 categorizeService 硬编码）
- ✅ 显式删除 categorizeService() / parseComposeServices() / getProjectName() 硬编码
- ✅ 跨平台 Transport：Linux Unix Socket / Windows Named Pipe
- ✅ cache.go 迁移完成（override + 双层匹配机制保留）

### 1.6 service/manager.go — 服务视图
- [x] 完成
- ✅ ServiceView 融合声明态 + 运行态（LEFT JOIN by ServiceName）
- ✅ Category 直接取自 parser 的 label（不再模糊匹配）
- ✅ DeclaredImage = ExpandVars(image, .env)，与 RunningImage 对比做 ImageDiff
- ✅ imagesMatch 处理 :latest 缺省和 docker.io 前缀
- ✅ ReloadCompose() 支持配置变更后热重载

### 1.7 service/upgrade.go — 升级编排
- [x] 完成
- ✅ PullImage 异步拉取 + 状态追踪（实例变量替代包级全局）
- ✅ ApplyUpgrade: `up --no-deps`（不拉起依赖）
- ✅ RebuildService: `up --force-recreate --no-deps`
- ✅ ImageSource 校验：build 类型服务拒绝拉取/升级
- ✅ 升级/重建后自动更新 state + 刷新 cache

### 1.8 service/state.go — 状态文件
- [x] 完成
- ✅ .composeboard-state.json v2，每服务记录 image + env 已生效值
- ✅ 首次启动自动初始化基线（不产生漂移告警）
- ✅ 不读取、不迁移旧版 .deployboard-state.json（D-2 裁决）
- ✅ GetPendingEnvChanges 返回未生效的变量变更
- ✅ 原子写入（tmp + rename）

### 1.9 service/profiles.go — Profiles 管理
- [x] 完成
- ✅ ListProfiles 改为配置启用态（enabled / disabled）
- ✅ EnableProfile: `compose up --profile xxx`
- ✅ DisableProfile: `stop` + `rm -f`

### 1.10 API 层重构
- [x] 完成
- ✅ api/handler.go：持有 Manager/Upgrade/Profiles/State/Cache/DockerCli
- ✅ api/services.go：服务列表 + 启停重启 + 容器 env 查看
- ✅ api/upgrade.go：pull / pull-status / upgrade / rebuild
- ✅ api/profiles.go：profiles 列表 / enable / disable
- ✅ api/env.go：.env 读取（行级 + 原始）/ 保存（备份 + 热重载）
- ✅ api/host.go：主机信息
- ✅ api/logs.go：日志（一次性 + SSE 流式）
- ✅ main.go 全量重写：串联全部模块，所有 TODO 替换为实际路由
- ✅ 编译通过、gofmt 通过

### 1.11 前端重构（容器→服务架构转型）
- [x] 完成

#### 后端微调
- ✅ `GET /api/settings/project`：项目信息 API（settings.go）
- ✅ `PUT /api/env`：HTTP 方法标准化（POST→PUT）
- ✅ `PendingEnv`：ServiceView 恢复变更跟踪字段
- ✅ `GetCommandInfo`：暴露 compose 命令/版本
- ✅ Handler 注入 ProjectName

#### 前端基础设施
- ✅ api.js 重写：服务导向路径 + SSE 日志流 + 错误码映射
- ✅ sidebar.js：导航路径/图标更新，全部走 i18n labelKey
- ✅ status-badge.js：not_deployed 状态支持 + `$t()` 国际化
- ✅ header.js：页面标题/退出按钮全上 `$t()`
- ✅ style.css：服务三态样式 + Profile 分组 + 本地构建角标 + 变量变更 diff

#### 核心页面重构
- ✅ services.js：替代 containers.js，服务声明态管理
  - 同步操作（start/stop/restart）立即刷新
  - 异步操作（upgrade/rebuild）2s 轮询 + 5min 超时 + 失败快反（T-2）
  - poll 闭包身份校验防污染（T-7）
  - 15s 定时刷新（T-1）
  - extractVersion 纯函数抽取（R-5）
- ✅ dashboard.js：项目信息卡片 + 服务三态统计
- ✅ logs.js：WebSocket→SSE(EventSource) + 401 处理 + 连续错误≥ 3 次自动断开（T-6）
- ✅ env.js：字段模型适配后端 EnvEntry（type/raw），表格保存走 entries 路径（M-1/M-2）

#### i18n 完善
- ✅ 所有页面模板硬编码中文全部替换为 `$t()` 调用
- ✅ zh.json / en.json 完全对称（补充 table/modal/toast/env/logs ~30 个 key）
- ✅ statusLabel / profileStatusLabel / status-badge 统一走 `$t('services.status.*')`
- ✅ `[\u4e00-\u9fa5]` 正则扫描确认：所有 JS 文件零硬编码中文（仅保留注释）

#### 清理与文档
- ✅ 删除旧版 containers.js
- ✅ app.js 旧 URL 重定向（/containers → /services）
- ✅ ARCHITECTURE.md 同步（logs→SSE、settings.go 描述更新）

#### 延后事项（已归档至 Phase 2）
- ⚠️ R-2：WriteEnvEntries 用 Raw 保留引号/空格 → 并入 Phase 2 env 编辑器深度优化
- ⚠️ T-3：extractVersion 私有 registry 端口误识别 / T-4：profileLoading map 泄漏 → services.js 拆分重构时顺手清理
- ⚠️ api.js i18n：基础层无 `$t` 上下文 → Phase 2 错误处理统一化议题独立评审

### 1.12 配置 + 品牌更新
- [x] 完成（所有事项在 Phase 1.1 已提前完成）
- ✅ `main.go` banner → ComposeBoard（1.1 已改）
- ✅ `index.html` 标题/meta → ComposeBoard（1.1 已改）
- ✅ `api.js` token key → `composeboard_token`（1.1 已改）
- ✅ `config.yaml.template` 完整示例（1.1 已创建）
- ✅ 全局 `deployboard` 残留扫描：零结果

---

## Phase 2: 增值功能

### 2.1 设置页
- [x] 评估后关闭
- ℹ️ 高优先级功能（项目信息/Compose版本/Docker信息/语言切换）已在 Phase 1 的 Dashboard + Header 中实现
- ℹ️ 剩余项（生命周期钩子/开机自启动）依赖 Phase 3 部署向导，暂不单独做

### 2.2 SSE 日志增强
- [x] 完成
- ✅ 前端：SSE 断线自动重连（指数退避 1s→10s 封顶，持续重连）
- ✅ 前端：断线重连 banner（橙色脉冲动画提示重连进度）
- ✅ 前端：区分用户主动断开 vs 异常断线，仅异常触发重连
- ✅ 前端：onerror 按 readyState 区分 CLOSED/CONNECTING 两种场景
- ✅ 后端三层 Bug 修复：
  - L1: 续挂游标 RFC3339→Unix 时间戳，避免 Docker API 拒绝 since 参数
  - L2: `isAttachableLogStatus()` 门控，exited 容器不再盲目 attach
  - L3: 容器重建后 containerID 比对，主动断开旧 reader 防止挂在旧容器
- ✅ 单元测试覆盖：extractLogSinceValue / isAttachableLogStatus / 容器 ID 变更检测

### 2.3 .env 编辑器优化
- [x] 完成
- ✅ 后端：WriteEnvEntries 优先使用 Raw 保留原始格式（引号/行内注释/缩进）
- ✅ 前端：表格模式保存时只重建值被修改过的条目，未修改的保留原始 raw

### 2.4 Web 终端
- [ ] 未开始

### 2.5 错误处理 i18n 统一化
- [x] 完成
- ✅ api.js 硬编码中文（认证过期/请求失败）改为 `I18n.t()` 调用
- ✅ 新增 i18n key：`common.request_failed`、`logs.reconnect_*` 系列

### 2.6 services.js 组件拆分
- [x] 完成
- ✅ 组件拆分：ServiceTable / UpgradeModal / ConfirmDialog
- ✅ 页内收口：新增 `services-rules.js` 与 `services-ops.js`，规则层与操作态辅助层分离
- ✅ 顺手清理：T-3 extractVersion 私有 registry 误识别、T-4 profileLoading 泄漏
- ✅ 状态机重构：服务操作 loading 判定统一改为单服务实时轮询 `/api/services/:name/status`
- ✅ Profile 语义收口：三态改为配置启用态 `enabled / disabled`，移除 `partial / 补齐启用`
- ✅ 缓存一致性：单服务实时查询同步回写服务缓存，避免 15s 列表刷新把操作中服务刷回旧状态

---

## 变更日志

| 日期 | 任务 | 变更内容 |
|------|------|---------|
| 2026-04-21 | 1.1 | ✅ 项目初始化完成：Go module、目录结构、auth/host/config 迁移、i18n 基础设施（135 key 双语对称）、品牌更新、main.go 路由骨架、编译通过 |
| 2026-04-21 | 1.1-fix | ✅ Review 修复 9 项 |
| 2026-04-21 | 1.2 | ✅ compose/parser.go：YAML 解析 + label category + VarRefs + Profiles |
| 2026-04-21 | 1.3 | ✅ compose/env.go：行级模型 + ExpandVars 8 种 Compose 变量语法 |
| 2026-04-21 | 1.4 | ✅ compose/executor.go：v1/v2 自动检测 + CLI 封装 |
| 2026-04-21 | 1.5 | ✅ docker/client.go + cache.go：标签原生过滤 + 跨平台 Transport |
| 2026-04-21 | 1.6 | ✅ service/manager.go：声明态+运行态 LEFT JOIN + ImageDiff 检测 |
| 2026-04-21 | 1.7 | ✅ service/upgrade.go：异步 Pull + ApplyUpgrade + RebuildService |
| 2026-04-21 | 1.8 | ✅ service/state.go：状态基线 + 旧版迁移 + env 变更检测 |
| 2026-04-21 | 1.10 | ✅ API 层完成：7 个 API 文件 + main.go 全量串联，所有 TODO 已替换为实际路由 |
| 2026-04-22 | 1.11 | ✅ 前端重构完成：containers→services 架构转型、后端微调（settings/PUT env/PendingEnv）、SSE 日志、分档时序策略 |
| 2026-04-22 | 1.11-fix | ✅ 两轮外部 Review 修复：M-1/M-2 env 模型适配、T-7 poll 竞态、T-1 定时刷新、T-2 失败快反、T-6 SSE 限制、全量 i18n 搬家 |
| 2026-04-23 | 2.2 | ✅ SSE 日志增强：前端断线自动重连 + 后端三层 Bug 修复（since 格式/exited 门控/容器 ID 变更） |
| 2026-04-23 | 2.3 | ✅ .env 编辑器优化：WriteEnvEntries 用 Raw 保留引号、前端按需重建 |
| 2026-04-23 | 2.5 | ✅ 错误处理 i18n：api.js 硬编码中文→I18n.t()、新增 request_failed 等 key |
| 2026-04-23 | 2.6 | ✅ 服务页组件拆分收尾：ServiceTable / UpgradeModal / ConfirmDialog 落地，页内规则层与操作态辅助层分离 |
| 2026-04-23 | 2.6-state-machine | ✅ 服务页状态机重构：单服务实时轮询、服务缓存回写、Profile 配置启用态、移除 partial / 补齐启用 |
| 2026-04-23 | 2.6-warning | ✅ 服务运行态告警：新增 `startup_warning`，统一标记 unhealthy / created 超时 / restarting 超时 的异常服务 |
| 2026-04-23 | 2.6-actions | ✅ 服务行按钮矩阵收口：created / restarting 异常态放开日志、重启、停止、升级、重建等恢复入口 |
