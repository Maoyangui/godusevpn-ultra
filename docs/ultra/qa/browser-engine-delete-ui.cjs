// Fixed jsdom/Wails mocks only: no product, browser, cache, Windows API,
// network, clipboard, profile storage or real deletion is used.
'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {JSDOM}=require(process.argv[2]||'jsdom');
const root=path.resolve(__dirname,'../../..'),source=name=>fs.readFileSync(path.join(root,'web/dist',name),'utf8');
const clone=value=>JSON.parse(JSON.stringify(value)),wait=(ms=20)=>new Promise(r=>setTimeout(r,ms));
const engine='chrome-for-testing',version='156.0.8078.47',other='155.0.8000.9';
const region={presetId:'en-us',language:'en-US',locale:'en-US',timezone:'America/Los_Angeles',acceptLanguages:['en-US','en']};
const environment={mode:'browser-emulation',os:'macos',devicePresetId:'macos-desktop',osVersion:'13.0.0'};
const config={schemaVersion:1,engineId:'webview2',profileId:'delete-fixture',identity:{mode:'manual',userAgent:' Exact chosen UA/1 '},region,viewport:{width:1280,height:720},webrtc:'disable-non-proxied-udp',webgl:'runtime-default',webgpu:'runtime-default',permissions:{camera:'block',microphone:'block',geolocation:'block',notifications:'block',clipboardRead:'block'},cookies:'runtime-default',exitCleanup:'retain-profile'};
const initial=[{engineId:engine,version},{engineId:engine,version:other},{engineId:'firefox',version:'147.0.1'}].map(v=>({...v,treeVerified:true,versionVerified:true,fileCount:17}));
const status=name=>({startId:name==='closed'?'':'a'.repeat(64),canCancelStart:name==='starting',status:{engineId:engine,version,state:name,ready:name==='ready',canNavigate:name==='ready'}});
const hold=()=>{let resolve;const promise=new Promise(r=>resolve=r);return {promise,resolve}};
function fixture(){
 const dom=new JSDOM(source('index.html').replace(/<script[\s\S]*?<\/script>/g,''),{runScripts:'outside-only',url:'http://example.invalid/',pretendToBeVisual:true});
 const w=dom.window,calls=[],errors=[],unexpected=[];let inventory=clone(initial),mode='ok',onDelete=null,onStatus=null,current=status('closed');
 const timeout=w.setTimeout.bind(w);w.setTimeout=(callback,ms,...args)=>ms===600?-1:timeout(callback,ms,...args);
 const api={
  GetState:async()=>({platform:'windows',service:true,lang:'zh',theme:'light',view:{profiles:[],nodes:[],settings:{},state:{status:'disconnected'}}}),
  PrivacyReport:async()=>({platform:'windows',controls:[],capabilities:[],warnings:[]}),PrivacyFirewallReport:async()=>({supported:true,rules:[]}),PrivacyHelperStatus:async()=>({state:'not_requested'}),PrivacyTimeReport:async()=>({supported:false,choices:[],status:'unsupported'}),BrowserHostReport:async()=>({supported:true,engines:[]}),
  BrowserProfileDefaults:async()=>({config:clone(config),regions:[clone(region)],graphicsPresets:[],environmentPresets:[{config:clone(environment),recommendedViewport:{width:1280,height:720}}],hostAvailable:true}),
  TrialStatus:async()=>({enabled:true,status:'available',plan:'free',purchaseUrl:'https://godusevpn.org/subscription/'}),
  TrialClaim:async()=>({status:'claimed',subscriptionUrl:'https://sub.example/free',purchaseUrl:'https://godusevpn.org/subscription/'}),
  UpsertTrialProfile:async()=>[],
  BrowserEngineCatalog:async()=>{calls.push(['catalog']);return {releases:initial.map(v=>({engineId:v.engineId,version:v.version})),fetchedAt:'fixed synthetic time'}},
  BrowserEngineVersions:async()=>{calls.push(['inventory']);return clone(inventory)},
  BrowserEngineProfileStatus:async()=>{calls.push(['status']);return onStatus?onStatus():clone(current)},
  BrowserEngineProfileBeginLaunch:async()=>{calls.push(['begin']);return clone(current)},
  BrowserEngineDelete:async(id,v)=>{calls.push(['delete',id,v]);if(onDelete)return onDelete(id,v);if(mode==='error')throw Error('fixed deletion refused');if(mode==='mismatch')return {engineId:id,version:other,deleted:true};if(mode!=='stale-inventory')inventory=inventory.filter(item=>item.engineId!==id||item.version!==v);return {engineId:id,version:v,deleted:true,files:17,bytes:1234}},
 };
 w.go={main:{App:new Proxy(api,{get:(object,key)=>object[key]||(()=>{unexpected.push(String(key));throw Error('unexpected native API '+String(key))})})}};w.runtime={EventsOn(){}};
 w.fetch=()=>{throw Error('network forbidden')};w.XMLHttpRequest=function(){throw Error('network forbidden')};w.HTMLElement.prototype.scrollIntoView=function(){};
 Object.defineProperty(w.navigator,'clipboard',{value:{writeText(){throw Error('clipboard forbidden')}}});w.document.execCommand=()=>{throw Error('clipboard forbidden')};w.addEventListener('error',event=>errors.push(String(event.error||event.message)));
 const context=dom.getInternalVMContext();for(const file of ['i18n.js','flags.js','privacy.js','app.js'])vm.runInContext(source(file),context,{filename:file});
 const q=selector=>w.document.querySelector(selector),count=kind=>calls.filter(call=>call[0]===kind).length;
 const edit=(selector,value,event='input')=>{const input=q(selector);assert.ok(input,selector);input.value=value;input.dispatchEvent(new w.Event(event,{bubbles:true}))};
 const button=(id=engine,v=version)=>q('[data-browser-delete-engine="'+id+'"][data-browser-delete-version="'+v+'"]');
 async function click(selector){assert.ok(q(selector),selector);q(selector).click();await wait()}
 const draft=()=>Object.fromEntries(['privacy-browser-ua','privacy-browser-identity-mode','privacy-browser-environment','privacy-browser-os-version','privacy-browser-width','privacy-browser-height','privacy-browser-profile-name','privacy-proxy-host','privacy-proxy-port','privacy-proxy-username','privacy-proxy-password','privacy-browser-route'].map(id=>[id,q('#'+id).value]));
 return {w,q,calls,errors,unexpected,context,count,edit,click,button,draft,
  setMode:value=>{mode=value},setStatus:value=>{current=status(value)},onDelete:hook=>{onDelete=hook},onStatus:hook=>{onStatus=hook},remove:(id,v)=>{inventory=inventory.filter(item=>item.engineId!==id||item.version!==v)},
  async open(){await wait();vm.runInContext("nav('privacy',{tab:'browser'})",context);await wait();await click('[data-browser-pane="versions"]');assert.equal(q('#privacy-engine-version').placeholder,'先刷新官方目录');await click('#privacy-engine-local')},
  async confirm(id=engine,v=version){assert.ok(button(id,v),'downloaded version has a delete control');button(id,v).click();await wait();assert.ok(q('#dialog-yes'),'delete requires confirmation');q('#dialog-yes').click();await wait()},
  dispose(){dom.window.close()},
 };
}
async function exactDeletionAndDraftRetention(f){
 await f.open();assert.ok(f.button(),'downloaded versions expose a lightweight delete button');assert.match(f.button().getAttribute('aria-label'),new RegExp(version.replaceAll('.','\\.')));assert.match(f.button().title,/内核/);
 await f.click('#privacy-engine-refresh');assert.equal(f.q('#privacy-engine-version').placeholder,'选择或输入目录中的准确版本');
 f.q('[data-browser-use-engine="'+engine+'"][data-browser-use-version="'+version+'"]').click();await wait();
 f.edit('#privacy-browser-profile-name','保留中文配置');f.edit('#privacy-browser-ua',' Exact manual UA remains /9 ');f.edit('#privacy-browser-environment','macos-desktop','change');f.edit('#privacy-proxy-host','synthetic.proxy.invalid');f.edit('#privacy-proxy-username',' original user ');f.edit('#privacy-proxy-password',' original secret ');
 await f.click('[data-browser-pane="versions"]');f.edit('#privacy-engine-kind','firefox','change');f.edit('#privacy-engine-version','147.0.1');const before=f.draft();
 f.button().click();await wait();assert.match(f.q('#dialog-yes').closest('#dialog')?.textContent||f.w.document.body.textContent,/保留.*配置.*浏览数据/);f.q('#dialog-no').click();await wait();assert.equal(f.count('delete'),0);assert.equal(f.count('status'),0,'confirmation cancellation has no backend side effect');
 f.setMode('error');await f.confirm();assert.deepEqual(f.calls.filter(c=>c[0]==='delete').at(-1),['delete',engine,version],'row identity wins over the unrelated version input');assert.ok(f.button());assert.deepEqual(f.draft(),before);assert.doesNotMatch(f.q('.privacy-engine-manager').textContent,/已删除.*内核/);
 f.setMode('mismatch');const reads=f.count('inventory');await f.confirm();assert.equal(f.count('inventory'),reads,'mismatched deletion receipts cannot refresh or publish success');assert.ok(f.button());assert.deepEqual(f.draft(),before);
 f.setMode('stale-inventory');await f.confirm();assert.equal(f.count('inventory'),reads+1);assert.ok(f.button(),'a success receipt is insufficient when inventory still contains the version');assert.deepEqual(f.draft(),before);assert.doesNotMatch(f.q('.privacy-engine-manager').textContent,/已删除.*内核/);
 f.setMode('ok');await f.confirm();assert.equal(f.button(),null);assert.ok(f.button(engine,other));assert.equal(f.q('#privacy-browser-engine-version').value,'','deleting the selected version clears only that version selection');assert.equal(f.q('#privacy-browser-engine').value,engine,'never switches to another installed engine');assert.deepEqual(f.draft(),before);assert.match(f.q('.privacy-engine-manager').textContent,/重新下载/);assert.equal(f.count('inventory'),reads+2,'only a precise success rereads the actual inventory');
 assert.equal(f.q('#privacy-engine-kind').value,'firefox');assert.equal(f.q('#privacy-engine-version').value,'147.0.1','the unrelated manager input is preserved');
}
async function staleAndPendingOperations(f){
 await f.open();const old=f.button();old.click();await wait();const lateYes=f.q('#dialog-yes');await f.click('[data-privacy-tab="system"]');lateYes.click();old.click();await wait();assert.equal(f.count('delete'),0);assert.equal(f.count('status'),0);
 await f.click('[data-privacy-tab="browser"]');await f.click('[data-browser-pane="versions"]');const detached=f.button();await f.click('#privacy-engine-local');detached.click();await wait();assert.equal(f.q('#dialog-yes'),null);assert.equal(f.count('delete'),0);
 const pending=hold();f.onDelete(()=>pending.promise);await f.confirm();assert.equal(f.count('delete'),1);assert.equal(f.button().disabled,true);f.button().dispatchEvent(new f.w.Event('click',{bubbles:true}));await wait();assert.equal(f.count('delete'),1,'pending deletion blocks duplicate submissions');
 await f.click('[data-privacy-tab="system"]');await f.click('[data-privacy-tab="browser"]');f.remove(engine,version);pending.resolve({engineId:engine,version,deleted:true});await wait();assert.ok(f.button(),'late success from an old tab generation cannot rewrite the current list');assert.doesNotMatch(f.q('.privacy-engine-manager').textContent,/已删除.*内核/);
 await f.click('[data-browser-pane="versions"]');await f.click('#privacy-engine-local');assert.equal(f.button(),null,'an explicit fresh local read reconciles an earlier completed deletion');
}
async function activeStateBlocksDeletion(f,name){
 await f.open();f.onStatus(()=>status(name));f.setStatus(name);f.edit('#privacy-engine-version',version);f.edit('#privacy-engine-route','proxy','change');f.edit('#privacy-proxy-host','synthetic.proxy.invalid');
 // Deletion performs a fresh status preflight. Active, cancelling and unknown
 // owners must be rejected without removing the cached engine row.
 await f.confirm();assert.equal(f.count('status'),1,name+' performs a fresh deletion preflight');assert.equal(f.count('delete'),0,name+' refuses deletion');assert.ok(f.button(),name+' keeps the cached version row');
}
async function freshActiveOwnerRefusesBeforeDelete(f){
 await f.open();f.setStatus('ready');await f.confirm();assert.equal(f.count('status'),1);assert.equal(f.count('delete'),0,'fresh active state must refuse before calling Delete');assert.ok(f.button());assert.match(f.q('.privacy-engine-manager [role="alert"]').textContent,/关闭|状态/);
}
async function lateStatusCannotAuthorizeDeletion(f){
 await f.open();const pending=hold();f.onStatus(()=>pending.promise);await f.confirm();assert.equal(f.count('status'),1);assert.equal(f.count('delete'),0);
 await f.click('[data-privacy-tab="system"]');await f.click('[data-privacy-tab="browser"]');pending.resolve(status('closed'));await wait();assert.equal(f.count('delete'),0,'a late preflight cannot authorize deletion after a tab generation change');assert.ok(f.button());
}
async function run(){
 const scenarios=[['exact deletion and draft retention',exactDeletionAndDraftRetention],['stale and pending',staleAndPendingOperations],['fresh active owner',freshActiveOwnerRefusesBeforeDelete],['late preflight',lateStatusCannotAuthorizeDeletion],...['ready','starting','cancelling','closed_filters_unknown','closing_or_unknown','status_unknown'].map(name=>[name,f=>activeStateBlocksDeletion(f,name)])];
 const failures=[];for(const [name,test]of scenarios){const f=fixture();try{await test(f);assert.deepEqual(f.errors,[]);assert.deepEqual(f.unexpected,[])}catch(error){failures.push(name+': '+error.message)}finally{f.dispose()}}
 if(failures.length)throw Error(failures.join('\n'));console.log(JSON.stringify({scope:'isolated downloaded-engine deletion DOM',exactVersionRowAndReceipt:true,confirmationCancellationNoCalls:true,actualInventoryReread:true,failuresPreserveListAndDraft:true,selectedVersionClearedWithoutSwitching:true,manualUAEnvironmentProxyAndProfilesPreserved:true,activeUnknownAndPendingRefused:true,staleDOMConfirmationAndResultsIgnored:true,catalogAwarePlaceholder:true,noProductBrowserNetworkOrFilesystemDeletion:true}));
}
run().catch(error=>{console.error(error);process.exitCode=1});
