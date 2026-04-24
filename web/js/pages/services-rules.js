/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * 服务页规则层
 * 纯函数：按钮规则、展示模型、操作完成判据
 */
const ServicesRules = {
    compareServices(left, right) {
        return String(left?.name || '').localeCompare(String(right?.name || ''), 'zh-Hans-CN', {
            numeric: true,
            sensitivity: 'base'
        });
    },

    groupRequiredServices(services) {
        const groups = { backend: [], frontend: [], base: [], init: [], other: [] };
        (services || []).filter(service => !service.profiles || service.profiles.length === 0).forEach(service => {
            const category = service.category || 'other';
            if (!groups[category]) groups[category] = [];
            groups[category].push(service);
        });

        for (const key of Object.keys(groups)) {
            groups[key].sort((a, b) => this.compareServices(a, b));
            if (groups[key].length === 0) {
                delete groups[key];
            }
        }

        return groups;
    },

    buildServiceRow(service, options = {}) {
        const currentImage = service.running_image || service.declared_image || service.image_ref || '';
        const pendingEnv = Array.isArray(service.pending_env) ? service.pending_env : [];
        const hasImageDiff = !!service.image_diff;
        const hasEnvDiff = pendingEnv.length > 0 && !hasImageDiff;
        const loading = !!options.loading;

        return {
            key: service.name,
            service,
            loading,
            display: {
                name: service.name,
                showBuildBadge: service.image_source === 'build',
                currentVersion: this.extractVersion(currentImage),
                nextVersion: hasImageDiff ? this.extractVersion(service.declared_image || '') : '',
                hasEnvDiff,
                envChangedTitle: hasEnvDiff ? pendingEnv.join(', ') : '',
                status: service.status,
                startupWarning: !!service.startup_warning,
                ports: Array.isArray(service.ports) ? service.ports : [],
                cpuText: service.status === 'running' && typeof service.cpu === 'number' ? service.cpu.toFixed(1) + '%' : '—',
                memoryText: service.status === 'running' ? this.formatBytes(service.mem_usage) : '—',
                uptimeText: service.state || '—'
            },
            actions: loading ? [] : this.buildServiceActions(service, options.enabledProfiles)
        };
    },

    buildServiceActions(service, enabledProfiles = new Set()) {
        const actions = [];
        const profileNames = Array.isArray(service.profiles) ? service.profiles : [];
        const canStartOptional = profileNames.length === 0 || profileNames.some(name => enabledProfiles.has(name));
        const status = service.status;

        if (status === 'running') {
            actions.push('restart', 'stop');
        }
        if (status === 'exited') {
            actions.push('start');
        }
        if (status === 'created') {
            actions.push('start', 'restart');
        }
        if (status === 'restarting') {
            actions.push('restart', 'stop');
        }
        if (status === 'not_deployed' && service.image_source !== 'build' && canStartOptional) {
            actions.push('start');
        }

        if (status !== 'not_deployed') {
            actions.push('show-env');
            actions.push('go-logs');
        }
        if (status === 'running') {
            actions.push('go-terminal');
        }

        if (status !== 'not_deployed' && service.image_diff) {
            actions.push('upgrade');
        }
        if (status !== 'not_deployed' && service.pending_env && service.pending_env.length > 0 && !service.image_diff) {
            actions.push('rebuild');
        }

        return [...new Set(actions)];
    },

    evaluateServiceOperation(action, fresh, op = {}) {
        const startedChanged = !!fresh.started_at && fresh.started_at !== (op.startedBefore || '');
        const containerChanged = !!fresh.container_id && fresh.container_id !== (op.containerIdBefore || '');
        const runtimeChanged = startedChanged || containerChanged;

        let done = false;
        if (action === 'stop' && (fresh.status === 'exited' || fresh.status === 'not_deployed')) {
            done = true;
        }
        if (action === 'start' && fresh.status === 'running') {
            done = true;
        }
        if (['restart', 'upgrade', 'rebuild'].includes(action) && fresh.status === 'running' && runtimeChanged) {
            done = true;
        }
        if (action === 'upgrade' && done && fresh.image_diff) {
            done = false;
        }
        if (action === 'rebuild' && done && fresh.pending_env && fresh.pending_env.length > 0) {
            done = false;
        }

        let failedSample = false;
        if (action !== 'stop' && (fresh.status === 'exited' || fresh.status === 'restarting')) {
            failedSample = action === 'start' ? !runtimeChanged : runtimeChanged;
        }

        return { done, failedSample };
    },

    extractVersion(imageStr) {
        if (!imageStr) return '—';
        const slashIdx = imageStr.lastIndexOf('/');
        const repoTag = slashIdx >= 0 ? imageStr.substring(slashIdx + 1) : imageStr;
        const colonIdx = repoTag.lastIndexOf(':');
        if (colonIdx > 0) return repoTag.substring(colonIdx + 1);
        return repoTag;
    },

    formatBytes(bytes) {
        if (!bytes) return '—';
        const units = ['B', 'KB', 'MB', 'GB'];
        let index = 0;
        let value = bytes;
        while (value >= 1024 && index < units.length - 1) {
            value /= 1024;
            index++;
        }
        return value.toFixed(1) + ' ' + units[index];
    }
};
