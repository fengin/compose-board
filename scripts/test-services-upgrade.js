const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const rulesPath = path.join(__dirname, '..', 'web', 'js', 'pages', 'services-rules.js');
const modalPath = path.join(__dirname, '..', 'web', 'js', 'components', 'upgrade-modal.js');
const tablePath = path.join(__dirname, '..', 'web', 'js', 'components', 'service-table.js');
const source = [
    fs.readFileSync(rulesPath, 'utf8'),
    fs.readFileSync(modalPath, 'utf8'),
    fs.readFileSync(tablePath, 'utf8'),
    'globalThis.__servicesRules = ServicesRules;',
    'globalThis.__upgradeModal = UpgradeModal;',
    'globalThis.__serviceTable = ServiceTable;'
].join('\n');
const sandbox = { globalThis: {}, console, setTimeout: () => {} };
vm.createContext(sandbox);
vm.runInContext(source, sandbox, { filename: 'services-upgrade.js' });

const rules = sandbox.globalThis.__servicesRules;
const modal = sandbox.globalThis.__upgradeModal;
const table = sandbox.globalThis.__serviceTable;
const tests = [];

function test(name, fn) {
    tests.push({ name, fn });
}

function registryService(overrides = {}) {
    return {
        name: 'api',
        status: 'running',
        image_source: 'registry',
        image_diff: false,
        running_image: 'registry.local/api:1.0.0',
        declared_image: 'registry.local/api:1.0.0',
        pending_env: [],
        config_diff: false,
        ...overrides
    };
}

function translate(key) {
    const values = {
        'services.modal.no_version_change': '无变更',
        'services.modal.pull_hint': '拉取目标版本',
        'services.modal.repull_hint': '重新拉取当前版本'
    };
    return values[key] || key;
}

function modalContext(service) {
    return {
        service,
        hasImageDiff: !!service.image_diff,
        $t: translate
    };
}

test('已部署 registry 服务在镜像版本无变化时仍显示升级按钮', () => {
    const actions = rules.buildServiceActions(registryService());
    assert.equal(actions.includes('upgrade'), true);
});

test('已停止的 registry 服务也显示升级按钮', () => {
    const actions = rules.buildServiceActions(registryService({ status: 'exited' }));
    assert.equal(actions.includes('upgrade'), true);
});

test('build 服务不显示升级按钮', () => {
    const actions = rules.buildServiceActions(registryService({
        image_source: 'build',
        image_diff: true
    }));
    assert.equal(actions.includes('upgrade'), false);
});

test('未部署 registry 服务不显示升级按钮', () => {
    const actions = rules.buildServiceActions(registryService({ status: 'not_deployed' }));
    assert.equal(actions.includes('upgrade'), false);
});

test('镜像版本变化信息继续显示在服务列表', () => {
    const row = rules.buildServiceRow(registryService({
        image_diff: true,
        declared_image: 'registry.local/api:2.0.0'
    }));
    assert.equal(row.display.currentVersion, '1.0.0');
    assert.equal(row.display.nextVersion, '2.0.0');
    assert.equal(row.display.hasImageDiff, true);
    assert.equal(table.methods.actionClass('upgrade', row), 'act-upgrade');
});

test('镜像版本无变化时升级按钮使用普通操作样式且图标不变', () => {
    const row = rules.buildServiceRow(registryService());
    assert.equal(row.display.hasImageDiff, false);
    assert.equal(table.methods.actionClass('upgrade', row), 'act-default');
    assert.equal(table.methods.actionClass('show-env', row), 'act-default');
    assert.equal(table.methods.actionIcon('upgrade'), '⬆');
});

test('镜像版本无变化时弹窗目标版本显示无变更并提示重新拉取', () => {
    const context = modalContext(registryService());
    assert.equal(modal.computed.targetVer.call(context), '无变更');
    assert.equal(modal.computed.pullHint.call(context), '重新拉取当前版本');
});

test('镜像版本变化时弹窗显示新版本并提示拉取目标版本', () => {
    const service = registryService({
        image_diff: true,
        declared_image: 'registry.local/api:2.0.0'
    });
    const context = modalContext(service);
    assert.equal(modal.computed.targetVer.call(context), '2.0.0');
    assert.equal(modal.computed.pullHint.call(context), '拉取目标版本');
});

test('同标签强制重建后可按容器变化结束升级状态', () => {
    const result = rules.evaluateServiceOperation('upgrade', registryService({
        container_id: 'new-container',
        started_at: '2026-08-12T10:00:00Z'
    }), {
        containerIdBefore: 'old-container',
        startedBefore: '2026-08-12T09:00:00Z'
    });
    assert.deepEqual(JSON.parse(JSON.stringify(result)), { done: true, failedSample: false });
});

async function run() {
    for (const { name, fn } of tests) {
        await fn();
        console.log(`通过: ${name}`);
    }
    console.log('服务升级行为测试全部通过');
}

run().catch(error => {
    console.error(error);
    process.exitCode = 1;
});
