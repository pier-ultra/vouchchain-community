// 构建后复制 Scalar 浏览器 standalone bundle 到 dist/vendor/，
// 使 /scalar 页面（Go 内嵌 ScalarHTML）引用本地 JS，不依赖被墙的 jsdelivr CDN。
// 用法：npm run build 时由 package.json "build" 脚本自动调用；
//      也可单独运行：node scripts/copy-scalar.mjs
import { mkdirSync, copyFileSync, existsSync, statSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = join(__dirname, '..');

const src = join(root, 'node_modules/@scalar/api-reference/dist/browser/standalone.js');
const outDir = join(root, 'dist/vendor');
const out = join(outDir, 'scalar-standalone.js');

if (!existsSync(src)) {
  console.error('[copy-scalar] 源不存在，请先 npm install：', src);
  process.exit(1);
}
mkdirSync(outDir, { recursive: true });
copyFileSync(src, out);
console.log(`[copy-scalar] -> ${out} (${(statSync(out).size / 1024 / 1024).toFixed(1)} MB)`);
