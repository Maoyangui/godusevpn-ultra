// Fixed DOM/API mocks only. No browser process, helper, clipboard, OS or network.
'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {JSDOM}=require(process.argv[2]||'jsdom');
const root=path.resolve(__dirname,'../../..'),source=name=>fs.readFileSync(path.join(root,'web/dist',name),'utf8');
const clone=value=>JSON.parse(JSON.stringify(value)),wait=(ms=15)=>new Promise(resolve=>setTimeout(resolve,ms));
const A='a'.repeat(64),B='b'.repeat(64);
const region={presetId:'en-us',language:'en-US',locale:'en-US',timezone:'America/Los_Angeles',acceptLanguages:['en-US','en']};
const config={schemaVersion:1,engineId:'webview2',profileId:'start-fixture',identity:{mode:'manual',userAgent:'Fixture exact UA/1'},region,viewport:{width:1280,height:720},webrtc:'disable-non-proxied-udp',webgl:'runtime-default',webgpu:'runtime-default',permissions:{camera:'block',microphone:'block',geolocation:'block',notifications:'block',clipboardRead:'block'},cookies:'runtime-default',exitCleanup:'retain-profile'};
const state=(startId,name,canCancelStart=name==='starting')=>({startId,canCancelStart,canLaunch:false,code:name,status:{sessionId:'c'.repeat(64),engineId:'chrome-for-testing',version:'156.0.8078.47',state:name,ready:name==='ready',canNavigate:name==='ready'},detail:'fixed mock status'});
function deferred(){let resolve,reject;const promise=new Promise((yes,no)=>{resolve=yes;reject=no;});return {promise,resolve,reject};}

