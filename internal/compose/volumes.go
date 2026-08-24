package compose

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// VolumeMount Compose 服务声明中的挂载关系。
type VolumeMount struct {
	Type     string
	Source   string
	Target   string
	ReadOnly bool
}

// ReadServiceVolumes 独立读取指定服务的 volumes，不改变现有声明态解析模型。
func ReadServiceVolumes(projectDir string, serviceName string) ([]VolumeMount, error) {
	filePath, err := FindComposeFile(projectDir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取 Compose 文件失败: %w", err)
	}

	var raw struct {
		Services map[string]struct {
			Volumes interface{} `yaml:"volumes"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析 Compose YAML 失败: %w", err)
	}
	service, ok := raw.Services[serviceName]
	if !ok {
		return nil, fmt.Errorf("Compose 服务不存在: %s", serviceName)
	}
	return parseVolumeMounts(service.Volumes), nil
}

func parseVolumeMounts(value interface{}) []VolumeMount {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}

	result := make([]VolumeMount, 0, len(items))
	for _, item := range items {
		switch mount := item.(type) {
		case string:
			if parsed, ok := parseShortVolumeMount(mount); ok {
				result = append(result, parsed)
			}
		case map[string]interface{}:
			parsed := VolumeMount{
				Type:     stringValue(mount["type"]),
				Source:   firstStringValue(mount, "source", "src"),
				Target:   firstStringValue(mount, "target", "dst", "destination"),
				ReadOnly: boolValue(mount["read_only"]),
			}
			if parsed.Type == "" {
				parsed.Type = inferMountType(parsed.Source)
			}
			if parsed.Target != "" {
				result = append(result, parsed)
			}
		}
	}
	return result
}

func parseShortVolumeMount(value string) (VolumeMount, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return VolumeMount{}, false
	}

	separator := shortVolumeSeparator(value)
	if separator < 0 {
		return VolumeMount{Type: "volume", Target: value}, true
	}

	source := value[:separator]
	rest := value[separator+1:]
	target := rest
	mode := ""
	if last := strings.LastIndex(rest, ":"); last >= 0 {
		target = rest[:last]
		mode = rest[last+1:]
	}
	if target == "" {
		return VolumeMount{}, false
	}
	return VolumeMount{
		Type:     inferMountType(source),
		Source:   source,
		Target:   target,
		ReadOnly: hasReadOnlyMode(mode),
	}, true
}

func shortVolumeSeparator(value string) int {
	for i := 0; i+1 < len(value); i++ {
		if value[i] != ':' || (value[i+1] != '/' && value[i+1] != '\\') {
			continue
		}
		// Windows 盘符 C:/path 不是 source/target 分隔符。
		if i == 1 && isASCIIAlpha(value[0]) {
			continue
		}
		return i
	}
	return -1
}

func inferMountType(source string) string {
	if source == "" {
		return "volume"
	}
	if strings.Contains(source, "$") || strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "\\") || isWindowsAbsPath(source) {
		return "bind"
	}
	return "volume"
}

func isWindowsAbsPath(value string) bool {
	return len(value) >= 3 && isASCIIAlpha(value[0]) && value[1] == ':' && (value[2] == '/' || value[2] == '\\')
}

func isASCIIAlpha(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func hasReadOnlyMode(mode string) bool {
	for _, part := range strings.Split(mode, ",") {
		if strings.TrimSpace(part) == "ro" {
			return true
		}
	}
	return false
}

func firstStringValue(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(values[key]); value != "" {
			return value
		}
	}
	return ""
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

func boolValue(value interface{}) bool {
	result, ok := value.(bool)
	return ok && result
}
