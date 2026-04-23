/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * 两阶段升级弹窗组件（Pull Image -> Confirm Apply）
 * Pull 流程在组件内，真正升级由父页面编排。
 * 第二步确认升级后弹窗立即关闭，行内 loading 接管反馈。
 */
const UpgradeModal = {
    template: `
    <div class="modal-overlay" v-if="visible" @click.self="$emit('close')">
        <div class="modal" style="max-width:460px">
            <div class="modal-header">
                <h3 class="modal-title">⬆ {{ $t('services.actions.upgrade') }} — {{ serviceName }}</h3>
                <button class="modal-close" @click="$emit('close')">✕</button>
            </div>
            <div class="modal-body">
                <div class="confirm-dialog">
                    <div style="margin-bottom:12px">
                        <div class="form-label">{{ $t('services.modal.current_version') }}</div>
                        <div class="mono" style="padding:8px 14px;background:var(--color-bg-muted);border-radius:var(--radius);font-size:0.9rem">{{ currentVer }}</div>
                    </div>
                    <div style="margin-bottom:16px">
                        <div class="form-label">{{ $t('services.modal.target_version') }}</div>
                        <div class="mono" style="padding:8px 14px;background:#ECFDF5;border-radius:var(--radius);font-size:0.9rem;color:#065F46;font-weight:600">{{ targetVer }}</div>
                    </div>
                    <div v-if="pullStatus === 'none'" style="background:#FFFBEB;padding:12px 16px;border-radius:var(--radius);font-size:0.85rem;color:#92400E;margin-bottom:16px">
                        ℹ️ {{ $t('services.modal.pull_hint') }}
                    </div>
                    <div v-if="pullStatus === 'pulling'" style="display:flex;align-items:center;gap:10px;padding:14px;background:var(--color-bg-muted);border-radius:var(--radius);margin-bottom:16px">
                        <div class="loading-spinner" style="width:18px;height:18px;flex-shrink:0"></div>
                        <span style="font-size:0.9rem;color:var(--color-fg-secondary)">{{ $t('services.pull_progress') }}</span>
                    </div>
                    <div v-if="pullStatus === 'success'" style="padding:12px 16px;background:#ECFDF5;border-radius:var(--radius);font-size:0.85rem;color:#065F46;margin-bottom:16px">
                        ✅ {{ $t('services.pull_success') }}
                    </div>
                    <div v-if="pullStatus === 'failed'" style="padding:12px 16px;background:#FEF2F2;border-radius:var(--radius);font-size:0.85rem;color:#991B1B;margin-bottom:16px">
                        ❌ {{ $t('services.pull_failed') }}：{{ pullMessage }}
                    </div>
                    <div v-if="pullStatus === 'success'" style="background:#FFFBEB;padding:12px 16px;border-radius:var(--radius);font-size:0.85rem;color:#92400E;margin-bottom:16px">
                        ⚠️ {{ $t('services.modal.upgrade_warning') }}
                    </div>
                    <div class="btn-group" style="margin-top:20px">
                        <button class="btn btn-ghost" @click="$emit('close')">{{ $t('common.cancel') }}</button>
                        <button
                            v-if="pullStatus === 'none' || pullStatus === 'failed'"
                            class="btn btn-primary"
                            @click="startPull"
                            :disabled="pulling"
                        >
                            🔽 {{ $t('services.actions.pull') }}
                        </button>
                        <button
                            v-if="pullStatus === 'success'"
                            class="btn btn-primary"
                            @click="$emit('apply', service)"
                        >
                            🚀 {{ $t('common.confirm') }}{{ $t('services.actions.upgrade') }}
                        </button>
                    </div>
                </div>
            </div>
        </div>
    </div>
    `,
    props: {
        visible: Boolean,
        service: { type: Object, default: null }
    },
    emits: ['close', 'apply'],
    data() {
        return {
            pullStatus: 'none',
            pullMessage: '',
            pulling: false
        };
    },
    computed: {
        serviceName() {
            return this.service ? this.service.name : '';
        },
        currentVer() {
            return this.service ? ServicesRules.extractVersion(this.service.running_image) : '—';
        },
        targetVer() {
            return this.service ? ServicesRules.extractVersion(this.service.declared_image) : '—';
        }
    },
    watch: {
        visible(val) {
            if (val) {
                this.resetState();
            }
        }
    },
    methods: {
        resetState() {
            this.pullStatus = 'none';
            this.pullMessage = '';
            this.pulling = false;
        },
        async startPull() {
            this.pulling = true;
            this.pullStatus = 'pulling';
            try {
                await API.pullImage(this.serviceName);
            } catch (e) {
                this.pullStatus = 'failed';
                this.pullMessage = e.message;
                this.pulling = false;
                return;
            }

            const capturedName = this.serviceName;
            const pollPull = async () => {
                if (!this.visible || this.serviceName !== capturedName) return;
                try {
                    const status = await API.getPullStatus(capturedName);
                    if (status.status === 'success') {
                        this.pullStatus = 'success';
                        this.pulling = false;
                        return;
                    }
                    if (status.status === 'failed') {
                        this.pullStatus = 'failed';
                        this.pullMessage = status.message || this.$t('services.pull_failed');
                        this.pulling = false;
                        return;
                    }
                } catch (e) { /* 继续轮询 */ }

                if (this.visible && this.pullStatus === 'pulling') {
                    setTimeout(pollPull, 2000);
                }
            };

            setTimeout(pollPull, 2000);
        }
    }
};
