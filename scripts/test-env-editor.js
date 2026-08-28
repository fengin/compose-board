const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const pagePath = path.join(__dirname, '..', 'web', 'js', 'pages', 'env.js');
const lineDiffPath = path.join(__dirname, '..', 'web', 'js', 'line-diff.js');
const source = `${fs.readFileSync(lineDiffPath, 'utf8')}\nglobalThis.__lineDiffUtils = LineDiffUtils;\n${fs.readFileSync(pagePath, 'utf8')}\nglobalThis.__envEditorUtils = EnvEditorUtils;\nglobalThis.__envPage = EnvPage;`;
const sandbox = {
    globalThis: {},
    console,
    API: { saveEnvFile: async () => {} },
    Toast: { error: () => {} }
};
vm.createContext(sandbox);
vm.runInContext(source, sandbox, { filename: pagePath });

const envEditor = sandbox.globalThis.__envEditorUtils;
const envPage = sandbox.globalThis.__envPage;
const lineDiff = sandbox.globalThis.__lineDiffUtils;

function findVariable(entries, key) {
    const entry = entries.find(item => item.type === 'variable' && item.key === key);
    assert.ok(entry, `未找到变量 ${key}`);
    return entry;
}

function normalized(value) {
    return JSON.parse(JSON.stringify(value));
}

function assertDiff(oldContent, newContent, expected) {
    assert.deepEqual(normalized(envEditor.buildDiff(oldContent, newContent)), expected);
}

function assertLineDiff(oldContent, newContent, expected) {
    assert.deepEqual(normalized(lineDiff.buildChanges(oldContent, newContent)), expected);
}

function createPageContext(rawText, entries = envEditor.parseEntries(rawText)) {
    const context = {
        env: {
            editMode: 'table',
            entries,
            rawText,
            originalRawText: rawText,
            diffModal: { visible: false, lines: [] },
            hasChanges: true,
            saving: false,
            fileNotExist: false,
            showSaveTip: false
        }
    };
    context.getEnvCurrentContent = () => envPage.methods.getEnvCurrentContent.call(context);
    return context;
}

const tests = [];

function test(name, fn) {
    tests.push({ name, fn });
}

test('表格编辑不会误报未编辑的双引号变量', () => {
    const original = '# config\nQUOTED="keep value"\nTARGET=before\n';
    const entries = envEditor.withBaselines([
        { type: 'comment', raw: '# config', line: 1 },
        { type: 'variable', key: 'QUOTED', value: 'keep value', raw: 'QUOTED="keep value"', line: 2 },
        { type: 'variable', key: 'TARGET', value: 'before', raw: 'TARGET=before', line: 3 }
    ]);
    findVariable(entries, 'TARGET').value = 'after';

    const current = envEditor.buildContent(entries);
    assert.equal(current, '# config\nQUOTED="keep value"\nTARGET=after\n');
    assertDiff(original, current, [
        { type: 'remove', text: 'TARGET=before' },
        { type: 'add', text: 'TARGET=after' }
    ]);
});

test('表格多行编辑仅重建被编辑的行并保留单双引号', () => {
    const original = 'DOUBLE="keep value"\nSINGLE=\'keep value\'\nFIRST=one\nSECOND=two\n';
    const entries = envEditor.parseEntries(original);
    findVariable(entries, 'SINGLE').value = 'updated value';
    findVariable(entries, 'SECOND').value = 'changed';

    const current = envEditor.buildContent(entries);
    assert.equal(current, 'DOUBLE="keep value"\nSINGLE=\'updated value\'\nFIRST=one\nSECOND=changed\n');
    assertDiff(original, current, [
        { type: 'remove', text: "SINGLE='keep value'" },
        { type: 'remove', text: 'SECOND=two' },
        { type: 'add', text: "SINGLE='updated value'" },
        { type: 'add', text: 'SECOND=changed' }
    ]);
});

test('文本模式切换到表格模式不会规范化未编辑行', () => {
    const rawText = 'TEXT_EDITED="from text mode"\nUNCHANGED=keep\n';
    const entries = envEditor.parseEntries(rawText);

    assert.equal(envEditor.buildContent(entries), rawText);
    assert.equal(findVariable(entries, 'TEXT_EDITED').value, 'from text mode');
    assert.equal(findVariable(entries, 'TEXT_EDITED')._originalValue, 'from text mode');
});

test('文本修改后继续表格编辑会保留文本修改和原始格式', () => {
    const pageOriginal = 'TEXT_EDITED="before text mode"\nTABLE_TARGET=before\n';
    const rawText = 'TEXT_EDITED="from text mode"\nTABLE_TARGET=before\n';
    const entries = envEditor.parseEntries(rawText);
    findVariable(entries, 'TABLE_TARGET').value = 'after';

    const current = envEditor.buildContent(entries);
    assert.equal(current, 'TEXT_EDITED="from text mode"\nTABLE_TARGET=after\n');
    assertDiff(pageOriginal, current, [
        { type: 'remove', text: 'TEXT_EDITED="before text mode"' },
        { type: 'remove', text: 'TABLE_TARGET=before' },
        { type: 'add', text: 'TEXT_EDITED="from text mode"' },
        { type: 'add', text: 'TABLE_TARGET=after' }
    ]);
});

