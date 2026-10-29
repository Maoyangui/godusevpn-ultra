// Isolated DOM and fake bridge only. No native clipboard, service or network.
const assert = require('node:assert/strict');
const fs = require('node:fs'), path = require('node:path'), vm = require('node:vm');
const {JSDOM} = require(process.argv[2] || 'jsdom');
const root = path.resolve(__dirname, '../../..');
const read = name => fs.readFileSync(path.join(root, 'web/dist', name), 'utf8');
const dom = new JSDOM(read('index.html').replace(/<script[\s\S]*?<\/script>/g,''), {runScripts:'outside-only',url:'http://example.invalid/',pretendToBeVisual:true});
const w=dom.window, q=s=>w.document.querySelector(s), calls=[],errors=[];
const state={platform:'windows',version:'fixture',service:true,svcState:'running',lang:'zh',theme:'light',view:{profiles:[{id:'fixture'}],settings:{},state:{status:'disconnected'}}};
let report={supported:true,options:{enabled:false,delaySeconds:30},pending:false,lastCode:'idle',closed:false};
let settingsFail=false,saveFail=false,resolveSave=null,blockSave=false,resolveReport=null,blockReport=false;
const clone=value=>JSON.parse(JSON.stringify(value));
const api={GetState:async()=>state,GetSettings:async()=>{if(settingsFail)throw Error('SERVICE_DOWN');return {};},GetAutostart:async()=>false,
  GetProfiles:async()=>[{id:'fixture',name:'Synthetic profile',url:'https://example.invalid/sub?token=DO_NOT_COPY_FROM_JS',active:true,nodeCount:1}],
  ClipboardPrivacyReport:async()=>{if(blockReport)return new Promise(resolve=>{resolveReport=resolve;});return clone(report);},
  ClipboardPrivacySave:async options=>{calls.push(['save',clone(options)]);if(blockSave)return new Promise(resolve=>{resolveSave=resolve;});if(saveFail)throw Error('SYNTHETIC_ERROR_NO_SECRET');report={...report,options:clone(options),lastCode:options.enabled?'idle':'disabled'};return clone(report);},
  ClipboardPrivacyCopyProfile:async id=>{calls.push(['copy',id]);report={...report,pending:report.options.enabled,lastCode:report.options.enabled?'pending':'copied_without_timer',deadlineUnixMilli:Date.now()+30000};return clone(report);},
  ClipboardPrivacyCancel:async()=>{calls.push(['cancel']);report={...report,pending:false,lastCode:'cancelled'};return clone(report);},
  ClipboardPrivacyClearNow:async()=>{calls.push(['clear']);report={...report,pending:false,lastCode:'changed_preserved'};return clone(report);}
};
w.go={main:{App:new Proxy(api,{get:(o,k)=>o[k]||(()=>{calls.push([k]);return Promise.resolve();})})}};
w.runtime={EventsOn:()=>{}};w.HTMLElement.prototype.scrollIntoView=function(){};
w.navigator.clipboard={writeText:async()=>{throw Error('secret must not use JS clipboard');}};
w.document.execCommand=()=>{throw Error('secret must not use copy fallback');};
w.addEventListener('error',e=>errors.push(String(e.error||e.message)));
const ctx=dom.getInternalVMContext();
for(const file of ['i18n.js','flags.js','privacy.js','app.js'])vm.runInContext(read(file),ctx,{filename:file});
const settle=()=>new Promise(resolve=>setTimeout(resolve,30));
const nav=name=>vm.runInContext(`nav(${JSON.stringify(name)})`,ctx);
async function run(){
  await settle();nav('settings');await settle();
  assert.equal(q('#clip-enabled').checked,false);assert.equal(q('#clip-delay').min,'5');assert.equal(q('#clip-delay').max,'600');
  assert.equal(q('#clip-cancel').disabled,true);assert.equal(q('#clip-clear').disabled,true);
  q('#clip-delay').value='4';q('#clip-save').click();await settle();assert.equal(calls.filter(c=>c[0]==='save').length,0);
  q('#clip-delay').value='45.5';q('#clip-save').click();await settle();assert.equal(calls.filter(c=>c[0]==='save').length,0);
  q('#clip-delay').value='45';q('#clip-enabled').checked=true;q('#clip-save').click();await settle();
  assert.deepEqual(calls.find(c=>c[0]==='save'),['save',{enabled:true,delaySeconds:45}]);
  assert.equal(q('#save').disabled,true);assert.equal(calls.some(c=>c[0]==='SaveSettings'),false);
  saveFail=true;q('#clip-save').click();await settle();assert.equal(q('#clip-save').disabled,false);assert.match(q('#clip-status').textContent,/操作未完成/);saveFail=false;
  nav('profiles');await settle();q('[data-act="copy-secret"]').click();await settle();
  assert.deepEqual(calls.find(c=>c[0]==='copy'),['copy','fixture']);assert.equal(JSON.stringify(calls).includes('DO_NOT_COPY_FROM_JS'),false);
  nav('settings');await settle();assert.equal(q('#clip-cancel').disabled,false);q('#clip-cancel').click();await settle();assert.equal(q('#clip-clear').disabled,true);
  report={...report,pending:true,lastCode:'pending'};nav('settings');await settle();q('#clip-clear').click();await settle();assert.match(q('#clip-status').textContent,/已改变/);
  settingsFail=true;nav('settings');await settle();assert.ok(q('#clipboard-settings'));q('#clip-delay').value='60';q('#clip-save').click();await settle();assert.equal(report.options.delaySeconds,60);settingsFail=false;
  blockSave=true;nav('settings');await settle();q('#clip-save').click();await settle();nav('profiles');await settle();resolveSave(clone(report));await settle();assert.equal(q('#clipboard-settings'),null);assert.ok(q('[data-act="copy-secret"]'));blockSave=false;
  blockReport=true;nav('settings');await settle();nav('profiles');await settle();resolveReport(clone(report));await settle();assert.equal(q('#clipboard-settings'),null);assert.ok(q('[data-act="copy-secret"]'));blockReport=false;
  delete api.ClipboardPrivacyCopyProfile;nav('profiles');await settle();assert.equal(q('[data-act="copy-secret"]'),null);
  assert.deepEqual(errors,[]);console.log(JSON.stringify({scope:'isolated DOM',defaultOff:true,boundedDuration:true,separateSettings:true,profileIDOnly:true,noJSSecretCopy:true,cancelAndConditionalClear:true,serviceDownSettings:true,stalePageBlocked:true}));
}
run().catch(e=>{console.error(e);process.exitCode=1;}).finally(()=>dom.window.close());
