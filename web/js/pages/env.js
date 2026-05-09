/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * 配置修改页面 — Tab 结构（环境变量 + Compose 配置）
 * 排版结构：
 *   页头: "配置修改" 标题 (card-header 层)
 *   操作栏: [Tab切换]  [文件路径]  [刷新] [保存]
 *   内容区: 各 Tab 的编辑内容
 */
const EnvPage = {
    template: `
    <div>
        <div class="card no-hover env-page-card">
            <!-- 页头: 标题独占一行 -->
            <div class="card-header">
                <h2 class="card-title">{{ $t('config.title') }}</h2>
            </div>

            <!-- 操作栏: Tab 切换 + 文件路径 + 刷新 + 保存 -->
            <div class="config-toolbar">
                <div class="config-tabs">
                    <button
                        class="config-tab"
                        :class="{ active: activeTab === 'env' }"
                        @click="activeTab = 'env'"
                    >{{ $t('config.tab_env') }}</button>
                    <button
                        class="config-tab"
                        :class="{ active: activeTab === 'compose' }"
                        @click="switchToCompose"
                    >{{ $t('config.tab_compose') }}</button>
                </div>
                <div class="config-toolbar-right">
                    <span v-if="currentFilePath" class="config-filepath mono">{{ currentFilePath }}</span>
                    <span v-if="currentFileNotExist" class="config-file-hint">{{ $t('config.file_not_exist') }}</span>
                    <span v-if="currentHasChanges" class="config-unsaved">● {{ $t('env.unsaved') }}</span>
                    <button class="btn btn-ghost btn-sm" @click="downloadCurrentTab" :disabled="currentFileNotExist && !currentHasChanges">⬇️ {{ $t('config.download') }}</button>
                    <button class="btn btn-ghost btn-sm" @click="refreshCurrentTab" :disabled="currentLoading">↻ {{ $t('env.refresh') }}</button>
                    <button class="btn btn-primary btn-sm" @click="saveCurrentTab" :disabled="!currentHasChanges || currentSaving">
                        💾 {{ $t('env.save') }}
                    </button>
                </div>
            </div>

            <!-- ========== Tab 1: 环境变量(.env) ========== -->
            <template v-if="activeTab === 'env'">
                <!-- 加载中 -->
                <div v-if="env.loading" style="text-align:center;padding:40px">
                    <div class="loading-spinner" style="margin:0 auto"></div>
                </div>

                <template v-else>
                    <!-- 文件不存在提示 -->
                    <div v-if="env.fileNotExist" class="config-empty-hint">
                        📄 {{ $t('config.env_not_exist_hint') }}
                    </div>

                    <!-- 编辑模式切换 -->
                    <div class="env-mode-switch">
                        <button
                            class="btn btn-sm"
                            :class="env.editMode === 'table' ? 'btn-primary' : 'btn-ghost'"
                            @click="env.editMode = 'table'"
                        >{{ $t('env.table_mode') }}</button>
                        <button
                            class="btn btn-sm"
                            :class="env.editMode === 'raw' ? 'btn-primary' : 'btn-ghost'"
                            @click="env.editMode = 'raw'"
                        >{{ $t('env.text_mode') }}</button>
                    </div>

                    <!-- 表格模式 -->
                    <div v-if="env.editMode === 'table'" class="table-wrapper env-table-scroll hover-scroll">
                        <table class="table env-config-table">
                            <thead>
                                <tr>
                                    <th style="width:280px">{{ $t('env.key') }}</th>
                                    <th>{{ $t('env.value') }}</th>
                                </tr>
                            </thead>
                            <tbody>
                                <template v-for="(entry, i) in env.entries" :key="i">
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
                                                @input="markEnvChanged"
                                            >
                                        </td>
                                    </tr>
                                </template>
                            </tbody>
                        </table>
                    </div>

                    <!-- 原文模式 -->
                    <div v-if="env.editMode === 'raw'" class="env-raw-scroll" style="height:100%">
                        <code-editor
                            v-model="env.rawText"
                            @input="markEnvChanged"
                        ></code-editor>
                    </div>
                </template>
            </template>

            <!-- ========== Tab 2: Compose 配置 ========== -->
            <template v-if="activeTab === 'compose'">
                <!-- 加载中 -->
                <div v-if="compose.loading" style="text-align:center;padding:40px">
                    <div class="loading-spinner" style="margin:0 auto"></div>
                </div>

                <!-- 编辑器 -->
                <div v-else style="height:100%">
                    <!-- 文件不存在提示 -->
                    <div v-if="compose.fileNotExist" class="config-empty-hint">
                        📄 {{ $t('config.compose_not_exist_hint') }}
                    </div>
                    <div class="env-raw-scroll" style="height:100%">
                        <code-editor
                            v-model="compose.content"
                            @input="markComposeChanged"
                        ></code-editor>
                    </div>
                </div>
            </template>
        </div>

        <!-- Diff 预览弹窗（.env） -->
        <div class="modal-overlay" v-if="env.diffModal.visible" @click.self="env.diffModal.visible = false">
            <div class="modal" style="max-width:700px">
                <div class="modal-header">
                    <h3 class="modal-title">{{ $t('env.diff_title') }}</h3>
                    <button class="modal-close" @click="env.diffModal.visible = false">✕</button>
                </div>
                <div class="modal-body">
                    <div class="diff-view">
                        <div v-for="(line, i) in env.diffModal.lines" :key="i"
                            class="diff-line"
                            :class="line.type"
                        >
                            <span class="diff-marker">{{ line.type === 'add' ? '+' : line.type === 'remove' ? '-' : ' ' }}</span>
                            <span class="mono">{{ line.text }}</span>
                        </div>
                        <div v-if="env.diffModal.lines.length === 0" style="padding:20px;text-align:center;color:var(--color-fg-tertiary)">
                            {{ $t('env.no_diff') }}
                        </div>
                    </div>
                    <div class="btn-group" style="margin-top:20px">
                        <button class="btn btn-ghost" @click="env.diffModal.visible = false">{{ $t('common.cancel') }}</button>
                        <button class="btn btn-primary" @click="saveEnv" :disabled="env.saving">
                            <span v-if="env.saving" class="loading-spinner" style="width:14px;height:14px;border-top-color:#fff"></span>
                            <span v-else>{{ $t('env.confirm_save_btn') }}</span>
                        </button>
                    </div>
                </div>
            </div>
        </div>

        <!-- Diff 预览弹窗（Compose） -->
        <div class="modal-overlay" v-if="compose.diffModal.visible" @click.self="compose.diffModal.visible = false">
            <div class="modal" style="max-width:700px">
                <div class="modal-header">
                    <h3 class="modal-title">{{ $t('compose.diff_title') }}</h3>
                    <button class="modal-close" @click="compose.diffModal.visible = false">✕</button>
                </div>
                <div class="modal-body">
                    <div class="diff-view">
                        <div v-for="(line, i) in compose.diffModal.lines" :key="i"
                            class="diff-line"
                            :class="line.type"
                        >
                            <span class="diff-marker">{{ line.type === 'add' ? '+' : line.type === 'remove' ? '-' : ' ' }}</span>
                            <span class="mono">{{ line.text }}</span>
                        </div>
                        <div v-if="compose.diffModal.lines.length === 0" style="padding:20px;text-align:center;color:var(--color-fg-tertiary)">
                            {{ $t('compose.no_diff') }}
                        </div>
                    </div>
                    <div class="btn-group" style="margin-top:20px">
                        <button class="btn btn-ghost" @click="compose.diffModal.visible = false">{{ $t('common.cancel') }}</button>
                        <button class="btn btn-primary" @click="saveCompose" :disabled="compose.saving">
                            <span v-if="compose.saving" class="loading-spinner" style="width:14px;height:14px;border-top-color:#fff"></span>
                            <span v-else>{{ $t('compose.confirm_save_btn') }}</span>
                        </button>
                    </div>
                </div>
            </div>
        </div>

        <!-- 保存成功提示（.env） -->
        <div class="modal-overlay" v-if="env.showSaveTip" @click.self="env.showSaveTip = false">
            <div class="modal" style="max-width:440px">
                <div class="modal-header">
                    <h3 class="modal-title">✅ {{ $t('env.save_tip_title') }}</h3>
                    <button class="modal-close" @click="env.showSaveTip = false">✕</button>
                </div>
                <div class="modal-body">
                    <div class="confirm-dialog">
                        <p style="color:var(--color-fg-secondary);line-height:1.8">
                            {{ $t('env.save_tip_message') }}<br>
                            <strong style="color:var(--color-fg)">{{ $t('env.save_tip_note') }}</strong>
                        </p>
                        <div class="btn-group">
                            <button class="btn btn-ghost" @click="env.showSaveTip = false">{{ $t('env.save_tip_dismiss') }}</button>
                            <button class="btn btn-primary" @click="goToServices">{{ $t('env.save_tip_goto') }} →</button>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <!-- 保存成功提示（Compose） -->
        <div class="modal-overlay" v-if="compose.showSaveTip" @click.self="compose.showSaveTip = false">
            <div class="modal" style="max-width:500px">
                <div class="modal-header">
                    <h3 class="modal-title">✅ {{ $t('compose.save_tip_title') }}</h3>
                    <button class="modal-close" @click="compose.showSaveTip = false">✕</button>
                </div>
                <div class="modal-body">
                    <div class="confirm-dialog">
                        <p style="color:var(--color-fg-secondary);line-height:1.8">
                            {{ $t('compose.save_tip_message') }}<br>
                            <strong style="color:var(--color-fg)">{{ $t('compose.save_tip_note') }}</strong>
                        </p>
                        <div v-if="compose.lastSaveResult.added && compose.lastSaveResult.added.length > 0" style="margin-top:12px">
                            <div class="form-label">{{ $t('compose.services_added') }}</div>
                            <div class="mono" style="padding:8px 14px;background:#F0FDF4;border-radius:var(--radius);font-size:0.85rem;color:#166534">
                                {{ compose.lastSaveResult.added.join(', ') }}
                            </div>
                        </div>
                        <div v-if="compose.lastSaveResult.removed && compose.lastSaveResult.removed.length > 0" style="margin-top:12px">
                            <div class="form-label">{{ $t('compose.services_removed') }}</div>
                            <div class="mono" style="padding:8px 14px;background:#FEF2F2;border-radius:var(--radius);font-size:0.85rem;color:#991B1B">
                                {{ compose.lastSaveResult.removed.join(', ') }}
                            </div>
                        </div>
                        <div class="btn-group" style="margin-top:16px">
                            <button class="btn btn-ghost" @click="compose.showSaveTip = false">{{ $t('compose.save_tip_dismiss') }}</button>
                            <button class="btn btn-primary" @click="goToServices">{{ $t('compose.save_tip_goto') }} →</button>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
    `,
    data() {
        return {
            activeTab: 'env',
            // === 环境变量 Tab 数据 ===
            env: {
                entries: [],
                rawText: '',
                originalRawText: '',
                filePath: '',
                fileNotExist: false,
                loading: true,
                saving: false,
                editMode: 'table',
                hasChanges: false,
                showSaveTip: false,
                diffModal: { visible: false, lines: [] }
            },
            // === Compose 配置 Tab 数据 ===
            compose: {
                content: '',
                originalContent: '',
                filePath: '',
                fileNotExist: false,
                loading: false,
                loaded: false,
                saving: false,
                hasChanges: false,
                showSaveTip: false,
                diffModal: { visible: false, lines: [] },
                lastSaveResult: { added: [], removed: [] }
            }
        };
    },
    computed: {
        currentFilePath() {
            return this.activeTab === 'env' ? this.env.filePath : this.compose.filePath;
        },
        currentHasChanges() {
            return this.activeTab === 'env' ? this.env.hasChanges : this.compose.hasChanges;
        },
        currentLoading() {
            return this.activeTab === 'env' ? this.env.loading : this.compose.loading;
        },
        currentSaving() {
            return this.activeTab === 'env' ? this.env.saving : this.compose.saving;
        },
        currentFileNotExist() {
            return this.activeTab === 'env' ? this.env.fileNotExist : this.compose.fileNotExist;
        }
    },
    methods: {
        // === Tab 切换 ===
        switchToCompose() {
            this.activeTab = 'compose';
            if (!this.compose.loaded) {
                this.loadCompose();
            }
        },
        refreshCurrentTab() {
            if (this.activeTab === 'env') {
                this.loadEnv();
            } else {
                this.loadCompose();
            }
        },
        saveCurrentTab() {
            if (this.activeTab === 'env') {
                this.showEnvDiffPreview();
            } else {
                this.showComposeDiffPreview();
            }
        },
        downloadCurrentTab() {
            const token = API.getToken() || '';
            let url = '';
            if (this.activeTab === 'env') {
                url = '/api/env/download?token=' + encodeURIComponent(token);
            } else {
                url = '/api/compose-file/download?token=' + encodeURIComponent(token);
            }
            // 使用 a 标签打开避免打断当前页面部分状态
            const a = document.createElement('a');
            a.href = url;
            a.target = '_blank';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
        },

        // === 环境变量方法（保持原有逻辑） ===
        async loadEnv() {
            this.env.loading = true;
            try {
                const data = await API.getEnvFile();
                this.env.entries = (data.entries || []).map(e => {
                    if (e.type === 'variable') {
                        return { ...e, _originalValue: e.value };
                    }
                    return e;
                });
                this.env.rawText = data.raw_text || '';
                this.env.originalRawText = this.env.rawText;
                this.env.filePath = data.file_path || '';
                this.env.fileNotExist = !data.raw_text;
                this.env.hasChanges = false;
            } catch (e) {
                Toast.error(this.$t('env.load_failed') + ': ' + e.message);
            } finally {
                this.env.loading = false;
            }
        },
        markEnvChanged() {
            this.env.hasChanges = true;
        },
        getEnvCurrentContent() {
            if (this.env.editMode === 'raw') {
                return this.env.rawText;
            }
            const lines = [];
            for (const entry of this.env.entries) {
                if (entry.type === 'variable') {
                    lines.push(`${entry.key}=${entry.value}`);
                } else {
                    lines.push(entry.raw || '');
                }
            }
            return lines.join('\n') + '\n';
        },
        showEnvDiffPreview() {
            const newContent = this.getEnvCurrentContent();
            const oldLines = this.env.originalRawText.split('\n').filter(l => l.trim());
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

            this.env.diffModal = { visible: true, lines };
        },
        async saveEnv() {
            this.env.saving = true;
            try {
                if (this.env.editMode === 'table') {
                    const entries = this.env.entries.map(e => {
                        if (e.type === 'variable' && e.value !== e._originalValue) {
                            return { ...e, raw: `${e.key}=${e.value}` };
                        }
                        return e;
                    });
                    await API.saveEnvFile({ entries });
                } else {
                    await API.saveEnvFile({ content: this.env.rawText });
                }
                this.env.diffModal.visible = false;
                this.env.originalRawText = this.getEnvCurrentContent();
                this.env.hasChanges = false;
                this.env.rawText = this.env.originalRawText;
                this.env.fileNotExist = false;
                this.env.showSaveTip = true;
            } catch (e) {
                Toast.error(this.$t('env.save_failed') + ': ' + e.message);
            } finally {
                this.env.saving = false;
            }
        },

        // === Compose 配置方法 ===
        async loadCompose() {
            this.compose.loading = true;
            try {
                const data = await API.getComposeFile();
                this.compose.content = data.content || '';
                this.compose.originalContent = this.compose.content;
                this.compose.filePath = data.file_path || '';
                this.compose.fileNotExist = !data.content;
                this.compose.hasChanges = false;
                this.compose.loaded = true;
            } catch (e) {
                Toast.error(this.$t('compose.load_failed') + ': ' + e.message);
            } finally {
                this.compose.loading = false;
            }
        },
        markComposeChanged() {
            this.compose.hasChanges = true;
        },
        showComposeDiffPreview() {
            const oldLines = this.compose.originalContent.split('\n');
            const newLines = this.compose.content.split('\n');

            // 逐行对比，生成简单 diff
            const lines = [];
            const maxLen = Math.max(oldLines.length, newLines.length);
            for (let i = 0; i < maxLen; i++) {
                const oldLine = i < oldLines.length ? oldLines[i] : undefined;
                const newLine = i < newLines.length ? newLines[i] : undefined;
                if (oldLine === newLine) {
                    continue;
                }
                if (oldLine !== undefined && newLine !== undefined) {
                    lines.push({ type: 'remove', text: oldLine });
                    lines.push({ type: 'add', text: newLine });
                } else if (oldLine !== undefined) {
                    lines.push({ type: 'remove', text: oldLine });
                } else {
                    lines.push({ type: 'add', text: newLine });
                }
            }

            this.compose.diffModal = { visible: true, lines };
        },
        async saveCompose() {
            this.compose.saving = true;
            try {
                const result = await API.saveComposeFile({ content: this.compose.content });
                this.compose.diffModal.visible = false;
                this.compose.originalContent = this.compose.content;
                this.compose.hasChanges = false;
                this.compose.fileNotExist = false;
                this.compose.lastSaveResult = {
                    added: result.services_added || [],
                    removed: result.services_removed || []
                };
                this.compose.showSaveTip = true;
            } catch (e) {
                Toast.error(this.$t('compose.save_failed') + ': ' + e.message);
            } finally {
                this.compose.saving = false;
            }
        },

        // === 通用 ===
        goToServices() {
            this.env.showSaveTip = false;
            this.compose.showSaveTip = false;
            this.$router.push({ name: 'services' });
        }
    },
    watch: {
        'env.editMode'(newMode) {
            if (newMode === 'raw') {
                // 表格 → 原文：同步
                this.env.rawText = this.getEnvCurrentContent();
            } else {
                // 原文 → 表格：重新解析
                const lines = this.env.rawText.split('\n');
                this.env.entries = [];
                let lineNum = 0;
                for (const line of lines) {
                    lineNum++;
                    const trimmed = line.trim();
                    if (!trimmed) {
                        this.env.entries.push({ type: 'blank', raw: line, line: lineNum });
                    } else if (trimmed.startsWith('#')) {
                        this.env.entries.push({ type: 'comment', raw: line, line: lineNum });
                    } else {
                        const idx = trimmed.indexOf('=');
                        if (idx > 0) {
                            this.env.entries.push({
                                type: 'variable',
                                key: trimmed.substring(0, idx).trim(),
                                value: trimmed.substring(idx + 1).trim(),
                                raw: line,
                                line: lineNum
                            });
                        } else {
                            this.env.entries.push({ type: 'comment', raw: line, line: lineNum });
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
