// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// terminal 包管理 Web 终端会话生命周期。
package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/fengin/composeboard/internal/docker"
	"github.com/gorilla/websocket"
)

const (
	defaultMaxSessions = 8                // 最大并发终端会话数，超出时拒绝新连接
	pongWait           = 60 * time.Second // WebSocket 心跳超时，超过此时间未收到 pong 则断开
	pingPeriod         = 25 * time.Second // WebSocket 心跳发送间隔，需小于 pongWait
	writeWait          = 10 * time.Second // WebSocket 单次写操作超时
	execStartTimeout   = 10 * time.Second // Docker Exec create/start 握手超时
	shellProbeTimeout  = 5 * time.Second  // 单次 shell 探测超时
	maxControlBytes    = 64 * 1024        // WebSocket 控制消息（JSON）最大字节数
	outputBufferSize   = 32 * 1024        // Docker exec 输出读取缓冲区大小
	minTerminalCols    = 10
	maxTerminalCols    = 1000
	minTerminalRows    = 3
	maxTerminalRows    = 500
)

// StartOptions 描述一次终端会话的启动参数。
type StartOptions struct {
	ServiceName string
	ContainerID string
}

// SessionManager 管理 Web 终端活跃会话。
type SessionManager struct {
	dockerCli   *docker.Client
	maxSessions int

	mu       sync.Mutex
	sessions map[string]*Session
	shells   sync.Map // containerID -> shellSelection，容器重建后 ID 变化自然失效
}

type shellSelection struct {
	cmd  []string
	name string
}

// NewSessionManager 创建 Web 终端会话管理器。
func NewSessionManager(dockerCli *docker.Client, maxSessions int) *SessionManager {
	if maxSessions <= 0 {
		maxSessions = defaultMaxSessions
	}
	return &SessionManager{
		dockerCli:   dockerCli,
		maxSessions: maxSessions,
		sessions:    make(map[string]*Session),
	}
}

// Serve 创建并运行一个 Web 终端会话。
func (m *SessionManager) Serve(ctx context.Context, conn *websocket.Conn, opts StartOptions) {
	sessionID := fmt.Sprintf("%s-%d", opts.ServiceName, time.Now().UnixNano())
	session := &Session{
		id:          sessionID,
		serviceName: opts.ServiceName,
		containerID: opts.ContainerID,
		manager:     m,
		conn:        conn,
		done:        make(chan struct{}),
	}

	if !m.register(session) {
		session.writeControl(controlMessage{
			Type:    "error",
			Code:    "terminal.too_many_sessions",
			Message: "终端会话过多，请先关闭其他终端",
		})
		_ = conn.Close()
		return
	}
	defer m.unregister(sessionID)

	session.run(ctx)
}

func (m *SessionManager) register(session *Session) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.sessions) >= m.maxSessions {
		return false
	}
	m.sessions[session.id] = session
	return true
}

func (m *SessionManager) unregister(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
}

func (m *SessionManager) selectShell(ctx context.Context, containerID string) ([]string, string, error) {
	if cached, ok := m.shells.Load(containerID); ok {
		if shell, ok := cached.(shellSelection); ok {
			return shell.cmd, shell.name, nil
		}
	}

	platform, err := m.dockerCli.GetContainerPlatform(ctx, containerID)
	if err == nil && strings.Contains(strings.ToLower(platform), "windows") {
		if m.probeShell(ctx, containerID, []string{"cmd.exe", "/c", "echo ok"}) {
			return m.rememberShell(containerID, []string{"cmd.exe"}, "cmd.exe")
		}
		if m.probeShell(ctx, containerID, []string{"powershell.exe", "-NoLogo", "-NoProfile", "-Command", "Write-Output ok"}) {
			return m.rememberShell(containerID, []string{"powershell.exe", "-NoLogo", "-NoProfile"}, "powershell.exe")
		}
		return nil, "", fmt.Errorf("该 Windows 容器没有可用的 cmd.exe 或 powershell.exe，无法打开终端")
	}

	// 优先尝试 bash
	if m.probeShell(ctx, containerID, []string{"bash", "-c", "echo ok"}) {
		return m.rememberShell(containerID, []string{"bash"}, "bash")
	}

	// bash 不可用，尝试 /bin/sh
	if m.probeShell(ctx, containerID, []string{"/bin/sh", "-c", "echo ok"}) {
		return m.rememberShell(containerID, []string{"/bin/sh"}, "sh")
	}

	return nil, "", fmt.Errorf("该容器没有可用的 shell，无法打开终端")
}

func (m *SessionManager) probeShell(ctx context.Context, containerID string, cmd []string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, shellProbeTimeout)
	defer cancel()
	exitCode, err := m.dockerCli.RunExecCommand(probeCtx, containerID, cmd)
	return err == nil && exitCode == 0
}

func (m *SessionManager) rememberShell(containerID string, cmd []string, name string) ([]string, string, error) {
	m.shells.Store(containerID, shellSelection{cmd: cmd, name: name})
	return cmd, name, nil
}

// Session 表示一个 WebSocket 到 Docker Exec 的终端会话。
type Session struct {
	id          string
	serviceName string
	containerID string
	execID      string
	shellName   string

	manager *SessionManager
	conn    *websocket.Conn
	stream  *docker.ExecStream

	writeMu sync.Mutex
	closeMu sync.Mutex
	closed  bool
	done    chan struct{}
}

type controlMessage struct {
	Type     string `json:"type"`
	Data     string `json:"data,omitempty"`
	Cols     int    `json:"cols,omitempty"`
	Rows     int    `json:"rows,omitempty"`
	Shell    string `json:"shell,omitempty"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
	Reason   string `json:"reason,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
}

type sessionEnd struct {
	reason string
	err    error
}

