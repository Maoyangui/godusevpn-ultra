const assert = require('node:assert/strict');
const { cleanLink } = require('./clean-link.js');
const { Engine } = require('./core.js');
const manifest = require('./manifest.json');
function mock(options = {}) {
  const data = {}, actions = [];
  let userValue = true, extensionValue, control = options.control || 'controllable_by_this_extension';
  const api = { runtime: {}, storage: { local: {
    get(keys, cb) { cb(data); }, set(values, cb) { Object.assign(data, JSON.parse(JSON.stringify(values))); cb(); }
  } }, privacy: { network: { networkPredictionEnabled: {
    get(details, cb) { cb({ value: extensionValue === undefined ? userValue : extensionValue, levelOfControl: control }); },
    set(details, cb) { actions.push(['set', details.value]); extensionValue = options.mismatch ? true : details.value; control = 'controlled_by_this_extension'; cb(); },
    clear(details, cb) { actions.push(['clear']); extensionValue = undefined; control = 'controllable_by_this_extension'; cb(); }
  } } }, contentSettings: {} };
  return { api, data, actions, userChange(value) { userValue = value; } };
}
async function run() {
  assert.equal(manifest.host_permissions, undefined, 'extension cannot read arbitrary webpages');
  assert.equal(manifest.content_scripts, undefined, 'extension does not inject fingerprint overrides');
  assert.match(manifest.content_security_policy.extension_pages, /connect-src 'none'/, 'extension pages cannot contact remote services');
  const plain = 'https://example.com/a?x=a%20b#part';
  assert.equal(cleanLink(plain).url, plain, 'untouched queries preserve exact encoding');
  assert.equal(cleanLink('https://example.com/a?utm_source=x&ok=y').url, 'https://example.com/a?ok=y');
  assert.equal(cleanLink('https://example.com/a?UTM_SOURCE=x&fbclid=z#x').url, 'https://example.com/a#x');
  for (const url of ['https://example.com/oauth/callback?utm_source=x', 'https://example.com/pay?utm_source=x', 'https://example.com/a?X-Amz-Signature=x&utm_source=y', 'https://example.com/a?token=x&utm_source=y']) {
    assert.equal(cleanLink(url).url, url); assert.equal(cleanLink(url).skipped, true);
  }
  assert.throws(() => cleanLink('javascript:alert(1)'));
  assert.throws(() => cleanLink('https://user:pass@example.com/a'));
  const blocked = mock({ control: 'controlled_by_other_extensions' });
  await assert.rejects(new Engine(blocked.api).apply('prediction'), /策略|控制/);
  assert.equal(blocked.actions.length, 0, 'no mutation when another extension owns the setting');
  const normal = mock(), engine = new Engine(normal.api);
  await engine.report();
  assert.deepEqual(normal.actions, [], 'opening the panel only reads and never auto-applies protections');
  await engine.apply('prediction');
  assert.equal(normal.data.managed.prediction.original, true);
  assert.deepEqual(normal.actions, [['set', false]]);
  normal.userChange(false);
  await engine.restore('prediction');
  assert.deepEqual(normal.actions, [['set', false], ['clear']], 'restore clears only the extension layer');
  assert.equal((await engine.read({ type: 'privacy', path: 'network.networkPredictionEnabled' })).value, false, 'new underlying user setting is preserved');
  assert.equal(normal.data.managed.prediction, undefined);
  const mismatch = mock({ mismatch: true });
  await assert.rejects(new Engine(mismatch.api).apply('prediction'), /读回/);
  assert.ok(mismatch.data.managed.prediction, 'failed verification retains a reversible management record');
  assert.equal((await new Engine(normal.api).read({ type: 'privacy', path: 'network.absent' })).supported, false);
  assert.equal(JSON.stringify(normal.data).includes('https://'), false, 'journal contains no browsing URLs');
  console.log('Browser extension regression passed; no real browser settings changed.');
}
run().catch(error => { console.error(error); process.exitCode = 1; });