function fixture({legacy=false,tokens=[A,B]}={}){
  const dom=new JSDOM(source('index.html').replace(/<script[\s\S]*?<\/script>/g,''),{runScripts:'outside-only',url:'http://example.invalid/',pretendToBeVisual:true});
  const w=dom.window,calls=[],errors=[],polls=[];
  let current=state(A,'starting'),beginHook=null,statusHook=null,cancelHook=null,closeHook=null,authorizeHook=null,reportHook=null,helper='not_requested',nextToken=0,pollID=-1;
  // Start-loop sleeps are manually released. Other DOM timers keep their real
  // behavior; a fixture never waits for or contacts an actual browser backend.
  const nativeTimeout=w.setTimeout.bind(w),nativeClear=w.clearTimeout.bind(w);
  w.setTimeout=(callback,ms,...args)=>{if(ms===600){const id=pollID--;polls.push({id,callback,args});return id;}return nativeTimeout(callback,ms,...args);};
  w.clearTimeout=id=>{if(id<0){const index=polls.findIndex(p=>p.id===id);if(index>=0)polls.splice(index,1);}else nativeClear(id);};
  const api={
    GetState:async()=>({platform:'windows',service:true,lang:'zh',theme:'light',view:{profiles:[],nodes:[],settings:{},state:{status:'disconnected'}}}),
    PrivacyReport:async()=>reportHook?reportHook():({platform:'windows',controls:[],capabilities:[],warnings:[]}),PrivacyFirewallReport:async()=>({supported:true,rules:[]}),PrivacyHelperStatus:async()=>({state:helper}),PrivacyTimeReport:async()=>({supported:false,choices:[],status:'unsupported'}),BrowserHostReport:async()=>({supported:true,engines:[]}),
    PrivacyAuthorize:async()=>{calls.push(['authorize']);return authorizeHook?authorizeHook():{state:helper}},
    BrowserProfileDefaults:async()=>({config:clone(config),regions:[clone(region)],environmentPresets:[],graphicsPresets:[],hostAvailable:true,canLaunch:true}),
    BrowserEngineVersions:async()=>[{engineId:'chrome-for-testing',version:'156.0.8078.47',treeVerified:true,versionVerified:true,canLaunch:true}],
    BrowserEngineIdentity:async text=>({mode:JSON.parse(text).identity.mode,userAgent:'Fixture exact UA/1',applied:false,requiresRuntimeReadback:true}),
    BrowserEngineProfileLaunch:async(text,proxy)=>{calls.push(['legacy',JSON.parse(text),JSON.parse(proxy)]);return {...state(A,'ready'),detail:'fixed legacy ready'};},
    BrowserEngineProfileStatus:async()=>{calls.push(['status']);return statusHook?statusHook():clone(current);},
    BrowserEngineProfileCancelStart:async id=>{calls.push(['cancel',id]);if(cancelHook)return cancelHook(id);current=state(id,'cancelling',false);return clone(current);},
    BrowserEngineProfileClose:async()=>{calls.push(['close']);if(closeHook)return closeHook();current=state(current.startId,'closed',false);},
    BrowserEngineProfileNavigate:async()=>{throw Error('navigation forbidden in start UI fixture');},
    BrowserProxyTest:async()=>{throw Error('proxy probe forbidden in start UI fixture');},
    Connect:async()=>{throw Error('tunnel mutation forbidden');},Disconnect:async()=>{throw Error('tunnel mutation forbidden');},
  };
  if(!legacy)api.BrowserEngineProfileBeginLaunch=async(text,proxy)=>{calls.push(['begin',JSON.parse(text),JSON.parse(proxy)]);if(beginHook)return beginHook();current=state(tokens[nextToken++],'starting');return clone(current);};
  w.go={main:{App:new Proxy(api,{get:(object,key)=>object[key]||(()=>Promise.resolve())})}};w.runtime={EventsOn(){}};
  w.fetch=()=>{throw Error('network forbidden in start UI fixture');};w.HTMLElement.prototype.scrollIntoView=function(){};
  Object.defineProperty(w.navigator,'clipboard',{value:{writeText(){throw Error('clipboard forbidden');}}});
  w.document.execCommand=()=>{throw Error('clipboard forbidden');};
  w.addEventListener('error',event=>errors.push(String(event.error||event.message)));
  const context=dom.getInternalVMContext();for(const name of ['i18n.js','flags.js','privacy.js','app.js'])vm.runInContext(source(name),context,{filename:name});
  const q=selector=>w.document.querySelector(selector),count=kind=>calls.filter(call=>call[0]===kind).length,last=kind=>calls.filter(call=>call[0]===kind).at(-1);
  async function until(predicate,label){const end=Date.now()+2400;while(!predicate()&&Date.now()<end)await wait(10);assert.ok(predicate(),label);}
  function edit(selector,value,event='input'){const field=q(selector);assert.ok(field,selector);field.value=value;field.dispatchEvent(new w.Event(event,{bubbles:true}));}
  const text=()=>q('.privacy-browser-profile')?.textContent||'';
  return {w,q,calls,errors,count,last,until,edit,text,context,
    setStatus:value=>{current=clone(value);},
    onBegin:hook=>{beginHook=hook;},onStatus:hook=>{statusHook=hook;},onCancel:hook=>{cancelHook=hook;},onClose:hook=>{closeHook=hook;},
    onAuthorize:hook=>{authorizeHook=hook;},onReport:hook=>{reportHook=hook;},setHelper:value=>{helper=value;},
    async open(){await wait();vm.runInContext("nav('privacy',{tab:'browser'})",context);await until(()=>q('#privacy-browser-engine')&&!q('#privacy-browser-engine').disabled,'browser form loaded');edit('#privacy-browser-engine','chrome-for-testing','change');await until(()=>q('#privacy-browser-engine-version')?.value==='156.0.8078.47'&&!q('#privacy-browser-engine').disabled,'native version selected');edit('#privacy-proxy-host','synthetic.proxy.invalid');},
    async launch(){q('#privacy-browser-launch').click();await until(()=>count(legacy?'legacy':'begin')>0,'launch API submitted');await wait();},
    async poll(){await until(()=>polls.length>0,'start loop is waiting for its next poll');const poll=polls.shift();poll.callback(...poll.args);await wait();},
    dispose(){w.close();},
  };
}

