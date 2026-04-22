/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * i18n 轻量国际化模块
 * 提供 t(key, params) 函数，支持嵌套 key 和模板变量
 */

const I18n = {
    locale: localStorage.getItem('composeboard_locale') || 'zh',
    messages: {},
    loaded: false,

    /**
     * 加载指定语言的 locale 文件
     * @param {string} locale - 语言代码 ('zh' | 'en')
     * @returns {Promise<void>}
     */
    async load(locale) {
        try {
            const resp = await fetch(`/js/locales/${locale}.json`);
            if (!resp.ok) {
                console.error(`[i18n] 加载 ${locale}.json 失败: ${resp.status}`);
                return;
            }
            this.messages = await resp.json();
            this.locale = locale;
            this.loaded = true;
            localStorage.setItem('composeboard_locale', locale);
        } catch (err) {
            console.error(`[i18n] 加载语言包失败:`, err);
        }
    },

    /**
     * 翻译 key 为当前语言文本
     * @param {string} key - 点分隔路径，如 'services.status.running'
     * @param {Object} [params] - 模板变量，如 {count: 5}
     * @returns {string} 翻译后的文本，找不到则返回 key 本身
     */
    t(key, params) {
        const val = key.split('.').reduce((obj, k) => (obj && obj[k] !== undefined) ? obj[k] : null, this.messages);
        if (val === null || val === undefined) {
            // 开发模式下输出缺失 key 警告
            if (this.loaded) {
                console.warn(`[i18n] 缺失 key: ${key}`);
            }
            return key;
        }
        if (typeof val !== 'string') {
            return key;
        }
        // 替换模板变量 {name}
        if (params) {
            return val.replace(/\{(\w+)\}/g, (_, k) => (params[k] !== undefined ? params[k] : `{${k}}`));
        }
        return val;
    },

    /**
     * 获取当前语言代码
     * @returns {string}
     */
    getLocale() {
        return this.locale;
    },

    /**
     * 切换语言并重新加载
     * @param {string} locale
     * @returns {Promise<void>}
     */
    async switchLocale(locale) {
        await this.load(locale);
    }
};

// 导出到全局
window.I18n = I18n;
