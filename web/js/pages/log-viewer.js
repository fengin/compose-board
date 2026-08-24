/**
 * 日志查看入口包装器。
 * 文件日志未启用时只渲染原控制台页面；启用后由子面板在工具栏首项切换来源。
 */
const LogViewerPage = {
    template: `
    <div class="log-viewer-page">
        <console-log-panel
            v-if="sourceType === 'console'"
            :file-logs-enabled="fileLogsEnabled"
            source-type="console"
            @change-source="changeSource"
        ></console-log-panel>
        <file-log-panel
            v-else
            :initial-service="$route.query.service || ''"
            :bases="bases"
            source-type="file"
            @change-source="changeSource"
        ></file-log-panel>
    </div>
    `,
    data() {
        return {
            fileLogsEnabled: false,
            sourceType: 'console',
            bases: []
        };
    },
    methods: {
        changeSource(source) {
            this.sourceType = source === 'file' && this.fileLogsEnabled ? 'file' : 'console';
        }
    },
    async mounted() {
        try {
            const response = await API.getFileLogBases();
            this.bases = response.bases || [];
            this.fileLogsEnabled = !!response.enabled && this.bases.length > 0;
        } catch (error) {
            this.fileLogsEnabled = false;
            this.bases = [];
        }
    }
};
