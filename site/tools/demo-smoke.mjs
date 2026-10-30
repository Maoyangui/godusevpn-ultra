// 演示页冒烟:不开浏览器,把 site/dist 的演示、归属元数据和本地资源逐个核一遍。
// 想挡住的是这几种翻车:demo.js 没插进去(页面会去 fetch /api/,线上一片空白)、
// 客户端页面少拷了文件、演示数据里混进了真实地址或密钥、上线后静态资源 404。
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const SITE = path.join(path.dirname(fileURLToPath(import.meta.url)), '..');
const DIST = path.join(SITE, 'dist');
const APP = path.join(DIST, 'demo/app');
const fail = [];
const need = (cond, msg) => { if (!cond) fail.push(msg); };

need(fs.existsSync(DIST), 'site/dist 不存在,先跑 node site/build.mjs');
if (fs.existsSync(DIST)) {
  // 1. 演示注入
  const idx = path.join(APP, 'index.html');
  need(fs.existsSync(idx), 'demo/app/index.html 不在');
  if (fs.existsSync(idx)) {
    const html = fs.readFileSync(idx, 'utf8');
    const i = html.indexOf('<script src="demo.js">'), j = html.indexOf('<script src="api.js">');
    need(i >= 0, 'demo/app/index.html 里没有 demo.js');
    need(j >= 0, 'demo/app/index.html 里没有 api.js');
    need(i >= 0 && j >= 0 && i < j, 'demo.js 必须排在 api.js 前面,否则页面会去连真实后端');
  }
  // 2. 客户端页面该有的文件
  for (const f of ['app.js', 'api.js', 'i18n.js', 'flags.js', 'style.css', 'demo.js']) {
    need(fs.existsSync(path.join(APP, f)), 'demo/app/' + f + ' 少了');
  }
  need(fs.existsSync(path.join(APP, 'flags/hk.svg')), '旗帜没拷进来');
  // 3. 演示数据不能有真东西:示例域名之外的订阅地址、看着像密钥的长串
  const demo = fs.readFileSync(path.join(APP, 'demo.js'), 'utf8');
  const PUBLIC = ['1.1.1.1', '223.5.5.5', '8.8.8.8', 'cloudflare.com', 'www.cloudflare.com', 'ipwho.is', 'github.com'];
  for (const m of demo.matchAll(/https?:\/\/([a-z0-9.-]+)/gi)) {
    const host = m[1].toLowerCase();
    // 除了公用解析器和官网,别的域名一律得是示例域名 —— 免得哪天把真实面板地址写进演示里
    need(/(^|\.)example\.(com|org|net)$/.test(host) || PUBLIC.includes(host), '演示数据里有非示例域名:' + host);
  }
  need(!/[A-Za-z0-9_-]{28,}/.test(demo.replace(/https?:\/\/\S+/g, '')), '演示数据里有看着像密钥的长串');
  // 4. 站点入口
  for (const f of ['index.html', 'docs.html', 'demo/index.html', 'style.css', 'assets/favicon.svg']) {
    need(fs.existsSync(path.join(DIST, f)), f + ' 少了');
  }
  const shots = path.join(DIST, 'assets/shots');
  need(fs.existsSync(shots) && fs.readdirSync(shots).filter(f => f.endsWith('.png')).length >= 4, '截图少于 4 张');

  // 5. 站内引用与样式资源。build.mjs 也会拦 href/src,这里独立重扫产物，
  // 另外覆盖 CSS url()，避免构建脚本漏掉背景图、字体等资源。
  const skipExternal = u => /^(?:https?:|mailto:|data:|javascript:|\/\/|#)/i.test(u);
  const resolveLocal = (file, ref) => {
    const raw = ref.split(/[?#]/, 1)[0];
    if (!raw || skipExternal(raw)) return null;
    const target = raw.startsWith('/') ? path.join(DIST, raw) : path.resolve(path.dirname(file), raw);
    const rel = path.relative(DIST, target);
    if (rel.startsWith('..' + path.sep) || path.isAbsolute(rel)) return { target, outside: true };
    return { target: raw.endsWith('/') ? path.join(target, 'index.html') : target, outside: false };
  };
  const files = [];
  const walk = dir => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const file = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(file); else files.push(file);
    }
  };
  walk(DIST);
  for (const file of files) {
    const text = fs.readFileSync(file, 'utf8');
    if (file.endsWith('.html')) {
      for (const match of text.matchAll(/(?:href|src)="([^"#?]+)"/gi)) {
        const ref = match[1], resolved = resolveLocal(file, ref);
        if (!resolved) continue;
        need(!resolved.outside, path.relative(DIST, file) + ' 引用了产物目录外资源:' + ref);
        if (!resolved.outside) need(fs.existsSync(resolved.target), path.relative(DIST, file) + ' → ' + ref + ' 不存在');
      }
    }
    if (file.endsWith('.css')) {
      for (const match of text.matchAll(/url\(\s*(['"]?)([^'"\)]+)\1\s*\)/gi)) {
        const ref = match[2].trim(), resolved = resolveLocal(file, ref);
        if (!resolved) continue;
        need(!resolved.outside, path.relative(DIST, file) + ' 引用了产物目录外资源:' + ref);
        if (!resolved.outside) need(fs.existsSync(resolved.target), path.relative(DIST, file) + ' → ' + ref + ' 不存在');
      }
    }
  }

  // 6. docs/demo 是基础客户端文档与合成演示，不能再把 SEO 或下载入口指向原仓库，
  // 也不能让读者误以为网页已执行 Ultra 原生隐私/浏览器运行时。
  const docs = fs.readFileSync(path.join(DIST, 'docs.html'), 'utf8');
  const demoPage = fs.readFileSync(path.join(DIST, 'demo/index.html'), 'utf8');
  need(docs.includes('https://maoyangui.github.io/godusevpn-ultra/docs.html'), 'docs canonical 未指向 Ultra Pages');
  need(demoPage.includes('https://maoyangui.github.io/godusevpn-ultra/demo/'), 'demo canonical 未指向 Ultra Pages');
  for (const [name, html] of [['docs.html', docs], ['demo/index.html', demoPage]]) {
    need(!html.includes('github.com/Maoyangui/godusevpn"'), name + ' 仍含原仓库 GitHub 入口');
    need(!html.includes('github.com/Maoyangui/godusevpn/'), name + ' 仍含原仓库 GitHub 子路径入口');
    need(html.includes('基础客户端'), name + ' 未标明基础客户端范围');
    need(html.includes('Ultra 原生'), name + ' 未标明 Ultra 原生运行时边界');
  }
}

if (fail.length) {
  console.error('演示页冒烟没过:\n  ' + fail.join('\n  '));
  process.exit(1);
}
console.log('演示页冒烟通过');
