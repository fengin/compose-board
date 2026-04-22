# ComposeBoard 开发规范

> **作者**: 凌封  
> **网址**: https://fengin.cn  
> **代码库**: https://github.com/fengin/composeboard

---

## 1. 代码文件头部注释

### Go 文件

所有 `.go` 文件头部添加：

```go
// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

package xxx
```

### JavaScript 文件

所有 `.js` 文件头部添加：

```js
/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 */
```

### CSS 文件

```css
/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 */
```

### HTML 文件

```html
<!--
  ComposeBoard - Docker Compose 可视化管理面板
  作者：凌封
  网址：https://fengin.cn
-->
```

---

## 2. Go Module 路径

```
github.com/fengin/composeboard
```

所有 import 使用此路径：
```go
import (
    "github.com/fengin/composeboard/internal/compose"
    "github.com/fengin/composeboard/internal/docker"
    "github.com/fengin/composeboard/internal/service"
)
```

---

## 3. 视觉风格

**保持现有 DeployBoard 的视觉风格**，具体包括：

- 深色主题 + 透明卡片设计（glassmorphism）
- Inter 字体系列（本地 vendor，非 CDN）
- 现有的配色方案（主色/强调色/状态色）
- 侧边栏窄栏 + 图标导航
- Toast 通知风格
- 表格/卡片/按钮样式

> 新增页面（设置、终端、部署向导）沿用同一设计语言，不引入新的视觉体系。

---

## 4. 离线优先：不依赖外部资源

**所有前端依赖必须 vendor 到本地**，禁止使用外部 CDN/URL：

| 依赖 | 方式 | 路径 |
|------|------|------|
| Vue.js 3 | vendor | `web/js/vendor/vue.global.prod.js` |
| Vue Router 4 | vendor | `web/js/vendor/vue-router.global.prod.js` |
| Inter 字体 | vendor | `web/css/vendor/inter/` |
| xterm.js | vendor | `web/js/vendor/xterm.js` + `web/css/vendor/xterm.css` |
| i18n locale | 内置 | `web/js/locales/zh.json` + `en.json`（两者都需完整覆盖全部 key） |

**检查项**：
- `index.html` 中不得出现 `https://` 引用（除 meta 标签中的网址信息）
- `style.css` 中不得出现 `@import url('https://...')`
- 所有字体文件（.woff2）本地存放
- `zh.json` 和 `en.json` 必须同步维护，禁止新增单边 locale key

---

## 5. 轻量化要求

### 编译产物
- 单文件二进制（go:embed 嵌入前端）
- Linux amd64 目标大小 < 15MB
- 使用 `-ldflags="-s -w"` 去除调试信息

### 运行时资源
- 空闲时 CPU 占用 ≈ 0%（缓存按需刷新，无访问时暂停）
- 内存占用 < 30MB（无前端访问时）
- 不使用数据库，配置用 YAML 文件
- 状态文件用 JSON，不引入 SQLite 等

### 前端资源
- 不使用构建工具（无 webpack/vite/npm build）
- JS 文件按需加载（Vue Router 懒加载无法做，但文件本身保持精简）
- CSS 单文件，不引入 Tailwind 等重型框架
- vendor 库使用 `.prod.min.js` 压缩版本

---

## 6. 代码风格

### Go 代码
- 遵循 Go 官方代码规范（`gofmt` / `go vet`）
- 每个公开函数/类型添加注释
- 错误处理不使用 panic，统一 return error
- 日志使用 `log.Printf("[模块] 消息")` 格式
- 避免全局变量，通过构造函数注入依赖

### JavaScript 代码
- Vue 3 Options API 风格（与现有 DeployBoard 一致）
- 组件模板使用模板字符串（无 .vue 文件）
- API 调用统一通过 `API.xxx()` 封装
- 用户可见文本使用 `$t('key')`（i18n）

### CSS
- 使用 CSS 变量（`var(--xxx)`）管理主题
- 避免 `!important`
- 类名采用 BEM 或功能命名（如 `.service-card`、`.profile-group`）

### 注释要求
- 文件头部：模块用途说明
- 公开函数/类型：功能描述 + 参数说明（如有必要）
- 复杂逻辑：行内注释解释 why，而非 what
- 不需要逐行注释，保持代码自解释

---

## 7. 目录/文件命名

| 规则 | 示例 |
|------|------|
| Go 包名：小写单词 | `compose`、`service`、`docker` |
| Go 文件名：小写 + 下划线 | `service_manager.go`、`env.go` |
| JS 文件名：小写 + 连字符 | `services.js`、`confirm-dialog.js` |
| CSS 类名：小写 + 连字符 | `.service-card`、`.profile-group` |
| API 路径：小写 + 连字符 | `/api/services/:key/terminal` |
| locale key：小写 + 点分隔 | `services.status.running` |

---

## 8. i18n 一致性规范

### 8.1 硬规则

- **所有用户可见文本必须通过 `$t('key')` 访问**，禁止在 Vue 模板、JS 字符串、Toast、Alert、Confirm 中硬编码中文或英文
- `zh.json` 和 `en.json` 的 key 集合必须**完全一致**（对称 subset 等价）
- **不允许单边新增 key**：任何 PR 引入新 key 必须同时提交两个 locale 的翻译
- **不允许单边删除 key**：key 退役时两个 locale 同步删除
- key 命名采用 `<page>.<section>.<item>` 三段式，如 `services.profile.enable_button`

### 8.2 校验脚本

提交前在仓库根目录执行：

```bash
# 校验 zh.json 与 en.json 的 key 对称
node scripts/check-i18n-keys.js
```

脚本位于 `scripts/check-i18n-keys.js`（Phase 1 交付），行为：
1. 递归读取 `web/js/locales/zh.json` 和 `en.json`，展平为点分路径 key 列表
2. 计算对称差集：`zh - en`（英文缺失）、`en - zh`（中文缺失）
3. 任一差集非空 → 退出码 1，列出缺失 key；全对称 → 退出码 0
4. 可选附加：扫描 `web/js/` 下 `$t('...')` 调用，检测运行时使用但 locale 未定义的 key

### 8.3 CI 集成（后续）

- GitHub Actions 的 PR 流水线加入 `node scripts/check-i18n-keys.js` 步骤
- 不通过则阻断合并
- 开发期本地可用 `npm run i18n:check`（package.json 仅用于承载脚本，不引入构建）
