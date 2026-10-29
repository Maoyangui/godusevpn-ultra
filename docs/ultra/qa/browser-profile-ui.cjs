// Pure DOM fixtures. No product process, Windows API, browser runtime or network.
const assert = require('node:assert/strict');
const fs = require('node:fs'), path = require('node:path'), vm = require('node:vm');
const { JSDOM } = require(process.argv[2] || 'jsdom');
const root = path.resolve(__dirname, '../../..');
const source = name => fs.readFileSync(path.join(root, 'web/dist', name), 'utf8');
const dom = new JSDOM(source('index.html').replace(/<script[\s\S]*?<\/script>/g, ''), { runScripts: 'outside-only', url: 'http://example.invalid/', pretendToBeVisual: true });
const w = dom.window, calls = [], errors = [];
const clone = value => JSON.parse(JSON.stringify(value));
const regions = [
  { presetId: 'en-us-new-york', language: 'en-US', locale: 'en-US', timezone: 'America/New_York', acceptLanguages: ['en-US', 'en'] },
  { presetId: 'zh-cn-shanghai', language: 'zh-CN', locale: 'zh-CN', timezone: 'Asia/Shanghai', acceptLanguages: ['zh-CN', 'zh'] }
];
const config = { schemaVersion: 1, engineId: 'webview2', profileId: 'private', identity: { mode: 'native' }, region: regions[0], viewport: { width: 1280, height: 720 }, webrtc: 'disable-non-proxied-udp', webgl: 'runtime-default', webgpu: 'runtime-default', permissions: { camera: 'block', microphone: 'block', geolocation: 'block', notifications: 'block', clipboardRead: 'block' }, cookies: 'runtime-default', exitCleanup: 'retain-profile' };
const graphicsPresets = [{ mode: 'webgl-metadata', presetId: 'fixture-gpu', vendor: 'Fixture Vendor', renderer: 'ANGLE (Fixture GPU)' }];
const environmentPresets = [
  {config:{mode:'browser-emulation',os:'windows',devicePresetId:'windows-desktop',osVersion:'10.0.0'},recommendedViewport:{width:1280,height:720}},
  {config:{mode:'browser-emulation',os:'macos',devicePresetId:'macos-desktop',osVersion:'13.0.0'},recommendedViewport:{width:1280,height:720}},
  {config:{mode:'browser-emulation',os:'android',devicePresetId:'pixel-7',osVersion:'13.0.0'},model:'Pixel 7',recommendedViewport:{width:412,height:915}},
  {config:{mode:'browser-emulation',os:'ios',devicePresetId:'iphone-14',osVersion:'16.0.0'},model:'iPhone',recommendedViewport:{width:390,height:844}}
];
const engineReleases = [{engineId:'chrome-for-testing',version:'130.0.6723.58',channel:'stable'},{engineId:'chrome-for-testing',version:'131.0.1.1',channel:'fixture'},{engineId:'firefox',version:'133.0.3',channel:'release'}];
let engineInventory = [{engineId:'chrome-for-testing',version:'130.0.6723.58',treeVerified:true,versionVerified:true,canLaunch:true,fileCount:20}];
const state = { platform: 'windows', service: true, svcState: 'running', version: 'fixture', lang: 'zh', theme: 'light', view: { profiles: [{ id: 'fixture' }], nodes: [], profile: { id: 'fixture' }, settings: {}, state: { status: 'disconnected' } } };
let waitPreview = null, waitCatalog = null, defaultReads = 0, downloadRefused = false, lastReviewed = null;
const api = {
  GetState: async () => state,
  PrivacyReport: async () => ({ platform: 'windows', controls: [], capabilities: [], warnings: [] }),
  PrivacyFirewallReport: async () => ({ supported: true, rules: [], warnings: [] }),
  PrivacyHelperStatus: async () => ({ state: 'not_requested' }),
  PrivacyTimeReport: async () => ({ supported: false, choices: [], status: 'unsupported' }),
  BrowserHostReport: async () => ({ supported: true, recommended: 'webview', engines: [] }),
  BrowserProfileDefaults: async () => { defaultReads++; return { config: clone(config), regions: clone(regions), graphicsPresets: clone(graphicsPresets), environmentPresets:clone(environmentPresets), hostAvailable: true, canLaunch: true, status: 'available' }; },
  BrowserProfilePreview: async (text, seed) => {
    calls.push(['preview', JSON.parse(text), seed]);
    if (waitPreview) return waitPreview();
    const draft = JSON.parse(text);
    if (draft.viewport.width === 1281) throw Error('<img src=x onerror=bad()> fixture refusal');
    if (draft.environment && draft.environment.osVersion === '16.invalid') throw Error('fixture OS version refusal');
    if (seed) { draft.region = regions[1]; draft.viewport.width = draft.environment && ['android','ios'].includes(draft.environment.os) ? 390 : 1200; draft.randomizationSeed = seed; }
    lastReviewed = clone(draft);
    return { config: clone(draft), hostAvailable: true, applied: false, canLaunch: true, needsRuntimeReadback: true };
  },
  BrowserProfileLaunch: async text => { calls.push(['launch', JSON.parse(text)]); return { status: 'ready', detail: 'fixture opened' }; },
  BrowserProfileLaunchProxy: async (text, proxyJSON) => { calls.push(['launch-proxy',JSON.parse(text),JSON.parse(proxyJSON)]); return {status:'ready',detail:'fixture proxy opened; strict Ultra still required'}; },
  BrowserProfileClose: async () => { calls.push(['close']); },
  BrowserEngineCatalog: async proxyJSON => { calls.push(['engine-catalog',proxyJSON]); if(waitCatalog)return waitCatalog();return {releases:clone(engineReleases),fetchedAt:'fixture catalog timestamp'}; },
  BrowserEngineVersions: async () => { calls.push(['engine-local']);return clone(engineInventory); },
  BrowserEngineDownload: async text => { const request=JSON.parse(text);calls.push(['engine-download',request]);if(downloadRefused)throw Error('fixture download refused; no direct fallback');const installed={engineId:request.engineId,version:request.version,treeVerified:true,versionVerified:true,canLaunch:true,fileCount:17};engineInventory=engineInventory.filter(v=>v.engineId!==request.engineId||v.version!==request.version).concat([installed]);return clone(installed); },
  BrowserEngineProfileLaunch: async (text,proxyJSON) => { calls.push(['engine-launch',JSON.parse(text),JSON.parse(proxyJSON)]);throw Error('BROWSER_ENGINE_OPERATION_REFUSED'); },
  BrowserEngineProfileClose: async () => { calls.push(['engine-close']); }
};
api.BrowserProxyTest = async text => {
  const draft = JSON.parse(text); calls.push(['proxy', draft]);
  if (draft.host === 'refused.example') throw Error('backend details must not be displayed');
  return {status:'reachable',scope:'profile_proxy_probe_only',provider:'country.is',ip:'9.9.9.9',country:'US',city:'Fixture City',latencyMs:42,checkedAt:'fixed fixture'};
};
w.go = { main: { App: new Proxy(api, { get: (object, key) => object[key] || (() => Promise.resolve()) }) } };
w.runtime = { EventsOn: () => {} };
w.fetch = () => { throw Error('network forbidden in DOM fixtures'); };
w.HTMLElement.prototype.scrollIntoView = function () {};
w.addEventListener('error', event => errors.push(String(event.error || event.message)));
const context = dom.getInternalVMContext();
for (const name of ['i18n.js', 'flags.js', 'privacy.js', 'app.js']) vm.runInContext(source(name), context, { filename: name });
const q = selector => w.document.querySelector(selector);
const settle = () => new Promise(resolve => setTimeout(resolve, 25));
const edit = (id, value) => { const input = q(id); input.value = value; input.dispatchEvent(new w.Event('input', { bubbles: true })); };
const choose = (id, value) => { const input = q(id); input.value = value; input.dispatchEvent(new w.Event('change', { bubbles: true })); };
async function run() {
  try {
    await settle(); vm.runInContext("nav('privacy', {tab:'browser'})", context); await settle();
    assert.ok(q('#privacy-browser-preview'));
    assert.equal(q('#privacy-browser-launch').disabled, false);
    assert.equal(calls.length, 0, 'loading the form must never launch or configure a browser');
    assert.ok(q('.privacy-browser-profile [data-privacy-help]'));
    edit('#privacy-browser-id', 'private-2');
    q('#privacy-browser-preview').click(); await settle();
    assert.equal(q('#privacy-browser-launch').disabled, false);
    assert.equal(calls.at(-1)[1].profileId, 'private-2');
    assert.equal(Object.hasOwn(calls.at(-1)[1], 'binding'), false);
    edit('#privacy-browser-seed', 'fixture-seed');
    assert.equal(q('#privacy-browser-launch').disabled, false, 'editing invalidates the reviewed configuration immediately');
    q('#privacy-browser-random').click(); await settle();
    assert.equal(q('#privacy-browser-width').value, '1200');
    assert.equal(calls.at(-1)[2], 'fixture-seed');
    q('#privacy-browser-launch').click(); await settle();
    const launched = calls.find(item => item[0] === 'launch')[1];
    assert.equal(launched.viewport.width, 1200);
    assert.equal(launched.region.timezone, 'Asia/Shanghai');
    assert.equal(launched.randomizationSeed, 'fixture-seed');
    assert.equal(Object.hasOwn(launched, 'binding'), false);
    choose('#privacy-browser-graphics', 'fixture-gpu');
    assert.equal(q('#privacy-browser-gpu-vendor').value, 'Fixture Vendor');
    assert.equal(q('#privacy-browser-webgpu').value, 'disabled');
    assert.equal(q('#privacy-browser-webgpu').disabled, true);
    assert.equal(q('#privacy-browser-launch').disabled, false);
    const rendererInput = q('#privacy-browser-gpu-renderer'); rendererInput.focus();
    edit('#privacy-browser-gpu-renderer', 'ANGLE (Manual GPU)');
    assert.equal(w.document.activeElement, rendererInput, 'GPU edits preserve focus');
    assert.equal(q('#privacy-browser-graphics').value, 'manual');
    q('#privacy-browser-preview').click(); await settle();
    assert.equal(calls.at(-1)[1].graphics.renderer, 'ANGLE (Manual GPU)');
    assert.equal(calls.at(-1)[1].graphics.presetId, 'manual');
    assert.equal(calls.at(-1)[1].webgpu, 'disabled');
    choose('#privacy-browser-graphics', 'native');
    assert.equal(q('#privacy-browser-launch').disabled, false);
    assert.equal(q('#privacy-browser-gpu-vendor').disabled, true);
    assert.equal(q('#privacy-browser-webgpu').disabled, false);
    q('#privacy-browser-preview').click(); await settle();
    assert.equal(Object.hasOwn(calls.at(-1)[1], 'graphics'), false);
    assert.deepEqual(Array.from(q('#privacy-browser-environment').options,o=>o.value),['native','windows-desktop','macos-desktop','pixel-7','iphone-14']);
    assert.equal(q('#privacy-browser-os-version').disabled,true);
    for(const [index,preset] of environmentPresets.entries()){
      choose('#privacy-browser-environment',preset.config.devicePresetId);
      assert.equal(q('#privacy-browser-os-version').disabled,false);
      assert.equal(q('#privacy-browser-width').value,String(preset.recommendedViewport.width));
      assert.equal(q('#privacy-browser-height').value,String(preset.recommendedViewport.height));
      const mobile=['android','ios'].includes(preset.config.os);
      assert.equal(q('#privacy-browser-width').min,mobile?'320':'800');assert.equal(q('#privacy-browser-height').min,mobile?'568':'600');
      assert.equal(!!q('#privacy-browser-graphics option[value="fixture-gpu"]'),preset.config.os==='windows','non-Windows templates hide named Windows GPU presets');
      const osVersion=String(17+index)+'.2',width=mobile?360+index:1400+index,height=mobile?780+index:900+index;
      edit('#privacy-browser-os-version',osVersion);edit('#privacy-browser-width',width);edit('#privacy-browser-height',height);
      assert.equal(q('#privacy-browser-launch').disabled,false);assert.equal(q('#privacy-proxy-launch').disabled,false);
      q('#privacy-browser-preview').click();await settle();
      const request=calls.filter(c=>c[0]==='preview').at(-1)[1];
      assert.deepEqual(request.environment,{...preset.config,osVersion});assert.deepEqual(request.viewport,{width,height});
      const reads=defaultReads;q('#privacy-refresh').click();await settle();assert.equal(defaultReads,reads+1);
      assert.equal(q('#privacy-browser-environment').value,preset.config.devicePresetId);assert.equal(q('#privacy-browser-os-version').value,osVersion);
      assert.equal(q('#privacy-browser-width').value,String(width));assert.equal(q('#privacy-browser-height').value,String(height),'read-only refresh must preserve edited viewport');
    }
    edit('#privacy-browser-os-version','16.invalid');q('#privacy-browser-preview').click();await settle();
    assert.equal(q('#privacy-browser-os-version').value,'16.invalid');assert.equal(q('#privacy-browser-launch').disabled,false);assert.match(q('.privacy-browser-profile [role="alert"]').textContent,/fixture OS version refusal/);
    choose('#privacy-browser-environment','windows-desktop');choose('#privacy-browser-graphics','fixture-gpu');
    choose('#privacy-browser-environment','pixel-7');
    assert.equal(q('#privacy-browser-graphics').value,'native');assert.equal(q('#privacy-browser-webgpu').value,'runtime-default');assert.match(q('#privacy-browser-message').textContent,/已恢复原生 GPU/);
    assert.deepEqual(Array.from(q('#privacy-browser-graphics').options,o=>o.value),['native','manual']);
    choose('#privacy-browser-graphics','manual');edit('#privacy-browser-gpu-vendor','Manual non-Windows vendor');edit('#privacy-browser-gpu-renderer','Manual non-Windows renderer');
    edit('#privacy-browser-seed','mobile-seed');q('#privacy-browser-random').click();await settle();
    assert.equal(q('#privacy-browser-graphics').value,'manual');assert.equal(q('#privacy-browser-gpu-renderer').value,'Manual non-Windows renderer');
    assert.equal(calls.filter(c=>c[0]==='preview').at(-1)[1].graphics.presetId,'manual');
    choose('#privacy-browser-environment','macos-desktop');assert.equal(q('#privacy-browser-gpu-renderer').value,'Manual non-Windows renderer','confirmed manual metadata survives environment selection');
    choose('#privacy-browser-environment','iphone-14');q('#privacy-browser-preview').click();await settle();
    edit('#privacy-proxy-host', 'fixture.proxy'); edit('#privacy-proxy-username', 'fixture'); edit('#privacy-proxy-password', 'fixture-secret');
    assert.equal(calls.filter(item => item[0] === 'proxy').length, 0, 'proxy editing never calls an IP API automatically');
    q('#privacy-proxy-test').click(); await settle();
    assert.equal(calls.filter(item => item[0] === 'proxy').length, 1);
    assert.match(q('.privacy-browser-proxy .privacy-probe').textContent, /9\.9\.9\.9/);
    assert.doesNotMatch(q('.privacy-browser-proxy .privacy-probe').textContent, /fixture-secret/);
    assert.equal(q('#privacy-proxy-launch').disabled,false);
    const reviewedForProxy=clone(lastReviewed);q('#privacy-browser-width').value='3333'; // No input event: never an accepted/reviewed draft.
    q('#privacy-proxy-launch').click();await settle();
    const proxyLaunch=calls.find(c=>c[0]==='launch-proxy');assert.deepEqual(proxyLaunch[1],reviewedForProxy,'proxy launch must serialize the exact backend-reviewed config');
    assert.deepEqual(proxyLaunch[2],{type:'socks5',host:'fixture.proxy',port:1080,username:'fixture',password:'fixture-secret'});
    assert.equal(Object.hasOwn(proxyLaunch[1],'binding'),false);assert.equal(Object.hasOwn(proxyLaunch[1],'relayPid'),false);
    assert.match(q('#privacy-browser-message').textContent,/strict Ultra still required/);
    edit('#privacy-browser-os-version','17.2');assert.equal(q('#privacy-proxy-launch').disabled,false);
    edit('#privacy-proxy-host', 'refused.example');
    assert.equal(q('.privacy-browser-proxy .privacy-probe').hidden, true, 'editing invalidates old proxy evidence immediately');
    q('#privacy-proxy-test').click(); await settle();
    assert.match(q('.privacy-browser-proxy [role="alert"]').textContent, /没有改用直连/);
    assert.doesNotMatch(q('.privacy-browser-proxy [role="alert"]').textContent, /backend details/);
    assert.equal(calls.filter(c=>c[0].startsWith('engine-')).length,0,'catalog, download, inventory and engine launch require explicit actions');
    assert.equal(q('#privacy-engine-download').disabled,true);assert.equal(q('#privacy-engine-launch').disabled,true);
    q('#privacy-engine-refresh').click();await settle();assert.deepEqual(calls.find(c=>c[0]==='engine-catalog'),['engine-catalog','']);
    assert.deepEqual(Array.from(q('#privacy-engine-options').options,o=>o.value),['130.0.6723.58','131.0.1.1']);
    choose('#privacy-engine-kind','firefox');assert.equal(q('#privacy-engine-version').value,'');assert.deepEqual(Array.from(q('#privacy-engine-options').options,o=>o.value),['133.0.3']);
    edit('#privacy-proxy-host','fixture.proxy');choose('#privacy-engine-route','proxy');edit('#privacy-engine-version','133.0.3');
    assert.equal(q('#privacy-engine-download').disabled,false);const probesBeforeEngine=calls.filter(c=>c[0]==='proxy').length;
    q('#privacy-engine-refresh').click();await settle();const catalogRoute=JSON.parse(calls.filter(c=>c[0]==='engine-catalog').at(-1)[1]);
    assert.deepEqual(catalogRoute,{type:'socks5',host:'fixture.proxy',port:1080,username:'fixture',password:'fixture-secret'});
    q('#privacy-engine-download').click();await settle();const download=calls.find(c=>c[0]==='engine-download')[1];
    assert.equal(download.engineId,'firefox');assert.equal(download.version,'133.0.3');assert.deepEqual(JSON.parse(download.proxyJSON),catalogRoute);
    assert.match(q('.privacy-engine-manager [role="status"]').textContent,/启动隔离另行验证/);assert.match(q('.privacy-engine-manager .privacy-probe').textContent,/firefox.*133\.0\.3/s);
    q('#privacy-engine-local').click();await settle();assert.equal(calls.filter(c=>c[0]==='engine-local').length,1);assert.match(q('.privacy-engine-manager .privacy-probe').textContent,/chrome-for-testing/);
    downloadRefused=true;edit('#privacy-engine-version','133.0.4');q('#privacy-engine-download').click();await settle();
    assert.match(q('.privacy-engine-manager [role="alert"]').textContent,/fixture download refused/);assert.equal(q('#privacy-engine-version').value,'133.0.4');downloadRefused=false;
    edit('#privacy-engine-version','133.0.3');q('#privacy-engine-launch').click();await settle();
    assert.equal(calls.filter(c=>c[0]==='engine-launch').length,1,'Firefox metadata is forwarded to its host; this mock does not claim that the old example engine version runs the feature');
    const graphicsLaunch=calls.filter(c=>c[0]==='engine-launch').at(-1)[1];assert.deepEqual(graphicsLaunch.graphics,{mode:'webgl-metadata',presetId:'manual',vendor:'Manual non-Windows vendor',renderer:'Manual non-Windows renderer'});assert.equal(graphicsLaunch.webgpu,'disabled');assert.equal(graphicsLaunch.environment.os,'ios');
    choose('#privacy-browser-environment','native');choose('#privacy-browser-graphics','native');assert.equal(q('#privacy-browser-os-version').disabled,true);assert.equal(q('#privacy-browser-width').min,'800');
    choose('#privacy-engine-route','ultra');q('#privacy-engine-launch').click();await settle();assert.equal(calls.filter(c=>c[0]==='engine-launch').length,1);assert.match(q('.privacy-engine-manager [role="alert"]').textContent,/本页独立代理/);
    choose('#privacy-engine-route','proxy');q('#privacy-engine-launch').click();await settle();const engineLaunch=calls.filter(c=>c[0]==='engine-launch').at(-1);
    assert.deepEqual(engineLaunch[1],{engineId:'firefox',version:'133.0.3',profileId:'private-2',language:'zh-CN',timezone:'Asia/Shanghai',width:800,height:844,identity:{mode:'native'}});assert.deepEqual(engineLaunch[2],catalogRoute);
    assert.match(q('.privacy-engine-manager [role="alert"]').textContent,/BROWSER_ENGINE_OPERATION_REFUSED/);assert.equal(q('.privacy-engine-manager [role="status"]'),null,'backend refusal must not display ready');
    q('#privacy-engine-close').click();await settle();assert.equal(calls.filter(c=>c[0]==='engine-close').length,1);assert.equal(calls.filter(c=>c[0]==='proxy').length,probesBeforeEngine,'engine actions must not reuse or silently refresh the IP probe as launch proof');
    edit('#privacy-browser-width', '1281');
    assert.equal(q('#privacy-browser-launch').disabled, false);
    q('#privacy-browser-preview').click(); await settle();
    assert.equal(q('#privacy-browser-width').value, '1281', 'failure preserves the user draft');
    assert.equal(q('#privacy-browser-launch').disabled, false);
    assert.match(q('.privacy-browser-profile [role="alert"]').textContent, /fixture refusal/);
    assert.equal(q('.privacy-browser-profile img'), null, 'backend error is text, never HTML');
    assert.equal(calls.filter(item => item[0] === 'launch').length, 1);
    q('#privacy-browser-close').click(); await settle();
    assert.equal(calls.filter(item => item[0] === 'close').length, 1);
    let resolve;
    waitPreview = () => new Promise(done => { resolve = done; });
    q('#privacy-browser-preview').click(); await settle();
    const staleLaunch = q('#privacy-browser-launch');
    vm.runInContext("nav('home')", context);
    resolve({ config: clone(config), hostAvailable: true }); await settle();
    staleLaunch.click(); await settle();
    assert.equal(calls.filter(item => item[0] === 'launch').length, 1);
    waitPreview=null;vm.runInContext("nav('privacy', {tab:'browser'})",context);await settle();
    let resolveCatalog;waitCatalog=()=>new Promise(done=>{resolveCatalog=done;});q('#privacy-engine-refresh').click();await settle();const oldEngineCard=q('.privacy-engine-manager'),oldEngineText=oldEngineCard.textContent;vm.runInContext("nav('home')",context);
    resolveCatalog({releases:clone(engineReleases),fetchedAt:'STALE_CATALOG_SHOULD_NOT_APPEAR'});await settle();assert.equal(q('.view.privacy:not(.pop) .privacy-engine-manager'),null);assert.equal(oldEngineCard.textContent,oldEngineText,'late catalog response must not update the outgoing animated page');assert.equal(w.document.body.textContent.includes('STALE_CATALOG_SHOULD_NOT_APPEAR'),false);
    assert.deepEqual(errors, []);
    console.log(JSON.stringify({ scope: 'isolated DOM', previewBeforeLaunch: true, fourEnvironments:true,osVersionAndViewportDraftPreserved:true,nonWindowsNamedGPUHidden:true,manualGPUPreserved:true,editedDraftInvalidatesPreview: true, launchUsesReviewedConfig: true, proxyLaunchUsesReviewedConfig:true,explicitEngineCatalogDownloadInventory:true,unsupportedEngineLaunchRefused:true,noTransportSecretsInUI: true, escapedErrors: true, stalePageBlocked: true }));
  } finally { w.close(); }
}
run().catch(error => { console.error(error); process.exitCode = 1; });
