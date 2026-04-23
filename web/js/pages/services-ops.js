/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * 服务页操作态辅助层
 * 纯函数：排序、Profile 配置态映射、操作态 map 管理、loading 判定
 */
const ServicesOps = {
    compareNames(left, right) {
        return String(left || '').localeCompare(String(right || ''), 'zh-Hans-CN', {
            numeric: true,
            sensitivity: 'base'
        });
    },

    compareServices(left, right) {
        return this.compareNames(left?.name, right?.name);
    },

    getSortedProfileNames(profiles) {
        return Object.keys(profiles || {}).sort((a, b) => this.compareNames(a, b));
    },

    buildProfilesMap(profileList) {
        const mapped = {};
        const ordered = Array.isArray(profileList) ? [...profileList] : [];
        ordered.sort((a, b) => this.compareNames(a?.name, b?.name));

        for (const item of ordered) {
            if (!item?.name) continue;
            const enabled = item.enabled === true || item.status === 'enabled';
            mapped[item.name] = {
                name: item.name,
                enabled,
                status: enabled ? 'enabled' : 'disabled'
            };
        }

        return mapped;
    },

    getEnabledProfiles(profiles) {
        const enabled = new Set();
        for (const [name, profile] of Object.entries(profiles || {})) {
            if (profile?.status === 'enabled') {
                enabled.add(name);
            }
        }
        return enabled;
    },

    getServicesForProfile(services, profileName) {
        return (services || [])
            .filter(service => Array.isArray(service.profiles) && service.profiles.includes(profileName))
            .sort((a, b) => this.compareServices(a, b));
    },

    createServiceOp(seq, action, service) {
        return {
            seq,
            action,
            loading: true,
            startedBefore: service?.started_at || '',
            containerIdBefore: service?.container_id || '',
            failCount: 0
        };
    },

    createProfileOp(seq, action, profileName, serviceNames) {
        return {
            seq,
            action,
            profileName,
            loading: true,
            serviceNames: Array.isArray(serviceNames) ? [...serviceNames] : [],
            failureCounts: {}
        };
    },

    isCurrentOp(ops, name, seq) {
        const current = ops?.[name];
        return !!current && current.seq === seq;
    },

    removeOp(ops, name, seq) {
        if (!this.isCurrentOp(ops, name, seq)) {
            return { changed: false, next: ops };
        }

        const next = { ...(ops || {}) };
        delete next[name];
        return { changed: true, next };
    },

    pruneOps(ops, validNames) {
        const next = {};
        for (const [name, op] of Object.entries(ops || {})) {
            if (validNames.has(name)) {
                next[name] = op;
            }
        }
        return next;
    },

    isProfileLoading(profileOps, profileName) {
        return !!(profileOps?.[profileName] && profileOps[profileName].loading);
    },

    isServiceRowLoading({ service, profileName = '', serviceOps, profileOps }) {
        if (serviceOps?.[service.name]?.loading) {
            return true;
        }

        if (!profileName) {
            return false;
        }

        const profileOp = profileOps?.[profileName];
        if (!profileOp || !profileOp.loading) {
            return false;
        }

        return (profileOp.serviceNames || []).includes(service.name);
    },

    canEnableProfile(profile) {
        return !!profile && profile.status === 'disabled';
    },

    canDisableProfile(profile) {
        return !!profile && profile.status === 'enabled';
    },

    profileStatusLabel(status, translate) {
        const icons = { enabled: '✅ ', disabled: '⭕ ' };
        return (icons[status] || '') + translate('services.profile.status_' + status);
    }
};
