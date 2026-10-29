// Isolated DOM fixtures only: no service, file picker, system data or network.
const assert = require('node:assert/strict');
const fs = require('node:fs'), path = require('node:path'), vm = require('node:vm');
const {JSDOM} = require(process.argv[2] || 'jsdom');
const root = path.resolve(__dirname, '../../..');
const read = name => fs.readFileSync(path.join(root, 'web/dist', name), 'utf8');
const dom = new JSDOM(read('index.html').replace(/<script[\s\S]*?<\/script>/g,''), {runScripts:'outside-only',url:'http://example.invalid/',pretendToBeVisual:true});
const w = dom.window, q = selector => w.document.querySelector(selector);
const state = {platform:'windows',version:'fixture',service:true,svcState:'running',lang:'zh',theme:'light',view:{profiles:[{id:'fixture'}],settings:{},state:{status:'disconnected'}}};
const calls = [], errors = [], sha = 'a'.repeat(64);
let pendingPreview = null;
const preview = {id:'synthetic-id',sha256:sha,files:[{name:'summary.json',content:'fixture <img src=x onerror=bad()>',bytes:42}],warnings:[]};
w.go = {main:{App:new Proxy({GetState:async()=>state,GetLogs:async()=>[],PreviewDiag:async options=>{calls.push(['preview',options]);return pendingPreview ? pendingPreview() : preview;},ExportDiagPreview:async(...args)=>{calls.push(['save',...args]);return ''; }},{get:(o,k)=>o[k]||(()=>{calls.push([k]);return Promise.resolve();})})}};
w.runtime={EventsOn:()=>{}}; w.HTMLElement.prototype.scrollIntoView=function(){};
w.addEventListener('error',e=>errors.push(String(e.error||e.message)));
const ctx=dom.getInternalVMContext();
for(const file of ['i18n.js','flags.js','privacy.js','app.js'])vm.runInContext(read(file),ctx,{filename:file});
const settle=()=>new Promise(resolve=>setTimeout(resolve,25));
const nav=name=>vm.runInContext(`nav(${JSON.stringify(name)})`,ctx);
const open=()=>vm.runInContext('exportDiag()',ctx);
async function run(){
  await settle(); nav('logs');
  let work=open();
  assert.equal(q('#dialog').querySelectorAll('[data-diag-option]:checked').length,0);
  assert.equal(q('#dialog').querySelectorAll('[data-privacy-help]').length,7);
  assert.equal(q('#dialog').getAttribute('role'),'dialog');
  q('#dialog-yes').click(); await settle();
  assert.equal(calls.filter(c=>c[0]==='preview').length,1);
  assert.deepEqual(JSON.parse(JSON.stringify(calls[0][1])),{acknowledgeSensitive:false,logs:false,config:false,crashes:false,network:false});
  assert.equal(q('#dialog').querySelectorAll('img').length,0);
  q('#dialog-yes').click(); await work;
  assert.deepEqual(calls.find(c=>c[0]==='save'),['save','synthetic-id',sha]);

  work=open(); q('[data-diag-option="network"]').click();
  assert.equal(q('#dialog-yes').disabled,true);
  q('#diag-ack').click(); assert.equal(q('#dialog-yes').disabled,false);
  q('#dialog-yes').click(); await settle();
  const selected=calls.filter(c=>c[0]==='preview').at(-1)[1];
  assert.equal(selected.network,true);assert.equal(selected.acknowledgeSensitive,true);
  q('#dialog-no').click(); await work;
  const saved=calls.filter(c=>c[0]==='save').length;

  let resolve;pendingPreview=()=>new Promise(done=>{resolve=done;});
  work=open();q('#dialog-yes').click();await settle();nav('home');resolve(preview);await work;
  assert.equal(q('#dialog'),null);assert.equal(calls.filter(c=>c[0]==='save').length,saved);
  pendingPreview=null;nav('logs');work=open();q('#dialog-yes').click();await settle();
  const old=q('#dialog-yes');nav('home');old.click();await work;
  assert.equal(calls.filter(c=>c[0]==='save').length,saved);
  assert.deepEqual(errors,[]);assert.equal(calls.some(c=>c[0]==='ExportDiag'),false);
  console.log(JSON.stringify({scope:'isolated DOM',minimalByDefault:true,detailsRequireAcknowledgement:true,escapedPreview:true,sameSnapshot:true,stalePageBlocked:true}));
}
run().catch(error=>{console.error(error);process.exitCode=1;}).finally(()=>dom.window.close());
