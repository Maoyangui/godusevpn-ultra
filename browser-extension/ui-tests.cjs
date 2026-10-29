// Isolated DOM integration checks. Pass a jsdom module path as the first argument.
// The test has no real Wails bridge, privileged helper, network, or browser policy API.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { JSDOM } = require(process.argv[2] || 'jsdom');
const root = path.resolve(__dirname, '..');
const source = file => fs.readFileSync(path.join(root, 'web', 'dist', file), 'utf8');
const dom = new JSDOM(source('index.html').replace(/<script[\s\S]*?<\/script>/g, ''), { runScripts: 'outside-only', url: 'http://localhost/', pretendToBeVisual: true });
const { window } = dom;
const calls = [];
const uncaught = [];
const browserLogs = [];
const scrollTargets = [];
window.console.log = (...args) => browserLogs.push(args);
let cancelInspect = false, failClean = true, cancelExport = false;
let failPolicyReport = false, failFirewallReport = false, failDetection = false, helperState = 'connected';
let firewallRules = [];
let timeCurrent = 'China Standard Time', timeManaged = false, failTime = true, timeMode = 'normal', timeReads = 0;
const timeChoices = [{ id: 'China Standard Time', label: '(UTC+08:00) 北京' }, { id: 'UTC', label: '(UTC) 协调世界时' }];
window.addEventListener('error', event => uncaught.push(String(event.error || event.message)));
const uiState = { platform: 'windows', service: true, version: 'fixture', lang: 'zh', theme: 'light', view: { profiles: [], nodes: [], state: { status: 'disconnected' } } };
const control = { id: 'chrome-webrtc', title: 'Chrome：WebRTC 规则', category: 'browser', scope: '当前用户 / Chrome', description: '限制非代理 UDP。', impact: '可能影响会议。', status: 'available', readStatus: 'different', managed: false, requiresAdmin: false, canApply: true, restart: true };
const report = { platform: 'windows', admin: true, observerElevated: false, controls: [control, { id: 'serial', title: '原生硬件信息', category: 'guidance', description: '需隔离。', status: 'unsupported', canApply: false }, { id: 'timezone-language', title: '时区语言', category: 'guidance', status: 'manual', canApply: false, settingsId: 'dateandtime' }], capabilities: [], warnings: [] };
const detection = { generatedAt: 'fixture', observer: '固定测试：普通用户观察进程', admin: false, helperAuthorized: true, findings: [
  { id: 'language', title: '语言可见', category: '浏览器', audience: 'website', status: 'observed', summary: '当前可见', evidence: '本地', scope: '测试来源' },
  { id: 'serial', title: '序列号未知', category: '应用', audience: 'app', status: 'unknown', summary: '不可确认', evidence: '未采集', scope: '测试范围' }
], warnings: [] };
report.controls.push({ id: 'read-only-policy', title: '只读匹配但不可修改', category: 'windows', status: 'manual', readStatus: 'match', canApply: false, settingsId: 'privacy' });
// Keep this as a visible policy control so the fixture verifies the fallback
// label for an unrecognized future status. `guidance` controls intentionally
// belong to the separate help-only catalog and are not rendered as policy cards.
report.controls.push({ id: 'new-state', title: '未知返回状态', category: 'windows', status: 'unrecognized_future_state', canApply: false });
report.capabilities.push({ id: 'guide', title: '能力说明', status: 'manual', description: '需要手动处理。' });
window.go = { main: { App: new Proxy({
  GetState: async () => uiState,
  PrivacyReport: async () => { if (failPolicyReport) throw new Error('模拟策略读取失败'); return Object.assign({}, report, {admin:helperState === 'connected'}); },
  PrivacyHelperStatus: async () => ({state:helperState}),
  PrivacyAuthorize: async () => { calls.push(['PrivacyAuthorize']); helperState = 'connected'; return {state:helperState}; },
  PrivacyDetectionSnapshot: async () => { if (failDetection) throw new Error('模拟检测读取失败'); return {snapshotId:'fixture-snapshot', expiresAt:new Date(Date.now()+600000).toISOString(), report:detection}; },
  PrivacyPreviewDetectionExport: async selection => { assert.equal(selection.snapshotId,'fixture-snapshot'); assert.ok(['all','filtered'].includes(selection.scope)); const selected=detection.findings.filter(f=>(selection.scope==='all'||selection.findingIds.includes(f.id))&&(selection.includeEnvironment||!['timezone','locale'].includes(f.id))); return {previewId:'fixture-preview',expiresAt:new Date(Date.now()+600000).toISOString(),count:selected.length,content:JSON.stringify({findings:selected})}; },
  PrivacyTimeReport: async () => { timeReads++; return timeMode === 'unavailable' ? undefined : { supported: true, currentId: timeCurrent, choices: timeMode === 'empty' ? [] : timeChoices, managed: timeManaged, status: timeManaged ? 'configured' : 'available', detail: '固定测试时区报告。', daylightSavingDisabled: true }; },
  PrivacyTimeApply: async id => { calls.push(['PrivacyTimeApply', id]); if (failTime) throw new Error('模拟时区修改失败'); timeCurrent = id; timeManaged = true; return { status: 'configured', message: '模拟时区已设置。', changed: true }; },
  PrivacyTimeRestore: async () => { calls.push(['PrivacyTimeRestore']); timeCurrent = 'China Standard Time'; timeManaged = false; return { status: 'restored', message: '模拟原时区已恢复。', changed: true }; },
  PrivacyFirewallReport: async () => { if (failFirewallReport) throw new Error('模拟程序规则读取失败'); return { supported: true, elevated:helperState === 'connected', rules: firewallRules, unknownFilters: 2, warnings: [], loopback: 'unprotected', scope: '指定程序' }; },
  PrivacyChooseProgram: async () => 'C:\\Fixture\\program.exe',
  PrivacyFirewallApply: async rule => { calls.push(['PrivacyFirewallApply',rule]); firewallRules = [{rule:Object.assign({id:'fixture-rule'},rule),state:'filters_installed',installedFilters:2,expectedFilters:2}]; return {applied:true,verified:true,warning:'固定范围说明'}; },
  PrivacyFirewallRemove: async id => { calls.push(['PrivacyFirewallRemove',id]); firewallRules=[]; return {verified:true,warning:'固定范围说明'}; },
  PrivacyDetection: async () => { throw new Error('Legacy detection fixture must not be used'); },
  PrivacyApply: async id => { calls.push(['PrivacyApply', id]); throw new Error('模拟写入失败'); },
  PrivacyInspectImage: async () => { calls.push(['PrivacyInspectImage']); return cancelInspect ? null : { format: 'png', metadataBlocks: [{ type: 'eXIf', bytes: 128 }], removedBlocks: [], warning: '仍有其他可关联信息。', privateMetadata: 'GPS_CONTENT_MUST_NOT_BE_RENDERED' }; },
  PrivacyCleanImage: async () => { calls.push(['PrivacyCleanImage']); if (failClean) throw new Error('模拟图片处理失败'); return { format: 'png', metadataBlocks: [{ type: 'eXIf', bytes: 128 }], removedBlocks: [{ type: 'eXIf', bytes: 128 }], output: 'C:\\Fixture\\clean.png', warning: '保留像素和 ICC；不匿名化画面。' }; },
  PrivacyExportDetection: async id => { assert.equal(id,'fixture-preview'); calls.push(['PrivacyExportDetection']); return cancelExport ? '' : 'C:\\SensitiveLocalFolder\\report.json'; }
}, { get: (object, key) => object[key] || ((...args) => { calls.push([key, ...args]); return Promise.resolve(); }) }) } };
window.runtime = { EventsOn() {} };
window.HTMLElement.prototype.scrollIntoView = function () { scrollTargets.push(this.getAttribute('data-privacy-control') || this.id); };
const context = dom.getInternalVMContext();
for (const file of ['i18n.js', 'flags.js', 'privacy.js', 'app.js']) vm.runInContext(source(file), context, { filename: file });
const settle = () => new Promise(resolve => setTimeout(resolve, 30));
function assertHelpCoverage(container) {
  for (const item of container.querySelectorAll('button:not(.privacy-help), input, select, summary')) {
    const label = item.closest('label');
    const fieldHelp = /^(INPUT|SELECT)$/.test(item.tagName) && label && label.querySelectorAll('input,select').length === 1 && label.querySelector('.privacy-help');
    const ownHelp = item.querySelector('.privacy-help');
    assert.ok(ownHelp || fieldHelp || item.nextElementSibling && item.nextElementSibling.classList.contains('privacy-help'), 'missing field/action help for ' + (item.id || item.textContent));
  }
  for (const heading of container.querySelectorAll('h1, h2')) assert.ok(heading.querySelector('.privacy-help'), 'missing feature explanation for ' + heading.textContent);
}
async function run() {
  await settle();
  assert.ok(window.document.getElementById('ob-privacy'), 'privacy is available without subscription');
  assert.ok(window.document.getElementById('ob-detection'), 'detection is available without subscription');
  assert.ok(window.document.getElementById('ob-privacy').nextElementSibling.classList.contains('privacy-help'));
  assert.ok(window.document.getElementById('ob-detection').nextElementSibling.classList.contains('privacy-help'));
  window.document.getElementById('ob-privacy').click(); await settle();
  window.document.querySelector('[data-privacy-tab="browser"]').click();
  assert.ok(window.document.querySelector('[data-privacy-apply="chrome-webrtc"]'));
  assert.equal(window.document.querySelector('[data-privacy-apply="serial"]'), null, 'unsupported control cannot apply');
  assert.match(window.document.getElementById('stage').textContent, /管理员执行能力/);
  assert.match(window.document.getElementById('stage').textContent, /普通权限进程观察/);
  assert.match(window.document.getElementById('stage').textContent, /需要完全退出并重启/);
  const help = window.document.querySelector('.privacy-help'); help.click();
  assert.ok(window.document.getElementById('privacy-tooltip'));
  window.document.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Escape' }));
  assert.equal(window.document.getElementById('privacy-tooltip'), null, 'Escape closes lightweight tooltip');
  help.dispatchEvent(new window.MouseEvent('mouseover', { bubbles: true }));
  assert.ok(window.document.getElementById('privacy-tooltip'), 'hover displays tooltip');
  window.document.body.click();
  assert.equal(window.document.getElementById('privacy-tooltip'), null, 'outside click closes tooltip');
  window.document.querySelector('[data-privacy-apply="chrome-webrtc"]').click(); await settle();
  assert.equal(calls.length, 0, 'first click requests confirmation without applying');
  window.document.getElementById('dialog-yes').click(); await settle();
  assert.deepEqual(calls, [['PrivacyApply', 'chrome-webrtc']], 'privacy failure invokes no tunnel or mode change');
  assert.ok(window.document.querySelector('.view.privacy'), 'failed operation keeps privacy page open');
  assert.match(window.document.getElementById('stage').textContent, /模拟写入失败/);
  vm.runInContext("nav('detection')", context); await settle();
  assert.match(window.document.getElementById('stage').textContent, /无法确认/);
  assert.match(window.document.getElementById('stage').textContent, /普通用户观察进程/);
  assert.match(window.document.getElementById('stage').textContent, /管理员助手：已授权/);
  const filter = window.document.getElementById('detection-filter'); filter.value = 'exposed'; filter.dispatchEvent(new window.Event('change'));
  assert.match(window.document.getElementById('stage').textContent, /语言可见/);
  assert.doesNotMatch(window.document.getElementById('stage').textContent, /序列号未知/, 'unknown is not relabeled as protected or observed');
  const audience = window.document.getElementById('detection-audience'); audience.value = '应用'; audience.dispatchEvent(new window.Event('change'));
  assert.match(window.document.getElementById('stage').textContent, /没有符合筛选/);
  assert.deepEqual(calls, [['PrivacyApply', 'chrome-webrtc']], 'read-only detection and filters invoke no privileged mutations');
  window.document.getElementById('detection-export').click(); await settle();
  window.document.getElementById('detection-preview').click(); await settle();
  window.document.getElementById('detection-save').click(); await settle();
  assert.match(window.document.getElementById('toast').textContent, /已保存/);
  assert.doesNotMatch(window.document.body.textContent, /SensitiveLocalFolder/, 'export result path is not inserted into the UI');
  cancelExport = true;
  window.document.getElementById('detection-export').click(); await settle();
  window.document.getElementById('detection-preview').click(); await settle();
  window.document.getElementById('detection-save').click(); await settle();
  assert.match(window.document.getElementById('toast').textContent, /取消保存/);
  assert.ok(window.document.querySelector('.view.detection'), 'canceled export stays on detection page');
  vm.runInContext("nav('privacy')", context); await settle();
  assert.ok(window.document.querySelector('[data-privacy-settings="dateandtime"]'), 'manual timezone item has an actionable OS settings entry');
  assert.equal(window.document.querySelector('[data-privacy-apply="timezone-language"]'), null, 'manual settings do not present fake automatic modifications');
  window.document.querySelector('[data-privacy-tab="data"]').click();
  assert.equal(calls.filter(call => call[0] === 'PrivacyInspectImage' || call[0] === 'PrivacyCleanImage').length, 0, 'opening data tools does not inspect files');
  window.document.getElementById('privacy-image-inspect').click(); await settle();
  assert.match(window.document.getElementById('stage').textContent, /eXIf/);
  assert.match(window.document.getElementById('stage').textContent, /128/);
  assert.doesNotMatch(window.document.body.textContent, /GPS_CONTENT_MUST_NOT_BE_RENDERED/, 'metadata content is not displayed');
  cancelInspect = true;
  window.document.getElementById('privacy-image-inspect').click(); await settle();
  assert.match(window.document.getElementById('toast').textContent, /取消文件选择/);
  assert.match(window.document.getElementById('stage').textContent, /eXIf/, 'canceled file picker preserves labeled earlier report');
  window.document.getElementById('privacy-image-clean').click(); await settle();
  assert.match(window.document.getElementById('stage').textContent, /模拟图片处理失败/);
  assert.ok(window.document.querySelector('.view.privacy'), 'image failure keeps the original privacy page');
  failClean = false;
  window.document.getElementById('privacy-image-clean').click(); await settle();
  assert.match(window.document.getElementById('stage').textContent, /副本中已移除/);
  assert.match(window.document.getElementById('stage').textContent, /C:\\Fixture\\clean.png/);
  assert.match(window.document.getElementById('stage').textContent, /ICC/);
  assert.ok(window.document.querySelector('[data-privacy-nav="settings"]'), 'original log settings entry is retained');
  assert.ok(window.document.querySelector('[data-privacy-nav="logs"]'), 'original diagnostic log entry is retained');
  assert.equal(window.localStorage.length, 0, 'file paths and metadata are not persisted by the UI');
  assert.equal(browserLogs.length, 0, 'file paths and metadata are not logged');
  window.document.querySelector('[data-privacy-tab="system"]').click();
  assert.equal(window.document.getElementById('privacy-time-choice').value, 'China Standard Time', 'time picker starts at actual current ID');
  assert.equal(window.document.getElementById('privacy-time-apply').disabled, true, 'unchanged time selection cannot create a needless write');
  const callsBeforeTime = calls.length;
  let picker = window.document.getElementById('privacy-time-choice'); picker.value = 'UTC'; picker.dispatchEvent(new window.Event('change'));
  assert.equal(calls.length, callsBeforeTime, 'changing time selection never auto-applies');
  assert.match(window.document.getElementById('privacy-time-card').textContent, /待应用/);
  window.document.getElementById('privacy-time-apply').click(); await settle();
  assert.equal(calls.length, callsBeforeTime, 'time application requires separate confirmation');
  window.document.getElementById('dialog-yes').click(); await settle();
  assert.equal(timeCurrent, 'China Standard Time');
  assert.equal(window.document.getElementById('privacy-time-choice').value, 'UTC', 'failed write preserves pending choice');
  assert.match(window.document.getElementById('privacy-time-card').textContent, /模拟时区修改失败/);
  const readsAfterFailure = timeReads;
  window.document.getElementById('privacy-time-refresh').click(); await settle();
  assert.ok(timeReads > readsAfterFailure, 'time refresh reads the OS report');
  assert.equal(window.document.getElementById('privacy-time-choice').value, 'UTC', 'read-only refresh preserves a valid pending choice');
  failTime = false;
  window.document.getElementById('privacy-time-apply').click(); await settle();
  window.document.getElementById('dialog-yes').click(); await settle();
  assert.equal(timeCurrent, 'UTC');
  assert.equal(window.document.getElementById('privacy-time-choice').value, 'UTC');
  assert.match(window.document.getElementById('privacy-time-card').textContent, /当前 ID：UTC/);
  assert.ok(window.document.getElementById('privacy-time-restore'), 'time has its own restore action');
  window.document.getElementById('privacy-time-restore').click(); await settle();
  window.document.getElementById('dialog-yes').click(); await settle();
  assert.equal(timeCurrent, 'China Standard Time');
  assert.equal(window.document.getElementById('privacy-time-choice').value, 'China Standard Time');
  assert.equal(window.document.getElementById('privacy-time-restore'), null);
  timeMode = 'empty'; window.document.getElementById('privacy-time-refresh').click(); await settle();
  assert.equal(window.document.getElementById('privacy-time-choice'), null, 'no OS choices means no invented options');
  assert.ok(window.document.querySelector('#privacy-time-card [data-privacy-settings="dateandtime"]'));
  timeMode = 'unavailable'; window.document.getElementById('privacy-time-refresh').click(); await settle();
  assert.equal(window.document.getElementById('privacy-time-choice'), null, 'unavailable API does not reuse stale choices');
  assert.match(window.document.getElementById('privacy-time-card').textContent, /上次读回夏令时偏好（当前未确认）/);
  assert.match(window.document.getElementById('privacy-time-card').textContent, /上次报告：/);
  report.observerElevated = true;
  vm.runInContext("nav('privacy')", context); await settle();
  assert.match(window.document.querySelector('.privacy-hero').textContent, /已提升进程观察/);
  assert.doesNotMatch(window.document.querySelector('.privacy-hero').textContent, /普通权限进程观察/);
  vm.runInContext("nav('privacy', {controlId:'read-only-policy'})", context); await settle();
  assert.ok(scrollTargets.includes('read-only-policy'), 'detection navigation locates manual controls without apply buttons');
  assert.match(window.document.querySelector('[data-privacy-control="read-only-policy"]').textContent, /只读证据：固定策略值符合目标/);
  assert.equal(window.document.querySelector('[data-privacy-control="new-state"] .privacy-badge').textContent, '无法确认', 'future states cannot claim safety');
  for (const tab of ['system', 'network', 'browser', 'data', 'boundaries']) {
    window.document.querySelector('[data-privacy-tab="' + tab + '"]').click();
    assertHelpCoverage(window.document.querySelector('.view.privacy'));
  }
  window.document.querySelector('[data-privacy-tab="network"]').click();
  assert.match(window.document.getElementById('privacy-panel').textContent, /2 条无法识别的过滤器/);
  window.document.querySelector('[data-privacy-tab="browser"]').click();
  vm.runInContext("privacyLocalProbe = async () => ({'模拟浏览器项':'固定值，未读取真实硬件'})", context);
  window.document.getElementById('privacy-probe').click(); await settle();
  assert.ok(window.document.querySelector('.privacy-probe dt .privacy-help'), 'individual probe result has a lightweight explanation');
  vm.runInContext("nav('detection')", context); await settle();
  assertHelpCoverage(window.document.querySelector('.view.detection'));
  failDetection = true; window.document.getElementById('detection-refresh').click(); await settle();
  assert.match(window.document.getElementById('stage').textContent, /历史结果，当前未确认/);
  assert.equal(window.document.getElementById('detection-export').disabled, true, 'stale detection cannot export');
  failDetection = false; window.document.getElementById('detection-refresh').click(); await settle();
  assert.equal(window.document.getElementById('detection-export').disabled, false);
  detection.findings.push({id:'browser-test',title:'浏览器入口测试',category:'浏览器',audience:'website',status:'unknown',summary:'需实际浏览器',settingsId:'browser'});
  window.document.getElementById('detection-refresh').click(); await settle();
  window.document.querySelector('[data-detection-settings="browser"]').click(); await settle();
  assert.equal(window.document.querySelector('[data-privacy-tab="browser"]').getAttribute('aria-selected'), 'true');
  assert.equal(calls.filter(call => call[0] === 'PrivacyOpenSettings' && call[1] === 'browser').length,0,'browser guidance does not open default apps');
  const beforeNavigationWrite = calls.filter(call => call[0] === 'PrivacyApply').length;
  window.document.querySelector('[data-privacy-apply="chrome-webrtc"]').click(); await settle();
  vm.runInContext("nav('detection')", context); await settle();
  const oldConfirm = window.document.getElementById('dialog-yes'); if (oldConfirm) oldConfirm.click(); await settle();
  assert.equal(calls.filter(call => call[0] === 'PrivacyApply').length, beforeNavigationWrite, 'old page confirmation cannot submit');
  vm.runInContext("nav('privacy', {tab:'browser'})", context); await settle();
  failPolicyReport = true; window.document.getElementById('privacy-refresh').click(); await settle();
  assert.match(window.document.getElementById('stage').textContent,/历史结果，当前未确认/);
  assert.equal(window.document.querySelector('[data-privacy-apply="chrome-webrtc"]'),null,'failed refresh cannot reuse writable old evidence');
  failPolicyReport = false; window.document.getElementById('privacy-refresh').click(); await settle();
  assert.ok(window.document.querySelector('[data-privacy-apply="chrome-webrtc"]'));
  helperState='lost'; window.document.getElementById('privacy-refresh').click(); await settle();
  window.document.querySelector('[data-privacy-tab="network"]').click();
  window.document.getElementById('privacy-program').click(); await settle();
  assert.equal(window.document.getElementById('privacy-firewall-add').disabled,true,'lost helper cannot submit WFP rules');
  window.document.getElementById('privacy-authorize').click(); await settle();
  window.document.getElementById('dialog-yes').click(); await settle();
  assert.equal(calls.filter(call=>call[0]==='PrivacyAuthorize').length,1);
  assert.equal(window.document.getElementById('privacy-firewall-add').disabled,false);
  window.document.getElementById('privacy-firewall-add').click(); await settle();
  window.document.getElementById('dialog-yes').click(); await settle();
  assert.match(window.document.getElementById('toast').textContent,/过滤器已安装/);
  assert.ok(!window.document.getElementById('toast').classList.contains('err'),'scope warning does not turn successful install red');
  failFirewallReport=true; window.document.getElementById('privacy-refresh').click(); await settle();
  assert.match(window.document.getElementById('privacy-panel').textContent,/历史结果，当前未确认/);
  assert.equal(window.document.querySelector('[data-privacy-remove-rule]').disabled,true);
  failFirewallReport=false; window.document.getElementById('privacy-refresh').click(); await settle();
  window.document.querySelector('[data-privacy-remove-rule]').click(); await settle();
  window.document.getElementById('dialog-yes').click(); await settle();
  assert.match(window.document.getElementById('toast').textContent,/规则已移除/);
  assert.ok(!window.document.getElementById('toast').classList.contains('err'));
  assert.equal(calls.filter(call => /^(Connect|Disconnect|SetMode)$/.test(call[0])).length, 0, 'data tools and export do not alter the tunnel');
  assert.deepEqual(uncaught, [], 'fast navigation and early confirmation leave no deferred focus errors');
  console.log('UI integration passed: navigation, confirmation, tooltips, failure isolation, detection, image tools, export, timezone, observer identity.');
}
run().then(() => dom.window.close()).catch(error => { console.error(error); dom.window.close(); process.exitCode = 1; });
