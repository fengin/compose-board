/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * API 请求封装
 * 统一处理认证、错误码、基础 URL
 */
const API = {
    baseURL: '',

    // 401 回调（由 Vue App 设置）
    onUnauthorized: null,

    // 获取存储的 token
    getToken() {
        return localStorage.getItem('composeboard_token');
    },

    // 存储 token
    setToken(token) {
        localStorage.setItem('composeboard_token', token);
    },

    // 清除 token
    clearToken() {
        localStorage.removeItem('composeboard_token');
    },

    // 检查是否已认证
    isAuthenticated() {
        const token = this.getToken();
        if (!token) return false;
        try {
            const payload = JSON.parse(atob(token.split('.')[1]));
            return payload.exp * 1000 > Date.now();
        } catch {
            return false;
        }
    },

    // 通用请求方法（G-1: 增强错误码解析）
    async request(method, path, body = null) {
        const headers = { 'Content-Type': 'application/json' };
        const token = this.getToken();
        if (token) {
            headers['Authorization'] = `Bearer ${token}`;
        }

        const options = { method, headers };
        if (body) {
            options.body = JSON.stringify(body);
        }

        const resp = await fetch(`${this.baseURL}${path}`, options);

        // 401 处理：login 请求的 401 是密码错误，不触发登出
        if (resp.status === 401 && !path.includes('/auth/login')) {
            this.clearToken();
            if (this.onUnauthorized) {
                this.onUnauthorized();
            }
            throw new Error(I18n.t('auth.token_expired'));
        }

        const data = await resp.json();
        if (!resp.ok) {
            // G-1: 保留错误码供前端按业务分支处理
            const err = new Error(data.error || I18n.t('common.request_failed', { status: resp.status }));
            err.code = data.code || '';
            err.status = resp.status;
            throw err;
        }
        return data;
    },

    // 便捷方法
    get(path) { return this.request('GET', path); },
    post(path, body) { return this.request('POST', path, body); },
    put(path, body) { return this.request('PUT', path, body); },

    // === 认证 ===
    async login(username, password) {
        const data = await this.post('/api/auth/login', { username, password });
        if (data.token) {
            this.setToken(data.token);
        }
        return data;
    },

    // === 主机信息 ===
    getHostInfo() { return this.get('/api/host/info'); },

    // === 服务管理（按服务名操作） ===
    getServices() { return this.get('/api/services'); },
    getServiceStatus(name) { return this.get(`/api/services/${name}/status`); },
    startService(name) { return this.post(`/api/services/${name}/start`); },
    stopService(name) { return this.post(`/api/services/${name}/stop`); },
    restartService(name) { return this.post(`/api/services/${name}/restart`); },
    getServiceEnv(name) { return this.get(`/api/services/${name}/env`); },

    // === 升级与重建 ===
    pullImage(name) { return this.post(`/api/services/${name}/pull`); },
    getPullStatus(name) { return this.get(`/api/services/${name}/pull-status`); },
    applyUpgrade(name) { return this.post(`/api/services/${name}/upgrade`); },
    rebuildService(name) { return this.post(`/api/services/${name}/rebuild`); },

    // === Profiles ===
    getProfiles() { return this.get('/api/profiles'); },
    enableProfile(name) { return this.post(`/api/profiles/${name}/enable`); },
    disableProfile(name) { return this.post(`/api/profiles/${name}/disable`); },

    // === .env 配置（B-2: PUT 对齐文档） ===
    getEnvFile() { return this.get('/api/env'); },
    saveEnvFile(body) { return this.put('/api/env', body); },

    // === 设置 ===
    getProjectSettings() { return this.get('/api/settings/project'); },

    // === 日志（SSE 替代 WebSocket） ===
    getLogHistory(service, tail = 200) {
        return this.get(`/api/services/${service}/logs?tail=${tail}`);
    },

    // 创建 SSE 日志流连接
    createLogStream(service, tail = 100) {
        const token = this.getToken();
        const url = `${this.baseURL}/api/services/${service}/logs?follow=true&tail=${tail}&token=${token}`;
        return new EventSource(url);
    }
};

/**
 * Toast 通知管理
 */
const Toast = {
    container: null,

    init() {
        this.container = document.createElement('div');
        this.container.style.cssText = 'position:fixed;top:20px;right:20px;z-index:9999;display:flex;flex-direction:column;gap:8px;';
        document.body.appendChild(this.container);
    },

    show(message, type = 'info', duration = 3000) {
        if (!this.container) this.init();
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.textContent = message;
        this.container.appendChild(toast);
        setTimeout(() => {
            toast.style.opacity = '0';
            toast.style.transform = 'translateX(40px)';
            toast.style.transition = 'all 0.3s ease';
            setTimeout(() => toast.remove(), 300);
        }, duration);
    },

    success(msg) { this.show(msg, 'success'); },
    error(msg) { this.show(msg, 'error', 5000); },
    info(msg) { this.show(msg, 'info'); }
};
