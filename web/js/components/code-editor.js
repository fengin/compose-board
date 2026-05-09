/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 代码编辑器组件（支持行号和简单语法高亮）
 */
const CodeEditorComponent = {
    template: `
        <div class="code-editor-wrapper" :style="{ minHeight: minHeight }">
            <div class="code-editor-lines" ref="lines">
                <div v-for="n in lineCount" :key="n">{{ n }}</div>
            </div>
            <div class="code-editor-content">
                <pre class="code-editor-bg" aria-hidden="true" v-html="highlightedContent" ref="bg"></pre>
                <textarea 
                    class="code-editor-textarea" 
                    v-model="internalValue" 
                    @input="onInput"
                    @scroll="onScroll"
                    @keydown="onKeydown"
                    spellcheck="false"
                ></textarea>
            </div>
        </div>
    `,
    props: {
        modelValue: { type: String, default: '' },
        minHeight: { type: String, default: '500px' }
    },
    data() {
        return {
            internalValue: this.modelValue
        }
    },
    computed: {
        lineCount() {
            return (this.internalValue.match(/\n/g) || []).length + 1;
        },
        highlightedContent() {
            // 转义 HTML 并高亮注释
            let escaped = this.internalValue
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;');
            
            // Docker Compose 和 .env 都使用 # 作为注释
            const lines = escaped.split('\n');
            const highlightedLines = lines.map(line => {
                const commentIdx = line.indexOf('#');
                if (commentIdx !== -1) {
                    const before = line.substring(0, commentIdx);
                    // 仅当 # 为该行第一个非空字符，或者前面是空格时，才视为注释
                    if (before.trim() === '' || before.endsWith(' ')) {
                        const comment = line.substring(commentIdx);
                        return before + '<span style="color:#10B981;">' + comment + '</span>';
                    }
                }
                return line;
            });
            let result = highlightedLines.join('\n');
            // 如果最后一行以换行符结束，确保 <pre> 能正确渲染最后一行
            if (result.endsWith('\n')) {
                result += ' ';
            }
            return result;
        }
    },
    watch: {
        modelValue(newVal) {
            this.internalValue = newVal;
        }
    },
    methods: {
        onInput(e) {
            this.internalValue = e.target.value;
            this.$emit('update:modelValue', this.internalValue);
        },
        onScroll(e) {
            // 同步 textarea 与行号、高亮背景的滚动
            const top = e.target.scrollTop;
            const left = e.target.scrollLeft;
            if (this.$refs.lines) {
                this.$refs.lines.scrollTop = top;
            }
            if (this.$refs.bg) {
                this.$refs.bg.scrollTop = top;
                this.$refs.bg.scrollLeft = left;
            }
        },
        onKeydown(e) {
            // 支持 Tab 缩进
            if (e.key === 'Tab') {
                e.preventDefault();
                const start = e.target.selectionStart;
                const end = e.target.selectionEnd;
                this.internalValue = this.internalValue.substring(0, start) + '  ' + this.internalValue.substring(end);
                this.$emit('update:modelValue', this.internalValue);
                this.$nextTick(() => {
                    e.target.selectionStart = e.target.selectionEnd = start + 2;
                });
            }
        }
    }
};
