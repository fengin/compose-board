# Phase 1.11 前端重构实施计划

## 目标

将前端从旧版 DeployBoard「容器管理」视角重构为 ComposeBoard「服务声明 + 运行态融合」视角，对接新后端 API。

## 核心变化分析

### API 路径变更（旧 → 新）

| 旧路径 | 新路径 | 说明 |
|--------|--------|------|
| `GET /api/containers` | `GET /api/services` | 返回 ServiceView[] |
| `POST /api/containers/:id/start` | `POST /api/services/:name/start` | **按服务名而非容器ID** |
| `POST /api/containers/:id/stop` | `POST /api/services/:name/stop` | 同上 |
| `POST /api/containers/:id/restart` | `POST /api/services/:name/restart` | 同上 |
| `GET /api/containers/:id/env` | `GET /api/services/:name/env` | 同上 |
| `GET /api/containers/:id/status` | **删除** | 新架构不再需要 |
| `GET /api/containers/service/:name/status` | **删除** | 新架构不再需要 |
| `POST /api/upgrade/:name/pull` | `POST /api/services/:name/pull` | 路径统一 |
| `GET /api/upgrade/:name/pull` | `GET /api/services/:name/pull-status` | 路径统一 |
| `POST /api/upgrade/:name/apply` | `POST /api/services/:name/upgrade` | 路径统一 |
| `POST /api/rebuild/:name` | `POST /api/services/:name/rebuild` | 路径统一 |
| — | `GET /api/profiles` | 🆕 Profile 列表 |
| — | `POST /api/profiles/:name/enable` | 🆕 启用 Profile |
| — | `POST /api/profiles/:name/disable` | 🆕 停用 Profile |
| `GET /api/logs/:service/history` | `GET /api/services/:name/logs?tail=200` | SSE 统一路由 |
| WebSocket `/api/logs/:service` | `GET /api/services/:name/logs?follow=true` | **WebSocket → SSE** |

### 状态同步时序变化（关键）

**旧方式**：操作后按容器 ID 轮询 `/api/containers/:id/status`，通过 `started_at` 判断 restart 完成。

**新方式**：操作后轮询 `GET /api/services`（整体刷新），按服务名匹配目标。原因：
1. 新 API 按**服务名**操作，不再暴露容器 ID
2. 重建/升级会产生新容器 ID，旧 ID 失效
3. 后端已做缓存 + 乐观更新，轮询 services 列表无性能问题

**完成判定逻辑**：

| 操作 | 完成条件 |
|------|---------|
| start | `status === 'running'` |
| stop | `status === 'exited'` |
| restart | `status === 'running'`（简化，不再判断 started_at） |
| upgrade | `status === 'running' && image_diff === false` |
| rebuild | `status === 'running' && env_diff === false` |

### ServiceView 数据模型（新）

```json
{
  "name": "mysql",
  "category": "base",
  "image_ref": "${MYSQL_IMAGE}",
  "image_source": "registry",  // "registry" | "build" | "unknown"
  "profiles": [],
  "depends_on": ["init-mysql"],
  "has_build": false,
  "container_id": "abc123",
  "status": "running",         // "running" | "exited" | "not_deployed"
  "state": "Up 3 hours",
  "ports": [{"host_port": 3306, "container_port": 3306}],
  "health": "healthy",
  "cpu": 2.5,
  "mem_usage": 134217728,
  "mem_limit": 536870912,
  "mem_percent": 25.0,
  "declared_image": "mysql:8.0.36",
  "running_image": "mysql:8.0.35",
  "image_diff": true,
  "env_diff": false
}
```

---

## 修改文件清单

### 1. `web/js/api.js` — API 客户端重写

- 所有容器路径 → services 路径，按服务名调用
- 删除 `getContainerStatus` / `getContainerStatusByName`（不再需要）
- 新增 `getProfiles()` / `enableProfile(name)` / `disableProfile(name)`
- 日志从 WebSocket 改为 SSE（EventSource）
- 新增 `getProjectSettings()` 用于设置页/仪表盘

### 2. `web/js/pages/containers.js` → `services.js` — 核心页面重写

**结构变化**：
- 文件改名 `containers.js` → `services.js`
- 组件改名 `ContainersPage` → `ServicesPage`
- 数据源从 `containers[]` 改为 `services[]`
- **新增分组逻辑**：必选服务按 category 分组，可选服务按 profile 分组
- **新增三态展示**：running ✅ / exited ⏹ / not_deployed ⭕
- **新增 Profile 分组标题**：含 `[启用]` / `[停用]` / `[补齐启用]` 按钮

**操作按钮规则**：

| 状态 | image_source | profiles | 可用操作 |
|------|-------------|----------|---------|
| running | 任意 | 任意 | 重启、停止、ENV、LOG |
| exited | 任意 | 任意 | 启动 |
| not_deployed | registry | 空 | 启动（必选服务） |
| not_deployed | registry | 非空 | 无按钮（通过 Profile 启用） |
| not_deployed | build | 任意 | 无按钮（标注 [本地构建]） |

**时序简化**：
- 操作后设置行级 `_loading = true`
- 3 秒后开始轮询 `GET /api/services`
- 按服务名匹配，达到目标状态后 `_loading = false` + Toast

**升级弹窗**：保留两阶段交互（pull → apply），改用服务名调用

**重建弹窗**：保留确认逻辑

### 3. `web/js/pages/dashboard.js` — 仪表盘适配

- `API.getContainers()` → `API.getServices()`
- 状态统计增加 `not_deployed` 计数
- 分组逻辑：复用 `category` 字段 + profile 分组
- 新增项目信息卡片（调用 `GET /api/settings/project`）

### 4. `web/js/pages/logs.js` — 日志页 WebSocket → SSE

- 服务列表改为调用 `API.getServices()` 获取所有已部署服务
- `new WebSocket(...)` → `new EventSource(...)` + `?token=` query
- SSE `onmessage` 处理 `data:` 行

### 5. `web/js/app.js` — 路由更新

- `/containers` → `/services`，组件 `ServicesPage`
- 保留其他路由不变

### 6. `web/index.html` — 品牌 + 脚本引用

- `containers.js` → `services.js`

### 7. `web/js/i18n.js` + `locales/*.json` — i18n 文本

- 所有硬编码中文改为 `$t('key')`
- 新增 profile 相关 key

### 8. `main.go` — 路由注册

- 注册新 API 路径（确认后端路由与前端一致）

---

## 执行顺序

1. **后端路由确认**：检查 main.go 当前路由是否与前端预期一致，如有差异先修后端
2. **api.js**：重写 API 客户端（所有后续页面依赖此）
3. **services.js**：核心页面重写（工作量最大）
4. **dashboard.js**：仪表盘适配
5. **logs.js**：日志页 SSE 切换
6. **app.js + index.html**：路由 + 引用更新
7. **i18n**：文本国际化（贯穿以上所有步骤）
8. **编译验证**：`go build` 确保 embed 通过

## 验证计划

- `go build` 编译通过
- 浏览器访问，逐页检查功能
- 特别验证：start/stop/restart 状态同步、profile 启用/停用、升级两阶段流程

---

> [!IMPORTANT]
> **需要确认**：当前 `main.go` 的路由注册与上述前端预期的路径是否一致？如果不一致需要先调整后端路由。
