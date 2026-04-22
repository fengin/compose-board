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
        <!-- 项目信息卡片（B-1） -->
        <div class="card no-hover" style="margin-bottom:24px" v-if="projectInfo.project_name">
            <div class="card-header">
                <h2 class="card-title">{{ $t('dashboard.project_info') }}</h2>
            </div>
            <div style="padding:0 20px 16px;display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:12px">
                <div>
                    <div style="font-size:0.78rem;color:var(--color-fg-tertiary)">{{ $t('dashboard.project_name') }}</div>
                    <div style="font-weight:600">{{ projectInfo.project_name }}</div>
                </div>
                <div>
                    <div style="font-size:0.78rem;color:var(--color-fg-tertiary)">{{ $t('dashboard.compose_version') }}</div>
                    <div class="mono" style="font-size:0.9rem">{{ projectInfo.compose_command }} {{ projectInfo.compose_version }}</div>
                </div>
                <div>
                    <div style="font-size:0.78rem;color:var(--color-fg-tertiary)">{{ $t('dashboard.service_count') }}</div>
                    <div style="font-weight:600">{{ projectInfo.service_count || 0 }}</div>
                </div>
                <div v-if="projectInfo.profile_names && projectInfo.profile_names.length">
                    <div style="font-size:0.78rem;color:var(--color-fg-tertiary)">{{ $t('dashboard.profiles') }}</div>
                    <div class="mono" style="font-size:0.85rem">{{ projectInfo.profile_names.join(', ') }}</div>
                </div>
            </div>
        </div>

        <!-- 主机指标卡 -->
        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-icon blue">⚡</div>
                <div class="stat-value">{{ hostInfo.cpu_percent?.toFixed(1) || '—' }}%</div>
                <div class="stat-label">{{ $t('dashboard.cpu') }}</div>
                <div style="margin-top:8px;font-size:0.8rem;color:var(--color-fg-tertiary)">
                    {{ hostInfo.cpu_cores || '—' }} {{ $t('dashboard.cores') }}
                </div>
            </div>
            <div class="stat-card">
                <div class="stat-icon green">📊</div>
                <div class="stat-value">{{ hostInfo.mem_percent?.toFixed(1) || '—' }}%</div>
                <div class="stat-label">{{ $t('dashboard.memory') }}</div>
                <div style="margin-top:8px;font-size:0.8rem;color:var(--color-fg-tertiary)">
                    {{ formatBytes(hostInfo.mem_used) }} / {{ formatBytes(hostInfo.mem_total) }}
                </div>
            </div>
            <div class="stat-card">
                <div class="stat-icon amber">💾</div>
                <div class="stat-value">{{ hostInfo.disk_percent?.toFixed(1) || '—' }}%</div>
                <div class="stat-label">{{ $t('dashboard.disk') }}</div>
                <div style="margin-top:8px;font-size:0.8rem;color:var(--color-fg-tertiary)">
                    {{ formatBytes(hostInfo.disk_used) }} / {{ formatBytes(hostInfo.disk_total) }}
                </div>
            </div>
            <div class="stat-card">
                <div class="stat-icon red">🌐</div>
                <div class="stat-value" style="font-size:1.25rem">{{ hostInfo.ips?.[0] || '—' }}</div>
                <div class="stat-label">{{ $t('dashboard.host_ip') }}</div>
                <div style="margin-top:8px;font-size:0.8rem;color:var(--color-fg-tertiary)">
                    Docker {{ hostInfo.docker_version || '—' }}
                </div>
            </div>
        </div>

        <!-- 服务状态 -->
        <div class="card no-hover" style="margin-bottom:24px">
            <div class="card-header">
                <h2 class="card-title">{{ $t('dashboard.service_status') }}</h2>
                <div style="display:flex;gap:20px;font-size:0.9rem">
                    <span><span class="status-dot running"></span> <strong style="color:var(--color-running)">{{ statusCount.running }}</strong> {{ $t('dashboard.running') }}</span>
                    <span><span class="status-dot exited"></span> <strong style="color:var(--color-exited)">{{ statusCount.stopped }}</strong> {{ $t('dashboard.stopped') }}</span>
                    <span><span class="status-dot not_deployed"></span> <strong style="color:var(--color-fg-tertiary)">{{ statusCount.not_deployed }}</strong> {{ $t('dashboard.not_deployed') }}</span>
                </div>
            </div>
        </div>

        <!-- 服务卡片 (分组) -->
        <template v-for="(group, category) in groupedServices" :key="category">
            <div class="section-title" v-if="group.length > 0">
                {{ $t('services.category.' + category) || category }}
                <span class="count">{{ group.length }}</span>
            </div>
            <div class="service-grid" style="margin-bottom:24px">
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
