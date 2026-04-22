/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * Vue 3 + Vue Router 应用入口（本地 vendor 模式）
 */
const { createApp, ref, onMounted } = Vue;
const { createRouter, createWebHistory } = VueRouter;

// 路由定义
const routes = [
    { path: '/', name: 'dashboard', component: DashboardPage },
    { path: '/services', name: 'services', component: ServicesPage },
    { path: '/logs', name: 'logs', component: LogsPage },
    { path: '/env', name: 'env', component: EnvPage },
    // G-4: 旧 URL 兼容重定向
    { path: '/containers', redirect: '/services' }
];

const router = createRouter({
    history: createWebHistory(),
    routes
});

// 初始化 i18n 并创建 Vue App
(async function () {
    // 预加载语言包
    await I18n.load(I18n.locale);

    // 创建 Vue App
    const app = createApp({
        data() {
            return {
                isAuthenticated: API.isAuthenticated(),
                locale: I18n.locale
            };
        },
        created() {
            // 注册 401 回调：API 返回 401 时切换到登录态
            API.onUnauthorized = () => {
                this.isAuthenticated = false;
            };
        },
        methods: {
            onLoginSuccess() {
                this.isAuthenticated = true;
                // 确保 router 回到首页
                this.$nextTick(() => {
                    router.push('/');
                });
            },
            onLogout() {
                API.clearToken();
                this.isAuthenticated = false;
            },
            async switchLanguage() {
                const next = this.locale === 'zh' ? 'en' : 'zh';
                await I18n.switchLocale(next);
                this.locale = next;
            }
        },
        provide() {
            // 让子组件能访问响应式 locale 和切换方法
            return {
                getLocale: () => this.locale,
                switchLanguage: this.switchLanguage
            };
        }
    });

    // 注册 i18n 全局方法：通过 mixin 注入 $t()
    // 访问 this.$root.locale 建立响应式依赖，locale 变化时自动重渲染（不销毁组件）
    app.mixin({
        methods: {
            $t(key, params) {
                void this.$root.locale;
                return I18n.t(key, params);
            }
        }
    });

    // 注册全局组件
    app.component('login-page', LoginPage);
    app.component('app-sidebar', AppSidebar);
    app.component('app-header', AppHeader);
    app.component('status-badge', StatusBadge);

    // 使用路由
    app.use(router);

    // 挂载
    app.mount('#app');
})();
