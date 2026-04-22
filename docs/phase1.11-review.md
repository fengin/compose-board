# Phase 1.11 前端重构 Code Review 报告

**审查基线**: `front_implementation_plan-v2.md` + `PRODUCT_SPEC` + `DESIGN_DECISIONS` + `DEV_STANDARDS`  
**审查范围**: `web/js/api.js` / `pages/*.js` / `components/*.js` / `app.js` / `style.css` / `locales/*.json` + 后端同步改动  
**总体结论**: **🟢 通过 · 可进入 Phase 2**。阻塞项 B-1/B-2/B-3/B-4 与关键项 G-1~G-5 **全部落地**,但发现 **2 处字段模型不匹配**(M-1/M-2,近 Bug 级) 和 **大量硬编码中文**(M-6),建议在 Phase 2 开工前打一个"收尾补丁"。

---

## 一、✅ 做对的地方 —— 逐条验证上轮的 9 项阻塞/关键修复

| 项                        | 验证依据                                                                                                                                                                                          | 结论  |
| ------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --- |
| **B-1** Dashboard 项目信息   | 后端 `api/settings.go` 新增 `GetProjectSettings`,返回 `project_name / compose_command / compose_version / service_count / profile_names`;`main.go:166` 注册路由;`dashboard.js:11-34` + `fetchData` 正确消费 | ✅   |
| **B-2** `/api/env` 用 PUT | `main.go:160` `authorized.PUT("/env", ...)`;后端 `SaveEnvFile` 支持 `content` 或 `entries` 二选一                                                                                                     | ✅   |
| **B-3** sidebar 重构       | `sidebar.js`:`/` / `/services` / `/logs` / `/env`,logo "C",全部 `labelKey` 走 i18n                                                                                                               | ✅   |
| **B-4** 时序分档             | 同步档 `services.js:343` `await this.fetchServices()` 立即刷新;异步档 `services.js:442-471` `startAsyncPoll` 2s/次,5min 超时                                                                               | ✅   |
| **G-1** 错误码分支            | `services.js:346-353` 对 `services.start.build_not_supported / profile_required` 分支;后端 `services.go:84-100` `handleServiceError` 映射 409/404/400                                                | ✅   |
| **G-2** `pending_env` 恢复 | `manager.go:176-185` 写入 `PendingEnv []string`;`services.js:54-56` 表格列悬浮 title 显示具体变量;`services.js:413` rebuild 弹窗列出变更变量                                                                       | ✅   |
| **G-3** 状态/分组/角标 CSS     | `style.css:386-491` 全覆盖(`status-dot.not_deployed` / `.profile-group` / `.profile-status.enabled\|partial\|disabled` / `.badge-build` / `tr[data-status="not_deployed"]`)                      | ✅   |
| **G-4** 旧 URL 重定向        | `app.js:18` `{ path: '/containers', redirect: '/services' }`                                                                                                                                  | ✅   |
| **G-5** SSE 401 处理       | `logs.js:110-120` onerror 中检测 `API.isAuthenticated()` + 触发 `API.onUnauthorized`;`app.js:39-42` 全局切回登录态                                                                                        | ✅   |
| **i18n 对称**              | zh.json / en.json 完全对称(192 行 vs 192 行),结构一致                                                                                                                                                   | ✅   |

这一层做得扎实,不需要返工。

---

## 二、🟠 中度问题(建议 Phase 2 开工前修,近 Bug 级)

### M-1 env 页面字段模型与后端 `EnvEntry` 完全不匹配 🚨

后端 `compose.EnvEntry` 定义:

env.gointernal/compose

前端 `env.js` 实际用法:

env.jsweb/js/pages

- **`entry.comment` 字段后端根本不返回** → 表格模式下所有注释/空行都被当成 variable 行走 `v-else` 分支,显示空的 key/value 行
- `watch editMode` 重新解析时构造的也是 `{ key, value, comment }`,和后端模型不同
- **`saveEnv()` 表格模式保存时走 `{ content }` 而非 `{ entries }`**(`env.js:211`),白白浪费了后端的 entries 通路
- `getCurrentContent()` 拼接 `${key}=${value}` 丢失原始引号/空格/注释位置 — 后端虽有 Raw,但这里没用上

