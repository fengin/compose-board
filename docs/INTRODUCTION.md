# ComposeBoard 精简介绍

> 用于快速介绍、宣传、选型沟通和项目首页摘要。

## 一句话介绍

ComposeBoard 是一个极其轻量的 Docker Compose 可视化管理面板，用一个单文件程序为现有 Compose 项目提供服务管理、环境配置、日志查看和 Web 终端能力。

![系统概览](ui/系统概览.png)

## 解决的问题

Docker Compose 足够简单稳定，但日常运维经常需要反复执行命令：

- 查看哪些服务在运行、哪些已停止、哪些还没部署。
- 修改 `.env` 后判断哪些服务需要重建。
- 拉取镜像并重建单个服务。
- 查看容器日志。
- 进入容器排查问题。
- 管理可选服务组。

ComposeBoard 把这些常见操作做成一个轻量 Web 面板，同时保留 Compose 项目原有结构和命令行可控性。

## 核心功能

| 功能         | 说明                                |
| ---------- | --------------------------------- |
| 系统概览       | 查看项目、主机、Docker、CPU、内存、磁盘和服务状态     |
| 服务管理       | 展示 Compose 声明服务，支持启动、停止、重启、升级、重建  |
| Profile 管理 | 按 Compose Profiles 分组启用和停用可选服务    |
| 环境配置       | 在线编辑 `.env`，支持表格模式、文本模式、差异确认和自动备份 |
| 日志查看       | 查看历史日志和实时日志，服务重建后可继续跟随            |
| Web 终端     | 在浏览器内通过 Docker Exec 直连运行中容器       |
| 中英双语       | 支持中文和 English 运行时切换               |

![服务管理](ui/服务管理.png)

## 主要优势

| 优势         | 说明                                                     |
| ---------- | ------------------------------------------------------ |
| 极致轻量       | 单个程序文件大小22M左右，占用内存22~25 MB，CPU几乎为零，非常优秀                |
| 零数据库       | 不需要 MySQL、PostgreSQL、SQLite 或 BoltDB                   |
| 离线可用       | 前端资源全部内置，不依赖外部 CDN                                     |
| Compose 原生 | 不改造项目结构，读取 Compose YAML、`.env` 和 Docker Compose labels |
| 声明态视图      | 未部署服务也能在界面中看到                                          |
| 单项目专注      | 避免大平台复杂度，专门服务单机 Compose 运维                             |
| 跨平台        | 支持 Linux、Windows、macOS，支持 amd64 和 arm64                |

## 关键参数

| 参数        | 数据                                               |
| --------- | ------------------------------------------------ |
| 部署形态      | 单文件二进制                                           |
| 运行依赖      | 本机 Docker daemon、docker compose 或 docker-compose |
| 数据库       | 无                                                |
| 前端构建      | 无                                                |
| 二进制体积     | 约 21 到 23 MB                                     |
| 冷启动       | 约 1 秒                                            |
| 休眠内存      | 约 20 MB                                          |
| 活跃内存      | 约 25~28 MB                                       |
| 核心 API 延迟 | 本机测试小于 1 ms                                      |
| 默认端口      | 9090                                             |

## 与其他产品的差异

| 产品           | 更适合的场景                        |
| ------------ | ----------------------------- |
| ComposeBoard | 单机 Compose 项目，追求轻量、离线、单文件、低资源 |
| Portainer    | 通用容器管理、多环境和更完整容器平台能力          |
| 1Panel       | Linux 服务器全栈运维、网站、数据库、防火墙等综合管理 |
| Rancher      | Kubernetes 集群管理               |
| Dockge       | Compose 堆栈管理，偏 HomeLab 和容器化部署 |

ComposeBoard 的目标不是取代大平台，而是在一个 Compose 项目需要一个干净好用的小面板”时给出更轻的选择。

## 适合谁

- 使用 Docker Compose 部署中小型项目的开发者。
- 维护私有化部署环境的实施和运维人员。
- 希望给客户提供可视化运维入口的项目团队。
- 在低配云服务器、边缘节点或内网环境运行服务的人。
- 不想为单机 Compose 项目引入大型平台（如rancher/Portainer/1Panel/K8S等）的人。

## 当前边界

| 支持                | 当前不做               |
| ----------------- | ------------------ |
| 单机 Docker Compose | Kubernetes / Swarm |
| 一个实例管理一个项目        | 多项目统一平台            |
| 本地 Docker daemon  | 远程 Docker Host     |
| 单服务单容器视图          | 多副本编排管理            |
| `.env`、日志、终端、升级   | 镜像仓库凭据管理           |

## 快速部署

```powershell
Copy-Item config.yaml.template config.yaml
```

修改：

```yaml
project:
  dir: "/opt/compose-project"

auth:
  username: "admin"
  password: "changeme"
  jwt_secret: "please-change-this-secret"
```

运行：

```powershell
.\composeboard-windows-amd64.exe -config .\config.yaml
```

Linux：

```bash
chmod +x ./composeboard-linux-amd64
./composeboard-linux-amd64 -config ./config.yaml
```

访问：

```text
http://服务器IP:9090
```

## 推荐宣传语

极其轻量、离线、单文件的 Docker Compose 可视化管理面板，性能非常优秀。

为已有 Compose 项目补上服务管理、环境配置、日志和 Web 终端，不引入数据库，不改变项目结构，不把单机运维复杂化。



## 作者信息

作者：凌封  
作者主页：[https://fengin.cn](https://fengin.cn)  
AI 全书：[https://aibook.ren](https://aibook.ren)  
GitHub：[https://github.com/fengin/compose-board](https://github.com/fengin/compose-board)
