// Pure DOM fixtures against the real shipped UI. Every backend reply and saved
// record is in memory; no browser process, network, clipboard or system API runs.
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs'), path = require('node:path'), vm = require('node:vm');
const {JSDOM} = require(process.argv[2] || 'jsdom');
const root = path.resolve(__dirname, '../../..');
const source = name => fs.readFileSync(path.join(root, 'web/dist', name), 'utf8');
const clone = value => JSON.parse(JSON.stringify(value));
const delay = (ms = 25) => new Promise(resolve => setTimeout(resolve, ms));
const expectedHardware = count => ({mode: 'logical-processors', logicalProcessors: count});
const region = {presetId: 'en-us', language: 'en-US', locale: 'en-US', timezone: 'America/Los_Angeles', acceptLanguages: ['en-US', 'en']};
const defaults = {schemaVersion: 1, engineId: 'webview2', profileId: 'private', identity: {mode: 'manual', userAgent: 'Fixture exact UA/1'}, region, viewport: {width: 1280, height: 720}, webrtc: 'disable-non-proxied-udp', webgl: 'runtime-default', webgpu: 'runtime-default', permissions: {camera: 'block', microphone: 'block', geolocation: 'block', notifications: 'block', clipboardRead: 'block'}, cookies: 'runtime-default', exitCleanup: 'retain-profile'};

