# Phase 1.11 前端重构方案评审

**评审结论**: 方向正确,核心把握到位,但**有 4 处阻塞性不一致 + 5 处关键遗漏必须先对齐**,否则开工后会返工。

---

## 一、🔴 阻塞性问题(开工前必须裁决)

### B-1 `GET /api/settings/project` 后端尚未实现 —— 方案第 3/6 节

- 方案说 Dashboard 调用 `GET /api/settings/project` 取"项目信息卡片",但 **`main.go` 目前没注册这条路由**(属于 Phase 2.1 设置页的范畴,见 IMPLEMENTATION_PLAN §2.1)
- 结果:按方案写 Dashboard 会直接 404
- **二选一决策**:
  - **A** Phase 1.11 顺手补一个最小 `/api/settings/project`(返回 `project_name / project_dir / compose_file / compose_command / service_count / profiles`),前端一步到位
  - **B** Dashboard 项目卡片延到 Phase 2.1,前端 1.11 复用 `GET /api/services` + `GET /api/host/info` 自行聚合
- **建议 A**,约 30 行后端代码,避免 Dashboard 重写两次

### B-2 `/api/env` HTTP 方法冲突

| 来源                                          | 方法       |
| ------------------------------------------- | -------- |
| `DESIGN_DECISIONS §9` / `ARCHITECTURE §4.5` | **PUT**  |
| 后端 `main.go:160` 实际注册                       | **POST** |
| 旧前端 `api.js:102` 实际用                        | **PUT**  |
| 方案文档                                        | 没提       |

三方不一致。需要先决定:改后端用 PUT 对齐设计文档(推荐),还是改前端用 POST。**方案里必须显式列出**,否则 1.11 写完一测肯定报 405。

### B-3 sidebar 组件整个被漏了

方案的"修改文件清单"只提了 `app.js + index.html`,漏了 `web/js/components/sidebar.js`:

sidebar.jsweb/js/components

- `path` 需改为 `/services`
- `label` 改为 i18n key `$t('nav.services')`(zh.json 已经有 `nav.services: "服务管理"`)
- logo 字母 "D" 可能也要换(这是 DeployBoard 残留)
- 还要补 Phase 2 之后的 profile / terminal / settings / deploy 图标(或先隐藏)

### B-4 "3 秒后开始轮询"对同步操作是体感倒退

方案第 39 行的简化时序 **把所有操作都当异步处理**,但后端实现其实分两档:

| 操作                   | 后端实现                                                                | 前端应当                                  |
| -------------------- | ------------------------------------------------------------------- | ------------------------------------- |
| start/stop/restart   | **同步** `StartContainer/StopContainer` 返回后状态已确定 + cache override 已更新 | 操作返回立即刷新一次 services 列表即可,**无需 3 秒延迟** |
| pull/upgrade/rebuild | **异步** goroutine + `ForceRefresh()`                                 | 需要轮询(升级 2~5 分钟,重建 30s~2min)           |

统一"3 秒"让用户点"启动"按钮后 3 秒 loading,体验远差于旧版。建议:

- **同步档**: action 成功后立即 `fetchServices()`,行级 `_loading` 不超过 800ms(涵盖 HTTP 往返)
- **异步档**: 进入轮询(间隔 2s,最长 5min);pull 单独走 `pull-status` 接口

---

## 二、🟠 关键遗漏(会影响验收)

### G-1 错误码分支处理完全没提 —— 破坏 S-4 的设计

后端 Phase 1.10 修复中特意引入了错误码机制:

| HTTP | `code`                               | 场景                   |
| ---- | ------------------------------------ | -------------------- |
| 409  | `services.start.build_not_supported` | build 型未部署服务点启动      |
| 409  | `services.start.profile_required`    | Profile 可选服务点启动      |
| 404  | `services.not_deployed`              | 对未部署服务做 stop/restart |

方案的 API 客户端章节(`api.js` 第 80-85 行)完全没提 `code` 字段处理。`zh.json` 已备好 `services.start.build_not_supported` 文案,但方案没说前端要在拦截器里把 `err.code` 映射到对应 i18n key。**P0 级体验影响**。

### G-2 `pending_env` → `env_diff` 语义简化,方案没说明 UI 影响

- 旧模型: `pending_env: ["MYSQL_VERSION", "REDIS_VERSION"]` — 可列出具体变更变量
- 新模型: `env_diff: true` — 只知有变更,**后端已丢失具体变量名**

方案的 ServiceView 模型示例保留了 `env_diff: false`,但没说 UI 上怎么展示"哪些变量变了"。旧 `containers.js:45-49` 是用 `ctr.pending_env.join(', ')` 作 title 悬浮提示的,新版只能显示"配置已变更"。

**二选一**:

- 接受降级,UI 只显示布尔标识
- 要求后端 `ServiceView` 恢复 `pending_env []string` 字段(代价 10 行,`service/manager.go:170-177` + state 已有 `GetPendingEnvChanges` 现成数据)

**建议后者**,成本极低,信息量差距很大。

### G-3 `status-badge.js` 和 `style.css` 没提

- `status-badge` 组件目前只支持 `running / exited`,需加 `not_deployed` 样式(⭕ 灰色)
- Profile 分组标题、`[本地构建]` 角标、三态徽章(`enabled ✅ / partial ⚠ / disabled ⭕`)都是新增 UI 元素,**没有对应 CSS**
- style.css 至少要加: `.profile-group-header` / `.service-row[data-status="not_deployed"]` / `.badge-build-local` / `.profile-status-*`

