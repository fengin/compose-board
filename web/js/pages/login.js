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
        <button class="btn-lang login-lang" @click="$emit('switch-language')" :title="langLabel">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M2 12h20"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10A15.3 15.3 0 0 1 12 2z"/></svg>
            <span>{{ langText }}</span>
        </button>
        <div class="login-card">
            <div class="login-logo">C</div>
            <h1 class="login-title">{{ $t('app.name') }}</h1>
            <p class="login-subtitle">{{ $t('app.subtitle') }}</p>

            <div v-if="error" class="login-error">{{ error }}</div>

            <form @submit.prevent="handleLogin">
                <div class="form-group">
                    <label class="form-label">{{ $t('auth.username') }}</label>
                    <input 
                        class="form-input" 
                        type="text" 
                        v-model="username" 
                        :placeholder="$t('auth.username')"
                        autocomplete="username"
                    >
                </div>
                <div class="form-group">
                    <label class="form-label">{{ $t('auth.password') }}</label>
                    <input 
                        class="form-input" 
                        type="password" 
                        v-model="password" 
                        :placeholder="$t('auth.password')"
                        autocomplete="current-password"
                    >
                </div>
                <button class="login-btn" type="submit" :disabled="loading">
                    {{ loading ? $t('auth.logging_in') : $t('auth.login_button') }}
                </button>
            </form>
        </div>
    </div>
    `,
    inject: ['getLocale'],
    data() {
        return { username: '', password: '', error: '', loading: false };
    },
    computed: {
        langText() {
            return this.getLocale() === 'zh' ? 'EN' : '中';
        },
        langLabel() {
            return this.getLocale() === 'zh' ? 'Switch to English' : '切换到中文';
        }
    },
    methods: {
        async handleLogin() {
            this.error = '';
            this.loading = true;
            try {
                await API.login(this.username, this.password);
                this.$emit('login-success');
            } catch (e) {
                this.error = e.message || this.$t('auth.login_failed');
            } finally {
                this.loading = false;
            }
        }
    }
};

