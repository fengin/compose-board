# Phase 1.11 前端重构实施计划（终版）

> 合并 Review 反馈后的最终方案。

## 目标

将前端从旧版 DeployBoard「容器管理」视角重构为 ComposeBoard「服务声明 + 运行态融合」视角，对接新后端 API。

---

## 一、后端微调（前置，3 项）

### 1.1 B-1: 补 `GET /api/settings/project`（~30 行）

> Dashboard 项目信息卡片依赖此接口，Phase 1.11 顺手补，避免重写两次。

**api/settings.go**（新建）：
```go
func (h *Handler) GetProjectSettings(c *gin.Context) {
    project := h.Manager.GetProject()
    // 返回: project_name, project_dir, compose_file, compose_command,
    //       compose_version, service_count, profile_names
}
```

**main.go**：注册 `authorized.GET("/settings/project", handler.GetProjectSettings)`

### 1.2 B-2: `/api/env` POST → PUT（1 行）

`main.go:160` 改 `authorized.PUT("/env", handler.SaveEnvFile)`，对齐 DESIGN_DECISIONS §9 和 ARCHITECTURE §4.5。

### 1.3 G-2: ServiceView 恢复 `pending_env []string`

`service/manager.go` 的 `ListServices()` 中，把 `stateM.GetPendingEnvChanges()` 的具体变量名列表写入 `PendingEnv` 字段，前端可展示"哪些变量变了"。

---

## 二、API 路径映射（旧 → 新，已确认）

| 旧路径 | 新路径 | 说明 |
|--------|--------|------|
| `GET /api/containers` | `GET /api/services` | ServiceView[] |
| `POST /api/containers/:id/start` | `POST /api/services/:name/start` | **按服务名** |
| `POST /api/containers/:id/stop` | `POST /api/services/:name/stop` | 同上 |
| `POST /api/containers/:id/restart` | `POST /api/services/:name/restart` | 同上 |
| `GET /api/containers/:id/env` | `GET /api/services/:name/env` | 同上 |
| `GET /api/containers/:id/status` | **删除** | 不再需要 |
| `GET /api/containers/service/:name/status` | **删除** | 不再需要 |
| `POST /api/upgrade/:name/pull` | `POST /api/services/:name/pull` | 统一前缀 |
| `GET /api/upgrade/:name/pull` | `GET /api/services/:name/pull-status` | 统一前缀 |
| `POST /api/upgrade/:name/apply` | `POST /api/services/:name/upgrade` | 统一前缀 |
| `POST /api/rebuild/:name` | `POST /api/services/:name/rebuild` | 统一前缀 |
| `PUT /api/env` | `PUT /api/env` | B-2 修正后 |
| — | `GET /api/profiles` | 🆕 |
| — | `POST /api/profiles/:name/enable` | 🆕 |
| — | `POST /api/profiles/:name/disable` | 🆕 |
| — | `GET /api/settings/project` | 🆕 B-1 |
| WebSocket `/api/logs/:service` | `GET /api/services/:name/logs?follow=true` | **WS → SSE** |

---

## 三、状态同步时序（分档策略，B-4）

### 同步档（start / stop / restart）

后端 `StartContainer` / `StopContainer` / `RestartContainer` 返回时状态已确定，cache 已更新。

```
用户点击 → 行级 _loading=true → await API.xxxService(name) → 立即 fetchServices() → _loading=false + Toast
```

**无轮询**。HTTP 往返 ~200ms，体感即时。

### 异步档（pull / upgrade / rebuild）

后端 goroutine 异步执行，需轮询。

```
用户点击 → 行级 _loading=true → await API.xxxService(name) → 每 2s 轮询 fetchServices()
→ 达到目标状态 → _loading=false + Toast

目标状态判定：
- upgrade: status=running && image_diff=false
- rebuild: status=running && pending_env.length=0
- pull: 单独走 pull-status 接口
```

最长轮询 5 分钟超时。

---

## 四、错误码处理（G-1）

`api.js` 的 `request()` 方法增强：

