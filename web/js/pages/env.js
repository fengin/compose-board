/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * .env 配置编辑页面 — 扁平化设计
 */
const EnvPage = {
    template: `
    <div>
        <div class="card no-hover env-page-card">
            <div class="card-header">
                <h2 class="card-title">{{ $t('env.title') }}</h2>
                <div style="display:flex;gap:8px;align-items:center">
                    <span v-if="filePath" class="mono" style="font-size:0.75rem;color:var(--color-fg-tertiary)">{{ filePath }}</span>
                    <button class="btn btn-ghost btn-sm" @click="loadEnv" :disabled="loading">↻ {{ $t('env.refresh') }}</button>
                    <button class="btn btn-primary btn-sm" @click="showDiffPreview" :disabled="!hasChanges || saving">
                        💾 {{ $t('env.save') }}
                    </button>
                </div>
            </div>

            <!-- 加载中 -->
            <div v-if="loading" style="text-align:center;padding:40px">
                <div class="loading-spinner" style="margin:0 auto"></div>
            </div>

            <!-- 编辑模式切换 -->
            <div v-else class="env-mode-switch">
                <button 
                    class="btn btn-sm" 
                    :class="editMode === 'table' ? 'btn-primary' : 'btn-ghost'"
                    @click="editMode = 'table'"
                >{{ $t('env.table_mode') }}</button>
                <button 
                    class="btn btn-sm" 
                    :class="editMode === 'raw' ? 'btn-primary' : 'btn-ghost'"
                    @click="editMode = 'raw'"
                >{{ $t('env.text_mode') }}</button>
                <span v-if="hasChanges" style="color:var(--color-accent);font-size:0.8rem;font-weight:600;display:flex;align-items:center;gap:4px">
                    ● {{ $t('env.unsaved') }}
                </span>
            </div>

            <!-- 表格模式 -->
            <div v-if="editMode === 'table' && !loading" class="table-wrapper env-table-scroll hover-scroll">
                <table class="table env-config-table">
                    <thead>
                        <tr>
                            <th style="width:280px">{{ $t('env.key') }}</th>
                            <th>{{ $t('env.value') }}</th>
                        </tr>
                    </thead>
                    <tbody>
                        <template v-for="(entry, i) in entries" :key="i">
                            <tr v-if="entry.type === 'comment'" style="background:var(--color-bg-muted)">
                                <td colspan="2" style="color:var(--color-fg-tertiary);font-size:0.8rem;font-style:italic">
                                    {{ entry.raw }}
                                </td>
                            </tr>
                            <tr v-else-if="entry.type === 'blank'">
                                <td colspan="2" style="height:8px"></td>
                            </tr>
                            <tr v-else-if="entry.type === 'variable'">
                                <td class="mono" style="font-weight:600;color:var(--color-primary);word-break:break-all;max-width:300px">{{ entry.key }}</td>
                                <td>
                                    <input 
                                        class="form-input" 
                                        style="padding:6px 10px;font-family:var(--font-mono);font-size:0.8rem"
                                        v-model="entry.value"
                                        @input="markChanged"
                                    >
                                </td>
                            </tr>
                        </template>
                    </tbody>
                </table>
            </div>

            <!-- 原文模式 -->
            <div v-if="editMode === 'raw' && !loading" class="env-raw-scroll">
                <textarea 
                    class="env-editor hover-scroll"
                    v-model="rawText"
                    @input="markChanged"
                    spellcheck="false"
                ></textarea>
            </div>
        </div>

        <!-- Diff 预览弹窗 -->
        <div class="modal-overlay" v-if="diffModal.visible" @click.self="diffModal.visible = false">
            <div class="modal" style="max-width:700px">
                <div class="modal-header">
                    <h3 class="modal-title">{{ $t('env.diff_title') }}</h3>
                    <button class="modal-close" @click="diffModal.visible = false">✕</button>
                </div>
                <div class="modal-body">
                    <div class="diff-view">
                        <div v-for="(line, i) in diffModal.lines" :key="i" 
                            class="diff-line"
                            :class="line.type"
                        >
                            <span class="diff-marker">{{ line.type === 'add' ? '+' : line.type === 'remove' ? '-' : ' ' }}</span>
                            <span class="mono">{{ line.text }}</span>
                        </div>
                        <div v-if="diffModal.lines.length === 0" style="padding:20px;text-align:center;color:var(--color-fg-tertiary)">
                            {{ $t('env.no_diff') }}
                        </div>
                    </div>
                    <div class="btn-group" style="margin-top:20px">
                        <button class="btn btn-ghost" @click="diffModal.visible = false">{{ $t('common.cancel') }}</button>
                        <button class="btn btn-primary" @click="saveEnv" :disabled="saving">
                            <span v-if="saving" class="loading-spinner" style="width:14px;height:14px;border-top-color:#fff"></span>
                            <span v-else>{{ $t('env.confirm_save_btn') }}</span>
                        </button>
                    </div>
                </div>
            </div>
        </div>
        <!-- 保存成功提示 -->
        <div class="modal-overlay" v-if="showSaveTip" @click.self="showSaveTip = false">
            <div class="modal" style="max-width:440px">
                <div class="modal-header">
                    <h3 class="modal-title">✅ {{ $t('env.save_tip_title') }}</h3>
                    <button class="modal-close" @click="showSaveTip = false">✕</button>
                </div>
                <div class="modal-body">
                    <div class="confirm-dialog">
                        <p style="color:var(--color-fg-secondary);line-height:1.8">
                            {{ $t('env.save_tip_message') }}<br>
                            <strong style="color:var(--color-fg)">{{ $t('env.save_tip_note') }}</strong>
                        </p>
                        <div class="btn-group">
                            <button class="btn btn-ghost" @click="showSaveTip = false">{{ $t('env.save_tip_dismiss') }}</button>
                            <button class="btn btn-primary" @click="goToServices">{{ $t('env.save_tip_goto') }} →</button>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
    `,
    data() {
        return {
            entries: [],
            rawText: '',
            originalRawText: '',
            filePath: '',
            loading: true,
            saving: false,
            editMode: 'table',
            hasChanges: false,
            showSaveTip: false,
            diffModal: { visible: false, lines: [] }
        };
    },
    methods: {
        async loadEnv() {
            this.loading = true;
            try {
                const data = await API.getEnvFile();
                this.entries = (data.entries || []).map(e => {
                    if (e.type === 'variable') {
                        return { ...e, _originalValue: e.value };
                    }
                    return e;
                });
                this.rawText = data.raw_text || '';
                this.originalRawText = this.rawText;
                this.filePath = data.file_path || '';
                this.hasChanges = false;
            } catch (e) {
                Toast.error(this.$t('env.load_failed') + ': ' + e.message);
            } finally {
                this.loading = false;
            }
        },
        markChanged() {
            this.hasChanges = true;
        },
        getCurrentContent() {
            if (this.editMode === 'raw') {
                return this.rawText;
            }
            // M-1: 从 entries 重建文本，使用 raw 保留注释/空行原始格式
            const lines = [];
            for (const entry of this.entries) {
                if (entry.type === 'variable') {
                    // 变量行：用修改后的 key=value
                    lines.push(`${entry.key}=${entry.value}`);
                } else {
                    // 注释/空行：保留原始内容
                    lines.push(entry.raw || '');
                }
            }
            return lines.join('\n') + '\n';
        },
        showDiffPreview() {
            const newContent = this.getCurrentContent();
            const oldLines = this.originalRawText.split('\n').filter(l => l.trim());
            const newLines = newContent.split('\n').filter(l => l.trim());

            const lines = [];
            const oldSet = new Set(oldLines);
            const newSet = new Set(newLines);

            for (const l of oldLines) {
                if (!newSet.has(l)) {
                    lines.push({ type: 'remove', text: l });
                }
            }
            for (const l of newLines) {
                if (!oldSet.has(l)) {
                    lines.push({ type: 'add', text: l });
                }
            }

            this.diffModal = { visible: true, lines };
        },
        async saveEnv() {
            this.saving = true;
            try {
                let result;
                if (this.editMode === 'table') {
                    // 只重建值被修改过的变量的 raw，未修改的保留原始 raw（含引号/行内注释）
                    const entries = this.entries.map(e => {
                        if (e.type === 'variable' && e.value !== e._originalValue) {
                            return { ...e, raw: `${e.key}=${e.value}` };
                        }
                        return e;
                    });
                    result = await API.saveEnvFile({ entries });
                } else {
                    // 原文模式走 content
                    result = await API.saveEnvFile({ content: this.rawText });
                }
                this.diffModal.visible = false;
                this.originalRawText = this.getCurrentContent();
                this.hasChanges = false;
                this.rawText = this.originalRawText;
                this.showSaveTip = true;
            } catch (e) {
                Toast.error(this.$t('env.save_failed') + ': ' + e.message);
            } finally {
                this.saving = false;
            }
        },
        goToServices() {
            this.showSaveTip = false;
            this.$router.push({ name: 'services' });
        }
    },
    watch: {
        editMode(newMode) {
            if (newMode === 'raw') {
                // 表格 → 原文：同步
                this.rawText = this.getCurrentContent();
            } else {
                // 原文 → 表格：重新解析（构造后端 EnvEntry 兼容格式）
                const lines = this.rawText.split('\n');
                this.entries = [];
                let lineNum = 0;
                for (const line of lines) {
                    lineNum++;
                    const trimmed = line.trim();
                    if (!trimmed) {
                        this.entries.push({ type: 'blank', raw: line, line: lineNum });
                    } else if (trimmed.startsWith('#')) {
                        this.entries.push({ type: 'comment', raw: line, line: lineNum });
                    } else {
                        const idx = trimmed.indexOf('=');
                        if (idx > 0) {
                            this.entries.push({
                                type: 'variable',
                                key: trimmed.substring(0, idx).trim(),
                                value: trimmed.substring(idx + 1).trim(),
                                raw: line,
                                line: lineNum
                            });
                        } else {
                            this.entries.push({ type: 'comment', raw: line, line: lineNum });
                        }
                    }
                }
            }
        }
    },
    mounted() {
        this.loadEnv();
    }
};
