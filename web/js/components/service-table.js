/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * 服务表格组件（纯展示）
 * Props: rows
 * Events: action({ service, type })
 */
const ServiceTable = {
    template: `
    <div class="table-wrapper">
        <table class="table">
            <thead>
                <tr>
                    <th style="width:28px"></th>
                    <th style="min-width:120px">{{ $t('services.table.service_name') }}</th>
                    <th style="min-width:130px">{{ $t('services.table.image_version') }}</th>
                    <th style="width:100px">{{ $t('services.table.status') }}</th>
                    <th style="width:100px">{{ $t('services.table.ports') }}</th>
                    <th style="width:90px">{{ $t('services.table.cpu') }}</th>
                    <th style="width:100px">{{ $t('services.table.memory') }}</th>
                    <th style="width:110px">{{ $t('services.table.uptime') }}</th>
                    <th style="width:220px">{{ $t('services.table.actions') }}</th>
                </tr>
            </thead>
            <tbody>
                <tr v-for="row in rows" :key="row.key" :data-status="row.display.status">
                    <td><span class="status-dot" :class="row.display.status"></span></td>
                    <td>
                        <div style="font-weight:600">{{ row.display.name }}</div>
                        <span v-if="row.display.showBuildBadge" class="badge-build">🔧 {{ $t('services.labels.local_build') }}</span>
                    </td>
                    <td>
                        <span class="mono">{{ row.display.currentVersion }}</span>
                        <div v-if="row.display.nextVersion" style="margin-top:4px">
                            <span class="version-diff">🔸 → {{ row.display.nextVersion }}</span>
                        </div>
                        <div v-if="row.display.hasEnvDiff" style="margin-top:4px">
                            <span class="env-diff" :title="row.display.envChangedTitle">🔸 {{ $t('services.labels.env_changed') }}</span>
                        </div>
                    </td>
                    <td>
                        <div style="display:flex;flex-direction:column;align-items:flex-start;gap:4px">
                            <span class="status-badge" :class="row.display.status">{{ statusLabel(row.display.status) }}</span>
                            <span v-if="row.display.startupWarning" class="startup-warning">{{ $t('services.labels.startup_warning') }}</span>
                        </div>
                    </td>
                    <td class="mono">
                        <span v-for="(p, i) in row.display.ports" :key="i">
                            {{ p.host_port }}→{{ p.container_port }}<br v-if="i < row.display.ports.length - 1">
                        </span>
                        <span v-if="row.display.ports.length === 0" style="color:var(--color-fg-tertiary)">—</span>
                    </td>
                    <td class="mono">{{ row.display.cpuText }}</td>
                    <td class="mono">{{ row.display.memoryText }}</td>
                    <td style="font-size:0.78rem;color:var(--color-fg-secondary)">{{ row.display.uptimeText }}</td>
                    <td>
                        <div v-if="row.loading" style="display:flex;justify-content:center;align-items:center;height:56px">
                            <span class="loading-spinner" style="width:20px;height:20px"></span>
                        </div>
                        <div v-else class="action-grid">
                            <button
                                v-for="action in row.actions"
                                :key="action"
                                class="act-btn"
                                :class="actionClass(action)"
                                @click="$emit('action', { service: row.service, type: action })"
                            >
                                {{ actionIcon(action) }} {{ actionLabel(action) }}
                            </button>
                            <span v-if="!row.actions || row.actions.length === 0" style="color:var(--color-fg-tertiary)">—</span>
                        </div>
                    </td>
                </tr>
            </tbody>
        </table>
    </div>
    `,
    props: {
        rows: { type: Array, default: () => [] }
    },
    emits: ['action'],
    methods: {
        statusLabel(status) {
            const key = 'services.status.' + status;
            const translated = this.$t(key);
            return translated !== key ? translated : status;
        },
        actionClass(type) {
            const classes = {
                stop: 'act-danger',
                start: 'act-primary',
                restart: 'act-default',
                'show-env': 'act-default',
                'go-logs': 'act-default',
                'go-terminal': 'act-default',
                upgrade: 'act-upgrade',
                rebuild: 'act-rebuild'
            };
            return classes[type] || 'act-default';
        },
        actionIcon(type) {
            const icons = {
                start: '▶',
                stop: '■',
                restart: '↻',
                'show-env': '⚙',
                'go-logs': '📋',
                'go-terminal': '▣',
                upgrade: '⬆',
                rebuild: '🔄'
            };
            return icons[type] || '•';
        },
        actionLabel(type) {
            const keys = {
                start: 'services.actions.start',
                stop: 'services.actions.stop',
                restart: 'services.actions.restart',
                'show-env': 'services.actions.view_env',
                'go-logs': 'services.actions.view_logs',
                'go-terminal': 'services.actions.open_terminal',
                upgrade: 'services.actions.upgrade',
                rebuild: 'services.actions.rebuild'
            };
            return this.$t(keys[type] || type);
        }
    }
};
