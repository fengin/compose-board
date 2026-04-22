// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// env.go 负责 .env 文件的行级解析、写入、变量展开。
// 变量来源仅限本地 .env 文件，不读取 os.Environ()，
// 保证镜像差异检测的可复现性。
package compose

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// EnvEntry .env 文件中的一行（保留注释、空行、顺序）
type EnvEntry struct {
	Type  string `json:"type"`            // "variable" | "comment" | "blank"
	Key   string `json:"key,omitempty"`   // 变量名（仅 variable 类型）
	Value string `json:"value,omitempty"` // 变量值（仅 variable 类型）
	Raw   string `json:"raw"`             // 原始行内容
	Line  int    `json:"line"`            // 行号（1-based）
}

// ParseEnvFile 逐行解析 .env 文件，保留注释行和空行
func ParseEnvFile(path string) ([]EnvEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 .env 文件失败: %w", err)
	}
	defer file.Close()

	var entries []EnvEntry
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)

		if trimmed == "" {
			// 空行
			entries = append(entries, EnvEntry{
				Type: "blank",
				Raw:  raw,
				Line: lineNum,
			})
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			// 注释行
			entries = append(entries, EnvEntry{
				Type: "comment",
				Raw:  raw,
				Line: lineNum,
			})
			continue
		}

		// 变量行：KEY=VALUE
		if idx := strings.Index(trimmed, "="); idx > 0 {
			key := strings.TrimSpace(trimmed[:idx])
			value := strings.TrimSpace(trimmed[idx+1:])
			// 剥离外层引号（Compose CLI 行为一致）
			value = stripQuotes(value)
			entries = append(entries, EnvEntry{
				Type:  "variable",
				Key:   key,
				Value: value,
				Raw:   raw,
				Line:  lineNum,
			})
		} else {
			// 无法识别的行，保留为注释
			entries = append(entries, EnvEntry{
				Type: "comment",
				Raw:  raw,
				Line: lineNum,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 .env 文件失败: %w", err)
	}

	return entries, nil
}

// ReadEnvVars 读取 .env 文件，返回纯变量 map（内部用，忽略注释和空行）
func ReadEnvVars(path string) (map[string]string, error) {
	entries, err := ParseEnvFile(path)
	if err != nil {
		return nil, err
	}

	vars := make(map[string]string)
	for _, e := range entries {
		if e.Type == "variable" {
			vars[e.Key] = e.Value
		}
	}
	return vars, nil
}

// WriteEnvEntries 按 EnvEntry 序列写回 .env 文件，保持原始格式
func WriteEnvEntries(path string, entries []EnvEntry) error {
	var lines []string
	for _, e := range entries {
		switch e.Type {
		case "variable":
			// 优先使用 Raw 保留原始格式（引号/行内注释/缩进）
			if e.Raw != "" {
				lines = append(lines, e.Raw)
			} else {
				lines = append(lines, fmt.Sprintf("%s=%s", e.Key, e.Value))
			}
		case "comment", "blank":
			lines = append(lines, e.Raw)
		}
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0644)
}

// WriteEnvRaw 将原始文本内容写回 .env 文件
func WriteEnvRaw(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// --- 变量展开 ---

// ExpandVars 支持 Compose 标准变量替换语法：
//   - ${VAR} / $VAR — 简单替换
//   - ${VAR:-default} — 未设置或为空时用 default
//   - ${VAR-default} — 未设置时用 default
//   - ${VAR:+replacement} — 设置且非空时用 replacement
//   - ${VAR+replacement} — 设置时用 replacement
//   - ${VAR:?error} / ${VAR?error} — 未设置时返回错误文本（不终止）
//
// 变量来源仅限 vars 参数（来自 .env），不读取 os.Environ()。
func ExpandVars(template string, vars map[string]string) string {
	if template == "" {
		return ""
	}

	// 先处理 ${...} 形式（支持修饰符）
	result := expandBraceVars(template, vars)

	// 再处理 $VAR 形式（简单变量，无修饰符）
	result = expandSimpleVars(result, vars)

	return result
}

// expandBraceVars 处理 ${...} 形式的变量替换
// 正则匹配 ${VAR_NAME} 和 ${VAR_NAME:-default} 等
var braceVarRegex = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(([:\-\+\?])([^}]*)|([\-\+\?])([^}]*))?\}`)

func expandBraceVars(template string, vars map[string]string) string {
	return braceVarRegex.ReplaceAllStringFunc(template, func(match string) string {
		// 解析组成部分
		inner := match[2 : len(match)-1] // 去掉 ${ 和 }
		name, modifier, defaultVal := parseBraceExpr(inner)

		val, exists := vars[name]
		isEmpty := val == ""

		switch modifier {
		case "":
			// ${VAR} — 简单替换
			if exists {
				return val
			}
			return ""
		case ":-":
			// ${VAR:-default} — 未设置或为空时用 default
			if !exists || isEmpty {
				return defaultVal
			}
			return val
		case "-":
			// ${VAR-default} — 未设置时用 default
			if !exists {
				return defaultVal
			}
			return val
		case ":+":
			// ${VAR:+replacement} — 设置且非空时用 replacement
			if exists && !isEmpty {
				return defaultVal
			}
			return ""
		case "+":
			// ${VAR+replacement} — 设置时用 replacement
			if exists {
				return defaultVal
			}
			return ""
		case ":?", "?":
			// ${VAR:?error} / ${VAR?error} — 未设置时保留原始表达式（不展开）
			shouldError := modifier == ":?" && (!exists || isEmpty) || modifier == "?" && !exists
			if shouldError {
				// 返回原始 match，让上层能识别未展开的变量
				return match
			}
			return val
		}
		return match
	})
}

// parseBraceExpr 解析 ${} 内部表达式
// 输入: "VAR_NAME:-default" → ("VAR_NAME", ":-", "default")
// 输入: "VAR_NAME" → ("VAR_NAME", "", "")
func parseBraceExpr(expr string) (name, modifier, defaultVal string) {
	// 查找第一个修饰符
	for i, ch := range expr {
		if ch == ':' && i+1 < len(expr) {
			next := expr[i+1]
			if next == '-' || next == '+' || next == '?' {
				return expr[:i], expr[i : i+2], expr[i+2:]
			}
		}
		if ch == '-' || ch == '+' || ch == '?' {
			return expr[:i], string(ch), expr[i+1:]
		}
	}
	return expr, "", ""
}

// expandSimpleVars 处理 $VAR 形式（不带花括号的简单变量）
var simpleVarRegex = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

func expandSimpleVars(template string, vars map[string]string) string {
	return simpleVarRegex.ReplaceAllStringFunc(template, func(match string) string {
		name := match[1:] // 去掉 $
		if val, ok := vars[name]; ok {
			return val
		}
		return ""
	})
}

// stripQuotes 剥离 value 的外层引号（Compose CLI 行为一致）
// "value" → value, 'value' → value, value → value
func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
