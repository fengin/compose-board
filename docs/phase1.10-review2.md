# ComposeBoard Phase 1.2–1.10 二次 Review 验证报告

**核查基线**: 原 Review 报告 `docs/phase1.10-review.md`(5 严重 + 14 中度 + 12 轻度 + 2 文档冲突 = 33 项) **核查结论**: **主体 18 项完全到位,7 项部分/折中,8 项按 P2 延后合规。有 1 项严重问题以折中路线"骗过编译但运行时仍可能失败",需立即关注**。

---

## 一、🔴 严重问题(5 项)

| #       | 问题                 | 状态            | 核查依据                                                         |
| ------- | ------------------ | ------------- | ------------------------------------------------------------ |
| **S-1** | Windows Named Pipe | ⚠️ **折中,未真修** | `transport_windows.go` 用 TCP 2375 代替 winio                   |
| **S-2** | PullImage ctx 丢弃   | ✅ **已修**      | `upgrade.go:77` 正确传 ctx,`executor.go:160` 用 `CommandContext` |
| **S-3** | timeout 数值错误       | ✅ **已修**      | `logs.go:35` `10*time.Second`                                |
| **S-4** | StartService 未拦截   | ✅ **已修**      | `lifecycle.go:51,59` 完整拦截 build + profile,返回 409 错误码         |
| **S-5** | 项目名目录名兜底           | ✅ **已修**      | `main.go:69-73` + `filepath.Join`                            |

### ⚠️ S-1 需要重点关注的隐患

transport_windows.gointernal/docker

**问题**:

1. 注释自述"**Docker Desktop 默认暴露 TCP 2375**"是错误理解 —— Docker Desktop **默认不暴露** TCP 2375,用户必须在 Settings → General 手动勾选 "Expose daemon on tcp://localhost:2375 without TLS"
2. 勾选该选项会**禁用 TLS**,Docker 官方在设置项旁边明确标注安全警告("Exposing daemon on TCP without TLS helps legacy clients connect to the daemon. It also makes yourself vulnerable to remote code execution attacks. Use with caution.")
3. `go.mod` 里**确实没有** `github.com/Microsoft/go-winio` 依赖 —— 这是 `ARCHITECTURE §2.4` 和 `IMPLEMENTATION_PLAN §1.5` 明确要求的
4. 注释写"Phase 3 引入 go-winio"本质是把 S-1 **降级为技术债延后**,不是"修复"

**后果**:

- Windows 开发者首次启动会遇到 `dial tcp 127.0.0.1:2375: connect: connection refused`
- 照着错误提示开启 TCP 后,本地 Docker 暴露在整个 localhost,任何浏览器扩展或同机应用都能发起 container create → 突破容器沙箱
- 当下规避成本极低:`go get github.com/Microsoft/go-winio` + 2 行 `winio.DialPipe` 调用

**建议**:必须在 Phase 1.11 之前真正引入 winio。**拆文件的好处已经享受了(build tag 生效),只差 import 和 2 行代码的事**。

---

## 二、🟠 中度问题(14 项)

| #        | 问题                          | 状态                 | 依据                                                                  |
| -------- | --------------------------- | ------------------ | ------------------------------------------------------------------- |
| **M-1**  | `${VAR:?error}` 语义          | ✅ **已修**           | `env.go:202-204` 返回原始 match                                         |
| **M-2**  | .env 引号剥离                   | ✅ **已修**           | `env.go:246-253` `stripQuotes`                                      |
| **M-3**  | Executor 加 ctx              | ✅ **已修**           | Up/Pull/Stop/Rm 全接口带 ctx + 默认 10 分钟兜底                               |
| **M-4**  | --profile 位置                | ✅ **已修**           | `buildArgs:189-191` 放在子命令前                                          |
| **M-5**  | -f 显式传入                     | ✅ **已修**           | `SetComposeFile` + `manager.go:88` 注入                               |
| **M-6**  | normalizeImage 带端口 registry | ✅ **已修**           | `manager.go:225-242` 按最后一个 `/` 判断                                   |
| **M-7**  | digest 处理                   | ❌ **未修**           | `normalizeImage` / `imagesMatch` 均无 `@sha256:` / 裸 ID 兜底            |
| **M-8**  | state 原子写                   | ✅ **已修**           | `state.go:245-251` Linux 直接 rename、Windows 先 Remove                 |
| **M-9**  | Rebuild 对 build 型           | ❌ **未修**           | `upgrade.go:150-171` 既无拦截也无警告日志                                     |
| **M-10** | Handler 业务下沉                | ✅ **大部分修**         | 新增 `lifecycle.go` + `handleServiceError` 错误码映射                      |
| **M-11** | `config.C` 下沉               | ✅ **按约定修**         | `api/env.go`、`api/host.go` 已去;auth.go 保留(约定)                        |
| **M-12** | 日志路径                        | ⚠️ **代码按约定、文档未同步** | 代码保留 SSE,但 `DESIGN_DECISIONS §9` / `ARCHITECTURE §4.6` 仍写 WebSocket |
| **M-13** | DisableProfile --profile    | ✅ **已修**           | `profiles.go:138-147` Stop/Rm 都传 profiles                           |
| **M-14** | transport 独立文件              | ✅ **已修**           | `transport_linux.go` + `transport_windows.go` 按 build tag 拆分        |

