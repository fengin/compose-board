/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * Web 终端页面 — xterm.js 按需加载
 */
let terminalAssetsPromise = null;
const TERMINAL_MIN_COLS = 10;
const TERMINAL_MAX_COLS = 1000;
const TERMINAL_MIN_ROWS = 3;
const TERMINAL_MAX_ROWS = 500;

function loadTerminalScript(src) {
    return new Promise((resolve, reject) => {
        const existing = document.querySelector(`script[src="${src}"]`);
        if (existing) {
            if (existing.dataset.failed === 'true') {
                existing.remove();
            } else {
                if (existing.dataset.loaded === 'true') {
                    resolve();
                } else {
                    existing.addEventListener('load', resolve, { once: true });
                    existing.addEventListener('error', reject, { once: true });
                }
                return;
            }
        }

        const script = document.createElement('script');
        script.src = src;
        script.dataset.loaded = 'false';
        script.onload = () => {
            script.dataset.loaded = 'true';
            resolve();
        };
        script.onerror = () => {
            script.dataset.failed = 'true';
            script.remove();
            reject(new Error(`Failed to load ${src}`));
        };
        document.body.appendChild(script);
    });
}

function loadTerminalStyle(href) {
    return new Promise((resolve, reject) => {
        const existing = document.querySelector(`link[href="${href}"]`);
        if (existing) {
            if (existing.dataset.failed === 'true') {
                existing.remove();
            } else if (existing.dataset.loaded === 'true' || existing.sheet) {
                resolve();
                return;
            } else {
                existing.addEventListener('load', resolve, { once: true });
                existing.addEventListener('error', reject, { once: true });
                return;
            }
        }

        const link = document.createElement('link');
        link.rel = 'stylesheet';
        link.href = href;
        link.dataset.loaded = 'false';
        link.onload = () => {
            link.dataset.loaded = 'true';
            resolve();
        };
        link.onerror = () => {
            link.dataset.failed = 'true';
            link.remove();
            reject(new Error(`Failed to load ${href}`));
        };
        document.head.appendChild(link);
    });
}

function loadTerminalAssets() {
    if (!terminalAssetsPromise) {
        terminalAssetsPromise = Promise.all([
            loadTerminalStyle('/css/vendor/xterm.css'),
            loadTerminalScript('/js/vendor/xterm.js')
        ])
            .then(() => loadTerminalScript('/js/vendor/xterm-addon-fit.js'))
            .catch(error => {
                terminalAssetsPromise = null;
                throw error;
            });
    }
    return terminalAssetsPromise;
}

function resolveTerminalCtor() {
    return window.Terminal || (window.Xterm && window.Xterm.Terminal);
}

function resolveFitAddonCtor() {
    if (window.FitAddon && window.FitAddon.FitAddon) return window.FitAddon.FitAddon;
    if (window.XtermFitAddon && window.XtermFitAddon.FitAddon) return window.XtermFitAddon.FitAddon;
    return window.FitAddon;
}

function isValidTerminalSize(cols, rows) {
    return cols >= TERMINAL_MIN_COLS &&
        cols <= TERMINAL_MAX_COLS &&
        rows >= TERMINAL_MIN_ROWS &&
        rows <= TERMINAL_MAX_ROWS;
}

