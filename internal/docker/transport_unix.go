// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

//go:build !windows

package docker

import (
	"net"
	"time"
)

// dialDockerSocket Linux/macOS 通过 Unix Socket 连接 Docker 守护进程
func dialDockerSocket() (net.Conn, error) {
	return net.DialTimeout("unix", "/var/run/docker.sock", 5*time.Second)
}