### ❌ M-7 / M-9 没做到位

**M-7 Digest 处理** —— 我们上一轮约定 "按对方 P2 延后,但加一行 `sha256:` 前缀兜底,成本接近 0"。验证:

manager.gointernal/service

**未加**建议的那行兜底:

—— 容器本地 tag 被 `docker image prune` 清理后,Docker API 会退化返回纯 image ID,继续误报。

**M-9 Rebuild 警告日志** —— 对方明确同意"保留行为 + 加警告日志"。验证:

upgrade.gointernal/service

**没有任何 build 型服务的识别**,既没拦截(允许 rebuild)也没加我们约定的警告日志("该服务为 build 型,若本地镜像不存在将触发重新构建")。这是承诺未兑现。

### ⚠️ M-12 代码通过约定,但三份文档没改

代码层面保留 SSE + `/api/services/:name/logs` **符合**上一轮决议。但——

| 文档                         | 当前内容                                                         | 问题  |
| -------------------------- | ------------------------------------------------------------ | --- |
| `DESIGN_DECISIONS §9`      | `GET (WS) /api/logs/:service` + `/api/logs/:service/history` | 未同步 |
| `ARCHITECTURE §4.6`        | 同上                                                           | 未同步 |
| `IMPLEMENTATION_PLAN §2.2` | "WebSocket 日志增强 / 自动重连"                                      | 未同步 |

**Phase 1.11 前端开发者对着文档写代码会连不上**。这是我们约定"不能接受文档/代码不一致"的反例,必须改掉。

---

## 三、🟡 轻度问题(12 项)

| #    | 问题                     | 状态                          | 说明                                                                                |
| ---- | ---------------------- | --------------------------- | --------------------------------------------------------------------------------- |
| T-1  | GetAllServices 排序      | ✅ **已修**                    | `parser.go:106-108` sort.Slice                                                    |
| T-2  | `$$` 字面量               | ❌ 按 P2 延后(合规)               |                                                                                   |
| T-3  | WriteEnvEntries 保留 Raw | ❌ 按 P2 延后(合规)               |                                                                                   |
| T-4  | filter URL 编码          | ⚠️ **部分修**                  | `ListContainers` 用了 `url.QueryEscape`,**但 `FindContainerByServiceName:343` 依然裸拼** |
| T-5  | 串行 inspect             | ❌ 按 P2 延后(合规)               |                                                                                   |
| T-6  | ctr.Names 长度检查         | ✅ **已修**                    | `client.go:151-153` `len(ctr.Names) > 0`                                          |
| T-7  | ListServices 返回 error  | ❌ 按 P2 延后(合规)               |                                                                                   |
| T-8  | Handler 字段 exported    | ❌ 按 P2 延后(合规)               |                                                                                   |
| T-9  | state version 校验       | ❌ 按 P2 延后(合规)               |                                                                                   |
| T-10 | TokenTTL 字段            | ❌ Phase 1.1 Review 决议不修(合规) |                                                                                   |
| T-11 | EnvDiff 合并到 service 层  | ✅ **已修**                    | `manager.go:169-177`                                                              |
| T-12 | DetectCommand 无锁       | ❌ 按 P2 延后(合规)               |                                                                                   |

### ⚠️ T-4 只修了一处

client.gointernal/docker

`GetContainerStatus:317` 也是裸拼,**共 2 处遗漏**。

---

## 四、📝 文档冲突(2 项)

### D-1 Profile `enabled` 语义 ⚠️ **未裁决**

`phase1.10-review.md:210` 里对方标注 **"需要你给我一个反馈做决定"**,代码仍按 IMPLEMENTATION_PLAN 走(`runningCount == total`),`DESIGN_DECISIONS §2` 的 "已部署(含已停止)" 未同步。**你还没做决定**。

### D-2 `.deployboard-state.json` 迁移 ✅ **代码已清理**,⚠️ **文档未同步**

- **代码**: `state.go:EnsureState` 当前版本**已无** rename 迁移逻辑,符合 "不迁移" 决议 ✅
- **PROGRESS.md §1.8** 仍写 "✅ 从旧版 .deployboard-state.json 一次性迁移" —— 与代码不符,需删除
- **IMPLEMENTATION_PLAN §1.8** 仍写迁移要求 —— 需按 DESIGN_DECISIONS §12 修正