const TerminalPage = {
    template: `
    <div class="terminal-page">
        <div class="terminal-toolbar">
            <select v-model="selectedService" @change="onServiceChange" :disabled="sessionActive">
                <option value="">— {{ $t('terminal.select_service') }} —</option>
                <option v-for="svc in services" :key="svc.name" :value="svc.name">{{ svc.name }}</option>
            </select>
            <button
                class="btn btn-sm"
                :class="sessionActive ? 'btn-danger' : 'btn-primary'"
                @click="toggleConnection"
                :disabled="!selectedService || loadingAssets || connecting"
            >
                {{ sessionActive ? '■ ' + $t('terminal.disconnect') : '▶ ' + $t('terminal.connect') }}
            </button>
            <button class="btn btn-sm btn-ghost" @click="refreshServices" :disabled="sessionActive || loadingServices">
                {{ $t('services.refresh') }}
            </button>
            <span v-if="selectedService" :style="statusStyle">
                <span class="status-dot" :class="statusInfo.dot"></span> {{ statusInfo.label }}
            </span>
        </div>

        <div v-if="notice" class="terminal-notice" :class="noticeType">
            {{ notice }}
        </div>

        <div class="terminal-shell-wrap">
            <div ref="terminal" class="terminal-shell"></div>
            <div v-if="emptyHintVisible" class="terminal-empty">
                {{ $t('terminal.empty_hint') }}
            </div>
        </div>
    </div>
    `,
    data() {
        return {
            services: [],
            selectedService: '',
            terminal: null,
            fitAddon: null,
            socket: null,
            resizeObserver: null,
            resizeTimer: null,
            state: 'idle',
            shellName: '',
            notice: '',
            noticeType: 'info',
            loadingAssets: false,
            loadingServices: false,
            userClosing: false,
            disconnectHintWritten: false,
            errorReceived: false,
            lastFitCols: 0,
            lastFitRows: 0,
            unmounted: false
        };
    },
    computed: {
        sessionActive() {
            return !!this.socket || this.state === 'connected' || this.state === 'connecting';
        },
        connecting() {
            return this.state === 'connecting';
        },
        emptyHintVisible() {
            return !this.terminal && !this.loadingAssets;
        },
        statusInfo() {
            switch (this.state) {
                case 'connecting':
                    return { label: this.$t('terminal.connecting'), color: 'var(--color-fg-secondary)', dot: 'waiting' };
                case 'connected':
                    return { label: this.shellName ? this.$t('terminal.connected_shell').replace('{shell}', this.shellName) : this.$t('terminal.connected'), color: 'var(--color-running)', dot: 'running' };
                case 'error':
                    return { label: this.$t('terminal.connection_error'), color: 'var(--color-danger)', dot: 'exited' };
                default:
                    return { label: this.$t('terminal.disconnected'), color: 'var(--color-fg-tertiary)', dot: 'exited' };
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
        }
    },
    methods: {
        async refreshServices() {
            this.loadingServices = true;
            try {
                const list = await API.getServices();
                if (this.unmounted) return;
                this.services = (list || [])
                    .filter(service => service.status === 'running')
                    .map(service => ({ name: service.name }))
                    .sort((a, b) => a.name.localeCompare(b.name, 'zh-Hans-CN', { numeric: true }));

                if (this.selectedService && !this.services.some(service => service.name === this.selectedService)) {
                    this.selectedService = '';
                    this.showNotice(this.$t('terminal.only_running'), 'warning');
                }
            } catch (e) {
                Toast.error(e.message);
            } finally {
                if (!this.unmounted) {
                    this.loadingServices = false;
                }
            }
        },
        async ensureTerminal() {
            if (this.terminal) return;
            this.loadingAssets = true;
            try {
                await loadTerminalAssets();
                if (this.unmounted || !this.$refs.terminal) return;
                const TerminalCtor = resolveTerminalCtor();
                const FitAddonCtor = resolveFitAddonCtor();
                if (!TerminalCtor || !FitAddonCtor) {
                    throw new Error(this.$t('terminal.asset_load_failed'));
                }

                this.fitAddon = new FitAddonCtor();
                this.terminal = new TerminalCtor({
                    cursorBlink: true,
                    scrollback: 1500,
                    fontFamily: '"JetBrains Mono", Consolas, monospace',
                    fontSize: 13,
                    lineHeight: 1.35,
                    theme: {
                        background: '#111827',
                        foreground: '#D1D5DB',
                        cursor: '#FBBF24',
                        selectionBackground: '#374151'
                    }
                });
                this.terminal.loadAddon(this.fitAddon);
                this.terminal.open(this.$refs.terminal);
                this.terminal.onData(data => this.sendInput(data));
                this.setupResizeObserver();
                this.fitTerminal();
            } finally {
                this.loadingAssets = false;
            }
        },
        setupResizeObserver() {
            if (this.resizeObserver || !window.ResizeObserver) return;
            this.resizeObserver = new ResizeObserver(() => this.scheduleResize());
            this.resizeObserver.observe(this.$refs.terminal);
        },
        toggleConnection() {
            if (this.sessionActive) {
                this.disconnect(true);
            } else {
                this.connect();
            }
        },
        async connect() {
            if (!this.selectedService) return;
            const selected = this.services.some(service => service.name === this.selectedService);
            if (!selected) {
                this.showNotice(this.$t('terminal.only_running'), 'warning');
                return;
            }

            try {
                await this.ensureTerminal();
                if (this.unmounted || !this.terminal) return;
            } catch (e) {
                this.state = 'error';
                this.showNotice(e.message || this.$t('terminal.asset_load_failed'), 'error');
                return;
            }

            this.notice = '';
            this.userClosing = false;
            this.disconnectHintWritten = false;
            this.errorReceived = false;
            this.state = 'connecting';
            this.shellName = '';
            this.terminal.clear();
            this.terminal.writeln(this.$t('terminal.connecting_service').replace('{name}', this.selectedService));

            const socket = API.createTerminalSocket(this.selectedService);
            socket.binaryType = 'arraybuffer';
            this.socket = socket;

            socket.onopen = () => {
                this.scheduleResize();
            };
            socket.onmessage = event => this.handleSocketMessage(event);
            socket.onerror = () => {
                this.state = 'error';
                this.showNotice(this.$t('terminal.connection_error'), 'error');
            };
            socket.onclose = () => {
                this.socket = null;
                if (this.userClosing) {
                    this.state = 'idle';
                    this.showNotice(this.$t('terminal.closed_by_user'), 'info');
                } else if (this.errorReceived) {
                    // error 消息已显示，不再覆盖
                    this.state = 'idle';
                } else if (this.state !== 'idle') {
                    this.state = 'idle';
                    this.showNotice(this.$t('terminal.abnormal_disconnect'), 'warning');
                }
                this.writeDisconnectHint();
            };
        },
        disconnect(userInitiated = false) {
            this.userClosing = userInitiated;
            if (this.socket) {
                this.sendControl({ type: 'close' });
                this.socket.close();
                this.socket = null;
            }
            this.state = 'idle';
            if (userInitiated) {
                this.showNotice(this.$t('terminal.closed_by_user'), 'info');
                this.writeDisconnectHint();
            }
        },
        handleSocketMessage(event) {
            if (typeof event.data === 'string') {
                this.handleControlMessage(event.data);
                return;
            }
            if (this.terminal && event.data) {
                this.terminal.write(new Uint8Array(event.data));
            }
        },
        handleControlMessage(data) {
            let msg = {};
            try {
                msg = JSON.parse(data || '{}');
            } catch (e) {
                return;
            }

            if (msg.type === 'ready') {
                this.state = 'connected';
                this.shellName = msg.shell || '';
                this.terminal.writeln(this.$t('terminal.connected').replace('{name}', this.selectedService));
                this.sendResize(); // 连接成功后主动同步一次当前尺寸
                this.scheduleResize();
            }
            if (msg.type === 'error') {
                this.errorReceived = true;
                this.state = 'error';
                const level = msg.code === 'terminal.no_shell' ? 'warning' : 'error';
                this.showNotice(this.terminalErrorMessage(msg), level);
            }
            if (msg.type === 'closed') {
                this.state = 'idle';
                this.showNotice(this.$t('terminal.session_closed'), 'info');
            }
        },
        sendInput(data) {
            if (!data || this.state !== 'connected') return;
            this.sendControl({ type: 'input', data });
        },
        sendControl(payload) {
            if (!this.socket || this.socket.readyState !== WebSocket.OPEN) return;
            this.socket.send(JSON.stringify(payload));
        },
        scheduleResize() {
            if (this.resizeTimer) {
                window.clearTimeout(this.resizeTimer);
            }
            this.resizeTimer = window.setTimeout(() => {
                this.resizeTimer = null;
                this.fitTerminal();
            }, 120);
        },
        fitTerminal() {
            if (!this.fitAddon || !this.terminal) return;
            try {
                this.fitAddon.fit();
            } catch (e) {
                return;
            }
            const cols = this.terminal.cols;
            const rows = this.terminal.rows;
            if (!isValidTerminalSize(cols, rows)) return;
            if (cols === this.lastFitCols && rows === this.lastFitRows) return;
            this.lastFitCols = cols;
            this.lastFitRows = rows;
            this.sendResize();
        },
        sendResize() {
            if (!this.terminal || this.state !== 'connected') return;
            const cols = this.terminal.cols;
            const rows = this.terminal.rows;
            if (isValidTerminalSize(cols, rows)) {
                this.sendControl({ type: 'resize', cols, rows });
            }
        },
        onServiceChange() {
            if (this.selectedService) {
                this.$router.replace({ name: 'terminal', query: { service: this.selectedService } });
            }
            this.notice = '';
        },
        showNotice(message, type = 'info') {
            this.notice = message;
            this.noticeType = type;
        },
        writeDisconnectHint() {
            if (!this.terminal || this.disconnectHintWritten || this.errorReceived) return;
            this.disconnectHintWritten = true;
            this.terminal.writeln('');
            this.terminal.writeln(this.$t('terminal.reconnect_new_shell'));
        },
        terminalErrorMessage(msg) {
            const codeMap = {
                'terminal.too_many_sessions': 'terminal.too_many_sessions',
                'terminal.no_shell': 'terminal.no_shell',
                'terminal.exec_create_failed': 'terminal.exec_create_failed',
                'terminal.exec_start_failed': 'terminal.exec_start_failed',
                'terminal.invalid_message': 'terminal.invalid_message',
                'terminal.unknown_message': 'terminal.unknown_message'
            };
            const key = codeMap[msg.code] || 'terminal.connection_error';
            const base = this.$t(key);
            if (['terminal.exec_create_failed', 'terminal.exec_start_failed'].includes(msg.code) && msg.message) {
                return base + ': ' + msg.message;
            }
            return base;
        }
    },
    async mounted() {
        await this.refreshServices();
        if (this.unmounted) return;
        const service = this.$route.query.service;
        if (service && this.services.some(item => item.name === service)) {
            this.selectedService = service;
            this.$nextTick(() => this.connect());
        } else if (service) {
            this.showNotice(this.$t('terminal.only_running'), 'warning');
        }
    },
    beforeUnmount() {
        this.unmounted = true;
        this.disconnect(false);
        if (this.resizeObserver) {
            this.resizeObserver.disconnect();
            this.resizeObserver = null;
        }
        if (this.resizeTimer) {
            window.clearTimeout(this.resizeTimer);
            this.resizeTimer = null;
        }
        // 先安全释放 fitAddon（避免 terminal.dispose 内部重复 dispose 已卸载的 addon）
        if (this.fitAddon) {
            try { this.fitAddon.dispose(); } catch (_) { /* addon 可能未加载 */ }
            this.fitAddon = null;
        }
        if (this.terminal) {
            try { this.terminal.dispose(); } catch (_) { /* 忽略 addon 状态不一致错误 */ }
            this.terminal = null;
        }
    }
};
