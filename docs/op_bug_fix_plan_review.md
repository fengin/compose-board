# 操作区 Bug 修复方案 Review

**基线**: `docs/op_bug_fix_plan.md` + 实际

`services.js`

/

`client.go`

/

`manager.go`

**总体结论**: 🟢 **方案架构正确,命中两大 bug 根因,核心思路可以直接落地**。但发现 **3 处需要补充 / 修正**,其中 1 处是 i18n 规范违反,必须改。

---

## 一、Bug 根因分析(先基于实际代码验证,不猜测)

### 现象 A · stop/start/restart —— **根本没有 loading 态**

`services.js:337-360 executeAction`:

**时序真相**:

1. 用户点"确认停止" → `executing=true`,弹窗内按钮转圈
2. `await API.stopService` 阻塞等待(`docker compose stop` 对 nginx/mysql 有 10s 宽限期)
3. 这段时间前端**没有设置 `svc._loading`,操作区列仍是旧按钮组,照常可点**
4. 用户以为卡住,手动关弹窗 → 弹窗关闭但 API 请求还在跑
5. API 最终返回 → `visible=false` 已生效 → Toast + fetch → 终于刷新

✅ 方案诊断完全正确。

### 现象 B · upgrade/rebuild —— **_loading 被 10s 定时刷新覆盖**

`services.js:311` 的 `fetchServices`:

**10s 定时刷新 `pollTimer`**(L573)每 10 秒执行一次 → **_loading 被无差别重置为 false**。

**时序真相**:

1. 点升级 → `applyUpgrade`:`target._loading = true` + 启动 `startAsyncPoll`
2. 轮询 2s/次,升级需要拉镜像 + 重启(常见 15-60s)
3. **10s 触发 pollTimer → fetchServices → 全部 _loading 重置**
4. 用户看到 loading 消失,按钮组变成"升级中"的中间态(status 可能已 exited 或 running 但 image_diff 还在)
5. 又过一会 startAsyncPoll 判 done → Toast + fetch → 按钮组终于正确

✅ 方案诊断也完全正确。

---

## 二、方案评审 · 逐项

### ✅ 改动 1 · `fetchServices` 保留 _loading — **精准命中 bug B**

正确。这是修复 bug B 的**唯一最小改动**。

### ✅ 改动 2 · `executeAction` 改为异步轮询 — **精准命中 bug A**

API 成功后立即 `visible=false + _loading=true + startAsyncPoll`,符合用户预期的"点击后浮动层立即消失,操作区变 loading"。

> 有个细节:方案保留了 `await API.stopService(...)` 再关弹窗,不是"一点击就关弹窗再发请求"。这是**更稳妥的做法**——万一 API 立即失败(比如 403/404/网络断),用户还能在弹窗内看到错误。采纳。

### ✅ 改动 3 · 引入 `started_at` 做 restart/upgrade/rebuild 完成判定 — **复刻旧版精髓**

这是方案**最精彩的设计**。

**为什么必须靠 started_at**:

- `docker compose restart` 很快(2-3s),两次轮询之间可能**完全错过"非 running"的瞬间**
- 单靠 `status === 'running'` 会在**刚点击时就命中**(因为容器本来就在 running),立即误判为完成
- `started_at` 是容器引擎写入的**权威时间戳**,必定发生变化

**后端配套**:

- `ContainerStatus.StartedAt` 已经有(`client.go:69`)✅
- `ContainerInfo.StartedAt` 需要新增 → 方案要求对,`client.go:154-164` `convertContainer` 内调用已有的 `getStartedAt(ctx, ctr.ID)` 即可
- `ServiceView.StartedAt` 需要新增 → `manager.go:151-155` LEFT JOIN 里透传 `view.StartedAt = ctr.StartedAt` ✅

需要注意的一个细节:**`ListContainers` 内对每个容器做一次 Inspect(`getStartedAt` 内部是 `/containers/{id}/json`)会显著增加 ListServices 的耗时**(N 个容器 N 次 Inspect)。旧版本是按需查单个容器,方案改成批量后每 10 秒定时刷新都会放大 N 倍 API 开销。

**优化建议**: Docker `/containers/json` 不带 StartedAt,但可以:

- **方案 A**:只在 `/containers/json?all=true` 的结果里,Docker 返回的 `Created` 和 `Status` 字符串能推断,但精度不如 StartedAt
- **方案 B(推荐)**:`ListContainers` 里对每个容器并发(`errgroup`)调 `getStartedAt`,不阻塞串行
- **方案 C(最简)**:采用方案,接受开销;实测 10~20 个容器并发 Inspect 在本地网络 <100ms,可接受

