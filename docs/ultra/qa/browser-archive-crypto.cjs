// Archive crypto and dangerous-field regression. This executes the same
// browser-side helper body with WebCrypto; it never starts a browser.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const crypto = require('node:crypto').webcrypto;

const source = fs.readFileSync(require('node:path').resolve(__dirname, '../../../web/dist/privacy.js'), 'utf8');
assert.match(source, /browserArchiveDiff\(archive\)/, 'archive import must build a difference preview');
assert.match(source, /const confirmed=await askConfirm\(diff/, 'archive import must require explicit confirmation');
assert.match(source, /已取消，当前草稿未改变/, 'archive cancel path must preserve the current draft');
const start = source.indexOf('  const archiveDangerousKey =');
const end = source.indexOf('  function browserV2GenerateSeed', start);
assert.ok(start >= 0 && end > start, 'archive helper body must exist');
const helper = source.slice(start, end);
const context = vm.createContext({ crypto, TextEncoder, TextDecoder, btoa, atob, JSON, Error, Uint8Array, Array, Object, String, Number, Boolean, Date });
const run = vm.runInContext(`(async()=>{${helper};
  const deep={}; let cursor=deep; for(let i=0;i<17;i++){cursor.next={};cursor=cursor.next;} cursor.panelToken='deep-token';
  const dirty={binding:{mode:'tunnel'},servicePid:12,proxyCredentials:{password:'secret'},panelToken:'token',extraArgs:['--unsafe'],apiKey:'api-secret',client_secret:'client-secret',safe:'kept',nested:{service_pid:4,keep:true},deep};
  const cleaned=browserArchiveClean(dirty);
  const encrypted=await browserArchiveEncrypt({archiveVersion:1,kind:'godusevpn-ultra-browser-profile',profile:{config:cleaned}},'a-long-test-password');
  const restored=await browserArchiveDecrypt(encrypted,'a-long-test-password');
  let tampered=false;
  try { const copy={...encrypted, encryptedPayload:encrypted.encryptedPayload.slice(0,-1)+'A'}; await browserArchiveDecrypt(copy,'a-long-test-password'); } catch (_) { tampered=true; }
  return {cleaned,encrypted,restored,tampered};
})()`, context);
run.then(result=>{
  assert.equal(result.cleaned.binding,undefined);
  assert.equal(result.cleaned.servicePid,undefined);
  assert.equal(result.cleaned.proxyCredentials,undefined);
  assert.equal(result.cleaned.panelToken,undefined);
  assert.equal(result.cleaned.extraArgs,undefined);
  assert.equal(result.cleaned.apiKey,undefined);
  assert.equal(result.cleaned.client_secret,undefined);
  assert.equal(result.cleaned.safe,'kept');
  assert.equal(result.cleaned.nested.service_pid,undefined);
  assert.equal(result.cleaned.nested.keep,true);
  assert.equal(JSON.stringify(result.cleaned.deep).includes('deep-token'),false);
  for (const key of ['archiveVersion','kind','cipher','kdf','salt','nonce','authTag','encryptedPayload']) assert.ok(result.encrypted[key] !== undefined, key);
  assert.equal(result.encrypted.cipher,'AES-256-GCM');
  assert.equal(result.encrypted.kdf.iterations,210000);
  assert.equal(result.restored.profile.config.safe,'kept');
  assert.equal(result.tampered,true);
  console.log('browser archive AES-GCM, dangerous-field scrubbing, and tamper rejection passed');
}).catch(error=>{ console.error(error); process.exitCode=1; });
