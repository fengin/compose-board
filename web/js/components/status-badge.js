/**
 * 状态徽章组件
 */
const StatusBadge = {
    props: {
        status: { type: String, default: 'unknown' }
    },
    template: `
    <span class="status-badge" :class="status">
        <span class="status-dot" :class="status"></span>
        {{ label }}
    </span>
    `,
    computed: {
        label() {
            const key = 'services.status.' + this.status;
            const translated = this.$t(key);
            // 如果 key 不存在，$t 会返回 key 本身，此时 fallback 显示原始状态
            return translated !== key ? translated : this.status;
        }
    }
};
