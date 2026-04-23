/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * 通用确认弹窗组件
 * Props: visible, title, message, warning, btnText, btnClass
 * Events: confirm, cancel
 */
const ConfirmDialog = {
    template: `
    <div class="modal-overlay" v-if="visible" @click.self="$emit('cancel')">
        <div class="modal" style="max-width:440px">
            <div class="modal-header">
                <h3 class="modal-title">{{ title }}</h3>
                <button class="modal-close" @click="$emit('cancel')">✕</button>
            </div>
            <div class="modal-body">
                <div class="confirm-dialog">
                    <div v-html="message"></div>
                    <div v-if="warning"
                        style="background:#FFFBEB;padding:12px 16px;border-radius:var(--radius);font-size:0.85rem;color:#92400E;margin:16px 0">
                        {{ warning }}
                    </div>
                    <div class="btn-group" style="margin-top:20px">
                        <button class="btn btn-ghost" @click="$emit('cancel')">{{ $t('common.cancel') }}</button>
                        <button class="btn" :class="btnClass || 'btn-primary'" @click="$emit('confirm')">{{ btnText }}</button>
                    </div>
                </div>
            </div>
        </div>
    </div>
    `,
    props: {
        visible: Boolean,
        title: { type: String, default: '' },
        message: { type: String, default: '' },
        warning: { type: String, default: '' },
        btnText: { type: String, default: '' },
        btnClass: { type: String, default: 'btn-primary' }
    },
    emits: ['confirm', 'cancel']
};
