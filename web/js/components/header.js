/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * 顶部栏组件
 */
const AppHeader = {
    template: `
    <header class="header">
        <div class="header-title">{{ pageTitle }}</div>
        <div class="header-actions">
            <button class="btn-lang" @click="$emit('switch-language')" :title="langLabel">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M2 12h20"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10A15.3 15.3 0 0 1 12 2z"/></svg>
                <span>{{ langText }}</span>
            </button>
            <div class="header-user">
                <div class="header-user-avatar">A</div>
                <span>admin</span>
            </div>
            <button class="btn btn-ghost btn-sm" @click="$emit('logout')">{{ $t('auth.logout') }}</button>
        </div>
    </header>
    `,
    inject: ['getLocale'],
    computed: {
        pageTitle() {
            const routeToKey = {
                'dashboard': 'dashboard.title',
                'services': 'services.title',
                'logs': 'logs.title',
                'terminal': 'terminal.title',
                'env': 'env.title'
            };
            const key = routeToKey[this.$route.name];
            return key ? this.$t(key) : 'ComposeBoard';
        },
        langText() {
            return this.getLocale() === 'zh' ? 'EN' : '中';
        },
        langLabel() {
            return this.getLocale() === 'zh' ? 'Switch to English' : '切换到中文';
        }
    }
};
