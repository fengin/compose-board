// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

//go:build windows

package docker

import (
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

const dockerPipeName = `\\.\pipe\docker_engine`

// dialDockerSocket Windows 通过 Named Pipe 连接 Docker 守护进程
func dialDockerSocket() (net.Conn, error) {
	timeout := 5 * time.Second
	return winio.DialPipe(dockerPipeName, &timeout)
}