func (s *Session) run(ctx context.Context) {
	defer s.close()

	cmd, shellName, shellErr := s.manager.selectShell(ctx, s.containerID)
	if shellErr != nil {
		s.sendError("terminal.no_shell", shellErr.Error())
		return
	}
	s.shellName = shellName

	execCtx, cancelExec := context.WithTimeout(ctx, execStartTimeout)
	execID, err := s.manager.dockerCli.CreateExec(execCtx, s.containerID, cmd, true)
	cancelExec()
	if err != nil {
		s.sendError("terminal.exec_create_failed", err.Error())
		return
	}
	s.execID = execID

	startCtx, cancelStart := context.WithTimeout(ctx, execStartTimeout)
	stream, err := s.manager.dockerCli.StartExec(startCtx, execID, true)
	cancelStart()
	if err != nil {
		s.sendError("terminal.exec_start_failed", err.Error())
		return
	}
	s.stream = stream

	if err := s.writeControl(controlMessage{Type: "ready", Shell: shellName}); err != nil {
		return
	}

	s.conn.SetReadLimit(maxControlBytes)
	_ = s.conn.SetReadDeadline(time.Now().Add(pongWait))
	s.conn.SetPongHandler(func(string) error {
		_ = s.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	endCh := make(chan sessionEnd, 2)
	go s.copyOutput(endCh)
	go s.readLoop(ctx, endCh)
	go s.pingLoop()

	end := sessionEnd{reason: "context_done"}
	select {
	case end = <-endCh:
	case <-ctx.Done():
		end.err = ctx.Err()
	}

	log.Printf("[TERMINAL] 会话结束: reason=%s err=%v", end.reason, end.err)
	exitCode := s.inspectExitCode(ctx)
	if end.reason == "" {
		end.reason = "closed"
	}
	_ = s.writeControl(controlMessage{
		Type:     "closed",
		Reason:   end.reason,
		ExitCode: exitCode,
	})
}

func (s *Session) readLoop(ctx context.Context, endCh chan<- sessionEnd) {
	for {
		messageType, payload, err := s.conn.ReadMessage()
		if err != nil {
			endCh <- sessionEnd{reason: closeReasonFromError(err), err: err}
			return
		}

		switch messageType {
		case websocket.TextMessage:
			var msg controlMessage
			if err := json.Unmarshal(payload, &msg); err != nil {
				s.sendError("terminal.invalid_message", "终端控制消息格式错误")
				continue
			}
			if s.handleControl(ctx, msg, endCh) {
				return
			}
		case websocket.BinaryMessage:
			if len(payload) > 0 && s.stream != nil {
				if _, err := s.stream.Write(payload); err != nil {
					endCh <- sessionEnd{reason: "docker_stdin_closed", err: err}
					return
				}
			}
		case websocket.CloseMessage:
			endCh <- sessionEnd{reason: "client_close"}
			return
		}
	}
}

func (s *Session) handleControl(ctx context.Context, msg controlMessage, endCh chan<- sessionEnd) bool {
	switch msg.Type {
	case "input":
		if msg.Data != "" && s.stream != nil {
			if _, err := s.stream.Write([]byte(msg.Data)); err != nil {
				endCh <- sessionEnd{reason: "docker_stdin_closed", err: err}
				return true
			}
		}
	case "resize":
		if validTerminalSize(msg.Cols, msg.Rows) && s.execID != "" {
			resizeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := s.manager.dockerCli.ResizeExec(resizeCtx, s.execID, msg.Cols, msg.Rows)
			cancel()
			if err != nil {
				log.Printf("[TERMINAL] resize 失败: service=%s cols=%d rows=%d err=%v", s.serviceName, msg.Cols, msg.Rows, err)
			}
		}
	case "close":
		endCh <- sessionEnd{reason: "client_close"}
		return true
	default:
		s.sendError("terminal.unknown_message", "未知终端控制消息")
	}
	return false
}

func (s *Session) copyOutput(endCh chan<- sessionEnd) {
	buf := make([]byte, outputBufferSize)
	for {
		n, err := s.stream.Read(buf)
		if n > 0 {
			if writeErr := s.writeBinary(buf[:n]); writeErr != nil {
				endCh <- sessionEnd{reason: "websocket_closed", err: writeErr}
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				endCh <- sessionEnd{reason: "exec_exit"}
			} else {
				endCh <- sessionEnd{reason: "docker_stdout_closed", err: err}
			}
			return
		}
	}
}

func (s *Session) pingLoop() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.writeMu.Lock()
			_ = s.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := s.conn.WriteMessage(websocket.PingMessage, nil)
			s.writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

func (s *Session) writeControl(msg controlMessage) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return s.conn.WriteJSON(msg)
}

func (s *Session) writeBinary(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return s.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (s *Session) sendError(code string, message string) {
	_ = s.writeControl(controlMessage{
		Type:    "error",
		Code:    code,
		Message: message,
	})
}

func (s *Session) inspectExitCode(ctx context.Context) *int {
	if s.execID == "" {
		return nil
	}
	inspectCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	inspect, err := s.manager.dockerCli.InspectExec(inspectCtx, s.execID)
	if err != nil || inspect.Running {
		return nil
	}
	return &inspect.ExitCode
}

func validTerminalSize(cols int, rows int) bool {
	return cols >= minTerminalCols &&
		cols <= maxTerminalCols &&
		rows >= minTerminalRows &&
		rows <= maxTerminalRows
}

func (s *Session) close() {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.done)
	if s.stream != nil {
		_ = s.stream.Close()
	}
	_ = s.conn.Close()
}

func closeReasonFromError(err error) string {
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		switch closeErr.Code {
		case websocket.CloseNormalClosure, websocket.CloseGoingAway:
			return "client_close"
		}
	}
	return "websocket_closed"
}
