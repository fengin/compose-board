/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * 仪表盘页面组件 — 扁平化设计
 */
const DashboardPage = {
    template: `
    <div>
        <!-- 1. 项目信息 -->
        <div class="card no-hover dash-section">
            <div class="card-header">
                <h2 class="card-title">{{ $t('dashboard.project_info') }}</h2>
            </div>
            <div class="dash-info-grid">
                <div class="dash-info-item">
                    <div class="dash-info-label">{{ $t('dashboard.project_name') }}</div>
                    <div class="dash-info-value">{{ projectInfo.project_name || '—' }}</div>
                </div>
                <div class="dash-info-item">
                    <div class="dash-info-label">{{ $t('dashboard.compose_version') }}</div>
                    <div class="dash-info-value mono">{{ projectInfo.compose_command && projectInfo.compose_version ? projectInfo.compose_command + ' ' + projectInfo.compose_version : '—' }}</div>
                </div>
                <div class="dash-info-item">
                    <div class="dash-info-label">{{ $t('dashboard.service_count') }}</div>
                    <div class="dash-info-value">{{ projectInfo.service_count != null ? projectInfo.service_count : '—' }}</div>
                </div>
                <div class="dash-info-item" v-if="projectInfo.profile_names && projectInfo.profile_names.length">
                    <div class="dash-info-label">{{ $t('dashboard.profiles') }}</div>
                    <div class="dash-info-value mono">{{ projectInfo.profile_names.join(', ') }}</div>
                </div>
            </div>
        </div>

        <!-- 2. 服务器信息 -->
        <div class="card no-hover dash-section">
            <div class="card-header">
                <h2 class="card-title">{{ $t('dashboard.server_info') }}</h2>
            </div>
            <div class="dash-server-grid">
                <!-- 操作系统 / IP（放最前面） -->
                <div class="dash-server-item">
                    <div class="stat-icon" style="background:#F5F3FF;color:#7C3AED">🖥️</div>
                    <div class="dash-server-detail">
                        <div class="dash-server-title">{{ $t('dashboard.os') }}</div>
                        <div class="dash-server-main">{{ hostInfo.platform || '—' }}</div>
                        <div class="dash-server-sub dash-server-sub-ip">
                            <span v-if="normalizedIPs.length" class="dash-popover-wrap">
                                <span class="dash-popover-target">
                                    {{ normalizedIPs.join(' / ') }}
                                </span>
                                <div class="dash-popover">
                                    <div class="dash-popover-title">{{ $t('dashboard.ip_list') }}</div>
                                    <div class="dash-popover-list">
                                        <div v-for="ip in normalizedIPs" :key="ip" class="dash-popover-item">{{ ip }}</div>
                                    </div>
                                </div>
                            </span>
                        </div>
                    </div>
                </div>
                <!-- CPU -->
                <div class="dash-server-item">
                    <div class="stat-icon blue">⚡</div>
                    <div class="dash-server-detail">
                        <div class="dash-server-title">{{ $t('dashboard.cpu') }}</div>
                        <div class="dash-server-main">{{ hostInfo.cpu_percent?.toFixed(1) || '—' }}%
                            <span class="dash-server-badge">{{ hostInfo.cpu_cores || '—' }} {{ $t('dashboard.cores') }}</span>
                        </div>
                        <div class="dash-server-sub dash-server-sub-cpu">
                            <span v-if="hostInfo.cpu_model" class="dash-popover-wrap">
                                <span class="dash-popover-target">{{ hostInfo.cpu_model }}</span>
                                <div class="dash-popover">
                                    <div class="dash-popover-text">{{ hostInfo.cpu_model }}</div>
                                </div>
                            </span>
                            <span v-else>—</span>
                        </div>
                    </div>
                </div>
                <!-- 内存 -->
                <div class="dash-server-item">
                    <div class="stat-icon green">📊</div>
                    <div class="dash-server-detail">
                        <div class="dash-server-title">{{ $t('dashboard.memory') }}</div>
                        <div class="dash-server-main">{{ hostInfo.mem_percent?.toFixed(1) || '—' }}%
                            <span class="dash-server-badge">{{ formatBytes(hostInfo.mem_total) }}</span>
                        </div>
                        <div class="dash-server-sub">{{ formatBytes(hostInfo.mem_used) }} {{ $t('dashboard.used') }}</div>
                    </div>
                </div>
                <!-- 磁盘 -->
                <div class="dash-server-item">
                    <div class="stat-icon amber">💾</div>
                    <div class="dash-server-detail">
                        <div class="dash-server-title">{{ $t('dashboard.disk') }}</div>
                        <div class="dash-server-main">{{ hostInfo.disk_percent?.toFixed(1) || '—' }}%
                            <span class="dash-server-badge">{{ formatBytes(hostInfo.disk_total) }}</span>
                        </div>
                        <div class="dash-server-sub">{{ formatBytes(hostInfo.disk_used) }} {{ $t('dashboard.used') }}</div>
                    </div>
                </div>
            </div>
        </div>

        <!-- 3. 服务状态（统计 + 分组服务卡片） -->
        <div class="card no-hover dash-section">
            <div class="card-header">
                <h2 class="card-title">{{ $t('dashboard.service_status') }}</h2>
                <div class="dash-status-summary">
                    <span><span class="status-dot running"></span> <strong style="color:var(--color-running)">{{ statusCount.running }}</strong> {{ $t('dashboard.running') }}</span>
                    <span><span class="status-dot exited"></span> <strong style="color:var(--color-exited)">{{ statusCount.stopped }}</strong> {{ $t('dashboard.stopped') }}</span>
                    <span><span class="status-dot not_deployed"></span> <strong style="color:var(--color-fg-tertiary)">{{ statusCount.not_deployed }}</strong> {{ $t('dashboard.not_deployed') }}</span>
                </div>
            </div>

            <!-- 分组服务卡片 -->
            <div style="padding:0 20px 20px">
                <template v-for="(group, category) in groupedServices" :key="category">
                    <div class="section-title" v-if="group.length > 0" style="margin-top:16px">
                        {{ $t('services.category.' + category) || category }}
                        <span class="count">{{ group.length }}</span>
                    </div>
                    <div class="service-grid">
                        <div
                            class="service-card"
                            v-for="svc in group"
                            :key="svc.name"
                            @click="goToService(svc)"
                        >
                            <span class="status-dot" :class="svc.status"></span>
                            <div class="service-info">
                                <div class="service-name">{{ svc.name }}</div>
                                <div class="service-image">{{ svc.state || '—' }}</div>
                            </div>
                            <span v-if="svc.ports && svc.ports.length" class="mono" style="color:var(--color-fg-tertiary);font-size:0.75rem">
                                :{{ svc.ports[0].host_port }}
                            </span>
                        </div>
                    </div>
                </template>
            </div>
        </div>

        <div v-if="!loading && services.length === 0" style="text-align:center;padding:60px;color:var(--color-fg-tertiary)">
            {{ $t('dashboard.no_services') }}
        </div>
    </div>
    `,
    data() {
        return {
            hostInfo: {},
            projectInfo: {},
            services: [],
            loading: true,
            fetching: false,
            refreshTimer: null,
            categoryLabels: {}
        };
    },
    computed: {
        statusCount() {
            const running = this.services.filter(s => s.status === 'running').length;
            const stopped = this.services.filter(s => s.status === 'exited').length;
            const not_deployed = this.services.filter(s => s.status === 'not_deployed').length;
            return { running, stopped, not_deployed, other: this.services.length - running - stopped - not_deployed };
        },
        groupedServices() {
            const groups = { base: [], backend: [], frontend: [], init: [], other: [] };
            this.services.forEach(s => {
                const cat = s.category || 'other';
                if (!groups[cat]) groups[cat] = [];
                groups[cat].push(s);
            });
            for (const key of Object.keys(groups)) {
                if (groups[key].length === 0) delete groups[key];
            }
            return groups;
        },
        normalizedIPs() {
            const seen = new Set();
            return (this.hostInfo.ips || [])
                .map(ip => String(ip || '').trim())
                .filter(ip => {
                    if (!ip || seen.has(ip)) return false;
                    seen.add(ip);
                    return true;
                });
        },
        visibleIPs() {
            return this.normalizedIPs.slice(0, 2);
        },
        hasMoreIPs() {
            return this.normalizedIPs.length > 2;
        }
    },
    methods: {
        async fetchData() {
            if (this.fetching) return;
            this.fetching = true;
            try {
                const [host, services, project] = await Promise.all([
                    API.getHostInfo(),
                    API.getServices(),
                    API.getProjectSettings()
                ]);
                this.hostInfo = host || {};
                this.services = services || [];
                this.projectInfo = project || {};
            } catch (e) {
                if (e.message && !e.message.includes('fetch')) {
                    Toast.error(e.message);
                }
            }
            this.fetching = false;
            this.loading = false;
        },
        formatBytes(bytes) {
            if (!bytes) return '—';
            const units = ['B', 'KB', 'MB', 'GB', 'TB'];
            let i = 0, val = bytes;
            while (val >= 1024 && i < units.length - 1) { val /= 1024; i++; }
            return val.toFixed(1) + ' ' + units[i];
        },
        goToService(svc) {
            this.$router.push({ name: 'services' });
        }
    },
    mounted() {
        this.fetchData();
        this.refreshTimer = setInterval(() => this.fetchData(), 15000);
    },
    beforeUnmount() {
        if (this.refreshTimer) clearInterval(this.refreshTimer);
    }
};