function fixture() {
  const dom = new JSDOM(source('index.html').replace(/<script[\s\S]*?<\/script>/g, ''), {runScripts: 'outside-only', url: 'http://example.invalid/', pretendToBeVisual: true});
  const w = dom.window, calls = [], errors = [], records = new Map();
  let revision = 0, pendingPreview = null, acceptedLaunches = 0;
  const plan = c => ({mode: c.identity.mode, status: 'planned', userAgent: c.identity.userAgent, applied: false, requiresRuntimeReadback: true, exactChromiumVersionKnown: false});
  const recordCall = (kind, value) => calls.push([kind, clone(value)]);
  // A fixed mock rejection, not a claim that a real backend has been run.
  function validateHardware(c) {
    if (!Object.hasOwn(c, 'hardware')) return;
    const h = c.hardware;
    if (!h || Object.keys(h).sort().join(',') !== 'logicalProcessors,mode' || h.mode !== 'logical-processors' || !Number.isInteger(h.logicalProcessors) || h.logicalProcessors < 1 || h.logicalProcessors > 64) throw Error('FIXTURE_HARDWARE_INTEGER_1_TO_64_REQUIRED');
  }
  async function webviewLaunch(text) {
    const c = JSON.parse(text); recordCall('webview-launch', c); validateHardware(c);
    acceptedLaunches++;
    return {detail: 'fixture only; no browser was started'};
  }
  const api = {
    GetState: async () => ({platform: 'windows', service: true, lang: 'zh', theme: 'light', view: {profiles: [], nodes: [], settings: {}, state: {status: 'disconnected'}}}),
    PrivacyReport: async () => ({platform: 'windows', controls: [], capabilities: [], warnings: []}),
    PrivacyFirewallReport: async () => ({supported: true, rules: []}),
    PrivacyHelperStatus: async () => ({state: 'not_requested'}),
    PrivacyTimeReport: async () => ({supported: false, choices: [], status: 'unsupported'}),
    BrowserHostReport: async () => ({supported: true, engines: []}),
    BrowserProfileDefaults: async () => ({config: clone(defaults), regions: [clone(region)], environmentPresets: [], graphicsPresets: [], identityPreview: plan(defaults), hostAvailable: true}),
    BrowserProfileIdentity: async text => {const c = JSON.parse(text); recordCall('webview-identity', c); validateHardware(c); return plan(c);},
    BrowserProfilePreview: async text => {const c = JSON.parse(text); recordCall('webview-preview', c); validateHardware(c); if (pendingPreview) return pendingPreview.promise; return {config: c, identityPreview: plan(c), hostAvailable: true};},
    BrowserProfileLaunch: webviewLaunch,
    BrowserProfileLaunchProxy: webviewLaunch,
    BrowserEngineVersions: async () => {recordCall('inventory', null); return [{engineId: 'chrome-for-testing', version: '156.0.8078.47', treeVerified: true, versionVerified: true}, {engineId: 'firefox', version: '147.0.1', treeVerified: true, versionVerified: true}];},
    BrowserEngineIdentity: async text => {const c = JSON.parse(text); recordCall('native-identity', c); validateHardware(c); return plan(c);},
    BrowserEngineProfileLaunch: async (text, proxyText) => {
      const c = JSON.parse(text); recordCall('native-launch', c); recordCall('launch-proxy', JSON.parse(proxyText)); validateHardware(c);
      if (c.engineId === 'firefox' && c.hardware) throw Error('UNEXPECTED_FIREFOX_CUSTOM_HARDWARE_API');
      acceptedLaunches++;
      return {detail: 'fixture only; no native engine was started'};
    },
    BrowserProfileSave: async text => {
      const request = JSON.parse(text); recordCall('save', request); validateHardware(request.config);
      const id = request.id || 'p-' + '1'.repeat(32);
      if (request.id) assert.equal(request.expectedRevision, records.get(id).revision, 'saved CPU record uses its expected revision');
      const record = {schemaVersion: 1, id, revision: (++revision).toString(16).padStart(64, '0'), name: request.name, config: {...clone(request.config), profileId: id}, engine: clone(request.engine), updatedAt: '2026-10-26T00:00:00Z', applied: false, requiresLaunchValidation: true};
      records.set(id, clone(record)); return record;
    },
    BrowserProfilesList: async () => {recordCall('list', null); return {records: Array.from(records.values(), clone)};},
    BrowserProfileLoad: async id => {recordCall('load', id); assert.ok(records.has(id)); return clone(records.get(id));},
  };
  w.go = {main: {App: new Proxy(api, {get: (object, key) => object[key] || (() => {throw Error('unexpected backend API ' + String(key));})})}};
  w.runtime = {EventsOn() {}};
  w.fetch = () => {throw Error('network forbidden in CPU DOM fixture');};
  w.HTMLElement.prototype.scrollIntoView = function () {};
  Object.defineProperty(w.navigator, 'clipboard', {value: {writeText() {throw Error('clipboard forbidden');}}});
  w.document.execCommand = () => {throw Error('clipboard forbidden');};
  w.addEventListener('error', event => errors.push(String(event.error || event.message)));
  const context = dom.getInternalVMContext();
  for (const name of ['i18n.js', 'flags.js', 'privacy.js', 'app.js']) vm.runInContext(source(name), context, {filename: name});
  const q = selector => w.document.querySelector(selector);
  const count = kind => calls.filter(call => call[0] === kind).length;
  const last = kind => calls.filter(call => call[0] === kind).at(-1)?.[1];
  function change(selector, value, event) {
    const element = q(selector); assert.ok(element, 'missing ' + selector); element.value = value;
    element.dispatchEvent(new w.Event(event, {bubbles: true}));
  }
  const choose = (selector, value) => change(selector, value, 'change');
  const edit = (selector, value) => change(selector, value, 'input');
  const click = async selector => {assert.ok(q(selector), selector); q(selector).click(); await delay();};
  async function until(predicate, label) {const end = Date.now() + 2500; while (!predicate() && Date.now() < end) await delay(10); assert.ok(predicate(), label);}
  return {
    w, q, calls, errors, records, count, last, choose, edit, click, context,
    get acceptedLaunches() {return acceptedLaunches;},
    async open() {await delay(); vm.runInContext("nav('privacy',{tab:'browser'})", context); await until(() => q('#privacy-browser-cpu-mode') && !q('#privacy-browser-cpu-mode').disabled, 'CPU form loaded');},
    async engine(engine) {const before = count('inventory'); choose('#privacy-browser-engine', engine); await until(() => !q('#privacy-browser-engine').disabled && (engine === 'webview2' || count('inventory') === before + 1), 'engine selection completed');},
    holdPreview() {let resolve; const promise = new Promise(done => {resolve = done;}); pendingPreview = {promise}; return {resolve};},
    close() {assert.deepEqual(errors, [], 'no unhandled DOM errors'); w.close();},
  };
}

