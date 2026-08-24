package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

var logHintEnvironmentKeys = map[string]struct{}{
	"LOGGING_PATH":            {},
	"LOG_PATH":                {},
	"LOG_DIR":                 {},
	"LOG_HOME":                {},
	"SPRING_APPLICATION_NAME": {},
	"SERVICE_NAME":            {},
}

// ContainerMount Docker Inspect 返回的实际挂载关系。
type ContainerMount struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	RW          bool   `json:"rw"`
}

// ServiceLogRuntime 文件日志目录动态关联所需的最小运行态信息。
// 环境变量仅保留日志路径和服务名称提示，不暴露其他运行时变量。
type ServiceLogRuntime struct {
	ContainerID   string
	ContainerName string
	Image         string
	LogHints      map[string]string
	Mounts        []ContainerMount
}

// GetServiceLogRuntime 获取服务当前容器的实际挂载和日志相关环境提示。
func (c *Client) GetServiceLogRuntime(ctx context.Context, serviceName string) (*ServiceLogRuntime, error) {
	info, err := c.GetServiceContainerInfo(ctx, serviceName, false)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/containers/%s/json", info.ID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("获取容器挂载信息失败: HTTP %d", resp.StatusCode)
	}

	var inspect struct {
		Config struct {
			Env []string `json:"Env"`
		} `json:"Config"`
		Mounts []struct {
			Type        string `json:"Type"`
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
			RW          bool   `json:"RW"`
		} `json:"Mounts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&inspect); err != nil {
		return nil, err
	}

	runtime := &ServiceLogRuntime{
		ContainerID:   info.ID,
		ContainerName: info.Name,
		Image:         info.Image,
		LogHints:      make(map[string]string),
		Mounts:        make([]ContainerMount, 0, len(inspect.Mounts)),
	}
	for _, item := range inspect.Config.Env {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, allowed := logHintEnvironmentKeys[strings.ToUpper(key)]; allowed {
			runtime.LogHints[strings.ToUpper(key)] = value
		}
	}
	for _, mount := range inspect.Mounts {
		runtime.Mounts = append(runtime.Mounts, ContainerMount{
			Type:        mount.Type,
			Source:      mount.Source,
			Destination: mount.Destination,
			RW:          mount.RW,
		})
	}
	return runtime, nil
}
