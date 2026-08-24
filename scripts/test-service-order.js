const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const sourcePath = path.join(__dirname, '..', 'web', 'js', 'service-order.js');
const source = fs.readFileSync(sourcePath, 'utf8') + '\nglobalThis.__serviceOrder = ServiceOrder;';
const sandbox = { globalThis: {} };
vm.createContext(sandbox);
vm.runInContext(source, sandbox, { filename: 'service-order.js' });
const serviceOrder = sandbox.globalThis.__serviceOrder;

const original = [
    { name: 'unlabeled-first', category: '' },
    { name: 'redis', category: 'base' },
    { name: 'api', category: 'backend' },
    { name: 'web', category: 'frontend' },
    { name: 'initializer', category: 'init' },
    { name: 'job', category: 'backend' },
    { name: 'mysql', category: 'middleware' },
    { name: 'explicit-other', category: 'other' },
    { name: 'unlabeled-last' }
];

const sorted = serviceOrder.sort(original);
assert.deepEqual(
    Array.from(sorted, service => service.name),
    ['api', 'job', 'redis', 'mysql', 'web', 'unlabeled-first', 'initializer', 'explicit-other', 'unlabeled-last']
);
assert.deepEqual(Array.from(original, service => service.name), [
    'unlabeled-first', 'redis', 'api', 'web', 'initializer', 'job', 'mysql', 'explicit-other', 'unlabeled-last'
]);

const defaultOnly = [
    { name: 'first' },
    { name: 'second', category: 'init' },
    { name: 'third', category: 'other' }
];
assert.deepEqual(
    Array.from(serviceOrder.sort(defaultOnly), service => service.name),
    ['first', 'second', 'third']
);

console.log('服务下拉分类顺序测试全部通过');