test('表格编辑后恢复原值时继续复用原始行', () => {
    const original = 'JVM_OPTS="-Xms512m -Xmx1024m"\nTARGET=before\n';
    const entries = envEditor.parseEntries(original);
    const jvm = findVariable(entries, 'JVM_OPTS');
    jvm.value = '-Xms768m -Xmx1024m';
    jvm.value = '-Xms512m -Xmx1024m';

    assert.equal(envEditor.buildContent(entries), original);
    assertDiff(original, envEditor.buildContent(entries), []);
});

test('页面差异弹窗只展示表格模式的真实修改', () => {
    const original = 'QUOTED="keep value"\nTARGET=before\n';
    const context = createPageContext(original);
    findVariable(context.env.entries, 'TARGET').value = 'after';

    envPage.methods.showEnvDiffPreview.call(context);
    assert.deepEqual(normalized(context.env.diffModal), {
        visible: true,
        lines: [
            { type: 'remove', text: 'TARGET=before' },
            { type: 'add', text: 'TARGET=after' }
        ]
    });
});

test('页面监听器从文本模式切表格模式会建立当前文本基线', () => {
    const rawText = 'QUOTED="from text mode"\nTARGET=before\n';
    const context = createPageContext('');
    context.env.editMode = 'raw';
    context.env.rawText = rawText;

    envPage.watch['env.editMode'].call(context, 'table');

    const quoted = findVariable(context.env.entries, 'QUOTED');
    assert.equal(quoted.value, 'from text mode');
    assert.equal(quoted._originalValue, 'from text mode');
    assert.equal(context.getEnvCurrentContent(), rawText);
});

test('Compose 中间新增多行时只展示新增内容', () => {
    const original = 'services:\n  kafka:\n    environment:\n      # 监听器配置\n      - KAFKA_CFG_LISTENERS=INTERNAL://:9092\n';
    const current = 'services:\n  kafka:\n    environment:\n      # KRaft Broker 心跳配置\n      - KAFKA_CFG_BROKER_HEARTBEAT_INTERVAL_MS=2000\n      - KAFKA_CFG_BROKER_SESSION_TIMEOUT_MS=30000\n      # 监听器配置\n      - KAFKA_CFG_LISTENERS=INTERNAL://:9092\n';

    assertLineDiff(original, current, [
        { type: 'add', text: '      # KRaft Broker 心跳配置' },
        { type: 'add', text: '      - KAFKA_CFG_BROKER_HEARTBEAT_INTERVAL_MS=2000' },
        { type: 'add', text: '      - KAFKA_CFG_BROKER_SESSION_TIMEOUT_MS=30000' }
    ]);
});

test('Compose 中间删除和替换时保持未修改行对齐', () => {
    assertLineDiff('first\nremove\nchange-before\nlast\n', 'first\nchange-after\nlast\n', [
        { type: 'remove', text: 'remove' },
        { type: 'remove', text: 'change-before' },
        { type: 'add', text: 'change-after' }
    ]);
});

test('Compose 重复行差异能够定位到真实新增行', () => {
    assertLineDiff('services:\n  - SAME\n  - SAME\nend\n', 'services:\n  - SAME\n  - NEW\n  - SAME\nend\n', [
        { type: 'add', text: '  - NEW' }
    ]);
});

test('Compose 的 CRLF 与 LF 换行差异不会误报整文件修改', () => {
    assertLineDiff('services:\r\n  app:\r\n    image: demo\r\n', 'services:\n  app:\n    image: demo\n', []);
});

test('Compose 页面差异弹窗使用行级序列对齐结果', () => {
    const context = {
        compose: {
            originalContent: 'before\nanchor\nafter\n',
            content: 'before\ninsert-one\ninsert-two\nanchor\nafter\n',
            diffModal: { visible: false, lines: [] }
        }
    };

    envPage.methods.showComposeDiffPreview.call(context);

    assert.deepEqual(normalized(context.compose.diffModal), {
        visible: true,
        lines: [
            { type: 'add', text: 'insert-one' },
            { type: 'add', text: 'insert-two' }
        ]
    });
});

test('页面表格保存仅提交实际重建行并同步后续编辑基线', async () => {
    const original = 'QUOTED="keep value"\nTARGET=before\n';
    const context = createPageContext(original);
    findVariable(context.env.entries, 'TARGET').value = 'after';
    const requests = [];
    sandbox.API.saveEnvFile = async request => requests.push(normalized(request));

    await envPage.methods.saveEnv.call(context);

    assert.equal(requests.length, 1);
    const submittedQuoted = findVariable(requests[0].entries, 'QUOTED');
    const submittedTarget = findVariable(requests[0].entries, 'TARGET');
    assert.equal(submittedQuoted.raw, 'QUOTED="keep value"');
    assert.equal(submittedTarget.raw, 'TARGET=after');
    assert.equal(context.env.originalRawText, 'QUOTED="keep value"\nTARGET=after\n');
    assert.equal(findVariable(context.env.entries, 'TARGET')._originalValue, 'after');
    assert.equal(context.env.hasChanges, false);
});

async function run() {
    for (const { name, fn } of tests) {
        try {
            await fn();
            console.log(`通过: ${name}`);
        } catch (error) {
            console.error(`失败: ${name}`);
            throw error;
        }
    }
    console.log('环境变量编辑器行为测试全部通过');
}

run().catch(error => {
    console.error(error);
    process.exitCode = 1;
});
