// Actual UI sources with a fixed DOM/Wails mock only. No native window,
// product process, service, browser engine, clipboard, or network is used.
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { JSDOM } = require(process.argv[2] || 'jsdom');
const root = path.resolve(__dirname, '../../..');
const source = name => fs.readFileSync(path.join(root, 'web/dist', name), 'utf8');
const html = source('index.html').replace(/<script[\s\S]*?<\/script>/g, '');
const sources = ['i18n.js', 'flags.js', 'privacy.js', 'app.js'].map(name => [name, source(name)]);
const pause = (ms = 0) => new Promise(resolve => setTimeout(resolve, ms));
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
};
const unhandled = [];
const onUnhandled = error => unhandled.push(String(error));
process.on('unhandledRejection', onUnhandled);

async function until(predicate, message) {
  const end = Date.now() + 2000;
  while (!predicate() && Date.now() < end) await pause(5);
  assert.ok(predicate(), message);
}

async function fixture(options = {}) {
  const dom = new JSDOM(html, { runScripts: 'outside-only', url: 'https://window-fixture.invalid/', pretendToBeVisual: true });
  const w = dom.window, calls = [], errors = [], handlers = {};
  const state = {
    platform: options.platform || 'windows', service: true, svcState: 'running',
    version: 'fixed-window-fixture', lang: options.lang || 'zh', theme: 'light',
    view: { profiles: [], nodes: [], settings: {}, state: { status: 'disconnected', wanted: false } },
  };
  let read = options.read || (() => options.maximized || false);
  let toggle = options.toggle || (() => true);
  const methods = {
    IsMaximized: (...args) => { calls.push(['IsMaximized', ...args]); return read(); },
    ToggleMaximize: (...args) => { calls.push(['ToggleMaximize', ...args]); return toggle(); },
  };
  const api = options.inherited ? Object.create(methods) : {};
  api.GetState = async () => state;
  if (!options.inherited) {
    if (options.readMethod !== false) api.IsMaximized = methods.IsMaximized;
    if (options.toggleMethod !== false) api.ToggleMaximize = methods.ToggleMaximize;
  }
  w.__web = !!options.web;
  w.__android = !!options.android;
  w.go = { main: { App: new Proxy(api, { get: (object, key) => object[key] || (() => { calls.push([String(key)]); return Promise.resolve(); }) }) } };
  w.runtime = { EventsOn: (name, handler) => { handlers[name] = handler; } };
  w.HTMLElement.prototype.scrollIntoView = function() {};
  const forbidden = () => { throw Error('network/native operation forbidden in window fixture'); };
  w.fetch = forbidden;
  w.XMLHttpRequest = forbidden;
  w.WebSocket = forbidden;
  Object.defineProperty(w.navigator, 'clipboard', { value: { writeText: forbidden, readText: forbidden } });
  w.addEventListener('error', event => errors.push(String(event.error || event.message)));
  const context = dom.getInternalVMContext();
  for (const [name, contents] of sources) vm.runInContext(contents, context, { filename: name });
  try {
    await until(() => !!handlers.state, 'actual application DOM initialization completes');
    await pause();
    const q = selector => w.document.querySelector(selector);
    return {
      w, calls, errors, q, button: q('#max-btn'),
      setRead: fn => { read = fn; }, setToggle: fn => { toggle = fn; },
      count: method => calls.filter(call => call[0] === method).length,
      resize: () => w.dispatchEvent(new w.Event('resize')),
      emitLanguage: lang => handlers.state({ ...state, lang }),
      close: () => {
        w.close();
        assert.deepEqual(errors, [], 'mock DOM must not raise window errors');
        assert.deepEqual(calls.filter(call => ['Connect', 'Disconnect', 'RepairService', 'RegisterController', 'SaveSettings', 'QuitApp', 'Minimize', 'HideWindow'].includes(call[0])), [], 'window-state fixture must not mutate product or lifecycle state');
      },
    };
  } catch (error) { w.close(); throw error; }
}