async function pendingCancellation(f){
  await f.open();const oldLaunch=f.q('#privacy-browser-launch');await f.launch();
  assert.match(f.text(),/正在准备浏览器，可以取消启动/);
  for(const selector of ['#privacy-browser-engine','#privacy-browser-engine-version','#privacy-browser-id','#privacy-browser-width','#privacy-browser-ua','#privacy-proxy-host'])assert.equal(f.q(selector).disabled,true,'pending startup disables '+selector);
  const before=f.count('begin');oldLaunch.click();f.q('#privacy-browser-launch').dispatchEvent(new f.w.Event('click',{bubbles:true}));f.q('#privacy-proxy-launch').dispatchEvent(new f.w.Event('click',{bubbles:true}));await wait();assert.equal(f.count('begin'),before,'pending/old controls cannot submit another launch');
  const held=deferred();f.onCancel(()=>held.promise);const oldCancel=f.q('[data-browser-cancel-start]');oldCancel.click();await f.until(()=>f.count('cancel')===1,'cancel submitted');assert.deepEqual(f.last('cancel'),['cancel',A]);assert.equal(f.q('[data-browser-cancel-start]').disabled,true,'repeated cancellation is disabled while its reply is pending');oldCancel.click();f.q('[data-browser-cancel-start]').dispatchEvent(new f.w.Event('click',{bubbles:true}));await wait();assert.equal(f.count('cancel'),1,'duplicate/detached buttons cannot send cancellation again');
  f.setStatus(state(A,'cancelling',false));held.resolve(state(A,'cancelling',false));await wait();assert.match(f.text(),/等待浏览器完成清理/);assert.doesNotMatch(f.text(),/本次启动已取消/,'cancel acknowledgement is not completion');
  await f.poll();assert.doesNotMatch(f.text(),/本次启动已取消/);assert.equal(f.q('#privacy-browser-launch').disabled,true);
  f.setStatus(state(A,'start_cancelled',false));await f.poll();await f.until(()=>!f.q('#privacy-browser-launch').disabled,'terminal status unlocks the form');assert.match(f.text(),/本次启动已取消/);assert.equal(f.q('[data-browser-cancel-start]'),null);
}

async function oldRunCannotCancelReplacement(f){
  await f.open();await f.launch();const oldCancel=f.q('[data-browser-cancel-start]');
  f.setStatus(state(A,'start_cancelled',false));await f.poll();await f.until(()=>!f.q('#privacy-browser-launch').disabled,'first run is terminal');
  f.q('#privacy-browser-launch').click();await f.until(()=>f.count('begin')===2&&f.q('[data-browser-cancel-start]')?.dataset.browserCancelStart===B,'replacement run has a new token');
  const before=f.count('cancel');oldCancel.click();await wait();assert.equal(f.count('cancel'),before,'old DOM must not cancel a replacement run');
  f.q('[data-browser-cancel-start]').click();await f.until(()=>f.count('cancel')===before+1,'new run cancelled');assert.deepEqual(f.last('cancel'),['cancel',B]);
  f.setStatus(state(B,'start_cancelled',false));await f.poll();await f.until(()=>!f.q('#privacy-browser-launch').disabled,'replacement finishes');
}

async function readyWinsCancellation(f){
  await f.open();await f.launch();f.setStatus(state(A,'ready',false));f.onCancel(()=>state(A,'ready',false));
  f.q('[data-browser-cancel-start]').click();await wait();assert.doesNotMatch(f.text(),/本次启动已取消/);await f.poll();await f.until(()=>!f.q('#privacy-browser-launch').disabled,'ready race resolves');assert.match(f.text(),/浏览器已打开并完成启动检查/);assert.equal(f.q('[data-browser-cancel-start]'),null);
  f.q('#privacy-browser-close').click();await f.until(()=>f.count('close')===1&&!f.q('#privacy-browser-close').disabled,'first ready session closes before a new run');f.q('#privacy-browser-launch').click();await f.until(()=>f.count('begin')===2&&f.q('[data-browser-cancel-start]')?.dataset.browserCancelStart===B,'second ready-race token is pending');
  f.setStatus(state(B,'ready',false));f.onCancel(()=>{throw Error('private cancellation raced with ready');});f.q('[data-browser-cancel-start]').click();await wait();await f.poll();await f.until(()=>!f.q('#privacy-browser-launch').disabled,'a rejected stale Cancel refreshes ready status');assert.deepEqual(f.last('cancel'),['cancel',B]);assert.match(f.text(),/浏览器已打开并完成启动检查/);assert.doesNotMatch(f.text(),/本次启动已取消|private cancellation raced/);
}

async function unknownCleanupRemainsInspectable(f){
  await f.open();await f.launch();f.setStatus(state(A,'closed_filters_unknown',false));await f.poll();await f.until(()=>!f.q('#privacy-browser-launch').disabled,'unknown cleanup returned control');
  assert.match(f.text(),/网络规则清理仍未确认/);assert.doesNotMatch(f.text(),/浏览器已关闭。|本次启动已取消/);assert.ok(f.q('[data-browser-read-start]'));assert.equal(f.q('#privacy-browser-close').disabled,false);
  const before=f.count('status');f.q('[data-browser-read-start]').click();await f.until(()=>f.count('status')===before+1&&!f.q('#privacy-browser-close').disabled,'manual inspection calls Status and returns control');assert.equal(f.q('[data-browser-read-start]').dataset.browserReadStart,A);
  f.onClose(()=>{});f.setStatus(state(A,'unknown',false));f.q('#privacy-browser-close').click();await f.until(()=>f.count('close')===1&&!f.q('#privacy-browser-close').disabled,'close with unknown readback returns');
  assert.doesNotMatch(f.text(),/浏览器已关闭。|本次启动已取消/,'unknown is never evidence of closure');assert.ok(f.q('[data-browser-read-start]'),'unknown cleanup keeps a manual status entry');
}