### G-4 `/containers` 旧 URL 的重定向未处理

老用户刷新浏览器书签 `http://.../containers` 会 404 到 SPA 兜底(登录页)。建议 `app.js` 路由表补:

### G-5 SSE 日志重连策略不清晰

方案只说"EventSource + ?token=",但没说:

- EventSource 原生有自动重连,但 **token 过期后 401 会让浏览器一直重试** → 需手动 `source.close()` + 触发重新登录
- PROGRESS 里 `2.2 WebSocket 断线重连` 说是 Phase 2 才做。那 1.11 的日志页"断线会发生什么"需要一个最低兜底(简单的状态 badge + 手动重连按钮)
- 或者干脆在 1.11 里把"原生自动重连 + 401 手动 close"做掉,Phase 2.2 只做 UI 优化

---

## 三、🟡 次要细节(建议一并纠正)

| #   | 问题                                     | 建议                                                                                                          |
| --- | -------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| D-1 | `port` 类型示例写成数字 `3306`                 | 后端实际返回字符串 `"3306"`,文档里改成 `"3306"` 以免误导                                                                      |
| D-2 | "完成判定"表中 `rebuild: env_diff === false` | 准确,但需注意:rebuild 对 `build:` 型不做 state 更新(新代码仅加了 log 警告),`env_diff` 可能保持 true,需在 rebuild 后强制刷 state           |
| D-3 | 方案第 113 行"升级弹窗保留两阶段"                   | 没说明 pull 失败重试 / 用户取消 怎么处理。后端目前没 cancel API,前端只能"忽略状态"。建议方案补一句                                               |
| D-4 | i18n 覆盖度                               | `zh.json` 已有完整 services/profile/terminal/settings key,**方案不需要再说"新增 profile 相关 key"** — 已经齐了,只需 en.json 对齐校验 |
| D-5 | 方案第 144-146 行 "main.go 路由注册确认"         | 我已帮你对过,后端路由与方案前端预期基本一致,除了 B-1 和 B-2。这一项可直接标注"已确认"并列出 B-1/B-2 两处差异                                           |

---

## 四、✅ 做对了的地方(不用改)

1. **API 路径映射表** — `/api/containers` → `/api/services`、`/api/upgrade/*` → `/api/services/:name/*` 基本准确
2. **WebSocket → SSE 识别** — 与后端 `logs.go` 实现一致
3. **三态展示 + Profile 分组** — 与 `DESIGN_DECISIONS §2 / PRODUCT_SPEC §3.3.3` 完全对齐
4. **操作按钮规则矩阵** — 覆盖了 build 型拦截、profile 服务无按钮、必选服务 up -d 四种组合
5. **按服务名操作(非容器 ID)** — 抓住了新架构的核心范式变化
6. **执行顺序** api.js → services.js → dashboard.js → logs.js → app.js — 依赖关系正确

---

## 五、修订建议(按优先级)

**开工前必须决定的 4 件事**:

1. **B-1** Dashboard 项目信息卡片:**Phase 1.11 顺手补后端 `/api/settings/project`** 还是延期?(建议补,30 行)
2. **B-2** `/api/env` 用 **PUT**(改后端)还是 **POST**(改前端)?(建议 PUT,对齐文档)
3. **B-4** 时序:`start/stop/restart` 改为**立即刷新**,不轮询;`pull/upgrade/rebuild` 走轮询
4. **G-2** `pending_env` 字段:**后端恢复**(10 行代价,信息量价值高),还是**前端接受降级**?(建议恢复)

**补进方案的文件清单**:

5. `web/js/components/sidebar.js` — path + label + logo
6. `web/js/components/status-badge.js` — `not_deployed` 样式
7. `web/css/style.css` — profile 分组 / 三态徽章 / 本地构建角标
8. `web/js/api.js` — 加 `err.code` 解析,映射到 i18n key
9. `web/js/app.js` — `/containers` 路由 redirect

**方案里该删的**:

10. "3 秒后开始轮询"(替换为分档策略)
11. "新增 profile 相关 key"(zh.json 已齐,只需 en.json 对齐)

---

## 六、修正后的文件清单建议

| #   | 文件                           | 动作                                         | 备注          |
| --- | ---------------------------- | ------------------------------------------ | ----------- |
| 1   | `api.js`                     | 重写                                         | +错误码解析      |
| 2   | `pages/services.js`          | 新建(替换 containers.js)                       | 大头          |
| 3   | `pages/dashboard.js`         | 适配                                         | 依赖 B-1 决策   |
| 4   | `pages/logs.js`              | SSE 切换                                     | +401 处理     |
| 5   | `pages/env.js`               | 方法对齐                                       | 依赖 B-2 决策   |
| 6   | `app.js`                     | 路由                                         | +redirect   |
| 7   | `components/sidebar.js`      | label + path                               | ⚠️ 方案漏了     |
| 8   | `components/status-badge.js` | not_deployed 样式                            | ⚠️ 方案漏了     |
| 9   | `css/style.css`              | 新 UI 样式                                    | ⚠️ 方案漏了     |
| 10  | `index.html`                 | 脚本引用                                       | 小           |
| 11  | `locales/en.json`            | 对齐 zh.json                                 | 脚本可校验       |
| 12  | `main.go`(可选)                | 补 `/api/settings/project` + `/api/env` PUT | 取决于 B-1/B-2 |
| 13  | `service/manager.go`(可选)     | 恢复 `pending_env []string`                  | 取决于 G-2     |

---