function label(f, maximized, lang = 'zh') {
  const expected = lang === 'en' ? (maximized ? 'Restore window' : 'Maximize') : (maximized ? '还原窗口' : '最大化');
  assert.equal(f.button.hidden, false);
  assert.equal(f.w.document.body.classList.contains('has-maximize'), true);
  assert.equal(f.button.title, expected);
  assert.equal(f.button.getAttribute('aria-label'), expected);
  assert.equal(f.button.getAttribute('aria-pressed'), String(maximized));
  assert.equal(f.button.querySelector('path').getAttribute('d'), maximized ? 'M8 8H5v11h11v-3M8 5h11v11H8z' : 'M6 6h12v12H6z');
}

async function platformGate() {
  const cases = [
    ['Windows desktop', {}, true],
    ['web panel even with Windows state', { web: true }, false],
    ['Android shell even with Windows state', { android: true }, false],
    ['Android platform', { platform: 'android' }, false],
    ['macOS', { platform: 'darwin' }, false],
    ['Linux', { platform: 'linux' }, false],
    ['legacy bridge', { readMethod: false, toggleMethod: false }, false],
    ['missing read method', { readMethod: false }, false],
    ['missing toggle method', { toggleMethod: false }, false],
    ['prototype methods are not an own Wails bridge', { inherited: true }, false],
  ];
  for (const [name, options, visible] of cases) {
    const f = await fixture(options);
    try {
      assert.equal(f.button.hidden, !visible, name);
      assert.equal(f.w.document.body.classList.contains('has-maximize'), visible, name + ' layout class');
      assert.equal(f.count('IsMaximized'), visible ? 1 : 0, name + ' startup read gate');
      assert.equal(f.count('ToggleMaximize'), 0, name + ' must not toggle at startup');
      if (!visible) {
        // Even a synthetic click on a hidden element has no native listener.
        f.button.dispatchEvent(new f.w.MouseEvent('click', { bubbles: true }));
        f.resize(); await pause();
        assert.equal(f.count('IsMaximized'), 0, name + ' resize stays inert');
        assert.equal(f.count('ToggleMaximize'), 0, name + ' hidden click stays inert');
      }
    } finally { f.close(); }
  }
}

async function initialAndLanguage() {
  for (const maximized of [false, true]) {
    const f = await fixture({ maximized });
    try {
      label(f, maximized);
      const reads = f.count('IsMaximized');
      f.emitLanguage('en'); label(f, maximized, 'en');
      f.emitLanguage('zh'); label(f, maximized);
      assert.equal(f.count('IsMaximized'), reads, 'translation uses last confirmed state without another native read');
      assert.equal(f.count('ToggleMaximize'), 0, 'state language events never toggle the window');
    } finally { f.close(); }
  }
}

async function toggleAndDuplicate() {
  const action = deferred();
  const f = await fixture({ toggle: () => action.promise });
  try {
    label(f, false);
    f.button.click();
    assert.equal(f.button.disabled, true);
    f.button.click();
    f.button.dispatchEvent(new f.w.MouseEvent('click', { bubbles: true }));
    assert.equal(f.count('ToggleMaximize'), 1, 'disabled and synthetic duplicate clicks cannot start a second toggle');
    label(f, false);
    f.setRead(() => true);
    action.resolve(true);
    await until(() => !f.button.disabled && f.count('IsMaximized') === 2, 'toggle completes and confirms actual window state');
    label(f, true);
    assert.deepEqual(f.calls.filter(call => ['ToggleMaximize', 'IsMaximized'].includes(call[0])), [['IsMaximized'], ['ToggleMaximize'], ['IsMaximized']], 'native methods receive no frontend geometry or toggle arguments');
    f.setToggle(() => false); f.setRead(() => false);
    f.button.click();
    await until(() => f.count('IsMaximized') === 3 && !f.button.disabled, 'restoring can be requested after completion');
    label(f, false);
  } finally { f.close(); }
}

async function resizeAndLateRead() {
  const f = await fixture();
  try {
    f.setRead(() => true); f.resize();
    await until(() => f.button.getAttribute('aria-pressed') === 'true', 'native/external resize reads back maximized state');
    const old = deferred(), fresh = deferred();
    f.setRead(() => old.promise); f.resize();
    f.setRead(() => fresh.promise); f.resize();
    fresh.resolve(false); await pause(); label(f, false);
    old.resolve(true); await pause(); label(f, false);
    assert.equal(f.count('ToggleMaximize'), 0, 'resize only reads and never inverts window state');
  } finally { f.close(); }
}

