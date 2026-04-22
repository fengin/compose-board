// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// logs.go 容器日志流 API。
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fengin/composeboard/internal/docker"
	"github.com/gin-gonic/gin"
)

const (
	logRetryDelay        = 1200 * time.Millisecond
	logSourceCheckDelay  = 800 * time.Millisecond
	logScannerBufferSize = 64 * 1024
	logScannerMaxToken   = 1024 * 1024
)

// GetContainerLogs GET /api/services/:name/logs
func (h *Handler) GetContainerLogs(c *gin.Context) {
	name := c.Param("name")
	tail := c.DefaultQuery("tail", "200")
	follow := c.DefaultQuery("follow", "false") == "true"

	ctx := c.Request.Context()
	if follow {
		if err := h.streamServiceLogs(c, ctx, name, tail); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Printf("日志流已中止: service=%s err=%v\n", name, err)
		}
		return
	}

	_, containerID, err := h.DockerCli.FindContainerByServiceName(ctx, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务未部署"})
		return
	}

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	reader, err := h.DockerCli.GetContainerLogs(ctx, containerID, tail, false, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer reader.Close()

	lines, err := readLogLines(reader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": lines})
}

func (h *Handler) streamServiceLogs(c *gin.Context, ctx context.Context, serviceName string, tail string) error {
	// 直接访问未部署服务时仍然返回 404；进入流式会话后再按服务持续跟随。
	if _, _, err := h.DockerCli.FindContainerByServiceName(ctx, serviceName); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务未部署"})
		return nil
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "不支持流式输出"})
		return nil
	}

	streamer := &serviceLogStreamer{
		dockerCli:   h.DockerCli,
		serviceName: serviceName,
		initialTail: tail,
	}
	return streamer.Stream(ctx, c.Writer, flusher)
}

type serviceLogStreamer struct {
	dockerCli   *docker.Client
	serviceName string
	initialTail string

	lastContainerID string
	lastTimestamp   string
	lastLine        string
	lastStatus      string
	skipReplayLine  bool
}

func (s *serviceLogStreamer) Stream(ctx context.Context, writer io.Writer, flusher http.Flusher) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		status, containerID, err := s.dockerCli.FindContainerByServiceName(ctx, s.serviceName)
		if err != nil {
			if errors.Is(err, docker.ErrNotFound) {
				if err := s.writeStatus(writer, flusher, "waiting"); err != nil {
					return err
				}
			}
			if !waitForNextAttempt(ctx, logRetryDelay) {
				return ctx.Err()
			}
			continue
		}
		if !isAttachableLogStatus(status.Status) {
			if err := s.writeStatus(writer, flusher, "waiting"); err != nil {
				return err
			}
			if !waitForNextAttempt(ctx, logRetryDelay) {
				return ctx.Err()
			}
			continue
		}

		tail, since := s.nextCursor(containerID)
		reader, err := s.dockerCli.GetContainerLogs(ctx, containerID, tail, true, since)
		if err != nil {
			if err := s.writeStatus(writer, flusher, "reconnecting"); err != nil {
				return err
			}
			if !waitForNextAttempt(ctx, logRetryDelay) {
				return ctx.Err()
			}
			continue
		}
		if err := s.writeStatus(writer, flusher, "streaming"); err != nil {
			_ = reader.Close()
			return err
		}

		done := make(chan struct{})
		var closeOnce sync.Once
		closeReader := func() {
			closeOnce.Do(func() {
				_ = reader.Close()
			})
		}
		go s.watchLogSource(ctx, containerID, done, closeReader)

		err = s.pipeReader(ctx, reader, writer, flusher)
		close(done)
		closeReader()
		if err != nil && !errors.Is(err, context.Canceled) {
			if err := s.writeStatus(writer, flusher, "reconnecting"); err != nil {
				return err
			}
		}
		if err == nil {
			if err := s.writeStatus(writer, flusher, "waiting"); err != nil {
				return err
			}
		}
		if !waitForNextAttempt(ctx, logRetryDelay) {
			return ctx.Err()
		}
	}
}

func (s *serviceLogStreamer) nextCursor(containerID string) (string, string) {
	if containerID != s.lastContainerID {
		s.lastContainerID = containerID
		s.lastTimestamp = ""
		s.lastLine = ""
		s.skipReplayLine = false
		return s.initialTail, ""
	}
	if s.lastTimestamp != "" {
		s.skipReplayLine = true
		return "0", s.lastTimestamp
	}
	s.skipReplayLine = false
	return s.initialTail, ""
}

