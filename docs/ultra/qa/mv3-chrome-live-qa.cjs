'use strict';
// Development integration only. Uses one newly created TEMP profile, retains
// it on every outcome, never deletes directories or changes Windows policies.
// Explicit existing Playwright module and system Chrome executable arguments.
const fs = require('fs');
const path = require('path');
const os = require('os');
const crypto = require('crypto');

const result = {
  schema: 'ultra-mv3-chrome-live-v1', status: 'not_tested', error: '',
  scope: 'current_host_fresh_temp_headless_chrome_only', mockBrowserAPI: false,
  browserVersion: '', manifestVersion: 3, extensionWorkerLoaded: false,
  productCoordinatorUsed: false, profileRetained: true, profileLeaf: '',
  pageRequestCounts: { extension: 0, blank: 0, otherBlocked: 0 },
  pageRequestsAreNotWholeProcessTraffic: true,
  remoteNavigationPerformed: false, windowsPolicyModified: false,
  existingProfileUsed: false, hardwareOrWebRTCProbeRun: false,
  cases: [], queue: { status: 'not_tested' }, sourceSHA256: {},
};
const output = path.join(__dirname, 'MV3_CHROME_INTEGRATION_RESULT.json');
let context;
const hash = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
function requireAbsoluteExisting(value, file) {
  if (!value || !path.isAbsolute(value)) throw Error('invalid_existing_tool_path');
  const info = fs.statSync(value);
  if (file ? !info.isFile() : !info.isDirectory()) throw Error('invalid_existing_tool_path');
  return value;
}

