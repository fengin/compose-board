/**
 * ComposeBoard - Docker Compose 可视化管理面板
 * 作者：凌封
 * 网址：https://fengin.cn
 *
 * i18n key 一致性校验脚本
 * 用法: node scripts/check-i18n-keys.js
 */

const fs = require('fs');
const path = require('path');

const LOCALES_DIR = path.join(__dirname, '..', 'web', 'js', 'locales');
const ZH_FILE = path.join(LOCALES_DIR, 'zh.json');
const EN_FILE = path.join(LOCALES_DIR, 'en.json');

/**
 * 递归展平 JSON 对象为点分路径 key 列表
 * @param {Object} obj
 * @param {string} prefix
 * @returns {string[]}
 */
function flattenKeys(obj, prefix = '') {
    const keys = [];
    for (const key of Object.keys(obj)) {
        const fullKey = prefix ? `${prefix}.${key}` : key;
        if (typeof obj[key] === 'object' && obj[key] !== null && !Array.isArray(obj[key])) {
            keys.push(...flattenKeys(obj[key], fullKey));
        } else {
            keys.push(fullKey);
        }
    }
    return keys;
}

// 读取 locale 文件
let zh, en;
try {
    zh = JSON.parse(fs.readFileSync(ZH_FILE, 'utf-8'));
} catch (err) {
    console.error(`❌ 无法读取 ${ZH_FILE}: ${err.message}`);
    process.exit(1);
}
try {
    en = JSON.parse(fs.readFileSync(EN_FILE, 'utf-8'));
} catch (err) {
    console.error(`❌ 无法读取 ${EN_FILE}: ${err.message}`);
    process.exit(1);
}

const zhKeys = new Set(flattenKeys(zh));
const enKeys = new Set(flattenKeys(en));

// 计算对称差集
const missingInEn = [...zhKeys].filter(k => !enKeys.has(k));
const missingInZh = [...enKeys].filter(k => !zhKeys.has(k));

let hasError = false;

if (missingInEn.length > 0) {
    console.error(`\n❌ 英文缺失 ${missingInEn.length} 个 key (zh.json 有, en.json 没有):`);
    missingInEn.forEach(k => console.error(`  - ${k}`));
    hasError = true;
}

if (missingInZh.length > 0) {
    console.error(`\n❌ 中文缺失 ${missingInZh.length} 个 key (en.json 有, zh.json 没有):`);
    missingInZh.forEach(k => console.error(`  - ${k}`));
    hasError = true;
}

if (hasError) {
    console.error(`\n💡 请确保 zh.json 和 en.json 的 key 完全一致`);
    process.exit(1);
} else {
    console.log(`✅ i18n key 一致性检查通过 (共 ${zhKeys.size} 个 key)`);
    process.exit(0);
}