**影响**: 表格模式完全不可用,保存会破坏 .env 格式。`M-6` 需一起修。

### M-2 env 表格模式下 comment 行消失 → 保存会误删注释

承 M-1:因为注释映射错了,保存时 `getCurrentContent()` 生成的文本里**不会有 `#` 注释行**,写回后 .env 里所有注释都没了(备份只在 `.bak` 文件里)。

### M-3 代码中大量硬编码中文 —— 违反 DEV_STANDARDS

虽然 `zh.json`/`en.json` 完整对称,但代码中**没有真正使用**,英文环境下这些文案会漏出中文:

| 文件                                | 位置                               | 硬编码样例                                                            |
| --------------------------------- | -------------------------------- | ---------------------------------------------------------------- |
| `services.js:399/406/429/436/449` | Toast                            | `'升级已启动...'` / `'升级失败: '` / `'重建已启动...'` / `'操作超时'`              |
| `services.js:461-462`             | labels                           | `{ upgrade: '升级完成', rebuild: '重建完成' }`                           |
| `services.js:488/493`             | modal                            | `title: '停用 Profile'` / `btnText: '确认停用'`                        |
| `services.js:538-543`             | `statusLabel/profileStatusLabel` | 整个 map 硬编码中文                                                     |
| `logs.js:26/42/94/114`            | 模板 + Toast                       | `'⏹ 断开'/'▶ 连接'` / `'选择服务...'` / `'日志已连接'` / `'认证已过期...'`         |
| `logs.js:17`                      | 模板                               | `行数` label                                                       |
| `env.js` 全文                       | 几乎全部                             | `title`/`按钮`/`Toast` 均未经 `$t()`                                  |
| `dashboard.js:65/108/122-128`     | 模板 + 数据                          | `主机 IP` / `未发现任何服务...` / `categoryLabels: { base: '基础服务', ... }` |
| `dashboard.js:43`                 | 模板                               | `{{ ... }} 核`                                                    |

i18n key 都已经备好(例如 `services.start_success` / `services.profile.confirm_disable` / `logs.disconnected`),**接入非常便宜**,强烈建议一轮搬家。

### M-4 `statusLabel` / 状态文案重复定义 3 处

- `services.js:538` `statusLabel()` 硬编码
- `dashboard.js:77-79` 统计行模板内联中文
- `components/status-badge.js:16-24` labels map 硬编码

同一组状态语义散落 3 个文件,维护成本+不一致风险。建议**统一走 `$t('services.status.' + status)`**(key 已在 `zh.json:52-58`)。

---

## 三、🟡 轻度问题(建议一起修,成本低)

### T-1 services 页面缺定时自动刷新

`services.js:553-555` `mounted` 只调一次 `fetchServices`。用户停留在服务管理页时,CPU/内存/运行时长一直静态,需要手动点刷新。

`dashboard.js:185` 有 `setInterval(..., 15000)` 参考实现。建议:

> `data` 里已经声明了 `pollTimer: null`(line 280),但从来没有赋值 — 看起来是半实现状态。

### T-2 `startAsyncPoll` 失败侧无快速反馈

`services.js:442-471` 升级/重建失败(容器 crash、镜像不兼容等)时,`done` 判定条件永远不为 true → 要等 **5 分钟超时** 才提示。建议:

- 连续 3 次查到 `fresh.status === 'exited' || 'restarting'` 即认为失败
- 失败时读取容器最后几行日志给出 Toast(可选)

### T-3 `extractVersion` 对无 tag 私有 registry 返回错误

`services.js:531-536` `split(':')` 对 `localhost:5000/foo`(无 tag)返回 `5000/foo`。本项目都是公网镜像影响不大,但这是跟后端 `normalizeImage` 一致的坑,建议改为"最后一个 `/` 之后再找 `:`"的同一套逻辑。

