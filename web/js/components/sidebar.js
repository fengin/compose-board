/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * 侧边栏导航组件 — 内联 SVG 图标
 */
const AppSidebar = {
    template: `
    <nav class="sidebar">
        <div class="sidebar-logo" @click="showAbout = true" title="About ComposeBoard">
            <img src="/img/logo-128.png" alt="ComposeBoard" width="40" height="40" style="border-radius:8px">
        </div>
        <div class="sidebar-nav">
            <router-link 
                v-for="item in navItems" 
                :key="item.path"
                :to="item.path" 
                class="sidebar-item"
                active-class="active"
            >
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" v-html="item.svg"></svg>
                <span class="tooltip">{{ $t(item.labelKey) }}</span>
            </router-link>
        </div>
        <div class="sidebar-bottom" @click="showAbout = true">
            <span class="sidebar-version">v{{ appVersion }}</span>
        </div>

        <!-- About 弹窗 -->
        <div class="about-overlay" v-if="showAbout" @click.self="showAbout = false">
            <div class="about-dialog">
                <button class="about-close" @click="showAbout = false">&times;</button>
                <div class="about-logo">
                    <img src="/img/logo-128.png" alt="ComposeBoard" width="72" height="72" style="border-radius:14px">
                </div>
                <h2 class="about-title">ComposeBoard</h2>
                <p class="about-desc">Docker Compose 可视化管理面板，提供服务管理、日志查看、Web 终端、环境配置等一站式运维能力。</p>
                <div class="about-meta">
                    <div class="about-meta-row">
                        <span class="about-meta-label">版本</span>
                        <span class="about-meta-value mono">v{{ appVersion }}</span>
                    </div>
                    <div class="about-meta-row">
                        <span class="about-meta-label">作者</span>
                        <span class="about-meta-value">凌封</span>
                    </div>
                </div>
                <div class="about-links">
                    <a href="https://fengin.cn" target="_blank" class="about-link">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M2 12h20"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
                        作者主页
                    </a>
                    <a href="https://aibook.ren" target="_blank" class="about-link">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"/><path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"/></svg>
                        AI全书
                    </a>
                    <a href="https://github.com/fengin/compose-board" target="_blank" class="about-link">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z"/></svg>
                        GitHub
                    </a>
                </div>
                <div class="about-copyright">© 2026 凌封 · fengin.cn</div>
            </div>
        </div>
    </nav>
    `,
    data() {
        return {
            showAbout: false,
            appVersion: 'dev',
            navItems: [
                { 
                    path: '/', 
                    labelKey: 'nav.dashboard',
                    svg: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>'
                },
                { 
                    path: '/services', 
                    labelKey: 'nav.services',
                    svg: '<path d="M22 8.5c0-.28-.22-.5-.5-.5h-19c-.28 0-.5.22-.5.5v11c0 .28.22.5.5.5h19c.28 0 .5-.22.5-.5V8.5z"/><path d="M7 8V6a1 1 0 0 1 1-1h8a1 1 0 0 1 1 1v2"/><line x1="12" y1="12" x2="12" y2="16"/><line x1="8" y1="12" x2="8" y2="16"/><line x1="16" y1="12" x2="16" y2="16"/>'
                },
                { 
                    path: '/logs', 
                    labelKey: 'nav.logs',
                    svg: '<path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/><polyline points="14 2 14 8 20 8"/><line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="16" y2="17"/>'
                },
                {
                    path: '/terminal',
                    labelKey: 'nav.terminal',
                    svg: '<polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/>'
                },
                { 
                    path: '/env', 
                    labelKey: 'nav.env',
                    svg: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>'
                }
            ]
        };
    },
    async mounted() {
        try {
            const info = await API.getProjectSettings();
            if (info && info.app_version) {
                this.appVersion = info.app_version;
            }
        } catch (e) { /* ignore */ }
    }
};