---

## 五、新发现的问题(修复过程中引入或遗漏)

### N-1 `GetContainerEnv` 绕过 Service 层,M-10 分层不彻底

services.gointernal/api

Handler 直接调两次 Docker client,和 M-10 修前的 Start/Stop 同构问题。应下沉到 `lifecycle.go` 或新增 `GetServiceEnv(ctx, name)`。

### N-2 `lifecycle.go:StartService` 重复查询容器(非 Bug,但效率问题)

第 48 行 `FindContainerByServiceName` 判断未部署,第 69 行又调同一接口。一次请求两次 Docker API 调用,可合并。

### N-3 `SaveEnvFile` 只支持 `content` 模式,不支持 `entries` 模式

env.gointernal/api

`DESIGN_DECISIONS §8` 明确要求 "表格模式 PUT → `{entries: []EnvEntry}`;原始模式 PUT → `{content: "..."}`"。前端表格编辑会无法提交。Phase 2.3 会踩这个坑。

### N-4 `config.go` 全局 `var C *Config` 仍保留

虽然业务代码已不依赖(除 auth 外),但 `Load()` 中 `C = cfg` 这行**没必要保留**。按约定 auth.go 留到 Phase 2 改,但全局变量本身可以等 auth 改完时一起清。

### N-5 M-14 和 S-1 的耦合

拆了 `transport_windows.go` / `transport_linux.go` ✅,但 Windows 版本内容本身是 TCP 2375 的错误实现(见 S-1)。这两项是"结构对、内容错"。

---

## 六、修复状态总览

---

## 七、给下一步的建议

### 🔴 必须在 Phase 1.11 前端重构前补做

1. **S-1 真修** —— `go get github.com/Microsoft/go-winio`,`transport_windows.go` 改用 `winio.DialPipe`。成本:新增 2-3 行 import + 3 行 dial 代码。不修会让 Windows 用户必须开启不安全的 TCP 2375。
2. **M-12 文档同步** —— 改 `DESIGN_DECISIONS §9`、`ARCHITECTURE §4.6`、`IMPLEMENTATION_PLAN §2.2` 三处为 SSE + 保留现有路径。否则前端对着文档写 WebSocket 会连不上。
3. **D-1 裁决** —— 你需要决定:`enabled = 全部 running` 还是 `enabled = 全部已部署(含已停止)`?我建议前者(当前代码),因为后者在 UI 上会让"已停止"的 profile 服务仍显示为 enabled,反直觉。**决定后同步修改 `DESIGN_DECISIONS §2 补充`**。
4. **D-2 文档同步** —— 删除 `PROGRESS.md §1.8` 的 "从旧版迁移",修正 `IMPLEMENTATION_PLAN §1.8`。
5. **N-3 SaveEnvFile 双模式支持** —— 前端 Phase 2.3 表格编辑会依赖。

### 🟠 顺手做(零成本)

6. **M-7 digest 兜底** —— `manager.go:imagesMatch` 加 1 行 `if strings.HasPrefix(running, "sha256:") { return true }`
7. **M-9 Rebuild 警告** —— `upgrade.go:RebuildService` 加 4 行 build 型警告日志
8. **T-4 另外两处** —— `FindContainerByServiceName:343` 和 `GetContainerStatus:317` 也改 `url.QueryEscape`
9. **N-1 GetContainerEnv 下沉** —— 放到 `lifecycle.go` 或 `manager.go`,保持 Handler 薄层一致性

### 🟡 Phase 2 再做

- N-2 lifecycle.go 查询去重
- N-4 清除 config.C 残留
- T-2/3/5/7/8/9/10/12 所有 P2 项

---

## 八、总体评价

**工作质量**: 80%(18/23 承诺项完全到位)。主体架构问题(M-10 Handler 业务下沉、M-3/M-4/M-5 Executor 完善、M-8 state 原子化)解决得相当扎实,说明对方确实吃透了 Review 的意图。

**扣分项**:

- **S-1 的 TCP 2375 折中方案**是本次唯一真正令人担心的地方。它"通过了编译和启动校验"但"未通过真实 Windows 场景",容易在不知情下被合并。这种"把严重问题降级为技术债延后"的做法需要明确红线 —— 如果 Windows 只是当前不支持,应该在启动日志里**显式报错并引导**,而不是默默失败或要求用户开启不安全设置。
- **M-7 / M-9 的承诺没兑现** —— 这是小问题(成本 5 行代码),但反映出对方在执行"零成本防御"类修复时有遗漏。

**是否可以开始 Phase 1.11**:

- 阻塞项(S-1 真修、M-12 文档、D-1 裁决、N-3 双模式)解决后可以开始
- 否则前端开发中会踩 3 个坑(Windows 连不上 / 日志 API 对不上 / env 表格保存失败)
