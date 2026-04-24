// ComposeBoard - Docker Compose 可视化管理面板
// 作者：凌封
// 网址：https://fengin.cn

package host

import (
	"fmt"
	"net"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

// HostInfo 主机信息
type HostInfo struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
	Uptime   uint64 `json:"uptime"`

	IPs []string `json:"ips"`

	CPUModel   string  `json:"cpu_model"`
	CPUCores   int     `json:"cpu_cores"`
	CPUPercent float64 `json:"cpu_percent"`

	MemTotal   uint64  `json:"mem_total"`
	MemUsed    uint64  `json:"mem_used"`
	MemPercent float64 `json:"mem_percent"`

	DiskTotal   uint64  `json:"disk_total"`
	DiskUsed    uint64  `json:"disk_used"`
	DiskPercent float64 `json:"disk_percent"`

	DockerVersion string `json:"docker_version"`
	DockerAPI     string `json:"docker_api"`
}

// GetHostInfo 采集主机信息（不含 Docker，Docker 信息由 API 层补充）
func GetHostInfo() (*HostInfo, error) {
	info := &HostInfo{}

	hostInfo, err := host.Info()
	if err == nil {
		info.Hostname = hostInfo.Hostname
		info.OS = hostInfo.OS
		info.Platform = fmt.Sprintf("%s %s", hostInfo.Platform, hostInfo.PlatformVersion)
		info.Uptime = hostInfo.Uptime
	}
	info.Arch = runtime.GOARCH

	info.IPs = getLocalIPs()

	cpuInfo, err := cpu.Info()
	if err == nil && len(cpuInfo) > 0 {
		info.CPUModel = cpuInfo[0].ModelName
	}
	info.CPUCores = runtime.NumCPU()

	cpuPercent, err := cpu.Percent(time.Second, false)
	if err == nil && len(cpuPercent) > 0 {
		info.CPUPercent = cpuPercent[0]
	}

	memInfo, err := mem.VirtualMemory()
	if err == nil {
		info.MemTotal = memInfo.Total
		info.MemUsed = memInfo.Used
		info.MemPercent = memInfo.UsedPercent
	}

	diskPath := "/"
	if runtime.GOOS == "windows" {
		diskPath = "C:\\"
	}
	diskInfo, err := disk.Usage(diskPath)
	if err == nil {
		info.DiskTotal = diskInfo.Total
		info.DiskUsed = diskInfo.Used
		info.DiskPercent = diskInfo.UsedPercent
	}

	return info, nil
}

type ipCandidate struct {
	ip    string
	score int
}

// getLocalIPs 获取本机 IPv4 候选地址，并按“用户最可能关心”排序。
// 多网卡/VPN/Docker/K8s 场景下不存在绝对正确 IP，因此保留候选列表交给前端折叠展示。
func getLocalIPs() []string {
	routeIP := detectRouteIP()
	candidates := collectIPCandidates(routeIP)
	if len(candidates) == 0 {
		if routeIP != "" {
			return []string{routeIP}
		}
		return nil
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].ip < candidates[j].ip
		}
		return candidates[i].score > candidates[j].score
	})

	ips := make([]string, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if seen[candidate.ip] {
			continue
		}
		seen[candidate.ip] = true
		ips = append(ips, candidate.ip)
	}
	return ips
}

func detectRouteIP() string {
	if conn, err := net.DialTimeout("udp4", "8.8.8.8:53", 2*time.Second); err == nil {
		routeIP := ""
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			routeIP = addr.IP.String()
		}
		_ = conn.Close()
		return routeIP
	}
	return ""
}

func collectIPCandidates(routeIP string) []ipCandidate {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	candidates := make([]ipCandidate, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if !isUsableIPv4(ip4) {
				continue
			}
			candidates = append(candidates, ipCandidate{
				ip:    ip4.String(),
				score: scoreHostIP(ip4, iface, routeIP),
			})
		}
	}

	if routeIP != "" && isUsableIPv4(net.ParseIP(routeIP).To4()) && !containsIP(candidates, routeIP) {
		candidates = append(candidates, ipCandidate{ip: routeIP, score: 1})
	}
	return candidates
}

func scoreHostIP(ip net.IP, iface net.Interface, routeIP string) int {
	score := 0
	name := strings.ToLower(iface.Name)

	if ip.String() == routeIP {
		score += 35
	}

	if isPrivateIPv4(ip) {
		score += 60
	} else {
		// 云主机或直连公网网卡时，公网地址可能正是用户需要的访问地址。
		score += 70
	}

	ip4 := ip.To4()
	switch {
	case ip4[0] == 192 && ip4[1] == 168:
		score += 40
	case ip4[0] == 10:
		score += 25
	case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
		score += 10
	case ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127:
		score -= 25
	case ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19):
		score -= 35
	}

	if iface.Flags&net.FlagPointToPoint != 0 {
		score -= 35
	}
	score += interfaceNameScore(name)
	return score
}

func interfaceNameScore(name string) int {
	for _, marker := range []string{
		"docker", "br-", "veth", "cni", "flannel", "cali", "kube", "podman",
		"container", "vbox", "virtualbox", "vmware", "hyper-v", "vethernet", "wsl",
	} {
		if strings.Contains(name, marker) {
			return -90
		}
	}
	for _, marker := range []string{"vpn", "tailscale", "zerotier", "wireguard", "wg", "tun", "tap", "ppp", "clash", "surge"} {
		if strings.Contains(name, marker) {
			return -50
		}
	}
	return 20
}

func isUsableIPv4(ip4 net.IP) bool {
	if ip4 == nil {
		return false
	}
	if ip4.IsLoopback() || ip4.IsUnspecified() || ip4.IsMulticast() {
		return false
	}
	// 169.254.0.0/16 — APIPA 链路本地地址（DHCP 失败时 Windows 自动分配）
	return !(ip4[0] == 169 && ip4[1] == 254)
}

func isPrivateIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	if ip4[0] == 10 {
		return true
	}
	if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
		return true
	}
	return ip4[0] == 192 && ip4[1] == 168
}

func containsIP(candidates []ipCandidate, ip string) bool {
	for _, candidate := range candidates {
		if candidate.ip == ip {
			return true
		}
	}
	return false
}
