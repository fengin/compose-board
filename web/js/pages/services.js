/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * 服务管理页面
 * 职责：
 * 1. 批量拉取列表基线（services + profiles）
 * 2. 单服务操作统一走实时状态轮询
 * 3. Profile 仅表达配置启用态，组内服务状态仍然以服务实时状态为准
 */
const ServicesPage = {
    template: `
    <div>
        <div class="card no-hover">
            <div class="card-header">
                <h2 class="card-title">{{ $t('services.title') }}</h2>
                <button class="btn btn-ghost btn-sm" @click="fetchPageData" :disabled="loading">
                    <span v-if="loading" class="loading-spinner" style="width:16px;height:16px"></span>
                    <span v-else>↻ {{ $t('services.refresh') }}</span>
                </button>
            </div>

            <template v-for="(rows, category) in requiredRowsByGroup" :key="category">
                <div class="section-title" v-if="rows.length > 0">
                    {{ $t('services.category.' + category) || category }}
                    <span class="count">{{ rows.length }}</span>
                </div>
                <service-table
                    :rows="rows"
                    style="margin-bottom:16px"
                    @action="handleRowAction"
                ></service-table>
            </template>

            <template v-for="profileName in profileNames" :key="profileName">
                <div class="profile-group">
                    <div class="profile-group-header">
                        <div class="profile-name">
                            <span>{{ $t('services.profile.label') }}: {{ profileName }}</span>
                            <span class="profile-status" :class="profiles[profileName].status">
                                {{ profileStatusLabel(profiles[profileName].status) }}
                            </span>
                        </div>
                        <div class="profile-actions">
                            <button
                                v-if="canEnableProfile(profiles[profileName])"
                                class="btn btn-sm btn-primary"
                                @click="doEnableProfile(profileName)"
                                :disabled="isProfileLoading(profileName)"
                            >
                                {{ $t('services.profile.enable_button') }}
                            </button>
                            <button
                                v-if="canDisableProfile(profiles[profileName])"
                                class="btn btn-sm btn-danger"
                                @click="confirmDisableProfile(profileName)"
                                :disabled="isProfileLoading(profileName)"
                            >
                                {{ $t('services.profile.disable_button') }}
                            </button>
                            <span v-if="isProfileLoading(profileName)" class="loading-spinner" style="width:16px;height:16px"></span>
                        </div>
                    </div>
                    <service-table
                        :rows="profileRowsByGroup[profileName] || []"
                        @action="handleRowAction"
                    ></service-table>
                </div>
            </template>

            <div v-if="!loading && services.length === 0" style="text-align:center;padding:40px;color:var(--color-fg-tertiary)">
                {{ $t('common.none') }}
            </div>
        </div>

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

        <confirm-dialog
            :visible="confirmModal.visible"
            :title="confirmModal.title"
            :message="confirmModal.message"
            :warning="confirmModal.warning"
            :btn-text="confirmModal.btnText"
            :btn-class="confirmModal.btnClass"
            @confirm="onConfirmModalConfirm"
            @cancel="closeConfirmModal"
        ></confirm-dialog>

        <upgrade-modal
            :visible="upgradeModal.visible"
            :service="upgradeModal.svc"
            @close="closeUpgradeModal"
            @apply="applyUpgrade"
        ></upgrade-modal>
    </div>
    `,
    data() {
        return {
            services: [],
            profiles: {},
            loading: false,
            serviceOps: {},
            profileOps: {},
            opSeq: 0,
            pollTimer: null,
            syncTimer: null,
            envModal: { visible: false, serviceName: '', data: {}, loading: false },
            confirmModal: { visible: false, title: '', message: '', warning: '', btnText: '', btnClass: '', onConfirm: null },
            upgradeModal: { visible: false, svc: null }
        };
    },
    computed: {
        enabledProfiles() {
            return ServicesOps.getEnabledProfiles(this.profiles);
        },
        requiredRowsByGroup() {
            const groups = ServicesRules.groupRequiredServices(this.services);
            const rowsByGroup = {};
            for (const category of Object.keys(groups)) {
                rowsByGroup[category] = groups[category].map(service => this.buildServiceRow(service));
            }
            return rowsByGroup;
        },
        profileRowsByGroup() {
            const rows = {};
            for (const profileName of this.profileNames) {
                const profileServices = this.getServicesForProfile(profileName);
                rows[profileName] = profileServices.map(service => this.buildServiceRow(service, profileName));
            }
            return rows;
        },
        profileNames() {
            return ServicesOps.getSortedProfileNames(this.profiles);
        }
    },
    methods: {
        async fetchPageData(options = {}) {
            const silent = !!options.silent;
            if (!silent) {
                this.loading = true;
            }

            try {
                const [services, profiles] = await Promise.all([
                    API.getServices(),
                    API.getProfiles()
                ]);
                this.syncServicesSnapshot(services);
                this.syncProfilesSnapshot(profiles);
            } catch (e) {
                if (e.message && !e.message.includes('fetch')) {
                    Toast.error(e.message);
                }
            } finally {
                if (!silent) {
                    this.loading = false;
                }
            }
        },

        syncServicesSnapshot(services) {
            this.services = Array.isArray(services) ? services : [];
            this.pruneServiceOps(this.services);
        },

        syncProfilesSnapshot(profileList) {
            this.profiles = ServicesOps.buildProfilesMap(profileList);
            this.pruneProfileOps(this.profiles);
        },

        schedulePageSync(delay = 600) {
            if (this.syncTimer) {
                clearTimeout(this.syncTimer);
            }
            this.syncTimer = setTimeout(() => {
                this.fetchPageData({ silent: true });
            }, delay);
        },

        buildServiceRow(service, profileName = '') {
            return ServicesRules.buildServiceRow(service, {
                loading: this.isServiceRowLoading(service, profileName),
                enabledProfiles: this.enabledProfiles
            });
        },

        getServicesForProfile(profileName) {
            return ServicesOps.getServicesForProfile(this.services, profileName);
        },

        getProfileServiceNames(profileName) {
            return this.getServicesForProfile(profileName).map(service => service.name);
        },

        setProfileStatus(profileName, status) {
            if (!profileName) return;
            const enabled = status === 'enabled';
            const current = this.profiles[profileName] || { name: profileName };
            this.profiles = {
                ...this.profiles,
                [profileName]: {
                    ...current,
                    name: profileName,
                    enabled,
                    status
                }
            };
        },

        patchServiceSnapshot(fresh) {
            if (!fresh || !fresh.name) return;
            const target = this.services.find(service => service.name === fresh.name);
            if (target) {
                Object.assign(target, fresh);
                return;
            }
            this.services = [...this.services, fresh];
        },

        handleRowAction(payload) {
            if (!payload || !payload.service || !payload.type) return;

            const { service, type } = payload;
            if (type === 'show-env') {
                this.showEnv(service);
                return;
            }
            if (type === 'go-logs') {
                this.goToLogs(service);
                return;
            }
            if (type === 'go-terminal') {
                this.goToTerminal(service);
                return;
            }
            if (type === 'upgrade') {
                this.confirmUpgrade(service);
                return;
            }
            if (type === 'rebuild') {
                this.confirmRebuild(service);
                return;
            }

            this.confirmAction(type, service);
        },

        closeConfirmModal() {
            this.confirmModal = { visible: false, title: '', message: '', warning: '', btnText: '', btnClass: '', onConfirm: null };
        },

        confirmAction(action, service) {
            const labels = {
                stop: this.$t('services.actions.stop'),
                restart: this.$t('services.actions.restart'),
                start: this.$t('services.actions.start')
            };
            const btnClasses = { stop: 'btn-danger', restart: 'btn-primary', start: 'btn-primary' };
            this.confirmModal = {
                visible: true,
                title: this.$t('services.modal.confirm_title'),
                message: '<p>' + this.$t('services.modal.confirm_action').replace('{action}', labels[action]).replace('{name}', service.name) + '</p>',
                warning: '',
                btnText: this.$t('services.modal.confirm_btn').replace('{action}', labels[action]),
                btnClass: btnClasses[action] || 'btn-primary',
                onConfirm: () => this.executeAction(action, service)
            };
        },

        onConfirmModalConfirm() {
            const onConfirm = this.confirmModal.onConfirm;
            this.closeConfirmModal();
            if (onConfirm) {
                onConfirm();
            }
        },

        async executeAction(action, service) {
            const seq = this.beginServiceOp(service, action);

            try {
                if (action === 'stop') {
                    await API.stopService(service.name);
                } else if (action === 'restart') {
                    await API.restartService(service.name);
                } else {
                    await API.startService(service.name);
                }
            } catch (e) {
                this.clearServiceOp(service.name, seq);
                if (e.code === 'services.start.build_not_supported') {
                    Toast.error(this.$t('services.start.build_not_supported'));
                } else if (e.code === 'services.start.profile_required') {
                    Toast.error(e.message);
                } else {
                    Toast.error(this.$t('common.error') + ': ' + e.message);
                }
                return;
            }

            Toast.info(service.name + ' ' + this.$t('services.toast.action_in_progress'));
            this.startAsyncPoll(service.name, seq);
        },

        confirmUpgrade(service) {
            this.upgradeModal = { visible: true, svc: service };
        },

        closeUpgradeModal() {
            this.upgradeModal = { visible: false, svc: null };
        },

        async applyUpgrade(service) {
            if (!service) return;

            const seq = this.beginServiceOp(service, 'upgrade');
            this.closeUpgradeModal();

            try {
                await API.applyUpgrade(service.name);
                Toast.info(this.$t('services.toast.upgrade_started'));
                this.startAsyncPoll(service.name, seq);
            } catch (e) {
                this.clearServiceOp(service.name, seq);
                Toast.error(this.$t('services.toast.upgrade_failed') + ': ' + e.message);
            }
        },

        confirmRebuild(service) {
            const varList = (service.pending_env || []).join(', ');
            this.confirmModal = {
                visible: true,
                title: '🔄 ' + this.$t('services.actions.rebuild') + ' — ' + service.name,
                message: '<div><div class="form-label">' + this.$t('services.modal.changed_vars') + '</div><div class="mono" style="padding:8px 14px;background:#F5F3FF;border-radius:var(--radius);font-size:0.85rem;color:#5B21B6">' + varList + '</div></div>',
                warning: '⚠️ ' + this.$t('services.modal.rebuild_warning'),
                btnText: this.$t('services.modal.confirm_rebuild'),
                btnClass: 'btn-primary',
                onConfirm: () => this.executeRebuild(service)
            };
        },

        async executeRebuild(service) {
            const seq = this.beginServiceOp(service, 'rebuild');

            try {
                await API.rebuildService(service.name);
                Toast.info(service.name + ' ' + this.$t('services.toast.rebuild_started'));
                this.startAsyncPoll(service.name, seq);
            } catch (e) {
                this.clearServiceOp(service.name, seq);
                Toast.error(this.$t('services.toast.rebuild_failed') + ': ' + e.message);
            }
        },

        startAsyncPoll(serviceName, seq) {
            const startTime = Date.now();
            const op = this.serviceOps[serviceName];
            if (!op) return;

            const maxWait = op.action === 'upgrade' ? 5 * 60 * 1000 : 2 * 60 * 1000;

            const finish = (ok, toastMessage) => {
                if (!this.clearServiceOp(serviceName, seq)) return;
                if (toastMessage) {
                    (ok ? Toast.success : Toast.error)(toastMessage);
                }
                this.schedulePageSync();
            };

            const check = async () => {
                if (!this.isCurrentServiceOp(serviceName, seq)) return;

                if (Date.now() - startTime > maxWait) {
                    finish(false, serviceName + ' ' + this.$t('services.toast.operation_timeout'));
                    return;
                }

                try {
                    const fresh = await API.getServiceStatus(serviceName);
                    if (!this.isCurrentServiceOp(serviceName, seq)) return;

                    this.patchServiceSnapshot(fresh);
                    const currentOp = this.serviceOps[serviceName];
                    const evaluation = ServicesRules.evaluateServiceOperation(currentOp.action, fresh, currentOp);
                    this.recordServiceFailureSample(serviceName, seq, evaluation.failedSample);

                    if (evaluation.done) {
                        const doneKeys = {
                            stop: 'stop_done',
                            start: 'start_done',
                            restart: 'restart_done',
                            upgrade: 'upgrade_done',
                            rebuild: 'rebuild_done'
                        };
                        finish(true, serviceName + ' ' + this.$t('services.toast.' + doneKeys[currentOp.action]));
                        return;
                    }

                    const updatedOp = this.serviceOps[serviceName];
                    if (updatedOp && updatedOp.failCount >= 3) {
                        finish(false, serviceName + ' ' + this.$t('services.toast.operation_failed'));
                        return;
                    }
                } catch (e) {
                    // 网络抖动或短暂查不到容器时继续轮询，由超时兜底
                }

                if (this.isCurrentServiceOp(serviceName, seq)) {
                    setTimeout(check, 3000);
                }
            };

            setTimeout(check, 1000);
        },

        async doEnableProfile(profileName) {
            const seq = this.beginProfileOp(profileName, 'enable');
            Toast.info(profileName + ' ' + this.$t('services.profile.enabling'));

            try {
                await API.enableProfile(profileName);
            } catch (e) {
                this.clearProfileOp(profileName, seq);
                Toast.error(e.message);
                return;
            }

            this.setProfileStatus(profileName, 'enabled');
            this.startProfilePoll(profileName, seq);
        },

        confirmDisableProfile(profileName) {
            const serviceNames = this.getProfileServiceNames(profileName).join(', ');
            this.confirmModal = {
                visible: true,
                title: this.$t('services.profile.disable_button') + ' ' + this.$t('services.profile.label'),
                message: '<p>' + this.$t('services.profile.confirm_disable').replace('{name}', profileName) + '</p><div class="mono" style="padding:8px 14px;background:#FEF2F2;border-radius:var(--radius);font-size:0.85rem;color:#991B1B">' + serviceNames + '</div>',
                warning: '',
                btnText: this.$t('services.modal.confirm_btn').replace('{action}', this.$t('services.profile.disable_button')),
                btnClass: 'btn-danger',
                onConfirm: () => this.doDisableProfile(profileName)
            };
        },

        async doDisableProfile(profileName) {
            const seq = this.beginProfileOp(profileName, 'disable');
            Toast.info(profileName + ' ' + this.$t('services.profile.disabling'));

            try {
                await API.disableProfile(profileName);
            } catch (e) {
                this.clearProfileOp(profileName, seq);
                Toast.error(e.message);
                return;
            }

            this.setProfileStatus(profileName, 'disabled');
            this.startProfilePoll(profileName, seq);
        },

        startProfilePoll(profileName, seq) {
            const startTime = Date.now();
            const maxWait = 3 * 60 * 1000;

            const finish = (ok, toastMessage) => {
                if (!this.clearProfileOp(profileName, seq)) return;
                if (toastMessage) {
                    (ok ? Toast.success : Toast.error)(toastMessage);
                }
                this.schedulePageSync();
            };

            const check = async () => {
                if (!this.isCurrentProfileOp(profileName, seq)) return;

                if (Date.now() - startTime > maxWait) {
                    finish(false, profileName + ' ' + this.$t('services.toast.operation_timeout'));
                    return;
                }

                const currentOp = this.profileOps[profileName];
                const serviceNames = currentOp?.serviceNames || [];
                if (serviceNames.length === 0) {
                    finish(true, null);
                    return;
                }

                try {
                    const results = await Promise.all(serviceNames.map(async serviceName => {
                        try {
                            const fresh = await API.getServiceStatus(serviceName);
                            return { ok: true, serviceName, fresh };
                        } catch (e) {
                            return { ok: false, serviceName };
                        }
                    }));

                    if (!this.isCurrentProfileOp(profileName, seq)) return;

                    let allDone = true;
                    let hasFailed = false;

                    for (const result of results) {
                        if (!result.ok) {
                            allDone = false;
                            continue;
                        }

                        this.patchServiceSnapshot(result.fresh);

                        if (currentOp.action === 'enable') {
                            if (result.fresh.status !== 'running') {
                                allDone = false;
                            }

                            const sample = ServicesRules.evaluateServiceOperation('start', result.fresh, {
                                startedBefore: '',
                                containerIdBefore: ''
                            });
                            this.recordProfileFailureSample(profileName, seq, result.serviceName, sample.failedSample);

                            const latestOp = this.profileOps[profileName];
                            const failCount = latestOp?.failureCounts?.[result.serviceName] || 0;
                            if (failCount >= 3) {
                                hasFailed = true;
                            }
                        } else {
                            if (result.fresh.status !== 'not_deployed') {
                                allDone = false;
                            }
                        }
                    }

                    if (hasFailed) {
                        finish(false, profileName + ' ' + this.$t('services.toast.operation_failed'));
                        return;
                    }

                    if (allDone) {
                        const messageKey = currentOp.action === 'enable'
                            ? 'services.profile.enable_success'
                            : 'services.profile.disable_success';
                        finish(true, this.$t(messageKey).replace('{name}', profileName));
                        return;
                    }
                } catch (e) {
                    // 网络异常继续轮询，由超时兜底
                }

                if (this.isCurrentProfileOp(profileName, seq)) {
                    setTimeout(check, 3000);
                }
            };

            setTimeout(check, 300);
        },

        beginServiceOp(service, action) {
            const seq = this.nextOpSeq();
            this.serviceOps = {
                ...this.serviceOps,
                [service.name]: ServicesOps.createServiceOp(seq, action, service)
            };
            return seq;
        },

        recordServiceFailureSample(serviceName, seq, failedSample) {
            if (!this.isCurrentServiceOp(serviceName, seq)) return;
            const current = this.serviceOps[serviceName];
            this.serviceOps = {
                ...this.serviceOps,
                [serviceName]: {
                    ...current,
                    failCount: failedSample ? (current.failCount || 0) + 1 : 0
                }
            };
        },

        clearServiceOp(name, seq) {
            const { changed, next } = ServicesOps.removeOp(this.serviceOps, name, seq);
            if (!changed) return false;
            this.serviceOps = next;
            return true;
        },

        isCurrentServiceOp(name, seq) {
            return ServicesOps.isCurrentOp(this.serviceOps, name, seq);
        },

        beginProfileOp(profileName, action) {
            const seq = this.nextOpSeq();
            this.profileOps = {
                ...this.profileOps,
                [profileName]: ServicesOps.createProfileOp(seq, action, profileName, this.getProfileServiceNames(profileName))
            };
            return seq;
        },

        recordProfileFailureSample(profileName, seq, serviceName, failedSample) {
            if (!this.isCurrentProfileOp(profileName, seq)) return;
            const current = this.profileOps[profileName];
            const failureCounts = {
                ...(current.failureCounts || {}),
                [serviceName]: failedSample ? ((current.failureCounts?.[serviceName] || 0) + 1) : 0
            };
            this.profileOps = {
                ...this.profileOps,
                [profileName]: {
                    ...current,
                    failureCounts
                }
            };
        },

        clearProfileOp(name, seq) {
            const { changed, next } = ServicesOps.removeOp(this.profileOps, name, seq);
            if (!changed) return false;
            this.profileOps = next;
            return true;
        },

        isCurrentProfileOp(name, seq) {
            return ServicesOps.isCurrentOp(this.profileOps, name, seq);
        },

        nextOpSeq() {
            this.opSeq += 1;
            return this.opSeq;
        },

        isProfileLoading(profileName) {
            return ServicesOps.isProfileLoading(this.profileOps, profileName);
        },

        isServiceRowLoading(service, profileName = '') {
            return ServicesOps.isServiceRowLoading({
                service,
                profileName,
                serviceOps: this.serviceOps,
                profileOps: this.profileOps
            });
        },

        pruneServiceOps(services) {
            const validNames = new Set((services || []).map(service => service.name));
            this.serviceOps = ServicesOps.pruneOps(this.serviceOps, validNames);
        },

        pruneProfileOps(profiles) {
            const validNames = new Set(Object.keys(profiles || {}));
            this.profileOps = ServicesOps.pruneOps(this.profileOps, validNames);
        },

        canEnableProfile(profile) {
            return ServicesOps.canEnableProfile(profile);
        },

        canDisableProfile(profile) {
            return ServicesOps.canDisableProfile(profile);
        },

        profileStatusLabel(status) {
            return ServicesOps.profileStatusLabel(status, this.$t.bind(this));
        },

        async showEnv(service) {
            this.envModal = { visible: true, serviceName: service.name, data: {}, loading: true };
            try {
                this.envModal.data = await API.getServiceEnv(service.name);
            } catch (e) {
                Toast.error(e.message);
            } finally {
                this.envModal.loading = false;
            }
        },

        goToLogs(service) {
            this.$router.push({ name: 'logs', query: { service: service.name } });
        },

        goToTerminal(service) {
            this.$router.push({ name: 'terminal', query: { service: service.name } });
        }
    },
    mounted() {
        this.fetchPageData();
        this.pollTimer = setInterval(() => this.fetchPageData({ silent: true }), 15000);
    },
    beforeUnmount() {
        if (this.pollTimer) clearInterval(this.pollTimer);
        if (this.syncTimer) clearTimeout(this.syncTimer);
    }
};
