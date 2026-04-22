# 第二轮修复 Review 复核报告

**基线**: 上一轮报告中的 P0(M-1/M-2/M-3/M-4/T-5) + P1(T-1/T-2/T-6/T-7) 共 9 项  
**结论**: 🟡 **6/9 已修 + 2/9 部分修 + 1/9 未修**。核心模型问题 M-1/M-2 已彻底解决,可以进入 Phase 2;但 i18n 搬家只做了一半,建议补一个"纯文案补丁"再往下推进。

---

## 一、✅ 完全修复(6 项)

| #       | 项                 | 验证依据                                                                       | 结论  |
| ------- | ----------------- | -------------------------------------------------------------------------- | --- |
| **M-1** | env 模型字段适配        | `env.js:55-74` 三态渲染 `variable/comment/blank`,用 `entry.raw` 展示注释行           | ✅   |
| **M-2** | 表格模式走 entries     | `env.js:220-228` 显式 `{ entries }`,同步后端 `PUT /api/env`                      | ✅   |
| **M-4** | status-badge 去硬编码 | `status-badge.js:14-21` `services.status.*` key + fallback 原状态码            | ✅   |
| **T-1** | 列表 10 秒定时刷新       | `services.js:573` `setInterval(fetchServices, 10000)` + `beforeUnmount` 清理 | ✅   |
| **T-2** | 异步操作失败快速反馈        | `services.js:448,470-481` 连续 3 次 `exited/restarting` 判失败                   | ✅   |
| **T-5** | env.js 补头部注释      | `env.js:1-7` 已补                                                            | ✅   |

---

## 二、✅ 基本修复(2 项 · 有细节)

### T-6 · SSE 重连限制 ✅ 方案可用,但有细节可改

`logs.js:63,95,121` 实现了 `errorCount >= 3 → disconnect`。细节观察:`onerror` 内先判 `!isAuthenticated()`,401 分支直接断,这是对的;但普通错误场景 `this.connected = false` 放在最后,而浏览器 EventSource 会自动重连到 `errorCount++` 到 3 才彻底 close,中间会有 2 次重连尝试——**这是预期行为,符合 T-6 的设计**,通过。

### T-7 · pollPull 身份校验 ✅

`services.js:384-387` 用 `capturedName` 做防闭包污染,并在每次轮询前校验弹窗 serviceName。通过。

---

## 三、⚠️ 未完成(1 项 P0) — **需要在 Phase 2 开工前补**

### M-3 · 硬编码中文搬家 —— **只做了 services.js 与 status-badge.js,env.js / logs.js / api.js 未搬家**

用正则 `[\u4e00-\u9fa5]` 对 `web/js` 四个关键文件扫描的结果:

#### (1) `env.js` —— 整张页面模板几乎都是硬编码中文(P0)

| 位置               | 硬编码文案                | 应替换为                                             |
| ---------------- | -------------------- | ------------------------------------------------ |
| `env.js:13`      | `.env 配置管理`          | `$t('env.title')`                                |
| `env.js:16`      | `↻ 刷新`               | `$t('services.refresh')`                         |
| `env.js:18`      | `💾 保存`              | `$t('env.save')`                                 |
| `env.js:25`      | `加载中`                | `$t('common.loading')`                           |
| `env.js:34,39`   | `表格模式 / 原文模式`        | `$t('env.table_mode')` / `$t('env.text_mode')`   |
| `env.js:41`      | `● 有未保存的修改`          | 需新增 `env.unsaved`                                |
| `env.js:50-51`   | `变量名 / 值`            | `$t('env.key')` / `$t('env.value')`              |
| `env.js:95`      | `变更确认`               | 需新增 `env.diff_title`                             |
| `env.js:108`     | `无变更`                | 需新增 `env.no_diff`                                |
| `env.js:112`     | `取消`                 | `$t('common.cancel')`                            |
| `env.js:115`     | `确认保存`               | 需新增 `env.confirm_save_btn`(或复用 `common.confirm`) |
| `env.js:125`     | `✅ 保存成功`             | 复用 `env.save_success`                            |
| `env.js:131-132` | ".env 文件已保存并创建备份..." | 需新增 `env.save_tip_message`                       |
| `env.js:135-136` | `我知道了 / 前往服务管理`      | 需新增 key                                          |
| `env.js:169,239` | Toast 文案             | 复用 `env.save_failed` 等                           |

