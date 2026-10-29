// Isolated DOM/Wails mock. No network, native service, helper, or real account.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {JSDOM} = require(process.argv[2] || 'jsdom');
const root = path.resolve(__dirname, '../../..');
const source = file => fs.readFileSync(path.join(root, 'web/dist', file), 'utf8');
const dom = new JSDOM(source('index.html').replace(/<script[\s\S]*?<\/script>/g, ''), {runScripts:'outside-only',url:'http://example.invalid/',pretendToBeVisual:true});
const {window} = dom;
const calls = [], errors = [], handlers = {};
const fixture = {platform:'windows',service:true,svcState:'running',version:'fixture',lang:'zh',theme:'light',view:{profiles:[{id:'fixture',name:'Fixture'}],nodes:[],profile:{id:'fixture',name:'Fixture'},node:'Fixture',mode:'global',guard:'on',uptime:1,settings:{noDirect:true},state:{status:'connected',wanted:true}}};
window.addEventListener('error', e => errors.push(String(e.error || e.message)));
window.go = {main:{App:new Proxy({GetState:async()=>fixture},{get:(o,k)=>o[k]||((...args)=>{calls.push([k,...args]);return Promise.resolve();})})}};
window.runtime = {EventsOn:(name,handler)=>{handlers[name]=handler;}};
window.HTMLElement.prototype.scrollIntoView = function(){};
const context = dom.getInternalVMContext();
for(const file of ['i18n.js','flags.js','privacy.js','app.js'])vm.runInContext(source(file),context,{filename:file});
const settle = ()=>new Promise(resolve=>setTimeout(resolve,30));
function emit(kind,hint,lang='zh') {
  handlers.state(Object.assign({},fixture,{service:kind==='running',svcState:kind,svcHint:hint||'',lang,view:Object.assign({},fixture.view,{state:{status:kind==='running'?'connected':'unknown',wanted:true}})}));
}
async function run(){
  await settle();
  assert.ok(handlers.state);
  emit('stopped');
  assert.ok(window.document.querySelector('#home-banner #repair'));
  emit('identity-unknown','identity');
  const q = s=>window.document.querySelector(s);
  assert.equal(q('#home-banner [data-kind]').dataset.kind,'identity-unknown');
  assert.equal(q('#home-banner').querySelectorAll('button').length,0);
  assert.equal(q('#svc-text').textContent,'服务身份未确认');
  assert.equal(q('#status').textContent,'服务身份未确认');
  assert.equal(q('#power-text').textContent,'状态未知');
  assert.equal(q('#guard-pill').hidden,true);
  q('#power').click();
  vm.runInContext("nav('about')",context);
  assert.equal(q('#repair').hidden,true);
  assert.match(q('#about-quit-help').textContent,/仅关闭界面/);
  emit('identity-unknown','response');
  assert.equal(q('#about-svc').textContent,'服务响应未确认');
  assert.equal(q('#repair').hidden,true);
  vm.runInContext("nav('home')",context);
  assert.equal(q('#home-banner [data-kind]').dataset.kind,'identity-response');
  assert.match(q('#home-banner').textContent,/操作结果未知/);
  assert.doesNotMatch(q('#home-banner').textContent,/未发送/);
  emit('identity-unknown','identity','en');
  assert.equal(q('#svc-text').textContent,'Service identity unconfirmed');
  assert.match(q('#home-banner').textContent,/No control request was sent/);
  emit('identity-unknown','response','en');
  assert.equal(q('#svc-text').textContent,'Service response unconfirmed');
  assert.doesNotMatch(q('#home-banner').textContent,/No control request was sent/);
  emit('running');
  assert.equal(q('#home-banner').textContent,'');
  assert.equal(q('#guard-pill').hidden,false);
  // Disconnect is a lifecycle transaction. While the daemon is tearing down
  // the TUN/guard, wanted=false must not turn the same control into Connect.
  handlers.state(Object.assign({},fixture,{service:true,svcState:'running',view:Object.assign({},fixture.view,{state:{status:'stopping',wanted:false}})}));
  assert.equal(q('#power-text').textContent,'断开中');
  assert.equal(q('#power').disabled,true);
  const lifecycleCalls = calls.length;
  q('#power').click();
  await settle();
  assert.equal(calls.length,lifecycleCalls);
  emit('running');
  assert.equal(q('#power').disabled,false);
  assert.equal(calls.filter(c=>['Connect','Disconnect','RepairService','RegisterController','SaveSettings','QuitApp'].includes(c[0])).length,0);
  assert.deepEqual(errors,[]);
  console.log(JSON.stringify({scope:'isolated DOM mock only',identityUnknown:true,responseResultUnknown:true,staleProtectionHidden:true,noAutomaticMutations:true}));
}
run().then(()=>window.close(),error=>{console.error(error);window.close();process.exitCode=1;});