async function main() {
  if (fs.existsSync(output)) throw Error('result_already_exists');
  const modulePath = requireAbsoluteExisting(process.argv[2], false);
  const chromeExecutable = requireAbsoluteExisting(process.argv[3], true);
  if (path.basename(chromeExecutable).toLowerCase() !== 'chrome.exe') throw Error('system_chrome_required');
  const { chromium } = require(modulePath);
  const extension = path.resolve(__dirname, '..', '..', '..', 'browser-extension');
  const manifest = JSON.parse(fs.readFileSync(path.join(extension, 'manifest.json'), 'utf8'));
  if (manifest.manifest_version !== 3 || manifest.background.service_worker !== 'background.js') throw Error('unexpected_extension');
  for (const file of ['manifest.json', 'core.js', 'background.js', 'panel.html', 'panel.js']) result.sourceSHA256[file] = hash(path.join(extension, file));

  const temp = path.resolve(os.tmpdir());
  const fresh = fs.mkdtempSync(path.join(temp, 'ultra-mv3-chrome-live-'));
  const relative = path.relative(temp, fresh);
  if (!relative || relative.startsWith('..') || path.isAbsolute(relative) || fs.lstatSync(fresh).isSymbolicLink()) throw Error('fresh_profile_boundary_failed');
  result.profileLeaf = path.basename(fresh);
  const profile = path.join(fresh, 'profile');
  fs.mkdirSync(profile, { recursive: false });
  try {
    context = await chromium.launchPersistentContext(profile, {
      executablePath: chromeExecutable, headless: true, timeout: 30000,
      ignoreDefaultArgs: ['--disable-extensions'],
      args: [
        `--disable-extensions-except=${extension}`, `--load-extension=${extension}`,
        '--disable-background-networking', '--disable-component-update',
        '--disable-sync', '--disable-domain-reliability', '--disable-default-apps',
        '--no-first-run', '--no-default-browser-check', '--metrics-recording-only',
        '--disable-features=OptimizationHints,MediaRouter,AutofillServerCommunication,Translate',
        '--headless=new',
      ],
    });
  } catch (_) { throw Error('chrome_launch_failed'); }
  result.browserVersion = context.browser().version();
  context.on('request', request => {
    const url = request.url();
    if (url.startsWith('chrome-extension://')) result.pageRequestCounts.extension++;
    else if (url === 'about:blank') result.pageRequestCounts.blank++;
  });
  await context.route('**/*', route => {
    const url = route.request().url();
    if (url.startsWith('chrome-extension://') || url === 'about:blank') return route.continue();
    result.pageRequestCounts.otherBlocked++;
    return route.abort();
  });
  let worker = context.serviceWorkers().find(entry => entry.url().startsWith('chrome-extension://') && entry.url().endsWith('/background.js'));
  if (!worker) {
    try { worker = await context.waitForEvent('serviceworker', { timeout: 15000, predicate: entry => entry.url().startsWith('chrome-extension://') && entry.url().endsWith('/background.js') }); }
    catch (_) { result.error = 'extension_worker_unavailable'; return; }
  }
  const identity = await worker.evaluate(() => ({ id: chrome.runtime.id, version: chrome.runtime.getManifest().version, mv: chrome.runtime.getManifest().manifest_version, coordinator: typeof coordinator !== 'undefined' }));
  if (identity.mv !== 3 || identity.version !== manifest.version || !identity.coordinator || !/^[a-p]{32}$/.test(identity.id)) throw Error('extension_worker_identity_mismatch');
  result.extensionWorkerLoaded = true;
  const page = await context.newPage();
  await page.goto(`chrome-extension://${identity.id}/panel.html`, { timeout: 10000 });
  // Uses the existing Client -> runtime message -> real background Coordinator.
  // No browser API is stubbed, instrumented or replaced. The sample origin is
  // only a contentSettings query parameter, never a navigation or request.
  const run = page.evaluate(async () => {
    const client = new UltraPrivacy.Client(chrome);
    const origin = 'https://example.invalid';
    const getManaged = () => new Promise((resolve, reject) => chrome.storage.local.get(['managed'], data => chrome.runtime.lastError ? reject(Error('journal_read_failed')) : resolve(data.managed || {})));
    const report = async id => (await client.report()).find(item => item.id === id);
    const sample = item => ({ supported: !!item.supported, unknown: !!item.unknown, value: item.value, control: item.control || '', writable: !!item.writable, managed: !!item.managed, stage: item.stage || '' });
    const equal = (a, b) => JSON.stringify(a) === JSON.stringify(b);
    const cases = [];
    if (Object.keys(await getManaged()).length) throw Error('fresh_journal_not_empty');
    for (const id of ['webrtc', 'prediction', 'thirdparty']) {
      const before = await report(id);
      const row = { id, status: 'not_tested', before: sample(before), applyAcknowledged: false, restored: false, journalRemoved: false };
      cases.push(row);
      if (!before.supported || before.unknown) { row.status = 'unsupported_or_unknown'; continue; }
      if (!before.writable || before.control === 'controlled_by_this_extension') { row.status = 'not_controllable_or_unexpected_owner'; continue; }
      let failed = false;
      try {
        const applied = await client.apply(id); row.applyAcknowledged = true;
        const after = await report(id), journal = await getManaged(), own = journal[id];
        row.after = sample(after);
        row.appliedReadback = equal(applied.value, before.desired) && equal(after.value, before.desired) && after.control === 'controlled_by_this_extension';
        row.journalActive = !!own && own.stage === 'active' && equal(own.original, before.value) && equal(own.desired, before.desired);
        if (!row.appliedReadback || !row.journalActive) failed = true;
      } catch (_) { failed = true; row.applyOutcome = 'failed_or_unknown'; }
      // One distinct restore of this test's own record, never an apply retry.
      const managed = await getManaged();
      if (managed[id]) {
        try {
          const restored = await client.restore(id), after = await report(id);
          row.restoreReadback = sample(after);
          row.restored = restored.clearAcknowledged === true && restored.verified === true && equal(after.value, before.value) && after.control !== 'controlled_by_this_extension';
          row.journalRemoved = !(await getManaged())[id];
        } catch (_) { row.restoreOutcome = 'failed_or_unknown'; }
      }
      row.status = !failed && row.restored && row.journalRemoved ? 'passed_setting_roundtrip' : 'failed_or_unknown';
      if (row.status !== 'passed_setting_roundtrip') return { cases, queue: { status: 'not_tested_after_uncertain_case' }, finalJournalEmpty: Object.keys(await getManaged()).length === 0 };
    }
    const contentID = 'content-notifications';
    const contentBefore = await report(contentID);
    const content = { id: contentID, status: 'not_tested', before: sample(contentBefore), source: origin, modeReadback: [], controlLevelUnavailable: true, verifiedScope: 'single_origin_API_sample_only' };
    cases.push(content);
    if (contentBefore.supported && !contentBefore.unknown) {
      let okay = true;
      for (const value of ['block', 'ask']) {
        try {
          await client.apply(contentID, { value, origin });
          const raw = await new Promise((resolve, reject) => chrome.contentSettings.notifications.get({ primaryUrl: origin + '/', secondaryUrl: origin + '/', incognito: false }, data => chrome.runtime.lastError ? reject(Error('content_read_failed')) : resolve(data)));
          const own = (await getManaged())[contentID];
          content.modeReadback.push({ requested: value, actual: raw.setting, journalActive: !!own && own.stage === 'active' && own.origin === origin && own.desired === value && equal(own.original, contentBefore.value) });
          if (raw.setting !== value || !content.modeReadback.at(-1).journalActive) { okay = false; break; }
        } catch (_) { okay = false; break; }
      }
      if ((await getManaged())[contentID]) {
        try {
          const restored = await client.restore(contentID), after = await report(contentID);
          content.clearAcknowledged = restored.clearAcknowledged === true;
          content.universalRestoreNotClaimed = restored.verified === false;
          content.sampleReturnedToBaseline = equal(after.value, contentBefore.value);
          content.journalRemoved = !(await getManaged())[contentID];
        } catch (_) { okay = false; }
      }
      content.status = okay && content.clearAcknowledged && content.universalRestoreNotClaimed && content.sampleReturnedToBaseline && content.journalRemoved ? 'passed_single_origin_sample_roundtrip' : 'failed_or_unknown';
      if (content.status !== 'passed_single_origin_sample_roundtrip') return { cases, queue: { status: 'not_tested_after_uncertain_case' }, finalJournalEmpty: Object.keys(await getManaged()).length === 0 };
    } else content.status = 'unsupported_or_unknown';

    const queue = { status: 'not_tested', simultaneousRequests: 5 };
    const beforeQueue = await report('prediction');
    if (beforeQueue.supported && beforeQueue.writable && !beforeQueue.unknown) {
      const replies = await Promise.allSettled([
        client.apply('prediction'), client.restore('prediction'), client.apply('prediction'), client.report(), client.restore('prediction'),
      ]);
      queue.allAcknowledged = replies.every(reply => reply.status === 'fulfilled');
      const orderedReport = replies[3].status === 'fulfilled' && replies[3].value.find(item => item.id === 'prediction');
      queue.orderedIntermediateActive = !!orderedReport && orderedReport.managed === true && orderedReport.stage === 'active' && orderedReport.control === 'controlled_by_this_extension';
      const after = await report('prediction');
      queue.baselineRestored = equal(after.value, beforeQueue.value) && after.control !== 'controlled_by_this_extension';
      queue.journalRemoved = !(await getManaged()).prediction;
      queue.status = queue.allAcknowledged && queue.orderedIntermediateActive && queue.baselineRestored && queue.journalRemoved ? 'passed_real_background_serialization' : 'failed_or_unknown';
    } else queue.status = 'unsupported_or_not_controllable';
    return { cases, queue, finalJournalEmpty: Object.keys(await getManaged()).length === 0 };
  });
  let deadline;
  const details = await Promise.race([run, new Promise((_, reject) => { deadline = setTimeout(() => reject(Error('operation_deadline_unknown')), 90000); })]).finally(() => clearTimeout(deadline));
  Object.assign(result, details);
  result.productCoordinatorUsed = true;
  result.status = details.finalJournalEmpty && details.cases.every(row => row.status.startsWith('passed_')) && details.queue.status.startsWith('passed_') ? 'passed_limited_integration' : 'partial_or_unknown';
}

(async () => {
  try { await main(); }
  catch (error) {
    const known = ['invalid_existing_tool_path', 'system_chrome_required', 'unexpected_extension', 'fresh_profile_boundary_failed', 'chrome_launch_failed', 'extension_worker_identity_mismatch', 'operation_deadline_unknown', 'fresh_journal_not_empty', 'result_already_exists'];
    result.error = known.includes(error.message) ? error.message : 'integration_failed_or_unknown';
    result.status = result.extensionWorkerLoaded ? 'failed_or_unknown' : 'not_tested';
  } finally {
    if (context) { try { await context.close(); } catch (_) { result.closeUnconfirmed = true; } }
  }
  if (!fs.existsSync(output)) fs.writeFileSync(output, JSON.stringify(result, null, 2) + '\n', { encoding: 'utf8', flag: 'wx' });
  process.stdout.write(JSON.stringify({ schema: result.schema, status: result.status, error: result.error, extensionWorkerLoaded: result.extensionWorkerLoaded, cases: result.cases.length, profileRetained: true }) + '\n');
  process.exitCode = result.status === 'passed_limited_integration' ? 0 : 2;
})();