#### (2) `logs.js` —— 模板 3 处硬编码(P0)

| 位置           | 硬编码                 | 应替换为                                                           |
| ------------ | ------------------- | -------------------------------------------------------------- |
| `logs.js:17` | `行数`                | 需新增 `logs.tail_lines`                                          |
| `logs.js:26` | `⏹ 断开 / ▶ 连接`       | 复用 `terminal.connect/disconnect`,或新增 `logs.connect/disconnect` |
| `logs.js:42` | `选择服务并点击"连接"查看实时日志` | 需新增 `logs.empty_hint`                                          |

#### (3) `api.js` —— 运行时错误文案硬编码(P1)

| 位置          | 硬编码                              | 建议                                                                                                                                      |
| ----------- | -------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| `api.js:63` | `throw new Error('认证已过期,请重新登录')` | 已有 `auth.token_expired` 可用;但 api.js 属基础层无 $t 上下文,建议直接 `throw new Error('auth.token_expired')` 让调用方 `$t(err.message)`,或保留中文(Phase 2 再统一) |
| `api.js:69` | `请求失败 (${resp.status})`          | 同上                                                                                                                                      |

#### (4) `services.js` —— **无硬编码模板文案** ✅(扫到的 32 条中文全部是代码注释,不计)

---

## 四、🟡 历史残留未修(1 项 P2,可延后)

### R-2 · `WriteEnvEntries` 未用 `Raw` 保留引号/空格

`compose/env.go:113-119` 目前:

env.gointernal/compose

前端 `env.js:224` 已经主动填充 `raw: ${key}=${value}`,即使后端支持 Raw 也写不进去(因为变量行的 Raw 被前端重置了)。  
**不会引起功能回归**,引号丢失是历史行为,Phase 2 和 "env 编辑器支持多行/带引号变量" 一起做即可。

---

## 五、对称性核查

### zh.json ⇔ en.json

逐层对比 `dashboard / services / env / logs / terminal / settings / common / auth / nav / app`,**所有 key 完全对称** ✅。两个文件都是 207 行、相同结构。

### 后端错误码 ⇔ 前端 key

- `services.start.build_not_supported` ✅ (前后端都有)
- `services.start.profile_required` → 前端 `api.js` 抛 `err.code`,`services.js:349` 直接显示 `e.message`(后端已返回带服务名的本地化文本),OK
- 其他错误码走 `Toast.error(common.error + e.message)`,通过

---

## 六、最终建议与行动项

### A. Phase 2 开工前必补(约 2 小时工作量)

**"纯文案补丁",不涉及逻辑改动**:

1. **env.js 模板 i18n 化**(改动 ~20 行 + 新增 ~8 个 key)
2. **logs.js 模板 i18n 化**(改动 ~3 行 + 新增 ~3 个 key)
3. **zh.json + en.json 对称新增 key**:

### B. 可选 / 延后

- R-2(env.go WriteEnvEntries 用 Raw) — Phase 2
- api.js 运行时错误文案搬家 — 涉及架构决策(是否让基础层感知 i18n),建议独立设计一次再统一做

### C. 非必要但建议

- `env.js:196-212` Diff 计算采用 `oldSet.has` 的简单对比,删除后立即加入的"修改同一行"情况会被当成"删 + 加"两行显示。属于 Phase 2 用户体验优化范畴,可延后。
- `dashboard.js:175` `goToService(svc)` 跳转后未聚焦该服务(上轮 D-2 已记录)。

---

## 七、是否可进入 Phase 2?

**建议先打 A 组补丁再推进**。理由:

1. 这是**上一轮 Review 明确要求的 P0 项**,未完成意味着前端仍有 ~30 处非 i18n 文案。
2. 补丁纯粹是文案搬家,**零逻辑风险**,不会影响其他代码。
3. 若 Phase 2 包含"多语言切换"类功能,现在欠账会被放大。
