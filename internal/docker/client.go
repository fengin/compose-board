// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// docker 包封装 Docker Engine API 的 HTTP 调用。
// 使用 com.docker.compose.project 标签原生过滤项目容器，
// 使用 com.composeboard.category 标签读取分类。
// 显式删除了旧版 categorizeService() 硬编码匹配。
package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNotFound 容器/服务不存在
var ErrNotFound = errors.New("not found")

// Docker Compose 标签 key
const (
	labelComposeProject = "com.docker.compose.project"
	labelComposeService = "com.docker.compose.service"
	labelBoardCategory  = "com.composeboard.category"
)

// Client Docker 客户端（直接 HTTP 调用 Docker Engine API）
type Client struct {
	httpClient  *http.Client
	projectName string // Compose 项目名（用于标签过滤）
}

// ContainerInfo 容器信息
type ContainerInfo struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	ServiceName string        `json:"service_name"`
	Image       string        `json:"image"`
	Status      string        `json:"status"`
	State       string        `json:"state"`
	StartedAt   string        `json:"started_at"` // ISO 时间戳，用于 restart 完成判定
	Ports       []PortMapping `json:"ports"`
	Created     int64         `json:"created"`
	Health      string        `json:"health"`
	Category    string        `json:"category"`
	CPU         float64       `json:"cpu"`
	MemUsage    uint64        `json:"mem_usage"`
	MemLimit    uint64        `json:"mem_limit"`
	MemPercent  float64       `json:"mem_percent"`
}

// PortMapping 端口映射
type PortMapping struct {
	HostPort      string `json:"host_port"`
	ContainerPort string `json:"container_port"`
	Protocol      string `json:"protocol"`
}

// ContainerStatus 轻量容器状态（操作后轮询用）
type ContainerStatus struct {
	Status    string `json:"status"`     // running / exited / ...
	State     string `json:"state"`      // "Up 42 hours" 等
	StartedAt string `json:"started_at"` // ISO 时间戳
	Image     string `json:"image"`
}

// NewClient 创建 Docker 客户端
// projectName 为 Compose 项目名（用于 label 过滤）
func NewClient(projectName string) (*Client, error) {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialDockerSocket()
		},
	}

	return &Client{
		httpClient:  &http.Client{Transport: transport, Timeout: 30 * time.Second},
		projectName: projectName,
	}, nil
}

// Close 关闭客户端
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

// GetDockerVersion 获取 Docker 版本
func (c *Client) GetDockerVersion(ctx context.Context) (string, string, error) {
	resp, err := c.doRequest(ctx, "GET", "/version", nil)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var ver dockerVersion
	if err := json.NewDecoder(resp.Body).Decode(&ver); err != nil {
		return "", "", err
	}
	return ver.Version, ver.APIVersion, nil
}

// ListContainers 获取当前项目的全部容器
// 使用 com.docker.compose.project 标签原生过滤
func (c *Client) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	// 使用 Docker API label filter，避免获取全部容器后本地过滤
	filter := url.QueryEscape(fmt.Sprintf(`{"label":["%s=%s"]}`, labelComposeProject, c.projectName))
	path := "/containers/json?all=true&filters=" + filter

	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("获取容器列表失败: %w", err)
	}
	defer resp.Body.Close()

	var containers []dockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, err
	}

	var result []ContainerInfo
	for _, ctr := range containers {
		info := c.convertContainer(ctr)

		// 健康状态 + 启动时间（复用同一次 inspect 调用，零额外开销）
		info.Health = "none"
		if ctr.State == "running" {
			if inspect, err := c.inspectContainer(ctx, ctr.ID); err == nil {
				if inspect.State.Health != nil {
					info.Health = inspect.State.Health.Status
				}
				info.StartedAt = inspect.State.StartedAt
			}
		} else {
			// 非 running 容器也获取 StartedAt（重建/停止场景判定需要）
			info.StartedAt = c.getStartedAt(ctx, ctr.ID)
		}

		result = append(result, info)
	}

	return result, nil
}

