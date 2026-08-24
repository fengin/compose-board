/**
 * 文件日志子面板。
 * 与 Docker 控制台日志保持独立连接，并提供服务级自动发现和人工映射。
 */
const FileLogPanel = {
    props: {
        initialService: { type: String, default: '' },
        bases: { type: Array, default: () => [] },
        sourceType: { type: String, default: 'file' }
    },
    template: `
    <div class="file-log-panel">
        <div class="log-toolbar file-log-toolbar">
            <select class="log-source-select" :value="sourceType" @change="$emit('change-source', $event.target.value)">
                <option value="console">{{ $t('logs.source_console') }}</option>
                <option value="file">{{ $t('logs.source_file') }}</option>
            </select>
            <select class="file-log-service-select" v-model="selectedService" @change="loadSource(false)">
                <option value="">— {{ $t('logs.select_service') }} —</option>
                <option v-for="svc in services" :key="svc" :value="svc">{{ svc }}</option>
            </select>
            <select class="file-log-directory-select" v-model="selectedDirectoryKey" @change="loadFiles" :disabled="directories.length === 0">
                <option value="">— {{ directoryPlaceholder }} —</option>
                <option v-for="directory in directories" :key="directory.key" :value="directory.key">
                    {{ directoryLabel(directory) }}
                </option>
            </select>
            <button class="btn btn-sm btn-ghost file-log-icon-button" @click="openMappingModal" :disabled="!selectedService" :aria-label="$t('logs.configure_directory')" data-tooltip="设置日志目录">⚙</button>
            <select class="file-log-file-select" v-model="selectedFilePath" @change="onFileChanged" :disabled="files.length === 0">
                <option value="">— {{ $t('logs.select_file') }} —</option>
                <option v-for="file in files" :key="file.path" :value="file.path">{{ fileLabel(file) }}</option>
            </select>
            <button class="btn btn-sm btn-ghost file-log-icon-button" @click="refreshSource" :disabled="loadingMetadata" :aria-label="$t('logs.refresh_files')" data-tooltip="刷新">↻</button>
            <label class="log-tail-label">
                {{ $t('logs.tail_lines') }}
                <input type="number" v-model.number="tailLines" min="10" max="5000" step="50">
            </label>
            <button class="btn btn-sm" :class="sessionActive ? 'btn-danger' : 'btn-primary'" @click="toggleConnection" :disabled="!sessionActive && !canFollow">
                {{ sessionActive ? '⏹ ' + $t('logs.btn_disconnect') : '▶ ' + $t('logs.btn_connect') }}
            </button>
            <button class="btn btn-sm btn-ghost" @click="downloadSelected" :disabled="!canDownload">↓ {{ $t('logs.download') }}</button>
            <button class="btn btn-sm btn-ghost" @click="autoScroll = !autoScroll">{{ $t('logs.auto_scroll') }}: {{ autoScroll ? 'ON' : 'OFF' }}</button>
            <button class="btn btn-sm btn-ghost" @click="clearLogs">{{ $t('logs.clear') }}</button>
            <span v-if="selectedFile" class="file-log-stream-status" :style="statusStyle">
                <span class="status-dot" :class="statusInfo.dot"></span> {{ statusInfo.label }}
            </span>
        </div>

        <div v-if="noticeText" class="file-log-notice">{{ noticeText }}</div>
        <div v-if="reconnectBanner" class="log-reconnect-banner">⚠ {{ reconnectBannerText }}</div>

        <div class="log-terminal hover-scroll" ref="terminal">
            <div v-if="logs.length === 0 && !sessionActive" class="log-empty-hint">{{ emptyHint }}</div>
            <div v-for="entry in logs" :key="entry.id" class="log-line" v-html="entry.html"></div>
        </div>

        <div v-if="mappingModal.visible" class="file-log-mapping-overlay" @click.self="closeMappingModal">
            <div class="file-log-mapping-dialog" role="dialog" aria-modal="true" :aria-label="$t('logs.mapping_title')">
                <div class="file-log-mapping-header">
                    <div>
                        <h3>{{ $t('logs.mapping_title') }}</h3>
                        <div class="file-log-mapping-service">{{ selectedService }}</div>
                    </div>
                    <button class="file-log-mapping-close" @click="closeMappingModal" aria-label="Close">×</button>
                </div>
                <div class="file-log-mapping-body">
                    <label class="file-log-mapping-field">
                        <span>{{ $t('logs.mapping_base') }}</span>
                        <select v-model="mappingModal.baseId" @change="onMappingBaseChanged">
                            <option v-for="base in bases" :key="base.id" :value="base.id">{{ base.name }}</option>
                        </select>
                    </label>
                    <label class="file-log-mapping-field">
                        <span>{{ $t('logs.mapping_relative_path') }}</span>
                        <div class="file-log-relative-input">
                            <span>{{ selectedBasePrefix }}</span>
                            <input v-model.trim="mappingModal.relativePath" @input="mappingModal.validation = null" placeholder="service/logs">
                        </div>
                    </label>

                    <div class="file-log-browser">
                        <div class="file-log-browser-header">
                            <span>{{ $t('logs.browse_directory') }}：/{{ mappingModal.browsePath }}</span>
                            <button class="btn btn-sm btn-ghost" @click="browseParent" :disabled="!mappingModal.browsePath">↑ {{ $t('logs.parent_directory') }}</button>
                        </div>
                        <div class="file-log-browser-list">
                            <button v-for="entry in mappingModal.browseEntries" :key="entry.path" @click="selectBrowseEntry(entry)">📁 {{ entry.name }}</button>
                            <div v-if="mappingModal.browseEntries.length === 0" class="file-log-browser-empty">{{ $t('logs.no_subdirectories') }}</div>
                        </div>
                        <div v-if="mappingModal.browseTruncated" class="file-log-browser-warning">{{ $t('logs.browse_truncated') }}</div>
                    </div>

                    <div v-if="mappingModal.validation" class="file-log-validation" :class="mappingModal.validation.valid ? 'valid' : 'invalid'">
                        <template v-if="mappingModal.validation.valid">
                            {{ $t('logs.mapping_valid') }}：{{ mappingModal.validation.resolved_path }} · {{ mappingModal.validation.file_count }} {{ $t('logs.log_files_count') }}
                        </template>
                        <template v-else>{{ mappingModal.validation.error }}</template>
                    </div>
                </div>
                <div class="file-log-mapping-footer">
                    <button v-if="source?.mapping" class="btn btn-sm btn-danger" @click="resetMapping">{{ $t('logs.restore_auto_match') }}</button>
                    <span class="file-log-footer-spacer"></span>
                    <button class="btn btn-sm btn-ghost" @click="closeMappingModal">{{ $t('common.cancel') }}</button>
                    <button class="btn btn-sm btn-ghost" @click="validateMapping" :disabled="mappingModal.loading">{{ $t('logs.validate_directory') }}</button>
                    <button class="btn btn-sm btn-primary" @click="saveMapping" :disabled="mappingModal.loading">{{ $t('common.save') }}</button>
                </div>
            </div>
        </div>
    </div>
    `,
    data() {
        return {
            services: [], selectedService: this.initialService || '', source: null,
            directories: [], selectedDirectoryKey: '', files: [], selectedFilePath: '', loadingMetadata: false,
            tailLines: 100, logs: [], pendingLogs: [], nextLogId: 0, maxLines: 2000, flushTimer: null,
            autoScroll: true, eventSource: null, connected: false, streamState: 'disconnected',
            reconnectAttempt: 0, reconnectTimer: null, reconnectBanner: false, userDisconnected: false,
            mappingModal: {
                visible: false, baseId: '', relativePath: '', browsePath: '', browseEntries: [],
                browseTruncated: false, validation: null, loading: false
            }
        };
    },
    computed: {
        selectedDirectory() { return this.directories.find(item => item.key === this.selectedDirectoryKey) || null; },
        selectedFile() { return this.files.find(item => item.path === this.selectedFilePath) || null; },
        selectedBase() { return this.bases.find(item => item.id === this.mappingModal.baseId) || null; },
        selectedBasePrefix() { return this.selectedBase ? this.selectedBase.name + ' / ' : '/'; },
        sessionActive() { return this.connected || !!this.eventSource || !!this.reconnectTimer; },
        canFollow() { return !!this.selectedDirectory && !!this.selectedFile?.followable; },
        canDownload() { return !!this.selectedDirectory && !!this.selectedFile?.downloadable; },
        directoryPlaceholder() {
            if (!this.selectedService) return this.$t('logs.select_directory');
            if (this.loadingMetadata) return this.$t('logs.loading_files');
            if (this.directories.length === 0) return this.$t('logs.no_file_logs');
            return this.$t('logs.select_directory');
        },
        statusInfo() {
            switch (this.streamState) {
                case 'connecting': return { label: this.$t('logs.connecting'), color: 'var(--color-fg-secondary)', dot: 'not_deployed' };
                case 'streaming': return { label: this.$t('logs.file_following'), color: 'var(--color-running)', dot: 'running' };
                case 'waiting': return { label: this.$t('logs.waiting_file'), color: 'var(--color-warning)', dot: 'waiting' };
                case 'rotating': return { label: this.$t('logs.file_rotating'), color: 'var(--color-warning)', dot: 'waiting' };
                case 'reconnecting': return { label: this.$t('logs.stream_reconnecting'), color: 'var(--color-warning)', dot: 'waiting' };
                default: return { label: this.$t('logs.disconnected'), color: 'var(--color-fg-tertiary)', dot: 'exited' };
            }
        },
        statusStyle() { return { color: this.statusInfo.color }; },
        reconnectBannerText() { return this.$t('logs.reconnect_banner').replace('{attempt}', this.reconnectAttempt); },
        noticeText() {
            if (this.selectedFile?.archived) return this.$t('logs.archive_download_only');
            if (!this.selectedService || this.loadingMetadata) return '';
            if (this.source?.mode === 'invalid_manual') return this.source.reason;
            if (this.source?.discovery_truncated) return this.$t('logs.discovery_truncated');
            if (this.source?.mode === 'unmatched' && this.directories.length === 0) return this.$t('logs.no_file_logs_hint');
            if (this.selectedDirectory && this.files.length === 0) return this.$t('logs.no_log_files');
            return '';
        },
        emptyHint() {
            if (this.selectedFile?.archived) return this.$t('logs.archive_download_only');
            if (this.selectedService && this.directories.length === 0) return this.$t('logs.no_file_logs_hint');
            return this.$t('logs.file_empty_hint');
        }
    },
    methods: {
        async fetchServices() {
            const list = await API.getServices();
            this.services = ServiceOrder.sort(list || []).map(service => service.name);
        },
        async loadSource(refresh = false) {
            this.disconnectForSelectionChange();
            this.source = null; this.directories = []; this.selectedDirectoryKey = ''; this.files = []; this.selectedFilePath = '';
            if (!this.selectedService) return;
            this.loadingMetadata = true;
            try {
                this.source = await API.getServiceFileLogSource(this.selectedService, refresh);
                this.directories = (this.source.directories || []).map(directory => ({ ...directory, key: this.directoryKey(directory) }));
                if (this.source.selected) {
                    this.selectedDirectoryKey = this.directoryKey(this.source.selected);
                    await this.loadFiles(false);
                }
            } catch (error) {
                Toast.error(error.message);
            } finally {
                this.loadingMetadata = false;
            }
        },
        async loadFiles(disconnect = true) {
            if (disconnect) this.disconnectForSelectionChange();
            this.files = []; this.selectedFilePath = '';
            if (!this.selectedDirectory) return;
            this.loadingMetadata = true;
            try {
                const response = await API.getFileLogFiles(this.selectedDirectory.base_id, this.selectedDirectory.path);
                this.files = response.files || [];
                const infoLog = this.files.find(file => file.followable && file.name.toLowerCase() === 'info.log');
                const selected = infoLog || this.files.find(file => file.followable) || this.files[0];
                if (selected) this.selectedFilePath = selected.path;
            } catch (error) { Toast.error(error.message); }
            finally { this.loadingMetadata = false; }
        },
        refreshSource() { return this.loadSource(true); },
        onFileChanged() { this.disconnectForSelectionChange(); },
        directoryKey(directory) { return encodeURIComponent(directory.base_id) + '::' + encodeURIComponent(directory.path || ''); },
        directoryLabel(directory) {
            const marker = directory.manual ? this.$t('logs.manually_configured') : (directory.recommended ? this.$t('logs.auto_matched') : '');
            const label = directory.path ? directory.base_name + ' / ' + directory.display_name : directory.base_name;
            return marker ? label + ' · ' + marker : label;
        },
        fileLabel(file) { return file.name + ' · ' + this.formatBytes(file.size) + ' · ' + this.formatTime(file.modified_at); },
        toggleConnection() { if (this.sessionActive) { this.userDisconnected = true; this.disconnect(); } else { this.userDisconnected = false; this.connect(); } },
        connect() {
            if (!this.canFollow) return;
            this.closeEventSource(); if (this.reconnectAttempt === 0) this.resetLogState();
            this.eventSource = API.createFileLogStream(this.selectedDirectory.base_id, this.selectedFile.path, this.tailLines);
            this.connected = true; this.streamState = 'connecting';
            this.eventSource.onopen = () => { this.connected = true; this.reconnectAttempt = 0; this.reconnectBanner = false; };
            this.eventSource.onmessage = event => this.enqueueLogLine(event.data);
            this.eventSource.addEventListener('status', event => { try { const payload = JSON.parse(event.data || '{}'); if (payload.state) this.streamState = payload.state; } catch (error) {} });
            this.eventSource.onerror = () => {
                if (!API.isAuthenticated()) { this.disconnect(); Toast.error(this.$t('auth.token_expired')); if (API.onUnauthorized) API.onUnauthorized(); return; }
                if (this.eventSource && this.eventSource.readyState === EventSource.CLOSED) { this.connected = false; this.closeEventSource(); if (!this.userDisconnected) this.scheduleReconnect(); }
                else { this.connected = false; this.streamState = 'reconnecting'; }
            };
        },
        scheduleReconnect() {
            this.reconnectAttempt++; this.streamState = 'reconnecting'; this.reconnectBanner = true;
            const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempt - 1), 10000);
            this.reconnectTimer = window.setTimeout(() => { this.reconnectTimer = null; this.connect(); }, delay);
        },
        disconnectForSelectionChange() { this.userDisconnected = true; this.disconnect(); this.resetLogState(); this.userDisconnected = false; },
        disconnect() { this.flushPendingLogs(); this.clearFlushTimer(); this.clearReconnectTimer(); this.closeEventSource(); this.connected = false; this.streamState = 'disconnected'; this.reconnectAttempt = 0; this.reconnectBanner = false; },
        closeEventSource() { if (this.eventSource) { this.eventSource.close(); this.eventSource = null; } },
        clearReconnectTimer() { if (this.reconnectTimer) { window.clearTimeout(this.reconnectTimer); this.reconnectTimer = null; } },
        downloadSelected() { if (this.canDownload) API.downloadFileLog(this.selectedDirectory.base_id, this.selectedFile.path); },
        clearLogs() { this.pendingLogs = []; this.logs = []; },
        resetLogState() { this.clearFlushTimer(); this.pendingLogs = []; this.logs = []; this.nextLogId = 0; },
        enqueueLogLine(line) {
            this.pendingLogs.push({ id: ++this.nextLogId, html: this.formatLogLine(line) });
            if (!this.flushTimer) this.flushTimer = window.setTimeout(() => { this.flushTimer = null; this.flushPendingLogs(); }, 60);
        },
        flushPendingLogs() {
            if (this.pendingLogs.length === 0) return;
            this.logs.push(...this.pendingLogs); this.pendingLogs = [];
            const overflow = this.logs.length - this.maxLines; if (overflow > 0) this.logs.splice(0, overflow);
            if (this.autoScroll) this.$nextTick(() => { const terminal = this.$refs.terminal; if (terminal) terminal.scrollTop = terminal.scrollHeight; });
        },
        clearFlushTimer() { if (this.flushTimer) { window.clearTimeout(this.flushTimer); this.flushTimer = null; } },
        formatLogLine(line) {
            let escaped = line.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
            escaped = escaped.replace(/\bERROR\b/g, '<span style="color:#ef4444;font-weight:600">ERROR</span>');
            escaped = escaped.replace(/\bWARN\b/g, '<span style="color:#f59e0b;font-weight:600">WARN</span>');
            escaped = escaped.replace(/\bINFO\b/g, '<span style="color:#22c55e">INFO</span>');
            escaped = escaped.replace(/\bDEBUG\b/g, '<span style="color:#64748b">DEBUG</span>');
            return escaped.replace(/^(\d{4}-\d{2}-\d{2}[T ][\d:.+-]+Z?)/, '<span style="color:#64748b">$1</span>');
        },
        formatBytes(value) {
            const bytes = Number(value || 0); if (bytes < 1024) return bytes + ' B';
            if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
            if (bytes < 1073741824) return (bytes / 1048576).toFixed(1) + ' MB';
            return (bytes / 1073741824).toFixed(1) + ' GB';
        },
        formatTime(value) { if (!value) return '-'; return new Date(value).toLocaleString(I18n.locale === 'zh' ? 'zh-CN' : 'en-US', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }); },

        async openMappingModal() {
            const existing = this.source?.mapping?.directories?.[0];
            const selected = this.selectedDirectory;
            this.mappingModal.visible = true;
            this.mappingModal.baseId = existing?.base_id || selected?.base_id || this.bases[0]?.id || '';
            this.mappingModal.relativePath = existing?.relative_path || selected?.path || '';
            this.mappingModal.validation = null;
            const path = this.mappingModal.relativePath ? this.parentPath(this.mappingModal.relativePath) : '';
            await this.browseMappingPath(path);
        },
        closeMappingModal() { this.mappingModal.visible = false; },
        async onMappingBaseChanged() { this.mappingModal.relativePath = ''; this.mappingModal.validation = null; await this.browseMappingPath(''); },
        async browseMappingPath(path) {
            if (!this.mappingModal.baseId) return;
            this.mappingModal.loading = true;
            try {
                const result = await API.browseFileLogDirectories(this.mappingModal.baseId, path);
                this.mappingModal.browsePath = result.path || '';
                this.mappingModal.browseEntries = result.entries || [];
                this.mappingModal.browseTruncated = !!result.truncated;
            } catch (error) { Toast.error(error.message); }
            finally { this.mappingModal.loading = false; }
        },
        selectBrowseEntry(entry) { this.mappingModal.relativePath = entry.path; this.mappingModal.validation = null; return this.browseMappingPath(entry.path); },
        browseParent() { return this.browseMappingPath(this.parentPath(this.mappingModal.browsePath)); },
        parentPath(path) { const parts = String(path || '').split('/').filter(Boolean); parts.pop(); return parts.join('/'); },
        async validateMapping() {
            this.mappingModal.loading = true;
            try { this.mappingModal.validation = await API.validateFileLogMapping(this.mappingModal.baseId, this.mappingModal.relativePath); }
            catch (error) { this.mappingModal.validation = { valid: false, error: error.message }; }
            finally { this.mappingModal.loading = false; }
            return !!this.mappingModal.validation?.valid;
        },
        async saveMapping() {
            if (!(await this.validateMapping())) return;
            this.mappingModal.loading = true;
            try {
                await API.saveServiceFileLogMapping(this.selectedService, [{ id: 'default', name: '', base_id: this.mappingModal.baseId, relative_path: this.mappingModal.relativePath }]);
                Toast.success(this.$t('logs.mapping_saved'));
                this.closeMappingModal();
                await this.loadSource(true);
            } catch (error) { Toast.error(error.message); }
            finally { this.mappingModal.loading = false; }
        },
        async resetMapping() {
            this.mappingModal.loading = true;
            try {
                await API.deleteServiceFileLogMapping(this.selectedService);
                Toast.success(this.$t('logs.mapping_reset'));
                this.closeMappingModal();
                await this.loadSource(true);
            } catch (error) { Toast.error(error.message); }
            finally { this.mappingModal.loading = false; }
        }
    },
    async mounted() { await this.fetchServices(); if (this.selectedService) await this.loadSource(false); },
    beforeUnmount() { this.userDisconnected = true; this.disconnect(); }
};
