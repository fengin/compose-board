// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

// executor.go 封装 docker-compose / docker compose CLI 调用。
// 所有 compose 命令统一经过此模块，自动检测 v1/v2 命令格式。
package compose

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// Executor Compose CLI 执行器
type Executor struct {
	projectDir  string // docker-compose 项目目录
	composeFile string // compose 文件路径（由 parser 注入，统一 -f 传入）
	command     string // "auto" | "docker-compose" | "docker compose"
	detected    string // 实际检测到的命令
	version     string // Compose 版本号
}

// UpOptions docker-compose up 的选项
type UpOptions struct {
	ForceRecreate bool     // --force-recreate
	NoDeps        bool     // --no-deps
	Profiles      []string // --profile xxx
}

// NewExecutor 创建 CLI 执行器
// command 参数: "auto" 自动检测, "docker-compose" 强制 v1, "docker compose" 强制 v2
func NewExecutor(projectDir, command string) *Executor {
	return &Executor{
		projectDir: projectDir,
		command:    command,
	}
}

// SetComposeFile 设置 compose 文件路径（由 ServiceManager 在解析后注入）
func (e *Executor) SetComposeFile(path string) {
	e.composeFile = path
}

// GetCommandInfo 返回已检测到的命令和版本
func (e *Executor) GetCommandInfo() (string, string) {
	return e.detected, e.version
}

// DetectCommand 检测可用的 compose 命令，返回命令路径和版本号
func (e *Executor) DetectCommand() (string, string, error) {
	if e.detected != "" {
		return e.detected, e.version, nil
	}

	if e.command != "" && e.command != "auto" {
		// 指定了具体命令
		ver, err := e.getVersion(e.command)
		if err != nil {
			return "", "", fmt.Errorf("指定的命令 %q 不可用: %w", e.command, err)
		}
		e.detected = e.command
		e.version = ver
		return e.detected, e.version, nil
	}

	// 自动检测：优先 docker compose (v2)
	if ver, err := e.getVersion("docker compose"); err == nil {
		e.detected = "docker compose"
		e.version = ver
		log.Printf("[COMPOSE] 检测到 docker compose v2: %s", ver)
		return e.detected, e.version, nil
	}

	// 回退 docker-compose (v1)
	if ver, err := e.getVersion("docker-compose"); err == nil {
		e.detected = "docker-compose"
		e.version = ver
		log.Printf("[COMPOSE] 检测到 docker-compose v1: %s", ver)
		return e.detected, e.version, nil
	}

	return "", "", fmt.Errorf("未找到 docker-compose 或 docker compose 命令")
}

// Up 执行 docker-compose up -d
// services 为空时启动全部服务
func (e *Executor) Up(ctx context.Context, services []string, opts UpOptions) error {
	args := []string{"up", "-d"}

	if opts.ForceRecreate {
		args = append(args, "--force-recreate")
	}
	if opts.NoDeps {
		args = append(args, "--no-deps")
	}

	args = append(args, services...)
	_, err := e.run(ctx, opts.Profiles, args...)
	return err
}

// Pull 执行 docker-compose pull
// services 为空时拉取全部服务
func (e *Executor) Pull(ctx context.Context, services []string) ([]byte, error) {
	args := []string{"pull"}
	args = append(args, services...)
	return e.run(ctx, nil, args...)
}

// Stop 执行 docker-compose stop
func (e *Executor) Stop(ctx context.Context, services []string, profiles []string) error {
	args := []string{"stop"}
	args = append(args, services...)
	_, err := e.run(ctx, profiles, args...)
	return err
}

// Rm 执行 docker-compose rm
func (e *Executor) Rm(ctx context.Context, services []string, force bool, profiles []string) error {
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, services...)
	_, err := e.run(ctx, profiles, args...)
	return err
}

// GetDetectedCommand 返回已检测到的命令（供外部展示）
func (e *Executor) GetDetectedCommand() string {
	return e.detected
}

// GetVersion 返回已检测到的版本号
func (e *Executor) GetVersion() string {
	return e.version
}

// --- 内部实现 ---

// run 执行 compose 命令并返回输出
func (e *Executor) run(ctx context.Context, profiles []string, args ...string) ([]byte, error) {
	cmd, _, err := e.DetectCommand()
	if err != nil {
		return nil, err
	}

	// 设置默认超时
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
	}

	// 构建完整命令行
	fullArgs := e.buildArgs(cmd, profiles, args)

	log.Printf("[COMPOSE] 执行: %s %s", fullArgs[0], strings.Join(fullArgs[1:], " "))

	c := exec.CommandContext(ctx, fullArgs[0], fullArgs[1:]...)
	c.Dir = e.projectDir

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	if err := c.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return stdout.Bytes(), fmt.Errorf("compose 命令失败: %s", strings.TrimSpace(errMsg))
	}

	return stdout.Bytes(), nil
}

// buildArgs 构建命令参数
// --profile 和 -f 作为全局选项放在子命令之前（v1 强制要求）
// "docker compose" → ["docker", "compose", --profile, -f, --project-directory, <subcommand>, args...]
// "docker-compose" → ["docker-compose", --profile, -f, --project-directory, <subcommand>, args...]
func (e *Executor) buildArgs(cmd string, profiles []string, args []string) []string {
	var fullArgs []string

	parts := strings.Fields(cmd)
	fullArgs = append(fullArgs, parts...)

	// 全局选项：--profile（必须在子命令前，v1 强制要求）
	for _, p := range profiles {
		fullArgs = append(fullArgs, "--profile", p)
	}

	// 全局选项：-f 显式指定 compose 文件（确保 parser 和 CLI 读同一文件）
	if e.composeFile != "" {
		fullArgs = append(fullArgs, "-f", e.composeFile)
	}

	// 全局选项：项目目录
	fullArgs = append(fullArgs, "--project-directory", e.projectDir)

	// 子命令 + 用户参数
	fullArgs = append(fullArgs, args...)

	return fullArgs
}

// getVersion 获取指定命令的版本号
func (e *Executor) getVersion(cmd string) (string, error) {
	parts := strings.Fields(cmd)
	args := append(parts, "version", "--short")

	c := exec.Command(args[0], args[1:]...)
	output, err := c.Output()
	if err != nil {
		return "", err
	}

	ver := strings.TrimSpace(string(output))
	// 去掉可能的 "v" 前缀
	ver = strings.TrimPrefix(ver, "v")
	return ver, nil
}
