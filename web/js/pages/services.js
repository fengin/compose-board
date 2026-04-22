/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * 服务管理页面组件 — 以 Compose 声明为主视图
 * 替代旧版 containers.js（按容器 ID 操作）
 */
const ServicesPage = {
    template: `
    <div>
        <div class="card no-hover">
            <div class="card-header">
                <h2 class="card-title">{{ $t('services.title') }}</h2>
                <button class="btn btn-ghost btn-sm" @click="fetchServices" :disabled="loading">
                    <span v-if="loading" class="loading-spinner" style="width:16px;height:16px"></span>
                    <span v-else>↻ {{ $t('services.refresh') }}</span>
                </button>
            </div>

            <!-- 必选服务（按 category 分组） -->
            <template v-for="(group, category) in requiredGroups" :key="category">
                <div class="section-title" v-if="group.length > 0">
                    {{ $t('services.category.' + category) || category }}
                    <span class="count">{{ group.length }}</span>
                </div>
                <div class="table-wrapper" style="margin-bottom:16px">
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
                                <th style="width:200px">{{ $t('services.table.actions') }}</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="svc in group" :key="svc.name" :data-status="svc.status">
                                <td><span class="status-dot" :class="svc.status"></span></td>
                                <td>
                                    <div style="font-weight:600">{{ svc.name }}</div>
                                    <span v-if="svc.image_source === 'build'" class="badge-build">🔧 {{ $t('services.labels.local_build') }}</span>
                                </td>
                                <td>
                                    <span class="mono">{{ getImageVersion(svc) }}</span>
                                    <div v-if="svc.image_diff" style="margin-top:4px">
                                        <span class="version-diff">🔸 → {{ getImageVersion({ declared_image: svc.declared_image }) }}</span>
                                    </div>
                                    <div v-if="svc.pending_env && svc.pending_env.length > 0 && !svc.image_diff" style="margin-top:4px">
                                        <span class="env-diff" :title="svc.pending_env.join(', ')">🔸 {{ $t('services.labels.env_changed') }}</span>
                                    </div>
                                </td>
                                <td>
                                    <span class="status-badge" :class="svc.status">{{ statusLabel(svc.status) }}</span>
                                </td>
                                <td class="mono">
                                    <span v-for="(p, i) in svc.ports" :key="i">
                                        {{ p.host_port }}→{{ p.container_port }}<br v-if="i < svc.ports.length - 1">
                                    </span>
                                    <span v-if="!svc.ports || svc.ports.length === 0" style="color:var(--color-fg-tertiary)">—</span>
                                </td>
                                <td class="mono">{{ svc.status === 'running' ? svc.cpu?.toFixed(1) + '%' : '—' }}</td>
                                <td class="mono">{{ svc.status === 'running' ? formatBytes(svc.mem_usage) : '—' }}</td>
                                <td style="font-size:0.78rem;color:var(--color-fg-secondary)">{{ svc.state || '—' }}</td>
                                <td>
                                    <div v-if="svc._loading" style="display:flex;justify-content:center;align-items:center;height:56px">
                                        <span class="loading-spinner" style="width:20px;height:20px"></span>
                                    </div>
                                    <div v-else class="action-grid">
                                        <!-- 运行中：重启 / 停止 -->
                                        <button v-if="svc.status === 'running'" class="act-btn act-default" @click="confirmAction('restart', svc)">↻ {{ $t('services.actions.restart') }}</button>
                                        <button v-if="svc.status === 'running'" class="act-btn act-danger" @click="confirmAction('stop', svc)">■ {{ $t('services.actions.stop') }}</button>
                                        <!-- 已停止：启动 -->
                                        <button v-if="svc.status === 'exited'" class="act-btn act-primary" @click="confirmAction('start', svc)">▶ {{ $t('services.actions.start') }}</button>
                                        <!-- 未部署 + 必选 + registry：启动 -->
                                        <button v-if="svc.status === 'not_deployed' && svc.image_source !== 'build' && (!svc.profiles || svc.profiles.length === 0)" class="act-btn act-primary" @click="confirmAction('start', svc)">▶ {{ $t('services.actions.start') }}</button>
                                        <!-- 通用：ENV / LOG -->
                                        <button v-if="svc.status !== 'not_deployed'" class="act-btn act-default" @click="showEnv(svc)">⚙ ENV</button>
                                        <button v-if="svc.status === 'running'" class="act-btn act-default" @click="goToLogs(svc)">📋 LOG</button>
                                        <!-- 条件：升级 / 重建 -->
                                        <button v-if="svc.image_diff" class="act-btn act-upgrade" @click="confirmUpgrade(svc)">⬆ {{ $t('services.actions.upgrade') }}</button>
                                        <button v-if="svc.pending_env && svc.pending_env.length > 0 && !svc.image_diff" class="act-btn act-rebuild" @click="confirmRebuild(svc)">🔄 {{ $t('services.actions.rebuild') }}</button>
                                    </div>
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </template>

            <!-- Profile 分组（可选服务） -->
            <template v-for="(profile, profileName) in profiles" :key="profileName">
                <div class="profile-group">
                    <div class="profile-group-header">
                        <div class="profile-name">
                            <span>{{ $t('services.profile.label') }}: {{ profileName }}</span>
                            <span class="profile-status" :class="profile.status">
                                {{ profileStatusLabel(profile.status) }}
                            </span>
                        </div>
                        <div class="profile-actions">
                            <button v-if="canEnableProfile(profile)"
                                class="btn btn-sm btn-primary" @click="doEnableProfile(profileName)"
                                :disabled="profileLoading[profileName]">
                                {{ enableProfileButtonLabel(profile) }}
                            </button>
                            <button v-if="canDisableProfile(profile)"
                                class="btn btn-sm btn-danger" @click="confirmDisableProfile(profileName, profile)"
                                :disabled="profileLoading[profileName]">
                                {{ $t('services.profile.disable_button') }}
                            </button>
                            <span v-if="profileLoading[profileName]" class="loading-spinner" style="width:16px;height:16px"></span>
                        </div>
                    </div>
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
                                    <th style="width:200px">{{ $t('services.table.actions') }}</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr v-for="svc in profile.services" :key="svc.name" :data-status="svc.status">
                                    <td><span class="status-dot" :class="svc.status"></span></td>
                                    <td><div style="font-weight:600">{{ svc.name }}</div></td>
                                    <td><span class="mono">{{ getImageVersion(svc) }}</span></td>
                                    <td><span class="status-badge" :class="svc.status">{{ statusLabel(svc.status) }}</span></td>
                                    <td class="mono">
                                        <span v-for="(p, i) in svc.ports" :key="i">{{ p.host_port }}→{{ p.container_port }}<br v-if="i < svc.ports.length - 1"></span>
                                        <span v-if="!svc.ports || svc.ports.length === 0" style="color:var(--color-fg-tertiary)">—</span>
                                    </td>
                                    <td class="mono">{{ svc.status === 'running' ? svc.cpu?.toFixed(1) + '%' : '—' }}</td>
                                    <td class="mono">{{ svc.status === 'running' ? formatBytes(svc.mem_usage) : '—' }}</td>
                                    <td style="font-size:0.78rem;color:var(--color-fg-secondary)">{{ svc.state || '—' }}</td>
                                    <td>
                                        <!-- C-2: Profile 服务也需要 _loading 渲染 -->
                                        <div v-if="svc._loading" style="display:flex;justify-content:center;align-items:center;height:56px">
                                            <span class="loading-spinner" style="width:20px;height:20px"></span>
                                        </div>
                                        <div v-else-if="svc.status === 'running'" class="action-grid">
                                            <button class="act-btn act-default" @click="confirmAction('restart', svc)">↻ {{ $t('services.actions.restart') }}</button>
                                            <button class="act-btn act-danger" @click="confirmAction('stop', svc)">■ {{ $t('services.actions.stop') }}</button>
                                            <button class="act-btn act-default" @click="showEnv(svc)">⚙ ENV</button>
                                            <button class="act-btn act-default" @click="goToLogs(svc)">📋 LOG</button>
                                        </div>
                                        <div v-else-if="svc.status === 'exited'" class="action-grid">
                                            <button class="act-btn act-primary" @click="confirmAction('start', svc)">▶ {{ $t('services.actions.start') }}</button>
                                            <button class="act-btn act-default" @click="showEnv(svc)">⚙ ENV</button>
                                        </div>
                                    </td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </template>

            <div v-if="!loading && services.length === 0" style="text-align:center;padding:40px;color:var(--color-fg-tertiary)">
                {{ $t('common.none') }}
            </div>
        </div>

        <!-- 环境变量弹窗 -->
        <div class="modal-overlay" v-if="envModal.visible" @click.self="envModal.visible = false">
            <div class="modal">
                <div class="modal-header">
                    <h3 class="modal-title">{{ $t('services.actions.view_env') }} — {{ envModal.serviceName }}</h3>
                    <button class="modal-close" @click="envModal.visible = false">✕</button>
                </div>
                <div class="modal-body">
                    <div v-if="envModal.loading" style="text-align:center;padding:20px">
                        <div class="loading-spinner" style="margin:0 auto"></div>
                    </div>
                    <table class="env-table" v-else>
                        <tr v-for="(val, key) in envModal.data" :key="key">
                            <td>{{ key }}</td>
                            <td>{{ val }}</td>
                        </tr>
                    </table>
                </div>
            </div>
        </div>

        <!-- 确认弹窗 -->
        <div class="modal-overlay" v-if="confirmModal.visible" @click.self="confirmModal.visible = false">
            <div class="modal" style="max-width:440px">
                <div class="modal-header">
                    <h3 class="modal-title">{{ confirmModal.title }}</h3>
                    <button class="modal-close" @click="confirmModal.visible = false">✕</button>
                </div>
                <div class="modal-body">
                    <div class="confirm-dialog">
                        <div v-html="confirmModal.message"></div>
                        <div v-if="confirmModal.warning"
                            style="background:#FFFBEB;padding:12px 16px;border-radius:var(--radius);font-size:0.85rem;color:#92400E;margin:16px 0">
                            {{ confirmModal.warning }}
                        </div>
                        <div class="btn-group" style="margin-top:20px">
                            <button class="btn btn-ghost" @click="confirmModal.visible = false">{{ $t('common.cancel') }}</button>
                            <button class="btn" :class="confirmModal.btnClass || 'btn-primary'" @click="confirmModal.onConfirm" :disabled="confirmModal.executing">
                                <span v-if="confirmModal.executing" class="loading-spinner" style="width:14px;height:14px;border-top-color:#fff"></span>
                                <span v-else>{{ confirmModal.btnText }}</span>
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <!-- 升级弹窗（两阶段） -->
        <div class="modal-overlay" v-if="upgradeModal.visible" @click.self="upgradeModal.visible = false">
            <div class="modal" style="max-width:460px">
                <div class="modal-header">
                    <h3 class="modal-title">⬆ {{ $t('services.actions.upgrade') }} — {{ upgradeModal.serviceName }}</h3>
                    <button class="modal-close" @click="upgradeModal.visible = false">✕</button>
                </div>
                <div class="modal-body">
                    <div class="confirm-dialog">
                        <div style="margin-bottom:12px">
                            <div class="form-label">{{ $t('services.modal.current_version') }}</div>
                            <div class="mono" style="padding:8px 14px;background:var(--color-bg-muted);border-radius:var(--radius);font-size:0.9rem">{{ upgradeModal.currentVer }}</div>
                        </div>
                        <div style="margin-bottom:16px">
                            <div class="form-label">{{ $t('services.modal.target_version') }}</div>
                            <div class="mono" style="padding:8px 14px;background:#ECFDF5;border-radius:var(--radius);font-size:0.9rem;color:#065F46;font-weight:600">{{ upgradeModal.targetVer }}</div>
                        </div>
                        <div v-if="upgradeModal.pullStatus === 'none'" style="background:#FFFBEB;padding:12px 16px;border-radius:var(--radius);font-size:0.85rem;color:#92400E;margin-bottom:16px">
                            ℹ️ {{ $t('services.modal.pull_hint') }}
                        </div>
                        <div v-if="upgradeModal.pullStatus === 'pulling'" style="display:flex;align-items:center;gap:10px;padding:14px;background:var(--color-bg-muted);border-radius:var(--radius);margin-bottom:16px">
                            <div class="loading-spinner" style="width:18px;height:18px;flex-shrink:0"></div>
                            <span style="font-size:0.9rem;color:var(--color-fg-secondary)">{{ $t('services.pull_progress') }}</span>
                        </div>
                        <div v-if="upgradeModal.pullStatus === 'success'" style="padding:12px 16px;background:#ECFDF5;border-radius:var(--radius);font-size:0.85rem;color:#065F46;margin-bottom:16px">
                            ✅ {{ $t('services.pull_success') }}
                        </div>
                        <div v-if="upgradeModal.pullStatus === 'failed'" style="padding:12px 16px;background:#FEF2F2;border-radius:var(--radius);font-size:0.85rem;color:#991B1B;margin-bottom:16px">
                            ❌ {{ $t('services.pull_failed') }}：{{ upgradeModal.pullMessage }}
                        </div>
                        <div v-if="upgradeModal.pullStatus === 'success'" style="background:#FFFBEB;padding:12px 16px;border-radius:var(--radius);font-size:0.85rem;color:#92400E;margin-bottom:16px">
                            ⚠️ {{ $t('services.modal.upgrade_warning') }}
                        </div>
                        <div class="btn-group" style="margin-top:20px">
                            <button class="btn btn-ghost" @click="upgradeModal.visible = false">{{ $t('common.cancel') }}</button>
                            <button v-if="upgradeModal.pullStatus === 'none' || upgradeModal.pullStatus === 'failed'"
                                class="btn btn-primary" @click="startPull" :disabled="upgradeModal.pulling">
                                🔽 {{ $t('services.actions.pull') }}
                            </button>
                            <button v-if="upgradeModal.pullStatus === 'success'"
                                class="btn btn-primary" @click="applyUpgrade" :disabled="upgradeModal.applying">
                                <span v-if="upgradeModal.applying" class="loading-spinner" style="width:14px;height:14px;border-top-color:#fff"></span>
                                <span v-else>🚀 {{ $t('common.confirm') }}{{ $t('services.actions.upgrade') }}</span>
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
    `,
    data() {
        return {
            services: [],
            profiles: {},
            loading: false,
            profileLoading: {},
            envModal: { visible: false, serviceName: '', data: {}, loading: false },
            confirmModal: { visible: false, title: '', message: '', warning: '', btnText: '', btnClass: '', executing: false, onConfirm: null },
            upgradeModal: { visible: false, serviceName: '', currentVer: '', targetVer: '', svc: null, pullStatus: 'none', pullMessage: '', pulling: false, applying: false },
            pollTimer: null
        };
    },
    computed: {
        // 必选服务按 category 分组（后端→前端→基础→初始化→其他，关注度递减）
        requiredGroups() {
            const groups = { backend: [], frontend: [], base: [], init: [], other: [] };
            this.services.filter(s => !s.profiles || s.profiles.length === 0).forEach(s => {
                const cat = s.category || 'other';
                if (!groups[cat]) groups[cat] = [];
                groups[cat].push(s);
            });
            // 组内按服务名字母排序（固定位置，便于持续观察）
            for (const key of Object.keys(groups)) {
                if (groups[key].length === 0) {
                    delete groups[key];
                } else {
                    groups[key].sort((a, b) => a.name.localeCompare(b.name));
                }
            }
            return groups;
        }
    },
    methods: {
        async fetchServices() {
            this.loading = true;
            try {
                const [services, profiles] = await Promise.all([
                    API.getServices(),
                    API.getProfiles()
                ]);
                // 保留正在操作中的 _loading 状态（同时覆盖主列表 + profile.services 双源）
                const loadingNames = new Set();
                this.services.forEach(s => { if (s._loading) loadingNames.add(s.name); });
                for (const pname of Object.keys(this.profiles || {})) {
                    (this.profiles[pname].services || []).forEach(s => {
                        if (s._loading) loadingNames.add(s.name);
                    });
                }

                this.services = (services || []).map(s => ({
                    ...s, _loading: loadingNames.has(s.name)
                }));

                const freshProfiles = profiles || {};
                for (const pname of Object.keys(freshProfiles)) {
                    const p = freshProfiles[pname];
                    if (p && Array.isArray(p.services)) {
                        p.services = p.services.map(s => ({
                            ...s, _loading: loadingNames.has(s.name)
                        }));
                    }
                }
                this.profiles = freshProfiles;
            } catch (e) {
                if (e.message && !e.message.includes('fetch')) {
                    Toast.error(e.message);
                }
            } finally {
                this.loading = false;
            }
        },

        // === 同步操作（start/stop/restart）— B-4: 立即刷新 ===
        confirmAction(action, svc) {
            const labels = { stop: this.$t('services.actions.stop'), restart: this.$t('services.actions.restart'), start: this.$t('services.actions.start') };
            const btnClasses = { stop: 'btn-danger', restart: 'btn-primary', start: 'btn-primary' };
            this.confirmModal = {
                visible: true,
                title: this.$t('services.modal.confirm_title'),
                message: '<p>' + this.$t('services.modal.confirm_action').replace('{action}', labels[action]).replace('{name}', svc.name) + '</p>',
                warning: '',
                btnText: this.$t('services.modal.confirm_btn').replace('{action}', labels[action]),
                btnClass: btnClasses[action] || 'btn-primary',
                executing: false,
                onConfirm: () => this.executeAction(action, svc)
            };
        },
        async executeAction(action, svc) {
            this.confirmModal.executing = true;
            // 操作前记录启动时间（用于 restart 完成判定）
            const startedBefore = svc.started_at || '';

            // 立即关闭弹窗 + 设置 loading（同步更新主列表 + profile.services 双源）
            this.confirmModal.visible = false;
            this.confirmModal.executing = false;
            this.updateServiceRefs(svc.name, { _loading: true });
            Toast.info(svc.name + ' ' + this.$t('services.toast.action_in_progress'));

            // API 后台执行（fire-and-forget），失败时通过 catch 处理
            const apiCall = action === 'stop' ? API.stopService(svc.name)
                : action === 'restart' ? API.restartService(svc.name)
                : API.startService(svc.name);

            apiCall.catch(e => {
                // API 失败：清除 loading，显示错误
                this.updateServiceRefs(svc.name, { _loading: false });
                if (e.code === 'services.start.build_not_supported') {
                    Toast.error(this.$t('services.start.build_not_supported'));
                } else if (e.code === 'services.start.profile_required') {
                    Toast.error(e.message);
                } else {
                    Toast.error(this.$t('common.error') + ': ' + e.message);
                }
            });

            // 启动轮询（与 API 并行）
            this.startAsyncPoll(svc.name, action, startedBefore);
        },

        // === 升级（异步 — 轮询） ===
        confirmUpgrade(svc) {
            this.upgradeModal = {
                visible: true,
                serviceName: svc.name,
                currentVer: this.extractVersion(svc.running_image),
                targetVer: this.extractVersion(svc.declared_image),
                svc: svc,
                pullStatus: 'none',
                pullMessage: '',
                pulling: false,
                applying: false
            };
        },
        async startPull() {
            const m = this.upgradeModal;
            m.pulling = true;
            m.pullStatus = 'pulling';
            try {
                await API.pullImage(m.serviceName);
            } catch (e) {
                m.pullStatus = 'failed';
                m.pullMessage = e.message;
                m.pulling = false;
                return;
            }
            // 轮询拉取状态（T-7: 用 capturedName 防止弹窗切换后闭包污染）
            const capturedName = m.serviceName;
            const pollPull = async () => {
                // 身份校验：弹窗已切换到其他服务时停止旧轮询
                if (this.upgradeModal.serviceName !== capturedName) return;
                try {
                    const st = await API.getPullStatus(capturedName);
                    if (st.status === 'success') { m.pullStatus = 'success'; m.pulling = false; return; }
                    if (st.status === 'failed') { m.pullStatus = 'failed'; m.pullMessage = st.message || this.$t('services.pull_failed'); m.pulling = false; return; }
                } catch (e) { /* 继续 */ }
                if (m.visible && m.pullStatus === 'pulling') setTimeout(pollPull, 2000);
            };
            setTimeout(pollPull, 2000);
        },
        async applyUpgrade() {
            const m = this.upgradeModal;
            m.applying = true;
            try {
                // 操作前记录启动时间
                const startedBefore = m.svc?.started_at || '';
                await API.applyUpgrade(m.serviceName);
                Toast.info(this.$t('services.toast.upgrade_started'));
                m.visible = false;
                m.applying = false;
                const target = this.services.find(s => s.name === m.serviceName);
                if (target) target._loading = true;
                this.startAsyncPoll(m.serviceName, 'upgrade', startedBefore);
            } catch (e) {
                Toast.error(this.$t('services.toast.upgrade_failed') + ': ' + e.message);
                m.applying = false;
            }
        },

        // === 重建（异步 — 轮询） ===
        confirmRebuild(svc) {
            const varList = svc.pending_env.join(', ');
            this.confirmModal = {
                visible: true,
                title: '🔄 ' + this.$t('services.actions.rebuild') + ' — ' + svc.name,
                message: '<div><div class="form-label">' + this.$t('services.modal.changed_vars') + '</div><div class="mono" style="padding:8px 14px;background:#F5F3FF;border-radius:var(--radius);font-size:0.85rem;color:#5B21B6">' + varList + '</div></div>',
                warning: '⚠️ ' + this.$t('services.modal.rebuild_warning'),
                btnText: this.$t('services.modal.confirm_rebuild'),
                btnClass: 'btn-primary',
                executing: false,
                onConfirm: () => this.executeRebuild(svc)
            };
        },
        async executeRebuild(svc) {
            this.confirmModal.executing = true;
            try {
                // 操作前记录启动时间
                const startedBefore = svc.started_at || '';
                await API.rebuildService(svc.name);
                Toast.info(svc.name + ' ' + this.$t('services.toast.rebuild_started'));
                this.confirmModal.visible = false;
                this.confirmModal.executing = false;
                const target = this.services.find(s => s.name === svc.name);
                if (target) target._loading = true;
                this.startAsyncPoll(svc.name, 'rebuild', startedBefore);
            } catch (e) {
                Toast.error(this.$t('services.toast.rebuild_failed') + ': ' + e.message);
                this.confirmModal.executing = false;
            }
        },

        // === 异步轮询（统一 stop/start/restart/upgrade/rebuild） ===
        startAsyncPoll(serviceName, action, startedBefore = '') {
            const startTime = Date.now();
            const maxWait = action === 'upgrade' ? 5 * 60 * 1000 : 2 * 60 * 1000;
            let failCount = 0;

            const check = async () => {
                if (Date.now() - startTime > maxWait) {
                    this.updateServiceRefs(serviceName, { _loading: false });
                    Toast.error(serviceName + ' ' + this.$t('services.toast.operation_timeout'));
                    this.fetchServices();
                    return;
                }
                try {
                    const freshList = await API.getServices();
                    const fresh = (freshList || []).find(s => s.name === serviceName);
                    if (!fresh) { setTimeout(check, 3000); return; }

                    // ★ 实时同步更新（主列表 + profile.services 双源）
                    this.updateServiceRefs(serviceName, {
                        status: fresh.status,
                        state: fresh.state,
                        ports: fresh.ports,
                        cpu: fresh.cpu,
                        mem_usage: fresh.mem_usage,
                        mem_limit: fresh.mem_limit,
                        mem_percent: fresh.mem_percent,
                        running_image: fresh.running_image,
                        image_diff: fresh.image_diff,
                        pending_env: fresh.pending_env,
                        started_at: fresh.started_at,
                        container_id: fresh.container_id
                    });

                    // ★ 目标状态判定
                    let done = false;
                    if (action === 'stop' && fresh.status === 'exited') done = true;
                    if (action === 'start' && fresh.status === 'running') done = true;
                    // restart/upgrade/rebuild: running + started_at 变化
                    if (['restart', 'upgrade', 'rebuild'].includes(action)
                        && fresh.status === 'running'
                        && fresh.started_at !== startedBefore) done = true;
                    // upgrade 额外检查 image_diff 消失
                    if (action === 'upgrade' && done && fresh.image_diff) done = false;
                    // rebuild 额外检查 pending_env 清空
                    if (action === 'rebuild' && done
                        && fresh.pending_env && fresh.pending_env.length > 0) done = false;

                    if (done) {
                        this.updateServiceRefs(serviceName, { _loading: false });
                        const doneKeys = { stop: 'stop_done', start: 'start_done',
                            restart: 'restart_done', upgrade: 'upgrade_done', rebuild: 'rebuild_done' };
                        Toast.success(serviceName + ' ' + this.$t('services.toast.' + doneKeys[action]));
                        this.fetchServices();
                        return;
                    }

                    // C-3: 异常状态快速失败（结合 started_at 区分中间态和真失败）
                    // - start: started_at 未变化且持续 exited → 启动失败
                    // - restart/upgrade/rebuild: started_at 已变化但又 exited → 新容器启动后崩溃
                    // 其它情况（正在停/拉起，started_at 未变）视为中间态，不计入失败
                    if ((fresh.status === 'exited' || fresh.status === 'restarting') && action !== 'stop') {
                        const changed = fresh.started_at && fresh.started_at !== startedBefore;
                        const reallyFailed = action === 'start' ? !changed : changed;
                        if (reallyFailed) {
                            failCount++;
                            if (failCount >= 3) {
                                this.updateServiceRefs(serviceName, { _loading: false });
                                Toast.error(serviceName + ' ' + this.$t('services.toast.operation_failed'));
                                this.fetchServices();
                                return;
                            }
                        } else {
                            failCount = 0;
                        }
                    } else {
                        failCount = 0;
                    }
                } catch (e) { /* 网络异常，继续轮询 */ }
                setTimeout(check, 3000);
            };
            // T-4: 首次轮询延迟 1 秒，对 stop/start 等轻操作更快响应
            setTimeout(check, 1000);
        },

        // === Profile 操作（与主列表一致：立即关弹窗 + 批量 loading + 轮询校验目标状态） ===
        async doEnableProfile(name) {
            this.profileLoading = { ...this.profileLoading, [name]: true };
            this.setProfileServicesLoading(name, true);
            Toast.info(name + ' ' + this.$t('services.profile.enabling'));

            try {
                await API.enableProfile(name);
            } catch (e) {
                this.profileLoading = { ...this.profileLoading, [name]: false };
                this.setProfileServicesLoading(name, false);
                Toast.error(e.message);
                return;
            }

            this.startProfilePoll(name, 'enable');
        },
        confirmDisableProfile(name, profile) {
            const serviceNames = (profile.services || []).map(s => s.name).join(', ');
            this.confirmModal = {
                visible: true,
                title: this.$t('services.profile.disable_button') + ' ' + this.$t('services.profile.label'),
                message: '<p>' + this.$t('services.profile.confirm_disable').replace('{name}', name) + '</p><div class="mono" style="padding:8px 14px;background:#FEF2F2;border-radius:var(--radius);font-size:0.85rem;color:#991B1B">' + serviceNames + '</div>',
                warning: '',
                btnText: this.$t('services.modal.confirm_btn').replace('{action}', this.$t('services.profile.disable_button')),
                btnClass: 'btn-danger',
                executing: false,
                onConfirm: () => this.doDisableProfile(name)
            };
        },
        async doDisableProfile(name) {
            // 立即关弹窗 + 批量 loading（与主列表 executeAction 一致）
            this.confirmModal.visible = false;
            this.confirmModal.executing = false;
            this.profileLoading = { ...this.profileLoading, [name]: true };
            this.setProfileServicesLoading(name, true);
            Toast.info(name + ' ' + this.$t('services.profile.disabling'));

            try {
                await API.disableProfile(name);
            } catch (e) {
                this.profileLoading = { ...this.profileLoading, [name]: false };
                this.setProfileServicesLoading(name, false);
                Toast.error(e.message);
                return;
            }

            this.startProfilePoll(name, 'disable');
        },
        startProfilePoll(profileName, action) {
            const startTime = Date.now();
            const maxWait = 3 * 60 * 1000;
            let failCount = 0;

            const finish = (ok, toastMsg) => {
                this.profileLoading = { ...this.profileLoading, [profileName]: false };
                this.setProfileServicesLoading(profileName, false);
                if (toastMsg) (ok ? Toast.success : Toast.error)(toastMsg);
                this.fetchServices();
            };

            const check = async () => {
                if (Date.now() - startTime > maxWait) {
                    finish(false, profileName + ' ' + this.$t('services.toast.operation_timeout'));
                    return;
                }
                try {
                    const freshProfiles = await API.getProfiles();
                    const freshProfile = freshProfiles ? freshProfiles[profileName] : null;

                    // ★ 实时 in-place 更新 profile.status + 下属服务数据（保留 _loading）
                    const current = this.profiles[profileName];
                    if (current && freshProfile) {
                        current.status = freshProfile.status;
                        for (const fs of (freshProfile.services || [])) {
                            this.updateServiceRefs(fs.name, {
                                status: fs.status,
                                state: fs.state,
                                ports: fs.ports,
                                cpu: fs.cpu,
                                mem_usage: fs.mem_usage,
                                mem_limit: fs.mem_limit,
                                mem_percent: fs.mem_percent,
                                running_image: fs.running_image,
                                image_diff: fs.image_diff,
                                pending_env: fs.pending_env,
                                started_at: fs.started_at,
                                container_id: fs.container_id
                            });
                        }
                    }

                    // ★ 目标状态判定
                    const services = freshProfile?.services || [];
                    const allRunning = services.length > 0 && services.every(s => s.status === 'running');
                    const allRemoved = services.length > 0 && services.every(s => s.status === 'not_deployed');
                    const allCreated = services.length > 0 && services.every(s => s.status !== 'not_deployed');
                    const hasUnhealthy = services.some(s => s.status === 'restarting' || s.status === 'exited');

                    const done = action === 'enable'
                        ? freshProfile && freshProfile.status === 'enabled' && allRunning
                        : freshProfile && freshProfile.status === 'disabled' && allRemoved;

                    if (done) {
                        const msgKey = action === 'enable'
                            ? 'services.profile.enable_success'
                            : 'services.profile.disable_success';
                        finish(true, this.$t(msgKey).replace('{name}', profileName));
                        return;
                    }

                    if (action === 'enable' && allCreated && hasUnhealthy) {
                        failCount++;
                        if (failCount >= 3) {
                            finish(false, profileName + ' ' + this.$t('services.toast.operation_failed'));
                            return;
                        }
                    } else {
                        failCount = 0;
                    }
                } catch (e) { /* 网络异常，继续轮询 */ }
                setTimeout(check, 3000);
            };
            setTimeout(check, 300);
        },

        // === 状态同步辅助（主列表 ↔ profile.services 双源一致性） ===
        updateServiceRefs(name, fields) {
            const target = this.services.find(s => s.name === name);
            if (target) Object.assign(target, fields);
            for (const pname of Object.keys(this.profiles || {})) {
                const p = this.profiles[pname];
                if (!p || !Array.isArray(p.services)) continue;
                const t = p.services.find(s => s.name === name);
                if (t) Object.assign(t, fields);
            }
        },
        setProfileServicesLoading(profileName, loading) {
            const p = this.profiles ? this.profiles[profileName] : null;
            if (!p || !Array.isArray(p.services)) return;
            for (const s of p.services) {
                this.updateServiceRefs(s.name, { _loading: loading });
            }
        },
        canEnableProfile(profile) {
            if (!profile) return false;
            if (profile.status === 'disabled') return true;
            if (profile.status !== 'partial') return false;
            return !this.profileHasRestartingServices(profile);
        },
        canDisableProfile(profile) {
            return !!profile && (profile.status === 'enabled' || profile.status === 'partial');
        },
        enableProfileButtonLabel(profile) {
            return profile && profile.status === 'partial'
                ? this.$t('services.profile.fix_button')
                : this.$t('services.profile.enable_button');
        },
        profileHasRestartingServices(profile) {
            return !!profile
                && Array.isArray(profile.services)
                && profile.services.some(s => s.status === 'restarting');
        },

        // === 工具方法 ===
        async showEnv(svc) {
            this.envModal = { visible: true, serviceName: svc.name, data: {}, loading: true };
            try {
                this.envModal.data = await API.getServiceEnv(svc.name);
            } catch (e) {
                Toast.error(e.message);
            } finally {
                this.envModal.loading = false;
            }
        },
        goToLogs(svc) {
            this.$router.push({ name: 'logs', query: { service: svc.name } });
        },
        getImageVersion(svc) {
            const image = svc.running_image || svc.declared_image || svc.image_ref || '';
            return this.extractVersion(image);
        },
        extractVersion(imageStr) {
            if (!imageStr) return '—';
            const parts = imageStr.split(':');
            if (parts.length > 1) return parts[parts.length - 1];
            return imageStr;
        },
        statusLabel(status) {
            const key = 'services.status.' + status;
            const t = this.$t(key);
            return t !== key ? t : status;
        },
        profileStatusLabel(status) {
            const icons = { enabled: '✅ ', partial: '⚠ ', disabled: '⭕ ' };
            return (icons[status] || '') + this.$t('services.profile.status_' + status);
        },
        formatBytes(bytes) {
            if (!bytes) return '—';
            const units = ['B', 'KB', 'MB', 'GB'];
            let i = 0, val = bytes;
            while (val >= 1024 && i < units.length - 1) { val /= 1024; i++; }
            return val.toFixed(1) + ' ' + units[i];
        }
    },
    mounted() {
        this.fetchServices();
        // T-1: 定时刷新，保持 CPU/内存/运行时长实时
        this.pollTimer = setInterval(() => this.fetchServices(), 10000);
    },
    beforeUnmount() {
        if (this.pollTimer) clearInterval(this.pollTimer);
    }
};
