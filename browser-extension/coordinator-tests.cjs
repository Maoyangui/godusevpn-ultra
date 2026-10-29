// Pure callback API/storage mocks; no browser settings, hosts or network.
const assert = require('node:assert/strict');
const { Engine, Coordinator, Client } = require('./core.js');
function fixture() {
  const data = {}, actions = [], state = {};
  let saves = 0, failAt = 0, readFailures = 0;
  const api = { runtime: {}, privacy: { network: {}, services: {} }, contentSettings: {}, storage: { local: {
    get(_keys, callback) { queueMicrotask(() => callback(JSON.parse(JSON.stringify(data)))); },
    set(values, callback) { saves++; queueMicrotask(() => { if (saves === failAt) { api.runtime.lastError = { message: 'synthetic storage failure' }; callback(); delete api.runtime.lastError; } else { Object.assign(data, JSON.parse(JSON.stringify(values))); callback(); } }); }
  } } };
  function setting(id, type) {
    state[id] = { user: type === 'content' ? 'ask' : true, extension: undefined };
    return {
      get(_query, callback) { queueMicrotask(() => { if (readFailures > 0) { readFailures--; api.runtime.lastError = {message:'synthetic read failure'}; callback(); delete api.runtime.lastError; return; } const s = state[id]; callback(type === 'content' ? { setting: s.extension === undefined ? s.user : s.extension } : { value: s.extension === undefined ? s.user : s.extension, levelOfControl: s.extension === undefined ? 'controllable_by_this_extension' : 'controlled_by_this_extension' }); }); },
      set(value, callback) { actions.push(['set', id, value]); assert.equal(data.managed[id].stage, 'applying', 'ownership must be durable before browser mutation'); state[id].extension = type === 'content' ? value.setting : value.value; queueMicrotask(callback); },
      clear(_query, callback) { actions.push(['clear', id]); state[id].extension = undefined; queueMicrotask(callback); }
    };
  }
  api.privacy.network.networkPredictionEnabled = setting('prediction');
  api.privacy.services.searchSuggestEnabled = setting('suggest');
  for (const kind of ['camera', 'microphone', 'location', 'notifications', 'clipboard', 'javascript', 'automaticDownloads']) api.contentSettings[kind] = setting('content-' + kind, 'content');
  const coordinator = new Coordinator(new Engine(api));
  api.runtime.sendMessage = (request, callback) => coordinator.request(request.action, request.id, request.options).then(result => callback({ ok: true, result }), error => callback({ ok: false, error: error.message }));
  return { api, data, actions, state, coordinator, failNext: offset => { failAt = saves + offset; }, failReads: count => { readFailures = count; } };
}
async function run() {
  const f = fixture(), a = new Client(f.api), b = new Client(f.api);
  await Promise.all([a.apply('prediction'), b.apply('suggest')]);
  assert.deepEqual(Object.keys(f.data.managed).sort(), ['prediction', 'suggest'], 'two panels retain both records');
  await Promise.all([a.restore('prediction'), b.apply('prediction'), a.restore('suggest')]);
  assert.deepEqual(Object.keys(f.data.managed), ['prediction'], 'interleaved restore/apply is serialized');
  const before = fixture(); before.failNext(1);
  await assert.rejects(new Client(before.api).apply('prediction'), /storage/);
  assert.equal(before.actions.length, 0, 'storage failure prevents browser write');
  const interrupted = fixture(); interrupted.failNext(2);
  await assert.rejects(new Client(interrupted.api).apply('prediction'), /storage/);
  assert.equal(interrupted.data.managed.prediction.stage, 'applying');
  const restarted = new Coordinator(new Engine(interrupted.api));
  assert.equal((await restarted.request('report')).find(item => item.id === 'prediction').stage, 'applying');
  await restarted.request('restore', 'prediction');
  assert.deepEqual(interrupted.data.managed, {}, 'prepared record remains restorable after worker restart');
  const recovery = fixture(), recoveryEngine = new Engine(recovery.api);
  await recoveryEngine.apply('prediction'); recovery.failReads(1);
  await recoveryEngine.restore('prediction');
  assert.equal(recovery.state.prediction.extension, undefined, 'recorded coverage can be cleared even if its preliminary read fails');
  await recoveryEngine.apply('prediction'); recovery.failReads(2);
  await assert.rejects(recoveryEngine.restore('prediction'), /read failure/);
  assert.equal(recovery.data.managed.prediction.stage, 'restoring', 'unverified clear retains recovery evidence');
  await recoveryEngine.restore('prediction');
  assert.deepEqual(recovery.data.managed, {}, 'later readback can finish recovery without rewriting a user value');
  const orphan = fixture(); orphan.state.prediction.extension = false;
  const e = new Engine(orphan.api);
  assert.equal((await e.report()).find(item => item.id === 'prediction').orphaned, true);
  await e.restore('prediction');
  assert.equal(orphan.state.prediction.extension, undefined, 'untracked own layer can be explicitly released');
  const modes = fixture(), engine = new Engine(modes.api);
  await assert.rejects(engine.apply('content-camera', { value: 'allow' }), /HTTPS/);
  await assert.rejects(engine.apply('content-microphone', { value: 'allow' }), /HTTPS/);
  await assert.rejects(engine.apply('content-javascript', { value: 'ask' }), /不支持/);
  assert.equal(modes.actions.length, 0, 'unsupported combinations never mutate');
  await engine.apply('content-camera', { value: 'allow', origin: 'https://fixture.invalid' });
  const camera = modes.actions.find(action => action[0] === 'set');
  assert.equal(camera[2].primaryPattern, 'https://fixture.invalid/*');
  assert.equal(camera[2].setting, 'allow');
  await engine.apply('content-location', { value: 'ask' });
  assert.equal(modes.data.managed['content-location'].desired, 'ask');
  await engine.restore('content-camera');
  assert.equal(modes.state['content-camera'].extension, undefined);
  for (const origin of ['https://user:pass@fixture.invalid', 'https://fixture.invalid/path', 'https://*.fixture.invalid', 'http://fixture.invalid']) await assert.rejects(engine.apply('content-camera', { value: 'allow', origin }), /来源|HTTPS/);
  console.log('Shared extension queue, crash records, orphan release and per-API permission modes passed (mock only).');
}
run().catch(error => { console.error(error); process.exitCode = 1; });