### T-4 `profileLoading` map 在 profile 消失后不清理

`services.js:475/483` 用扩展运算符累加,Profile 被删除后对应 key 永久残留在 map 中。对长期运行的面板是潜在泄漏。建议每次 `fetchServices()` 完成后用新 profiles 的 key 重建:

### T-5 `env.js` 缺少标准头部注释 —— 违反 DEV_STANDARDS

env.jsweb/js/pages

其他 JS 文件都有"作者:凌封 / 网址:[https://fengin.cn"标准四行头,`env.js`](https://fengin.xn--cn",`env-h46mv33b5igc13cpx1f.js`/) 少了。

### T-6 SSE 重连时 token 过期处理不完整

`logs.js:110-120` 仅在 **第一次 onerror** 时判定 `isAuthenticated()`。但 EventSource 原生会**自动重连**,如果重连途中 token 过期,浏览器会一直发 401,这个检测只在业务代码一侧生效,不会主动 `close()`。虽然 `onerror` 每次都触发判定,但判定用的 `isAuthenticated` 只检查 `localStorage.token` 是否存在 —— token **存在但已过期** 时浏览器会无限循环重连(后端直接 401 + 中间件拒绝握手)。

最小防护:在 `onerror` 里加连续错误计数,超过 3 次即 `close()` + 手动重连按钮。

### T-7 upgrade 弹窗 pull 轮询闭包与 modal 对象共享

`services.js:384-392` `pollPull` 闭包引用 `m = this.upgradeModal`。如果用户**关闭弹窗后再对另一个服务打开**,同一个 `upgradeModal` 对象会被新的 `confirmUpgrade` 覆盖,旧 pollPull 写回的 `pullStatus` 可能污染新服务的状态。应该在 pollPull 里 `if (m.serviceName !== capturedName) return;` 做身份校验。

---

## 四、📝 细节与规范

### D-1 `services.js` 557 行单文件过大

模板 + 逻辑 + 3 个弹窗(env/confirm/upgrade)混在一起。维护成本偏高。建议 Phase 2 拆:

- `components/ServiceTable.js`(必选 + profile 共用表格)
- `components/UpgradeModal.js`
- `components/ConfirmDialog.js`(已有变体但未抽出)

非阻塞项,暂可接受。

### D-2 Dashboard 的 `goToService` 未传 service 参数

dashboard.jsweb/js/pages

点击某个服务卡片只跳到列表页,并未定位/高亮该服务。Phase 2 建议加 `query: { highlight: svc.name }` 并在 services.js 里滚动到对应行。

### D-3 模板中仍有少量 inline style

services.js 的 `style="margin-bottom:16px"` 之类散落几十处。Phase 2 重构时建议收拢到 CSS 类里。

---

## 五、最终建议与收尾补丁清单

### P0 必修(Phase 2 开工前,约 80-120 行改动):

1. **M-1 / M-2 env 页面字段适配**
   - `entry.type === 'comment'` 判定 + 用 `entry.raw` 显示/编辑
   - 表格模式保存走 `{ entries }` 而非 `{ content }`,利用后端 `WriteEnvEntries` 保持 Raw
2. **M-3 / M-4 硬编码中文搬家**
   - 优先搬 Toast 文案、modal 标题/按钮、`statusLabel`、`categoryLabels`、`profileStatusLabel`
3. **T-5 env.js 补头部注释**

### P1 建议修(成本低):

4. **T-1 services.js 加 10 秒定时刷新**(已有 `pollTimer` 占位符,接上即可)
5. **T-2 失败侧快速反馈**(连续 3 次 exited → 失败)
6. **T-6 SSE 重连次数限制**(≥3 次直接 close)
7. **T-7 upgrade pollPull 身份校验**

### P2 延后(Phase 2 或后续重构):

- D-1 services.js 拆分
- T-3 extractVersion 跟 normalizeImage 对齐
- T-4 profileLoading 清理