async function failedStatusKeepsOwner(f){
  await f.open();await f.launch();f.onStatus(()=>{throw Error('private status transport failure');});await f.poll();await f.until(()=>f.q('[data-browser-read-start]'),'status failure preserves an inspectable owner');assert.equal(f.q('[data-browser-read-start]').dataset.browserReadStart,A);assert.doesNotMatch(f.text(),/浏览器已关闭。|本次启动已取消|private status transport failure/);
  f.onStatus(()=>state(B,'ready',false));f.q('[data-browser-read-start]').click();await f.until(()=>!f.q('#privacy-browser-close').disabled,'wrong-token status returns without replacing the owner');assert.equal(f.q('[data-browser-read-start]').dataset.browserReadStart,A,'a mismatched status token must not replace the retained run');assert.doesNotMatch(f.text(),/浏览器已打开并完成启动检查/);
  f.onStatus(null);f.setStatus(state(A,'closing_or_unknown',false));f.q('[data-browser-read-start]').click();await wait();assert.match(f.text(),/清理仍未确认/);assert.ok(f.q('[data-browser-read-start]'));
  f.setStatus(state(A,'closed',false));f.q('#privacy-browser-close').click();await f.until(()=>f.count('close')===1&&!f.q('#privacy-browser-close').disabled,'confirmed Close returns');assert.match(f.text(),/浏览器已关闭/);
}

async function departureCancelsThePendingToken(f){
  await f.open();await f.launch();const oldRoot=f.q('.privacy-browser-profile'),beforeText=oldRoot.textContent,oldCancel=f.q('[data-browser-cancel-start]');
  vm.runInContext("nav('home')",f.context);await wait();oldCancel.click();assert.equal(f.count('cancel'),0,'detached user controls do not submit');
  // A ready reply could arrive on the next poll. Departure still belongs to
  // the captured pending request and must issue only its scoped cancellation.
  f.setStatus(state(A,'ready',false));f.onCancel(()=>state(A,'ready',false));await f.poll();
  await f.until(()=>f.count('cancel')===1,'departure sends the captured pending token even if ready raced');assert.deepEqual(f.last('cancel'),['cancel',A]);assert.equal(oldRoot.textContent,beforeText,'late completion does not redraw the outgoing DOM');assert.equal(f.w.document.body.textContent.includes('浏览器已打开并完成启动检查'),false);assert.equal(f.q('.home')!==null,true,'late results cannot navigate back');
}

async function departureBeforeBeginReply(f){
  await f.open();const held=deferred();f.onBegin(()=>held.promise);await f.launch();const oldRoot=f.q('.privacy-browser-profile'),beforeText=oldRoot.textContent;vm.runInContext("nav('home')",f.context);await wait();
  held.resolve(state(B,'starting'));await f.until(()=>f.count('cancel')===1,'late Begin token is cancelled after leaving');assert.deepEqual(f.last('cancel'),['cancel',B]);assert.equal(oldRoot.textContent,beforeText);assert.equal(f.q('.home')!==null,true);
}

async function legacyLaunchRemainsCompatible(f){
  await f.open();await f.launch();await f.until(()=>!f.q('#privacy-browser-launch').disabled,'old synchronous launch completes');assert.equal(f.count('legacy'),1);assert.equal(f.count('begin')+f.count('status')+f.count('cancel'),0,'missing own Begin method uses only the old synchronous API');assert.match(f.text(),/fixed legacy ready/);
}

