/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * 日志查看页面组件 — SSE 实时流
 * 后端维护持续连接：容器停→waiting→容器恢复→自动streaming
 * 前端仅在 HTTP 连接本身断开时做自动重连（如 ComposeBoard 重启、网络断）
 */
const LogsPage = {
    template: `
    <div style="display: flex; flex-direction: column; height: 100%;">
        <div class="log-toolbar">
            <select v-model="selectedService" @change="reconnect">
                <option value="">— {{ $t('logs.select_service') }} —</option>
                <option v-for="svc in services" :key="svc" :value="svc">{{ svc }}</option>
            </select>
            <label style="display:flex;align-items:center;gap:6px;color:var(--color-fg-secondary);font-size:0.85rem">
                {{ $t('logs.tail_lines') }}
                <input type="number" v-model.number="tailLines" min="10" max="5000" step="50">
            </label>
            <button
                class="btn btn-sm"
                :class="sessionActive ? 'btn-danger' : 'btn-primary'"
                @click="toggleConnection"
                :disabled="!selectedService"
            >
                {{ sessionActive ? '⏹ ' + $t('logs.btn_disconnect') : '▶ ' + $t('logs.btn_connect') }}
            </button>
            <button class="btn btn-sm btn-ghost" @click="autoScroll = !autoScroll">
                {{ $t('logs.auto_scroll') }}: {{ autoScroll ? 'ON' : 'OFF' }}
            </button>
            <button class="btn btn-sm btn-ghost" @click="clearLogs">{{ $t('logs.clear') }}</button>
            <span v-if="selectedService" :style="statusStyle">
                <span class="status-dot" :class="statusInfo.dot"></span> {{ statusInfo.label }}
            </span>
        </div>

        <!-- 断线重连 banner -->
        <div v-if="reconnectBanner" class="log-reconnect-banner">
            ⚠ {{ reconnectBannerText }}
        </div>

        <div class="log-terminal hover-scroll" ref="terminal">
            <div v-if="logs.length === 0 && !sessionActive" style="color:var(--color-fg-tertiary);padding:40px;text-align:center">
                {{ $t('logs.empty_hint') }}
            </div>
            <div
                v-for="entry in logs"
                :key="entry.id"
                class="log-line"
                v-html="entry.html"
            ></div>
        </div>
    </div>
    `,
    data() {
        return {
            selectedService: '',
            services: [],
            tailLines: 100,
            logs: [],
            pendingLogs: [],
            connected: false,
            streamState: 'disconnected',
            autoScroll: true,
            eventSource: null,
            flushTimer: null,
            maxLines: 2000,
            nextLogId: 0,
            hasShownConnectedToast: false,
            // 重连相关（仅处理 HTTP 连接断开的场景）
            reconnectAttempt: 0,
            reconnectTimer: null,
            reconnectBanner: false,
            userDisconnected: false
        };
    },
    computed: {
        sessionActive() {
            return this.connected || !!this.eventSource || !!this.reconnectTimer;
        },
        statusInfo() {
            switch (this.streamState) {
                case 'connecting':
                    return { label: this.$t('logs.connecting'), color: 'var(--color-fg-secondary)', dot: 'not_deployed' };
                case 'streaming':
                    return { label: this.$t('logs.following'), color: 'var(--color-running)', dot: 'running' };
                case 'waiting':
                    return { label: this.$t('logs.waiting_container'), color: 'var(--color-warning)', dot: 'waiting' };
                case 'reconnecting':
                    return { label: this.$t('logs.stream_reconnecting'), color: 'var(--color-warning)', dot: 'waiting' };
                default:
                    return { label: this.$t('logs.disconnected'), color: 'var(--color-fg-tertiary)', dot: 'exited' };
            }
        },
        statusStyle() {
            return {
                color: this.statusInfo.color,
                fontSize: '0.8rem',
                display: 'flex',
                alignItems: 'center',
                gap: '4px'
            };
        },
        reconnectBannerText() {
            return this.$t('logs.reconnect_banner')
                .replace('{attempt}', this.reconnectAttempt);
        }
    },
    methods: {
        async fetchServices() {
            try {
                const list = await API.getServices();
                // 所有已部署的服务（运行中 + 已停止都可看日志）
                this.services = (list || [])
                    .filter(s => s.status !== 'not_deployed')
                    .map(s => s.name)
                    .sort();
            } catch (e) {
                // 静默
            }
        },
        toggleConnection() {
            if (this.sessionActive) {
                this.userDisconnected = true;
                this.disconnect();
            } else {
                this.userDisconnected = false;
                this.connect();
            }
        },
        connect() {
            if (!this.selectedService) return;
            this.closeEventSource();
            if (this.reconnectAttempt === 0) {
                this.resetLogState();
            }

            this.eventSource = API.createLogStream(this.selectedService, this.tailLines);
            this.connected = true;
            this.streamState = 'connecting';

            this.eventSource.onopen = () => {
                this.connected = true;
                // 重连成功
                if (this.reconnectAttempt > 0) {
                    Toast.success(this.$t('logs.reconnect_success'));
                } else if (!this.hasShownConnectedToast) {
                    this.hasShownConnectedToast = true;
                    Toast.info(this.selectedService + ' ' + this.$t('logs.connected'));
                }
                // 清理重连状态
                this.reconnectAttempt = 0;
                this.reconnectBanner = false;
            };

            this.eventSource.onmessage = (event) => {
                this.enqueueLogLine(event.data);
            };

            // 后端通过 status 事件通知连接状态变化（streaming/waiting/reconnecting）
            // 容器停止→后端发 waiting；容器恢复→后端自动重连日志→发 streaming
            // 前端只需更新 UI 状态，不需要重建连接
            this.eventSource.addEventListener('status', (event) => {
                try {
                    const payload = JSON.parse(event.data || '{}');
                    if (payload.state) {
                        this.streamState = payload.state;
                    }
                } catch (e) {
                    // 静默忽略状态事件解析失败
                }
            });

            // onerror 仅在 HTTP 连接本身断开时触发（ComposeBoard 重启/网络断）
            // 容器停止不会触发此事件——后端会保持 SSE 连接并发 status: waiting
            this.eventSource.onerror = () => {
                // G-5: 401 时手动关闭，防止无限重连
                if (!API.isAuthenticated()) {
                    this.disconnect();
                    Toast.error(this.$t('auth.token_expired'));
                    if (API.onUnauthorized) API.onUnauthorized();
                    return;
                }

                // EventSource readyState: CONNECTING=0, OPEN=1, CLOSED=2
                // 浏览器会自动重连 CONNECTING 状态的 EventSource
                // 只有 CLOSED 才需要我们手动重连
                if (this.eventSource && this.eventSource.readyState === EventSource.CLOSED) {
                    this.connected = false;
                    this.closeEventSource();

                    // 用户主动断开不重连
                    if (this.userDisconnected) {
                        this.streamState = 'disconnected';
                        return;
                    }

                    // 自动重连
                    this.scheduleReconnect();
                } else {
                    // CONNECTING 状态：浏览器正在自动重连，更新 UI 状态即可
                    this.connected = false;
                    this.streamState = 'reconnecting';
                }
            };
        },
        /** 关闭 EventSource（不清理业务状态） */
        closeEventSource() {
            if (this.eventSource) {
                this.eventSource.close();
                this.eventSource = null;
            }
        },
        /** 计划自动重连（指数退避: 1s, 2s, 4s, 封顶10s） */
        scheduleReconnect() {
            this.reconnectAttempt++;
            this.streamState = 'reconnecting';
            this.reconnectBanner = true;
            const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempt - 1), 10000);
            this.reconnectTimer = setTimeout(() => {
                this.reconnectTimer = null;
                this.connect();
            }, delay);
        },
        disconnect() {
            this.flushPendingLogs();
            this.clearFlushTimer();
            this.clearReconnectTimer();
            this.closeEventSource();
            this.connected = false;
            this.streamState = 'disconnected';
            this.hasShownConnectedToast = false;
            this.reconnectAttempt = 0;
            this.reconnectBanner = false;
        },
        clearReconnectTimer() {
            if (this.reconnectTimer) {
                clearTimeout(this.reconnectTimer);
                this.reconnectTimer = null;
            }
        },
        reconnect() {
            if (this.sessionActive) {
                this.userDisconnected = false;
                this.reconnectAttempt = 0;
                this.connect();
            }
        },
        clearLogs() {
            this.pendingLogs = [];
            this.logs = [];
        },
        resetLogState() {
            this.clearFlushTimer();
            this.pendingLogs = [];
            this.logs = [];
            this.nextLogId = 0;
            this.streamState = 'disconnected';
            this.hasShownConnectedToast = false;
        },
        enqueueLogLine(line) {
            this.pendingLogs.push({
                id: ++this.nextLogId,
                html: this.formatLogLine(line)
            });
            this.scheduleFlush();
        },
        scheduleFlush() {
            if (this.flushTimer) return;
            this.flushTimer = window.setTimeout(() => {
                this.flushTimer = null;
                this.flushPendingLogs();
            }, 60);
        },
        flushPendingLogs() {
            if (this.pendingLogs.length === 0) return;
            this.logs.push(...this.pendingLogs);
            this.pendingLogs = [];
            const overflow = this.logs.length - this.maxLines;
            if (overflow > 0) {
                this.logs.splice(0, overflow);
            }
            if (this.autoScroll) {
                this.$nextTick(() => {
                    const terminal = this.$refs.terminal;
                    if (terminal) terminal.scrollTop = terminal.scrollHeight;
                });
            }
        },
        clearFlushTimer() {
            if (this.flushTimer) {
                window.clearTimeout(this.flushTimer);
                this.flushTimer = null;
            }
        },
        formatLogLine(line) {
            let escaped = line
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;');

            escaped = escaped.replace(/\bERROR\b/g, '<span style="color:#ef4444;font-weight:600">ERROR</span>');
            escaped = escaped.replace(/\bWARN\b/g, '<span style="color:#f59e0b;font-weight:600">WARN</span>');
            escaped = escaped.replace(/\bINFO\b/g, '<span style="color:#22c55e">INFO</span>');
            escaped = escaped.replace(/\bDEBUG\b/g, '<span style="color:#64748b">DEBUG</span>');

            escaped = escaped.replace(/^(\d{4}-\d{2}-\d{2}T[\d:.]+Z?)/, '<span style="color:#64748b">$1</span>');

            return escaped;
        }
    },
    mounted() {
        this.fetchServices();
        if (this.$route.query.service) {
            this.selectedService = this.$route.query.service;
            this.$nextTick(() => this.connect());
        }
    },
    beforeUnmount() {
        this.userDisconnected = true;
        this.disconnect();
        this.clearFlushTimer();
    }
};
