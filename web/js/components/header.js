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
            <div class="header-user">
                <div class="header-user-avatar">A</div>
                <span>admin</span>
            </div>
            <button class="btn btn-ghost btn-sm" @click="$emit('logout')">{{ $t('auth.logout') }}</button>
        </div>
    </header>
    `,
    computed: {
        pageTitle() {
            const routeToKey = {
                'dashboard': 'dashboard.title',
                'services': 'services.title',
                'logs': 'logs.title',
                'env': 'env.title'
            };
            const key = routeToKey[this.$route.name];
            return key ? this.$t(key) : 'ComposeBoard';
        }
    }
};
