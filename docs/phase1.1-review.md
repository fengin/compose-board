Review 完成。整体 1.1 的骨架是跑通了（`go vet` 过、`i18n-check` 135 key 对称、编译产物 21MB），但是**有若干明显问题与文档约定偏离，部分是安全相关，1.2 继续推进前应先修复**。

下面按严重程度列一份问题清单，附出处与建议修法。

---

## 🔴 Blocker（安全/正确性，1.2 前必修）

### 1. JWT 校验未验算法 — 安全漏洞

<augment_code_snippet path="compose-board/internal/auth/auth.go" mode="EXCERPT"> ```go token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) { return []byte(config.C.Auth.JWTSecret), nil }) ``` </augment_code_snippet>

`keyfunc` 没检查 `token.Method`。`golang-jwt/v5` 对 `alg:none` 默认拒绝，但 HS256/RS256 混淆攻击仍可能命中。应加：

`if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {     return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"]) }`

### 2. 密码明文存储 + 明文比较 — 与 ARCHITECTURE 不一致

<augment_code_snippet path="compose-board/internal/auth/auth.go" mode="EXCERPT"> ```go if req.Username != config.C.Auth.Username || req.Password != config.C.Auth.Password { ``` </augment_code_snippet>

`ARCHITECTURE.md §3 AuthConfig` 规定 `Password` 为 bcrypt 散列。目前直接明文对比，`config.yaml.template` 的 `password: "changeme"` 也按明文。建议：`golang.org/x/crypto/bcrypt.CompareHashAndPassword`，首次启动若检测到明文则生成散列写回配置。

### 3. `JWTSecret` 空值回退到固定字符串 — 安全隐患

<augment_code_snippet path="compose-board/internal/config/config.go" mode="EXCERPT"> ```go if cfg.Auth.JWTSecret == "" { cfg.Auth.JWTSecret = "composeboard-default-secret-change-me" } ``` </augment_code_snippet>

文档约定是"为空则**自动生成**随机值**并写回 config.yaml**"。当前实现每次启动都用同一个固定字符串，所有部署实例的密钥一致，token 可伪造。应用 `crypto/rand` 生成 32+ 字节 base64，并回写配置。

### 4. 缺 `TokenTTL` 配置字段，硬编码 24h

- `ARCHITECTURE.md §3` 列了 `AuthConfig.TokenTTL string`（Go duration）- `config.go` 里压根没这个字段；`auth.go:44` 写死 `24 * time.Hour`

### 5. `NoRoute` 吞掉未匹配的 `/api/*` 请求 — 破坏前端错误处理

<augment_code_snippet path="compose-board/main.go" mode="EXCERPT"> ```go r.NoRoute(func(c *gin.Context) { c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML) }) ``` </augment_code_snippet>

任何未匹配的 `/api/xxx`（路径写错、尚未实现）会返回 `200 text/html`，前端 `resp.json()` 直接爆错，排查代价很高。NoRoute 里应先判断前缀：

`if strings.HasPrefix(c.Request.URL.Path, "/api/") {     c.JSON(http.StatusNotFound, gin.H{"error": "API not found"})     return }`

### 6. i18n 基础设施"完整对称"但**实际没接上 Vue**

PROGRESS.md 声称 i18n 基础设施完成，但：

- `app.js` 从未调用 `await I18n.load(I18n.locale)` 预加载（组件启动时 `messages={}`，所有 `$t` 落到 key fallback）- `app.js` 从未注册 `app.config.globalProperties.$t = I18n.t.bind(I18n)`（组件模板里 `{{ $t('x') }}` 会报 `$t is not a function`）- DEV_STANDARDS §1.1 明确要求"Vue 全局注册 `$t` 方法"

应在 `app.mount` 前加：

`await I18n.load(I18n.locale); app.config.globalProperties.$t = I18n.t.bind(I18n);`

### 7. 前端 4 处残留 "DeployBoard" 品牌 + 用户可见

| 文件                            | 行   | 内容                                         | 是否用户可见       |
| ----------------------------- | --- | ------------------------------------------ | ------------ |
| `web/js/app.js`               | 2   | `* DeployBoard Vue 应用入口`                   | 否（注释）        |
| `web/js/pages/login.js`       | 9   | `<h1 class="login-title">DeployBoard</h1>` | **是**（登录页标题） |
| `web/js/components/header.js` | 25  | `return titles[...]                        |              |
| `web/css/style.css`           | 2   | `DeployBoard — 扁平化设计系统`                    | 否（注释）        |

Login 页和 Header fallback 是用户第一眼就会看到的，必修。

### 8. 前端多处硬编码中文，违反 DEV_STANDARDS §8

DEV_STANDARDS §8.1 明确：**所有用户可见文本必须通过 `$t('key')` 访问**。实际上：

- `sidebar.js`：`label: '仪表盘' / '容器管理' / '日志查看' / '配置管理'`- `header.js`：`titles['dashboard'] = '仪表盘'`、按钮 `退出`、`admin`- `login.js`：`<p class="login-subtitle">Docker Compose 运维管理面板</p>`、`请输入用户名`、`登录中...` 等 10+ 处- `dashboard.js`：`CPU 使用率`、`内存使用率`、`容器状态` 等 15+ 处- `dashboard.js:94-100`：`categoryLabels` 硬编码整套（还有 `optional: '可选服务'`，与"optional 不再作为 category"的决策矛盾）- `auth.go:33/39` 后端返回 message `"请输入用户名和密码"` / `"用户名或密码错误"` 也是中文硬编码（应返回错误码，前端 i18n）

PROGRESS.md 应该明示"前端组件文本硬编码留待 §1.11 前端重构时一并迁移到 $t"，但不能自称"i18n 基础设施完成"的同时让这些文本继续硬编码——两者矛盾。

---

## 🟡 中等（1.11 前修复）

### 9. `gofmt -l` 报 3 个文件不合规 — 违反 DEV_STANDARDS §6

`internal\auth\auth.go     # import 未按 std/3rd 分组排序 internal\config\config.go # struct tag 列宽对齐不对 main.go                   # import 顺序`