async function run() {
  const f = fixture();
  try {
    await f.open();
    assert.equal(f.q('#privacy-browser-cpu-mode').closest('[role="tabpanel"]').id, 'privacy-editor-pane-fingerprint');
    assert.equal(f.q('#privacy-browser-cpu-mode').value, 'native');
    assert.equal(f.q('#privacy-browser-cpu').disabled, true);
    assert.equal(f.count('webview-launch') + f.count('native-launch'), 0, 'opening the form never starts a browser');

    f.choose('#privacy-browser-cpu-mode', 'logical-processors');
    assert.equal(f.q('#privacy-browser-cpu').value, '8', 'custom mode starts with its explicit default');
    assert.equal(f.q('#privacy-browser-cpu').disabled, false);
    f.edit('#privacy-browser-cpu', '16');
    await f.click('#privacy-browser-preview');
    assert.deepEqual(f.last('webview-preview').hardware, expectedHardware(16));
    await f.click('#privacy-browser-launch');
    assert.deepEqual(f.last('webview-launch').hardware, expectedHardware(16), 'WebView sends the reviewed CPU value unchanged');

    // Editing the CPU invalidates the previous preview; automatic preflight
    // must inspect the new value rather than launching the reviewed old one.
    f.edit('#privacy-browser-cpu', '32');
    assert.equal(f.q('#privacy-browser-preview-result').hidden, true);
    assert.equal(f.q('#privacy-browser-launch').textContent, '检查并打开');
    const beforeEdited = f.count('webview-preview');
    await f.click('#privacy-browser-launch');
    assert.equal(f.count('webview-preview'), beforeEdited + 1);
    assert.deepEqual(f.last('webview-preview').hardware, expectedHardware(32));
    assert.deepEqual(f.last('webview-launch').hardware, expectedHardware(32));

    f.edit('#privacy-browser-profile-name', 'CPU 参数测试');
    await f.click('#privacy-browser-profile-save');
    assert.deepEqual(f.last('save').config.hardware, expectedHardware(32));
    for (const pane of ['proxy', 'environment', 'versions', 'fingerprint']) await f.click('[data-browser-pane="' + pane + '"]');
    assert.equal(f.q('#privacy-browser-cpu').value, '32', 'saved CPU draft survives pane changes');
    await f.click('#privacy-browser-profile-list');
    const saved = Array.from(f.records.values())[0];
    f.choose('#privacy-browser-profile-select', saved.id);
    await f.click('#privacy-browser-profile-load');
    assert.equal(f.q('#privacy-browser-cpu-mode').value, 'logical-processors');
    assert.equal(f.q('#privacy-browser-cpu').value, '32', 'saved CPU value reloads unchanged');

    await f.engine('chrome-for-testing');
    f.edit('#privacy-proxy-host', 'synthetic.proxy.invalid');
    f.edit('#privacy-browser-cpu', '12');
    await f.click('#privacy-browser-preview');
    assert.deepEqual(f.last('native-identity').hardware, expectedHardware(12));
    await f.click('#privacy-browser-launch');
    assert.equal(f.last('native-launch').engineId, 'chrome-for-testing');
    assert.deepEqual(f.last('native-launch').hardware, expectedHardware(12), 'Chrome forwards the edited CPU value');

    // Switching engines preserves the request. Firefox explicitly refuses it
    // before either preview/launch backend API is called; no silent omission.
    await f.engine('firefox');
    assert.equal(f.q('#privacy-browser-cpu').value, '12');
    const beforeFirefox = f.calls.length;
    await f.click('#privacy-browser-preview');
    await f.click('#privacy-browser-launch');
    assert.equal(f.calls.length, beforeFirefox, 'Firefox custom CPU never reaches an API');
    assert.match(f.q('.privacy-editor-footer > [role="alert"]').textContent, /Firefox.*CPU/);
    f.choose('#privacy-engine-kind', 'firefox'); f.edit('#privacy-engine-version', '147.0.1'); f.choose('#privacy-engine-route', 'proxy');
    const beforeLegacyFirefox = f.calls.length;
    await f.click('#privacy-engine-launch');
    assert.equal(f.calls.length, beforeLegacyFirefox, 'legacy Firefox launcher cannot discard CPU settings');

    // Restoring native removes the field instead of transmitting zero/null or
    // an unsupported {mode:native} object, for both native and WebView routes.
    f.choose('#privacy-browser-cpu-mode', 'native');
    assert.equal(f.q('#privacy-browser-cpu').disabled, true);
    await f.click('#privacy-browser-launch');
    assert.equal(Object.hasOwn(f.last('native-launch'), 'hardware'), false);
    await f.engine('webview2');
    await f.click('#privacy-browser-profile-save');
    assert.equal(Object.hasOwn(f.last('save').config, 'hardware'), false);
    await f.click('#privacy-browser-launch');
    assert.equal(Object.hasOwn(f.last('webview-preview'), 'hardware'), false);
    assert.equal(Object.hasOwn(f.last('webview-launch'), 'hardware'), false);

    for (const engine of ['webview2', 'chrome-for-testing']) {
      await f.engine(engine);
      f.choose('#privacy-browser-cpu-mode', 'logical-processors');
      for (const invalid of ['0', '65', '2.5']) {
        f.edit('#privacy-browser-cpu', invalid);
        const beforeWebviewLaunch = f.count('webview-launch');
        const beforeLaunch = f.count('webview-launch') + f.count('native-launch');
        await f.click('#privacy-browser-preview');
        assert.deepEqual(f.last(engine === 'webview2' ? 'webview-preview' : 'native-identity').hardware, expectedHardware(Number(invalid)), 'bad numeric draft reaches the fixed rejecting mock unrounded');
        assert.equal(f.q('#privacy-browser-cpu').value, invalid, 'backend refusal preserves the CPU draft');
        assert.match(f.q('.privacy-editor-footer > [role="alert"]').textContent, /FIXTURE_HARDWARE_INTEGER_1_TO_64_REQUIRED/);
        assert.equal(f.count('webview-launch') + f.count('native-launch'), beforeLaunch, 'failed preview never starts a browser');
        const beforeAccepted = f.acceptedLaunches;
        await f.click('#privacy-browser-launch');
        assert.equal(f.acceptedLaunches, beforeAccepted, 'Open cannot pass the fixed rejecting backend with an invalid CPU value');
        if (engine === 'webview2') assert.equal(f.count('webview-launch'), beforeWebviewLaunch, 'failed automatic preflight never invokes the WebView launcher');
        else assert.deepEqual(f.last('native-launch').hardware, expectedHardware(Number(invalid)), 'native startup backend receives and rejects the invalid value without rewriting it');
        assert.equal(f.q('#privacy-browser-cpu').value, invalid, 'Open refusal also keeps the CPU draft');
        assert.match(f.q('.privacy-editor-footer > [role="alert"]').textContent, /FIXTURE_HARDWARE_INTEGER_1_TO_64_REQUIRED/);
      }
    }

    // Old controls from a redraw and an outgoing page cannot submit CPU data.
    f.edit('#privacy-browser-cpu', '4');
    const detachedLaunch = f.q('#privacy-browser-launch'), detachedSave = f.q('#privacy-browser-profile-save');
    f.choose('#privacy-browser-cpu-mode', 'native');
    const beforeDetached = f.calls.length;
    detachedLaunch.click(); detachedSave.click(); await delay();
    assert.equal(f.calls.length, beforeDetached, 'detached CPU page controls do not submit');
    await f.engine('webview2');
    f.choose('#privacy-browser-cpu-mode', 'logical-processors'); f.edit('#privacy-browser-cpu', '7');
    const pending = f.holdPreview(), beforePendingLaunch = f.count('webview-launch');
    const departedLaunch = f.q('#privacy-browser-launch'), departedSave = f.q('#privacy-browser-profile-save');
    departedLaunch.click(); await delay();
    const outgoingConfig = clone(f.last('webview-preview'));
    vm.runInContext("nav('home')", f.context); await delay();
    const afterDeparture = f.calls.length;
    departedLaunch.click(); departedSave.click(); await delay();
    assert.equal(f.calls.length, afterDeparture, 'departed CPU controls do not call the backend');
    pending.resolve({config: outgoingConfig, hostAvailable: true}); await delay();
    assert.equal(f.count('webview-launch'), beforePendingLaunch, 'a stale successful preflight cannot launch after leaving the page');
    assert.equal(f.calls.length, afterDeparture);

    console.log(JSON.stringify({scope: 'isolated CPU hardware DOM',webviewAndChromeExactHardware: true,editInvalidatesPreview: true,saveLoadAndPaneRetention: true,nativeOmitsField: true,firefoxRefusesBeforeAPI: true,mockBackendInvalidNumberDraftRetention: true,staleControlsAndPreflightCannotSubmit: true,noBrowserOrNetwork: true}));
  } finally {f.close();}
}
run().catch(error => {console.error(error); process.exitCode = 1;});