func (s *serviceLogStreamer) pipeReader(ctx context.Context, reader io.Reader, writer io.Writer, flusher http.Flusher) error {
	scanner := newLogScanner(reader)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}

		line := cleanLogLine(scanner.Text())
		if s.skipReplayLine {
			s.skipReplayLine = false
			if line == s.lastLine {
				continue
			}
		}
		if sinceValue := extractLogSinceValue(line); sinceValue != "" {
			s.lastTimestamp = sinceValue
		}
		s.lastLine = line

		if _, err := fmt.Fprintf(writer, "data: %s\n\n", line); err != nil {
			return err
		}
		flusher.Flush()
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (s *serviceLogStreamer) watchLogSource(
	ctx context.Context,
	attachedContainerID string,
	done <-chan struct{},
	closeReader func(),
) {
	ticker := time.NewTicker(logSourceCheckDelay)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			closeReader()
			return
		case <-done:
			return
		case <-ticker.C:
			if shouldRotate, err := s.shouldRotateLogSource(ctx, attachedContainerID); err == nil && shouldRotate {
				closeReader()
				return
			}
		}
	}
}

func (s *serviceLogStreamer) shouldRotateLogSource(ctx context.Context, attachedContainerID string) (bool, error) {
	status, currentContainerID, err := s.dockerCli.FindContainerByServiceName(ctx, s.serviceName)
	return shouldRotateLogSourceState(status, currentContainerID, attachedContainerID, err)
}

func readLogLines(reader io.Reader) ([]string, error) {
	var lines []string
	scanner := newLogScanner(reader)
	for scanner.Scan() {
		lines = append(lines, cleanLogLine(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func newLogScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, logScannerBufferSize), logScannerMaxToken)
	return scanner
}

func (s *serviceLogStreamer) writeStatus(writer io.Writer, flusher http.Flusher, state string) error {
	if s.lastStatus == state {
		return nil
	}
	payload, err := json.Marshal(map[string]string{"state": state})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "event: status\ndata: %s\n\n", payload); err != nil {
		return err
	}
	s.lastStatus = state
	flusher.Flush()
	return nil
}

func waitForNextAttempt(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func extractLogTimestamp(line string) string {
	if line == "" {
		return ""
	}
	firstField, _, ok := strings.Cut(line, " ")
	if !ok {
		return ""
	}
	if _, err := time.Parse(time.RFC3339Nano, firstField); err == nil {
		return firstField
	}
	if _, err := time.Parse(time.RFC3339, firstField); err == nil {
		return firstField
	}
	return ""
}

func extractLogSinceValue(line string) string {
	ts := extractLogTimestamp(line)
	if ts == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, ts)
		if err != nil {
			return ""
		}
	}
	// Docker logs API 使用 Unix 时间戳最稳妥，避免 RFC3339 字符串在续挂时被拒绝。
	return strconv.FormatInt(parsed.Unix(), 10)
}

func shouldRotateLogSourceState(
	status *docker.ContainerStatus,
	currentContainerID string,
	attachedContainerID string,
	err error,
) (bool, error) {
	if err != nil {
		if errors.Is(err, docker.ErrNotFound) {
			return true, nil
		}
		return false, err
	}
	if status == nil {
		return true, nil
	}
	if currentContainerID != attachedContainerID {
		return true, nil
	}
	if !isAttachableLogStatus(status.Status) {
		return true, nil
	}
	return false, nil
}

func isAttachableLogStatus(status string) bool {
	switch status {
	case "running", "restarting":
		return true
	default:
		return false
	}
}

// cleanLogLine 清理 Docker 日志前缀（8 字节 stream header）
func cleanLogLine(line string) string {
	// Docker multiplex stream: 前 8 字节为 header
	if len(line) >= 8 {
		// 检查是否有可打印字符
		header := line[:8]
		hasNonPrint := false
		for _, b := range []byte(header) {
			if b < 32 && b != '\t' {
				hasNonPrint = true
				break
			}
		}
		if hasNonPrint {
			line = line[8:]
		}
	}
	return strings.TrimRight(line, "\r\n")
}