建议在 Makefile 加一个 `fmt` / `fmt-check` target，CI 跑 `gofmt -d .` 退出非零则失败。

### 10. `config.go` 字段名与文档不一致

<augment_code_snippet path="compose-board/internal/config/config.go" mode="EXCERPT"> ```go type HostIPExtension struct { Enabled bool `yaml:"enabled"` EnvKey string `yaml:"env_key"` DetectOnStart bool `yaml:"detect_on_startup"` } ``` </augment_code_snippet>

- `DetectOnStart` vs ARCHITECTURE 里的 `DetectOnStartup`（yaml tag 对但 Go 字段名不一致，调用方命名会割裂）- `ServerConfig{Host, Port}` vs ARCHITECTURE 的 `Addr string`（实现更合理，但需**反向同步** ARCHITECTURE.md）

### 11. `sidebar.js` 与最终路由结构冲突

当前只有 4 项且路径为 `/containers`，IMPLEMENTATION_PLAN §1.11 要改为 `/services`，并新增 terminal/deploy/settings 共 7 项。这是 §1.11 的工作，但现在 sidebar 的 4 项直接决定了 §1.1 跑起来能看到什么，建议：

- 要么明确 PROGRESS.md 声明"前端导航 §1.11 重构"- 要么 §1.1 里就把 sidebar 导航项更新到目标结构（即便页面暂时空）

同类问题：`api.js` 里整套 `/api/containers/...` 旧 endpoints 保留，`pages/containers.js` 文件名也没改成 `services.js`。

### 12. `dashboard.js` categoryLabels 含 `optional`，违反最新决策

<augment_code_snippet path="compose-board/web/js/pages/dashboard.js" mode="EXCERPT"> ```js categoryLabels: { base: '基础服务', backend: '后端服务', frontend: '前端服务', init: '初始化容器', optional: '可选服务', other: '其他' } ``` </augment_code_snippet>

DESIGN_DECISIONS 明确：**optional 不再作为 category**，可选服务由 profile 分组表达。迁移时应顺手清理。

### 13. `host/info.go` Windows 磁盘路径仍有问题

<augment_code_snippet path="compose-board/internal/host/info.go" mode="EXCERPT"> ```go diskPath := "/" if runtime.GOOS == "windows" { diskPath = "C:" } ``` </augment_code_snippet>

`gopsutil/disk.Usage` 在 Windows 下需要 `"C:\\"`（尾部反斜杠）才能正确识别。`"C:"` 会被当作当前目录，返回不准或失败。PROGRESS 声称"增加 Windows 磁盘路径兼容"实际是半成品。

### 14. `i18n.js` 失败时静默降级

`load` 失败只打 `console.error`，`loaded` 仍是 false，但 `messages={}`。后续 `$t` 因 `loaded=false` 连 warning 也不打，UI 显示裸 key。建议：失败时 throw，让 `app.js` 决定是否用 fallback。

---

## 🟢 轻微（记一笔就行）

- `app.js:3` 注释 "Vue 3 + Vue Router (CDN 模式)" 与离线优先原则相悖（实际 vendor 本地加载），会误导后来人。- `main.go` banner 硬编码宽度 `%-18s`，`Version` 超 18 字符破坏 ASCII 边框。- Makefile `build` target 在 Windows 下产物 `bin/composeboard`（无扩展，go 会自动加 .exe），但 PROGRESS 里记的是 `composeboard.exe 21MB`——建议显式 `$(BINARY)$(EXE)` 处理扩展。- `auth.go` 后端返回的 error message 是中文字符串（"请输入用户名和密码"、"token 无效或已过期"），应改为错误码 + 前端 i18n 映射，否则英文用户永远看到中文错误。

---

## 建议处置

**1.2 动手前至少修掉 🔴 区块的 8 项**：

- #1–#5 属于安全/配置正确性，任何一项被线上复现都是高危- #6 `$t` 全局注册是后续所有页面的前置依赖，不修 §1.11 就无从谈起- #7–#8 是"品牌替换 + i18n 硬规则"的直接违规，不应留存到 §1.11 前堆积

**修 🟡 区块可以放到 §1.11 前端重构前**：批量清理 sidebar/api/pages 的旧 container 路径和硬编码文本，与 §1.11 合并为一次性工作。

**PROGRESS.md 应该补一笔**：

- "前端 vue 组件内 `$t` 全局注册 & 组件文本去硬编码"标注"§1.1 基础设施就绪 + §1.11 逐页迁移"，不要让 `✅ i18n 基础设施 135 key 完整对称` 掩盖掉"实际没接入 Vue"这一事实- "auth bcrypt / jwt_secret 自动生成 / TokenTTL" 若刻意延后，需要明确列为"与 ARCHITECTURE.md 暂时偏差"