// convertContainer 将 Docker API 容器转为 ContainerInfo
// 通过 labels 获取 ServiceName 和 Category，不再做服务名模糊匹配
func (c *Client) convertContainer(ctr dockerContainer) ContainerInfo {
	containerName := ""
	if len(ctr.Names) > 0 {
		containerName = strings.TrimPrefix(ctr.Names[0], "/")
	}
	info := ContainerInfo{
		ID:      ctr.ID[:12],
		Name:    containerName,
		Image:   ctr.Image,
		Status:  ctr.State,
		State:   ctr.Status,
		Created: ctr.Created,
	}

	// 从 Docker Compose 原生 label 读取 ServiceName
	if svc, ok := ctr.Labels[labelComposeService]; ok {
		info.ServiceName = svc
	}

	// 从 composeboard label 读取 Category，缺省 "other"
	if cat, ok := ctr.Labels[labelBoardCategory]; ok && cat != "" {
		info.Category = cat
	} else {
		info.Category = "other"
	}

	// 端口映射（去重：Docker 对 IPv4/IPv6 各返回一条）
	portSeen := make(map[string]bool)
	for _, p := range ctr.Ports {
		if p.PublicPort > 0 {
			key := fmt.Sprintf("%d:%d", p.PublicPort, p.PrivatePort)
			if !portSeen[key] {
				portSeen[key] = true
				info.Ports = append(info.Ports, PortMapping{
					HostPort:      fmt.Sprintf("%d", p.PublicPort),
					ContainerPort: fmt.Sprintf("%d", p.PrivatePort),
					Protocol:      p.Type,
				})
			}
		}
	}

	return info
}

// GetContainerStats 获取容器资源使用情况
func (c *Client) GetContainerStats(ctx context.Context, containerID string) (*ContainerInfo, error) {
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/containers/%s/stats?stream=false", containerID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var stats dockerStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, err
	}

	cpuPercent := 0.0
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
	cpuCount := len(stats.CPUStats.CPUUsage.PercpuUsage)
	if cpuCount == 0 {
		cpuCount = 1
	}
	if systemDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(cpuCount) * 100.0
	}

	memPercent := 0.0
	if stats.MemoryStats.Limit > 0 {
		memPercent = float64(stats.MemoryStats.Usage) / float64(stats.MemoryStats.Limit) * 100.0
	}

	return &ContainerInfo{
		CPU:        cpuPercent,
		MemUsage:   stats.MemoryStats.Usage,
		MemLimit:   stats.MemoryStats.Limit,
		MemPercent: memPercent,
	}, nil
}

// StartContainer 启动容器
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/containers/%s/start", containerID), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("启动失败: %s", string(body))
	}
	return nil
}

// StopContainer 停止容器
func (c *Client) StopContainer(ctx context.Context, containerID string) error {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/containers/%s/stop?t=10", containerID), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("停止失败: %s", string(body))
	}
	return nil
}

// RestartContainer 重启容器
func (c *Client) RestartContainer(ctx context.Context, containerID string) error {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/containers/%s/restart?t=10", containerID), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("重启失败: %s", string(body))
	}
	return nil
}

// GetContainerEnv 获取容器运行时环境变量
func (c *Client) GetContainerEnv(ctx context.Context, containerID string) (map[string]string, error) {
	inspect, err := c.inspectContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}

	envMap := make(map[string]string)
	for _, env := range inspect.Config.Env {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			value := parts[1]
			if isSensitiveKey(key) {
				value = maskValue(value)
			}
			envMap[key] = value
		}
	}
	return envMap, nil
}

// GetContainerLogs 获取容器日志流
func (c *Client) GetContainerLogs(ctx context.Context, containerID string, tail string, follow bool, since string) (io.ReadCloser, error) {
	path := buildContainerLogsPath(containerID, tail, follow, since)

	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("获取日志失败: %s", string(body))
	}
	return resp.Body, nil
}

// GetContainerStatus 直查单容器实时状态（不走缓存）
func (c *Client) GetContainerStatus(ctx context.Context, containerID string) (*ContainerStatus, error) {
	filter := url.QueryEscape(fmt.Sprintf(`{"id":["%s"]}`, containerID))
	resp, err := c.doRequest(ctx, "GET", "/containers/json?all=true&filters="+filter, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var containers []dockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		return nil, fmt.Errorf("容器不存在: %s: %w", containerID, ErrNotFound)
	}

	ctr := containers[0]
	startedAt := c.getStartedAt(ctx, ctr.ID)

	return &ContainerStatus{
		Status:    ctr.State,
		State:     ctr.Status,
		StartedAt: startedAt,
		Image:     ctr.Image,
	}, nil
}