async function initialReadCannotUndoToggle() {
  const initial = deferred();
  const f = await fixture({ read: () => initial.promise });
  try {
    f.setRead(() => true);
    f.button.click();
    await until(() => f.count('IsMaximized') === 2 && !f.button.disabled, 'toggle and subsequent read complete while startup read is pending');
    label(f, true);
    initial.resolve(false); await pause();
    label(f, true);
    f.emitLanguage('en'); label(f, true, 'en');
  } finally { f.close(); }
}

async function resizeDuringToggle() {
  const action = deferred(), stale = deferred();
  const f = await fixture({ toggle: () => action.promise });
  try {
    f.button.click();
    f.setRead(() => stale.promise); f.resize();
    assert.equal(f.button.disabled, true, 'resize does not unlock an in-flight toggle');
    f.setRead(() => true); action.resolve(true);
    await until(() => !f.button.disabled && f.count('IsMaximized') === 3, 'toggle always obtains a fresh final read after intervening resize');
    label(f, true);
    stale.resolve(false); await pause(); label(f, true);
  } finally { f.close(); }
}

async function failuresRemainRetryable() {
  const f = await fixture({ maximized: true, toggle: () => Promise.reject(Error('fixed private backend detail must not appear')) });
  try {
    label(f, true);
    f.button.click();
    await until(() => !f.button.disabled && f.count('IsMaximized') === 2, 'failed toggle releases busy state and refreshes');
    label(f, true);
    assert.equal(f.q('#toast').textContent, '窗口状态未能更新，请重试。');
    assert.doesNotMatch(f.q('#toast').textContent, /private backend/);
    f.emitLanguage('en'); label(f, true, 'en');
    f.button.click();
    await until(() => !f.button.disabled && f.count('IsMaximized') === 3, 'second failed toggle is retryable');
    assert.equal(f.q('#toast').textContent, 'Window state could not be updated. Please try again.');
    f.setToggle(() => false); f.setRead(() => false); f.button.click();
    await until(() => !f.button.disabled && f.count('IsMaximized') === 4, 'retry succeeds after backend recovers');
    label(f, false, 'en');
    f.setRead(() => Promise.reject(Error('fixed read failure'))); f.resize(); await pause();
    label(f, false, 'en');
    f.setRead(() => 'true'); f.resize(); await pause();
    label(f, false, 'en');
    f.setRead(() => true); f.resize();
    await until(() => f.button.getAttribute('aria-pressed') === 'true', 'read failure and nonboolean replies do not prevent the next valid read');
    label(f, true, 'en');
  } finally { f.close(); }
  const firstReadFails = await fixture({ read: () => Promise.reject(Error('fixed startup failure')) });
  try {
    assert.equal(firstReadFails.button.disabled, false);
    firstReadFails.setRead(() => true); firstReadFails.resize();
    await until(() => firstReadFails.button.getAttribute('aria-pressed') === 'true', 'a failed initial native read remains retryable');
    label(firstReadFails, true);
  } finally { firstReadFails.close(); }
}

async function run() {
  try {
    await platformGate();
    await initialAndLanguage();
    await toggleAndDuplicate();
    await resizeAndLateRead();
    await initialReadCannotUndoToggle();
    await resizeDuringToggle();
    await failuresRemainRetryable();
    await pause();
    assert.deepEqual(unhandled, [], 'all async mock paths must settle without unhandled rejections');
    console.log(JSON.stringify({ scope: 'fixed DOM/Wails mock only', platformAndOwnBridgeCases: 10, initialAndTranslatedLabels: true, duplicateToggleBlocked: true, resizeReadback: true, staleReadbackIgnored: true, apiFailuresRetryable: true, noNativeWindowProductBrowserClipboardOrNetwork: true }));
  } finally { process.removeListener('unhandledRejection', onUnhandled); }
}
run().catch(error => { console.error(error); process.exitCode = 1; });
