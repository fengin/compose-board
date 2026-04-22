/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * 登录页组件
 */
const LoginPage = {
    template: `
    <div class="login-wrapper">
        <div class="login-card">
            <div class="login-logo">C</div>
            <h1 class="login-title">ComposeBoard</h1>
            <p class="login-subtitle">Docker Compose 可视化管理面板</p>

            <div v-if="error" class="login-error">{{ error }}</div>

            <form @submit.prevent="handleLogin">
                <div class="form-group">
                    <label class="form-label">用户名</label>
                    <input 
                        class="form-input" 
                        type="text" 
                        v-model="username" 
                        placeholder="请输入用户名"
                        autocomplete="username"
                    >
                </div>
                <div class="form-group">
                    <label class="form-label">密码</label>
                    <input 
                        class="form-input" 
                        type="password" 
                        v-model="password" 
                        placeholder="请输入密码"
                        autocomplete="current-password"
                    >
                </div>
                <button class="login-btn" type="submit" :disabled="loading">
                    {{ loading ? '登录中...' : '登 录' }}
                </button>
            </form>
        </div>
    </div>
    `,
    data() {
        return { username: '', password: '', error: '', loading: false };
    },
    methods: {
        async handleLogin() {
            this.error = '';
            this.loading = true;
            try {
                await API.login(this.username, this.password);
                this.$emit('login-success');
            } catch (e) {
                this.error = e.message || '登录失败';
            } finally {
                this.loading = false;
            }
        }
    }
};