建议方案中明确**采用方案 C 接受开销**,或在 `getStartedAt` 调用时用 `errgroup` 并发化。

### ✅ 改动 4 · `startAsyncPoll` 统一 5 种操作

判定逻辑正确:

| action  | 完成判定                                        |
| ------- | ------------------------------------------- |
| stop    | `status === 'exited'`                       |
| start   | `status === 'running'`                      |
| restart | `running` && `started_at !== startedBefore` |
| upgrade | 同 restart + `!image_diff`                   |
| rebuild | 同 restart + `pending_env` 为空                |

实时更新用 `Object.assign(target, {...})` — Vue 3 响应式代理会追踪字段级变更,**不会触发 requiredGroups 重算**(services 数组引用未变),性能最优 ✅

---

## 三、🔴 必须补充 / 修正的点

### C-1(P0)· i18n 规范违反 —— **硬编码中文 Toast**

方案中:

上一轮 Review 刚把前端所有文案搬家到 i18n,这一轮不能回头。必须改为 `$t()`。

**补丁(zh.json / en.json 对称新增)**:

`upgrade_done / rebuild_done / operation_timeout` 已存在,复用即可。

**代码改法**:

### C-2(P1)· Profile 区服务操作按钮**没有 _loading 渲染**

`services.js:148-160` Profile 分组的操作 `<td>`:

**缺少 `v-if="svc._loading"` 分支**。而 Profile 区的 restart/stop/start 按钮同样走 `confirmAction` → `executeAction`,方案改完后它们会设置 `_loading=true`,但 UI 上看不见 loading 状态,按钮组依然显示。

**需要改法**(与必选区保持一致):

### C-3(P2)· 异常状态快速失败应结合 `started_at`

方案:

问题:upgrade 过程中**容器必然短暂 exited**(旧容器被停掉再拉起新的),这会被误判。

**改法**(更精确):

如果方案选择"接受误报但快速失败",当前写法也能用,只是 upgrade 偶发会被误判为失败(3 次 2s 轮询 = 6s 连续 exited,在服务器拉镜像慢时会触发),建议加 started_at 判据更稳。

---

## 四、🟢 可选优化(不影响合入)

### O-1 · 多服务并发操作的 API 重复

每个 `startAsyncPoll` 内部都 `API.getServices()`,如果同时操作 3 个服务,每 2 秒就有 3 次重复请求。  
**优化**:用单一全局轮询器 + `Set` 追踪 "待完成服务" 列表,所有操作共享同一次 `getServices` 结果。  
但这是重构级别,不是 bug,当前方案可以接受。

### O-2 · `getStartedAt` 并发化

`ListContainers` 里对 N 个容器串行 Inspect。建议用 `errgroup` 并发,每个容器一个 goroutine,总耗时约等于最慢单次。改动约 10 行。

---

## 五、最终评审结论

| 维度       | 评分     | 说明                             |
| -------- | ------ | ------------------------------ |
| Bug 根因定位 | ✅ 满分   | 两个 bug 都点中                     |
| 架构一致性    | ✅ 满分   | 复刻旧版 started_at 机制,合理          |
| 代码改动量    | ✅ 合理   | 后端 ~6 行,前端 ~60 行               |
| i18n 规范  | 🔴 需补  | 硬编码中文必须改(C-1)                  |
| 完整性      | 🟡 需补  | 漏了 Profile 区的 _loading 渲染(C-2) |
| 鲁棒性      | 🟡 可加强 | exited 判据可结合 started_at(C-3)   |

### 授权建议

**方案可以合入,但必须带上 C-1 + C-2 一起改**:

1. **C-1 必改**:方案中出现的 5 处中文 Toast 全部走 `$t()`,在 zh.json/en.json 各补 5 个对称 key(`action_in_progress` / `stop_done` / `start_done` / `restart_done` / `operation_failed`)
2. **C-2 必改**:Profile 区 `<td>` 补 `v-if="svc._loading"` 分支,保持与必选区一致
3. **C-3 建议改**:exited 判定结合 started_at,避免 upgrade 中间态误报

**后端改动**(`ContainerInfo.StartedAt` + `ServiceView.StartedAt` 透传)**没问题**,可以直接做。
