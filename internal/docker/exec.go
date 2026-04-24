// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// exec.go 封装 Docker Exec API，供 Web 终端使用。
package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ExecInspect Docker exec inspect 结果。
type ExecInspect struct {
	ID       string `json:"ID"`
	Running  bool   `json:"Running"`
	ExitCode int    `json:"ExitCode"`
}

// ExecStream 表示 Docker hijack 后的双向 exec 流。
type ExecStream struct {
	Conn   net.Conn
	Reader io.ReadCloser
}

// Read 从 Docker Exec 输出流读取数据。
func (s *ExecStream) Read(p []byte) (int, error) {
	return s.Reader.Read(p)
}

// Write 向 Docker Exec stdin 写入数据。
func (s *ExecStream) Write(p []byte) (int, error) {
	return s.Conn.Write(p)
}

// Close 关闭 Docker Exec hijacked stream。
func (s *ExecStream) Close() error {
	var err error
	if s.Reader != nil {
		err = s.Reader.Close()
	}
	if s.Conn != nil {
		if closeErr := s.Conn.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

type execCreateRequest struct {
	AttachStdin  bool     `json:"AttachStdin"`
	AttachStdout bool     `json:"AttachStdout"`
	AttachStderr bool     `json:"AttachStderr"`
	Tty          bool     `json:"Tty"`
	Cmd          []string `json:"Cmd"`
	Env          []string `json:"Env,omitempty"`
}

type execCreateResponse struct {
	ID string `json:"Id"`
}

type execStartRequest struct {
	Detach bool `json:"Detach"`
	Tty    bool `json:"Tty"`
}

// CreateExec 创建 Docker Exec 会话。
func (c *Client) CreateExec(ctx context.Context, containerID string, cmd []string, tty bool) (string, error) {
	body, err := json.Marshal(execCreateRequest{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          tty,
		Cmd:          cmd,
		Env:          []string{"TERM=xterm-256color"},
	})
	if err != nil {
		return "", err
	}

	path := fmt.Sprintf("/containers/%s/exec", url.PathEscape(containerID))
	resp, err := c.doRequest(ctx, "POST", path, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("创建 exec 失败: %s", string(data))
	}

	var result execCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.ID == "" {
		return "", fmt.Errorf("创建 exec 失败: Docker 未返回 exec id")
	}
	return result.ID, nil
}

// StartExec 启动 Docker Exec 并返回 hijacked 双向流。
// 实现参考 Docker SDK hijack.go: 用 hijackedConn 包装 conn+bufio.Reader，
// 通过 Write 发送请求，通过 ReadResponse 读响应头，后续数据从 bufio.Reader 读。
func (c *Client) StartExec(ctx context.Context, execID string, tty bool) (*ExecStream, error) {
	body, err := json.Marshal(execStartRequest{Detach: false, Tty: tty})
	if err != nil {
		return nil, err
	}

	conn, err := dialDockerSocket()
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	path := fmt.Sprintf("/exec/%s/start", url.PathEscape(execID))
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost"+path, bytes.NewReader(body))
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "tcp")
	req.ContentLength = int64(len(body))

	// 创建 hijackedConn：写入走 conn，读取走 bufio.Reader
	hc := &hijackedConn{Conn: conn, r: bufio.NewReader(conn)}

	// 写 HTTP 请求到底层 conn
	if err := req.Write(hc.Conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	// 用 bufio.Reader 读 HTTP 响应（头部被消费，后续数据留在 bufio 缓冲区）
	resp, err := http.ReadResponse(hc.r, req)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("启动 exec 失败(%d): %s", resp.StatusCode, string(data))
	}

	_ = conn.SetDeadline(time.Time{})

	// 返回 hijackedConn 作为读写流，Read 走 bufio.Reader，Write 走 conn
	return &ExecStream{Conn: conn, Reader: io.NopCloser(hc)}, nil
}

// hijackedConn 包装 net.Conn + bufio.Reader，参考 Docker SDK。
// Read 从 bufio.Reader 读取（确保不丢失已缓冲数据），Write 走底层 conn。
type hijackedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *hijackedConn) Read(b []byte) (int, error) {
	return c.r.Read(b)
}

// ResizeExec 调整 Docker Exec TTY 尺寸。
func (c *Client) ResizeExec(ctx context.Context, execID string, cols int, rows int) error {
	values := url.Values{}
	values.Set("w", fmt.Sprintf("%d", cols))
	values.Set("h", fmt.Sprintf("%d", rows))
	path := fmt.Sprintf("/exec/%s/resize?%s", url.PathEscape(execID), values.Encode())

	resp, err := c.doRequest(ctx, "POST", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("调整 exec 尺寸失败: %s", string(data))
	}
	return nil
}

// InspectExec 查询 Docker Exec 状态。
func (c *Client) InspectExec(ctx context.Context, execID string) (*ExecInspect, error) {
	path := fmt.Sprintf("/exec/%s/json", url.PathEscape(execID))
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("查询 exec 状态失败: %s", string(data))
	}

	var result ExecInspect
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RunExecCommand 执行一次短命令并返回退出码。
func (c *Client) RunExecCommand(ctx context.Context, containerID string, cmd []string) (int, error) {
	execID, err := c.CreateExec(ctx, containerID, cmd, false)
	if err != nil {
		return -1, err
	}
	stream, err := c.StartExec(ctx, execID, false)
	if err != nil {
		return -1, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = stream.Conn.SetDeadline(deadline)
	}
	// 读完所有输出，等待命令结束（conn 关闭 = 命令退出）
	_, _ = io.Copy(io.Discard, stream)
	_ = stream.Close()

	// 命令结束后查询退出码，可能需要短暂等待 Docker 更新状态
	var exitCode int
	for i := 0; i < 10; i++ {
		inspect, err := c.InspectExec(ctx, execID)
		if err != nil {
			return -1, err
		}
		if !inspect.Running {
			exitCode = inspect.ExitCode
			return exitCode, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return -1, fmt.Errorf("exec 命令超时未退出")
}

// GetContainerPlatform 返回容器平台，无法识别时回退 linux。
func (c *Client) GetContainerPlatform(ctx context.Context, containerID string) (string, error) {
	inspect, err := c.inspectContainer(ctx, containerID)
	if err != nil {
		return "", err
	}

	platform := strings.ToLower(strings.TrimSpace(inspect.Platform))
	if platform == "" {
		platform = strings.ToLower(strings.TrimSpace(inspect.Os))
	}
	if platform == "" {
		platform = "linux"
	}
	return platform, nil
}
