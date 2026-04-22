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
                isAuthenticated: API.isAuthenticated()
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
            }
        }
    });

    // 注册 i18n 全局方法，模板中通过 $t('key') 调用
    app.config.globalProperties.$t = I18n.t.bind(I18n);

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