async function authorizationRetryClearsPriorFailure(f){
  await f.open();f.q('[data-privacy-tab="system"]').click();await wait();
  f.q('#privacy-authorize').click();await wait();f.q('#dialog-no').click();await wait();assert.equal(f.count('authorize'),0,'confirmation cancellation never attempts authorization');
  f.onAuthorize(()=>{throw Error('first synthetic UAC cancellation')});
  f.q('#privacy-authorize').click();await wait();f.q('#dialog-yes').click();await f.until(()=>f.count('authorize')===1&&!f.q('#privacy-authorize').disabled,'first authorization rejection completes');
  assert.match(f.q('.privacy-hero [role="alert"]').textContent,/first synthetic UAC cancellation/);
  f.onAuthorize(()=>{f.setHelper('connected');return {state:'connected'}});f.onReport(()=>{throw Error('fresh report read failure')});
  f.q('#privacy-authorize').click();await wait();f.q('#dialog-yes').click();await f.until(()=>f.count('authorize')===2&&f.q('#privacy-authorize').textContent==='管理员已授权','explicit authorization retry succeeds');
  assert.equal(f.q('#privacy-authorize').disabled,true);assert.doesNotMatch(f.q('.privacy-hero').textContent,/first synthetic UAC cancellation|授权未完成/,'a successful retry must not restore a prior authorization error');
  assert.match(f.q('.privacy-hero [role="alert"]').textContent,/fresh report read failure/,'new refresh errors survive successful authorization');assert.equal(f.count('authorize'),2,'authorization never retries implicitly');
}

async function currentAuthorizationAndReadErrorsBothRemain(f){
  await f.open();f.q('[data-privacy-tab="system"]').click();await wait();
  f.onAuthorize(()=>{throw Error('current synthetic rejection')});f.onReport(()=>{throw Error('current synthetic report failure')});
  f.q('#privacy-authorize').click();await wait();f.q('#dialog-yes').click();await f.until(()=>f.count('authorize')===1&&!f.q('#privacy-authorize').disabled,'current rejection and report refresh complete');
  const error=f.q('.privacy-hero [role="alert"]').textContent;assert.match(error,/current synthetic rejection/);assert.match(error,/current synthetic report failure/,'current authorization failure must not overwrite a fresh report error');assert.equal(f.count('authorize'),1);
}

async function fixedGuardDiagnosticPreservesCleanupMeaning(f){
  await f.open();await f.launch();
  let result=state(A,'closed_filters_unknown',false);result.status.diagnostic={stage:'lease_code_seal',nativeCode:5};f.setStatus(result);await f.poll();
  await f.until(()=>!!f.q('[data-browser-read-start]'),'failed guard remains inspectable');
  assert.match(f.text(),/网络规则清理仍未确认/);assert.match(f.text(),/阶段 lease_code_seal · 系统代码 5/,'a bounded original failure survives cleanup unknown');assert.doesNotMatch(f.text(),/浏览器已关闭。/);
  result=state(A,'closed',false);result.status.diagnostic={stage:'wfp_verify',nativeCode:5};f.setStatus(result);f.q('[data-browser-read-start]').click();await wait();
  assert.match(f.text(),/浏览器已关闭。/,'a retained cause must not turn confirmed closure back into unknown');assert.match(f.text(),/阶段 wfp_verify · 系统代码 5/);
}

async function run(){
  const scenarios=[['pending and cancellation',pendingCancellation],['old token and DOM',oldRunCannotCancelReplacement],['ready race',readyWinsCancellation],['unknown cleanup',unknownCleanupRemainsInspectable],['failed status',failedStatusKeepsOwner],['departure during poll',departureCancelsThePendingToken],['departure before Begin reply',departureBeforeBeginReply],['legacy bridge',legacyLaunchRemainsCompatible,{legacy:true}],['authorization retry',authorizationRetryClearsPriorFailure],['current authorization and read errors',currentAuthorizationAndReadErrorsBothRemain],['fixed guard diagnostic',fixedGuardDiagnosticPreservesCleanupMeaning]];
  const failures=[];
  for(const [name,test,options]of scenarios){const f=fixture(options);try{await test(f);assert.deepEqual(f.errors,[],'no unhandled DOM errors');}catch(error){failures.push(name+': '+error.message);}finally{f.dispose();}}
  if(failures.length)throw Error(failures.join('\n'));
  console.log(JSON.stringify({scope:'isolated startup DOM with fixed APIs and manual poll clock',pendingVisibleAndLocked:true,scopedCancellationAndNoDuplicateOrOldDOM:true,statusConfirmsCancellation:true,readyRaceNotCancelled:true,unknownCleanupRemainsInspectable:true,failedStatusRetainsOwner:true,departureCancelsMatchingTokenWithoutLateUI:true,legacyOwnMethodFallback:true,explicitAuthorizationRetryClearsOldError:true,currentAuthorizationAndRefreshErrorsPreserved:true,fixedDiagnosticPreservesUnknownAndConfirmedClosure:true,noBrowserClipboardOSOrNetwork:true}));
}
run().catch(error=>{console.error(error);process.exitCode=1;});