// GetServiceContainerInfo 按服务名直查实时容器信息。
// 该方法不走缓存，适合操作后的单服务轮询。
func (c *Client) GetServiceContainerInfo(ctx context.Context, serviceName string, withStats bool) (*ContainerInfo, error) {
	filter := url.QueryEscape(fmt.Sprintf(`{"label":["%s=%s","%s=%s"]}`,
		labelComposeProject, c.projectName,
		labelComposeService, serviceName))
	resp, err := c.doRequest(ctx, "GET", "/containers/json?all=true&filters="+filter, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var containers []dockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		return nil, fmt.Errorf("未找到服务 %s 的容器: %w", serviceName, ErrNotFound)
	}

	ctr := selectBestContainer(containers)
	info := c.convertContainer(ctr)

	inspect, err := c.inspectContainer(ctx, ctr.ID)
	if err == nil {
		info.StartedAt = inspect.State.StartedAt
		info.Health = "none"
		if inspect.State.Health != nil {
			info.Health = inspect.State.Health.Status
		}
	}

	if info.StartedAt == "" {
		info.StartedAt = c.getStartedAt(ctx, ctr.ID)
	}
	if info.Health == "" {
		info.Health = "none"
	}

	if withStats && info.Status == "running" {
		stats, err := c.GetContainerStats(ctx, ctr.ID)
		if err == nil {
			info.CPU = stats.CPU
			info.MemUsage = stats.MemUsage
			info.MemLimit = stats.MemLimit
			info.MemPercent = stats.MemPercent
		}
	}

	return &info, nil
}

// FindContainerByServiceName 通过 compose.service 标签查找容器
func (c *Client) FindContainerByServiceName(ctx context.Context, serviceName string) (*ContainerStatus, string, error) {
	info, err := c.GetServiceContainerInfo(ctx, serviceName, false)
	if err != nil {
		return nil, "", err
	}

	return &ContainerStatus{
		Status:    info.Status,
		State:     info.State,
		StartedAt: info.StartedAt,
		Image:     info.Image,
	}, info.ID, nil
}

func buildContainerLogsPath(containerID string, tail string, follow bool, since string) string {
	values := url.Values{}
	values.Set("stdout", "true")
	values.Set("stderr", "true")
	values.Set("timestamps", "true")
	if follow {
		values.Set("follow", "true")
	} else {
		values.Set("follow", "false")
	}
	if tail != "" {
		values.Set("tail", tail)
	}
	if since != "" {
		values.Set("since", since)
	}
	return fmt.Sprintf("/containers/%s/logs?%s", containerID, values.Encode())
}

func selectBestContainer(containers []dockerContainer) dockerContainer {
	best := containers[0]
	for i := 1; i < len(containers); i++ {
		candidate := containers[i]
		if betterContainerCandidate(candidate, best) {
			best = candidate
		}
	}
	return best
}

func betterContainerCandidate(candidate dockerContainer, current dockerContainer) bool {
	if candidate.State == "running" && current.State != "running" {
		return true
	}
	if candidate.State != "running" && current.State == "running" {
		return false
	}
	return candidate.Created > current.Created
}

// --- 内部实现 ---

// doRequest 执行 Docker API 请求
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://localhost"+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.httpClient.Do(req)
}

// inspectContainer 获取容器详情
func (c *Client) inspectContainer(ctx context.Context, containerID string) (*dockerInspect, error) {
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/containers/%s/json", containerID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var inspect dockerInspect
	if err := json.NewDecoder(resp.Body).Decode(&inspect); err != nil {
		return nil, err
	}
	return &inspect, nil
}

// getStartedAt 获取容器启动时间
func (c *Client) getStartedAt(ctx context.Context, containerID string) string {
	inspResp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/containers/%s/json", containerID), nil)
	if err != nil {
		return ""
	}
	defer inspResp.Body.Close()
	var result struct {
		State struct {
			StartedAt string `json:"StartedAt"`
		} `json:"State"`
	}
	if json.NewDecoder(inspResp.Body).Decode(&result) == nil {
		return result.State.StartedAt
	}
	return ""
}

// isSensitiveKey 判断是否敏感变量名
func isSensitiveKey(key string) bool {
	sensitive := []string{"PASSWORD", "SECRET", "TOKEN", "PASS"}
	keyUpper := strings.ToUpper(key)
	for _, s := range sensitive {
		if strings.Contains(keyUpper, s) {
			return true
		}
	}
	return false
}

// maskValue 脱敏变量值
func maskValue(value string) string {
	if len(value) <= 2 {
		return "****"
	}
	return value[:1] + "****" + value[len(value)-1:]
}

// --- Docker API 数据结构 ---

type dockerContainer struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Created int64             `json:"Created"`
	Labels  map[string]string `json:"Labels"`
	Ports   []struct {
		IP          string `json:"IP"`
		PrivatePort int    `json:"PrivatePort"`
		PublicPort  int    `json:"PublicPort"`
		Type        string `json:"Type"`
	} `json:"Ports"`
}

type dockerInspect struct {
	State struct {
		StartedAt string `json:"StartedAt"` // 容器启动时间 ISO 时间戳
		Health    *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	Config struct {
		Env []string `json:"Env"`
	} `json:"Config"`
}

type dockerVersion struct {
	Version    string `json:"Version"`
	APIVersion string `json:"ApiVersion"`
}

type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
}
