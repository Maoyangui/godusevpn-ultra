// DOM-only extension pages. The runtime API is a fixed in-memory mock.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { JSDOM } = require(process.argv[2] || 'jsdom');
const source = file => fs.readFileSync(path.join(__dirname, file), 'utf8');
const strip = html => html.replace(/<script[\s\S]*?<\/script>/g, '');
const settle = () => new Promise(resolve => setTimeout(resolve, 15));
async function run() {
  const dom = new JSDOM(strip(source('panel.html')), { runScripts: 'outside-only', url: 'https://extension.invalid/', pretendToBeVisual: true });
  const w = dom.window, context = dom.getInternalVMContext(), calls = [];
  let failed = false, changed;
  w.chrome = { runtime: { sendMessage(message, callback) {
    calls.push(message);
    queueMicrotask(() => {
      if (failed) return callback({ ok: false, error: 'synthetic read failure' });
      if (message.action === 'report') return callback({ ok: true, result: w.UltraPrivacy.catalog.map(item => Object.assign({}, item, { supported: true, writable: true, value: item.desired, managed: false, checkedAt: 'fixture', sample: item.type === 'content' })) });
      callback({ ok: true, result: { sample: true } });
    });
  } }, storage: { onChanged: { addListener(fn) { changed = fn; } } } };
  for (const file of ['core.js', 'clean-link.js', 'panel.js']) vm.runInContext(source(file), context, { filename: file });
  await settle();
  function coverage() {
    for (const element of w.document.querySelectorAll('button:not(.help),input,textarea,select,summary')) assert.ok(element.nextElementSibling && element.nextElementSibling.classList.contains('help'), 'missing help: ' + (element.id || element.textContent));
    for (const button of w.document.querySelectorAll('.help')) assert.notEqual(button.getAttribute('aria-label'), '说明');
  }
  coverage();
  const js = w.document.querySelector('[data-mode="content-javascript"]');
  assert.deepEqual(Array.from(js.options).map(option => option.value), ['allow', 'block']);
  const camera = w.document.querySelector('[data-mode="content-camera"]');
  assert.deepEqual(Array.from(camera.options).map(option => option.value), ['allow', 'ask', 'block']);
  camera.value = 'allow'; camera.dispatchEvent(new w.Event('change'));
  let origin = w.document.querySelector('[data-origin="content-camera"]');
  assert.equal(w.document.querySelector('[data-request="content-camera"]').disabled, true);
  origin.value = 'http://fixture.invalid'; origin.dispatchEvent(new w.Event('input'));
  assert.equal(w.document.querySelector('[data-request="content-camera"]').disabled, true);
  origin.focus(); origin.value = 'https://fixture.invalid'; origin.dispatchEvent(new w.Event('input'));
  assert.equal(w.document.querySelector('[data-request="content-camera"]').disabled, false, 'a valid origin enables review immediately');
  assert.equal(w.document.activeElement, origin, 'typing must preserve the input and focus');
  w.document.querySelector('[data-request="content-camera"]').click();
  assert.ok(w.document.querySelector('[data-confirm="content-camera"]'));
  origin = w.document.querySelector('[data-origin="content-camera"]');
  origin.value = 'https://fixture.invalid/path'; origin.dispatchEvent(new w.Event('input'));
  assert.equal(w.document.querySelector('[data-confirm="content-camera"]'), null, 'changing origin cancels the prior confirmation');
  assert.equal(w.document.querySelector('[data-request="content-camera"]').disabled, true);
  origin.value = 'https://fixture.invalid'; origin.dispatchEvent(new w.Event('input'));
  w.document.querySelector('[data-request="content-camera"]').click();
  assert.equal(calls.filter(call => call.action === 'apply').length, 0);
  coverage();
  w.document.querySelector('[data-tab="links"]').click();
  assert.equal(w.document.querySelector('[data-confirm]'), null, 'tab change cancels pending operation');
  w.document.querySelector('[data-tab="settings"]').click();
  w.document.querySelector('[data-request="content-camera"]').click();
  w.document.querySelector('[data-confirm="content-camera"]').click(); await settle();
  const apply = calls.find(call => call.action === 'apply');
  assert.equal(apply.options.value, 'allow'); assert.equal(apply.options.origin, 'https://fixture.invalid');
  changed({ managed: {} }, 'local');
  assert.match(w.document.getElementById('controls').textContent, /历史结果，当前未确认/);
  assert.ok(w.document.querySelector('[data-request="prediction"]').disabled);
  failed = true; w.document.getElementById('refresh').click(); await settle();
  assert.match(w.document.getElementById('message').textContent, /历史结果已失效/);
  failed = false; w.document.getElementById('refresh').click(); await settle();
  assert.equal(w.document.querySelector('[data-request="prediction"]').disabled, false);
  coverage(); dom.window.close();
  const launcher = new JSDOM(strip(source('launcher.html')), { runScripts:'outside-only',url:'https://extension.invalid/' });
  let opens = 0; launcher.window.chrome = { runtime: { openOptionsPage() { opens++; } } };
  vm.runInContext(source('launcher.js'), launcher.getInternalVMContext());
  launcher.window.document.getElementById('open-help').click();
  assert.equal(launcher.window.document.getElementById('launcher-help').hidden, false);
  launcher.window.document.dispatchEvent(new launcher.window.KeyboardEvent('keydown', { key:'Escape' }));
  assert.equal(launcher.window.document.getElementById('launcher-help').hidden, true);
  assert.equal(opens, 0); launcher.window.document.getElementById('open-panel').click(); assert.equal(opens, 1);
  launcher.window.close();
  console.log('Extension/launcher DOM: per-API choices, confirmation cancellation, shared-change staleness and help coverage passed (mock only).');
}
run().catch(error => { console.error(error); process.exitCode = 1; });