```javascript
if (!resp.ok) {
    const data = await resp.json();
    const err = new Error(data.error || `请求失败 (${resp.status})`);
    err.code = data.code || '';
    err.status = resp.status;
    throw err;
}
```

services.js 中捕获并映射：

| HTTP | code | 前端行为 |
|------|------|---------|
| 409 | `services.start.build_not_supported` | Toast 提示"build 型服务需通过命令行启动" |
| 409 | `services.start.profile_required` | Toast 提示"请通过 Profile 管理启用" |
| 404 | `services.not_deployed` | Toast 提示"服务未部署" |

---

## 五、SSE 日志策略（G-5）

Phase 1.11 最低兜底：
- `EventSource` 原生自动重连
- `onerror` 中检测：若 `API.isAuthenticated()` 为 false → `source.close()` + 触发重新登录
- 连接状态 badge（已连接 ✅ / 已断开 ⭕）+ 手动重连按钮
- Phase 2.2 再做指数退避 + UI 优化

---

## 六、修改文件清单（13 个）

### 后端（3 个）

| # | 文件 | 动作 | 说明 |
|---|------|------|------|
| 1 | `internal/api/settings.go` | **新建** | B-1: `GetProjectSettings` |
| 2 | `main.go` | 修改 | B-1 注册路由 + B-2 POST→PUT |
| 3 | `internal/service/manager.go` | 修改 | G-2: 恢复 `PendingEnv []string` |

### 前端（10 个）

| # | 文件 | 动作 | 说明 |
|---|------|------|------|
| 4 | `web/js/api.js` | **重写** | 路径更新 + 错误码解析(G-1) + SSE 替代 WS |
| 5 | `web/js/pages/services.js` | **新建** | 替代 containers.js，核心页面 |
| 6 | `web/js/pages/dashboard.js` | 修改 | services 适配 + 项目信息卡片(B-1) |
| 7 | `web/js/pages/logs.js` | 修改 | WS→SSE + 401 处理(G-5) |
| 8 | `web/js/pages/env.js` | 修改 | PUT 对齐(B-2) |
| 9 | `web/js/app.js` | 修改 | 路由更新 + `/containers` redirect(G-4) |
| 10 | `web/js/components/sidebar.js` | 修改 | path + label + logo(B-3) |
| 11 | `web/js/components/status-badge.js` | 修改 | not_deployed 样式(G-3) |
| 12 | `web/css/style.css` | 修改 | profile 分组/三态徽章/本地构建角标(G-3) |
| 13 | `web/index.html` | 修改 | 脚本引用 containers→services |

### 不需要改的

- `web/js/locales/zh.json` — 已齐全（D-4）
- `web/js/locales/en.json` — 只需对齐校验（D-4）
- `web/js/i18n.js` — 无变化
- `web/js/pages/login.js` — 无变化

---

## 七、执行顺序

```
Phase A: 后端微调（10 分钟）
  1. manager.go — 恢复 PendingEnv
  2. settings.go — 新建 GetProjectSettings
  3. main.go — 注册路由 + PUT
  4. go build 验证

Phase B: 前端基础设施
  5. api.js — 重写（所有后续页面依赖）
  6. sidebar.js — 导航更新
  7. status-badge.js — not_deployed 样式
  8. style.css — 新 UI 样式

Phase C: 页面重构
  9. services.js — 核心页面（工作量最大）
  10. dashboard.js — 仪表盘
  11. logs.js — SSE 切换
  12. env.js — PUT 对齐

Phase D: 收尾
  13. app.js — 路由 + redirect
  14. index.html — 引用更新
  15. en.json — 对齐校验
  16. go build 最终验证
```

---

## 八、验证计划

- `go build` 编译通过
- 浏览器逐页检查：
  - Dashboard：项目信息卡片、服务状态统计、分组卡片
  - 服务管理：三态展示、Profile 分组、操作按钮矩阵
  - 特别验证：start（即时）、restart（即时）、upgrade（轮询）、rebuild（轮询）
  - 错误码：build 型启动 409、profile 服务启动 409
  - 日志：SSE 实时流、断线状态 badge
  - 环境配置：PUT 保存
