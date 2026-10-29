// Pure jsdom fixtures: fixed fake replies only. No product, native browser,
// proxy, network, system setting, credential store or clipboard is accessed.
'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {JSDOM}=require(process.argv[2]||'jsdom');
const root=path.resolve(__dirname,'../../..'),source=name=>fs.readFileSync(path.join(root,'web/dist',name),'utf8');
const dom=new JSDOM(source('index.html').replace(/<script[\s\S]*?<\/script>/g,''),{runScripts:'outside-only',url:'http://example.invalid/',pretendToBeVisual:true});
const w=dom.window,clone=value=>JSON.parse(JSON.stringify(value)),calls=[],errors=[];
const region={presetId:'en-us-pacific',language:'en-US',locale:'en-US',timezone:'America/Los_Angeles',acceptLanguages:['en-US','en']};
const config={schemaVersion:1,engineId:'webview2',profileId:'private',identity:{mode:'generated'},region,viewport:{width:1280,height:720},webrtc:'disable-non-proxied-udp',webgl:'runtime-default',webgpu:'runtime-default',permissions:{camera:'block',microphone:'block',geolocation:'block',notifications:'block',clipboardRead:'block'},cookies:'runtime-default',exitCleanup:'retain-profile'};
const proxy={type:'socks5',host:'fixture-proxy.invalid',port:1080,username:'fixture-user',password:'  fixture-correct-password  '};
const state={platform:'windows',service:true,svcState:'running',version:'fixture',lang:'zh',theme:'light',view:{profiles:[],nodes:[],settings:{},state:{status:'disconnected'}}};
const reachable={status:'reachable',scope:'profile_proxy_probe_only',provider:'country.is',ip:'203.0.113.23',country:'US',subdivision:'Fixture Region',city:'Fixture City',latencyMs:1103,checkedAt:'fixed synthetic time'};
let fail=false,pendingProxy=null,pendingPreview=null;
const api={
 GetState:async()=>state,
 PrivacyReport:async()=>({platform:'windows',controls:[],capabilities:[],warnings:[]}),
 PrivacyFirewallReport:async()=>({supported:true,rules:[],warnings:[]}),
 PrivacyHelperStatus:async()=>({state:'not_requested'}),
 PrivacyTimeReport:async()=>({supported:false,choices:[],status:'unsupported'}),
 BrowserHostReport:async()=>({supported:true,recommended:'webview',engines:[]}),
 BrowserProfileDefaults:async()=>({config:clone(config),regions:[clone(region)],graphicsPresets:[],environmentPresets:[],hostAvailable:true,status:'available'}),
 BrowserProfileIdentity:async text=>({mode:JSON.parse(text).identity.mode,status:'planned',userAgent:'Mozilla/5.0 Chrome/143.0.0.0',exactChromiumVersionKnown:false,applied:false}),
 BrowserProfilePreview:async text=>{calls.push(['preview',JSON.parse(text)]);if(pendingPreview)return pendingPreview.promise;return {config:JSON.parse(text),hostAvailable:true}},
 BrowserProxyTest:async text=>{calls.push(['proxy',JSON.parse(text)]);if(pendingProxy)return pendingProxy.promise;if(fail)throw Error('private fixture authentication failure must not be displayed');return clone(reachable)},
};
w.go={main:{App:new Proxy(api,{get:(object,key)=>object[key]||(()=>{throw Error('unexpected native API '+String(key))})})}};
w.runtime={EventsOn:()=>{}};w.fetch=()=>{throw Error('network forbidden')};
w.XMLHttpRequest=function(){throw Error('network forbidden')};
w.HTMLElement.prototype.scrollIntoView=function(){};
Object.defineProperty(w.navigator,'clipboard',{value:{writeText(){throw Error('clipboard forbidden')}}});
w.document.execCommand=()=>{throw Error('clipboard forbidden')};
w.addEventListener('error',event=>errors.push(String(event.error||event.message)));
const context=dom.getInternalVMContext();
for(const name of ['i18n.js','flags.js','privacy.js','app.js'])vm.runInContext(source(name),context,{filename:name});
const q=selector=>w.document.querySelector(selector),settle=()=>new Promise(resolve=>setTimeout(resolve,30));
const click=async selector=>{assert.ok(q(selector),selector);q(selector).click();await settle()};
const edit=(selector,value)=>{const input=q(selector);assert.ok(input,selector);input.value=value;input.dispatchEvent(new w.Event('input',{bubbles:true}));return input};
const choose=(selector,value)=>{const input=q(selector);assert.ok(input,selector);input.value=value;input.dispatchEvent(new w.Event('change',{bubbles:true}));return input};
const visible=element=>{if(!element||!element.isConnected)return false;for(let node=element;node;node=node.parentElement)if(node.hidden)return false;return true};
const proxyCalls=()=>calls.filter(call=>call[0]==='proxy');
const caption=()=>Array.from(q('.privacy-browser-proxy').querySelectorAll('p')).find(p=>p.textContent.includes('仅为此次代理探测结果'));
const hold=()=>{let resolve;const promise=new Promise(r=>resolve=r);return {promise,resolve}};
async function run(){try{
 await settle();vm.runInContext("nav('privacy',{tab:'browser'})",context);await settle();await click('[data-browser-pane="proxy"]');
 choose('#privacy-browser-route','proxy');for(const field of ['host','port','username','password'])edit('#privacy-proxy-'+field,String(proxy[field]));
 assert.equal(calls.length,0,'editing a proxy never tests, saves or launches implicitly');
 await click('#privacy-proxy-test');assert.equal(proxyCalls().length,1);assert.deepEqual(proxyCalls()[0][1],proxy);
 const result=q('.privacy-browser-proxy .privacy-probe'),resultCaption=caption();assert.equal(visible(result),true);assert.equal(visible(resultCaption),true);
 assert.match(result.textContent,/203\.0\.113\.23/);assert.equal(result.textContent.includes(proxy.password),false);
 const password=q('#privacy-proxy-password'),wrong=proxy.password+'-qa-invalid',beforeEdit=calls.length;
 password.focus();password.value=wrong;password.setSelectionRange(2,5);password.dispatchEvent(new w.Event('input',{bubbles:true}));
 assert.equal(result.hidden,true,'the existing probe selector remains compatible');
 assert.equal(visible(resultCaption),false,'success edit must immediately hide the entire old probe caption');
 assert.equal(q('.privacy-browser-proxy .privacy-badge').textContent,'尚未测试');assert.equal(calls.length,beforeEdit);
 assert.equal(q('#privacy-proxy-password'),password);assert.equal(w.document.activeElement,password);assert.equal(password.selectionStart,2);assert.equal(password.selectionEnd,5);
 fail=true;await click('#privacy-proxy-test');assert.equal(proxyCalls().length,2);assert.equal(proxyCalls()[1][1].password,wrong);
 const oldError=q('.privacy-browser-proxy [role="alert"]');assert.equal(visible(oldError),true);assert.match(oldError.textContent,/没有改用直连/);assert.doesNotMatch(oldError.textContent,/private fixture/);
 const restored=q('#privacy-proxy-password'),beforeRestore=calls.length;
 restored.focus();restored.value=proxy.password;restored.setSelectionRange(3,6);restored.dispatchEvent(new w.Event('input',{bubbles:true}));
 assert.equal(visible(oldError),false,'restoring credentials must immediately hide the old failed result');
 assert.equal(oldError.textContent,'','the stale authentication error must not remain as hidden text');assert.equal(calls.length,beforeRestore);
 assert.equal(q('#privacy-proxy-password'),restored);assert.equal(w.document.activeElement,restored);assert.equal(restored.selectionStart,3);assert.equal(restored.selectionEnd,6);
 assert.equal(q('.privacy-browser-proxy .privacy-badge').textContent,'尚未测试');assert.equal(q('#privacy-browser-route').value,'proxy');
 // Controls replaced by a test redraw cannot overwrite the newer draft.
 const oldPassword=restored,oldRoute=q('#privacy-browser-route');fail=false;await click('#privacy-proxy-test');assert.equal(proxyCalls().length,3);
 oldPassword.value='detached poison';oldPassword.dispatchEvent(new w.Event('input',{bubbles:true}));oldRoute.value='ultra';oldRoute.dispatchEvent(new w.Event('change',{bubbles:true}));
 assert.equal(proxyCalls().length,3,'detached controls never test implicitly');await click('#privacy-proxy-test');
 assert.deepEqual(proxyCalls()[3][1],proxy,'the next explicit test serializes the untouched current credentials');assert.equal(q('#privacy-browser-route').value,'proxy','detached route changes are ignored');
 // Disabled current inputs cannot mutate the accepted model while a proxy
 // request or browser configuration check is pending, even via synthetic input.
 pendingProxy=hold();q('#privacy-proxy-test').click();await settle();assert.equal(q('#privacy-proxy-password').disabled,true);
 edit('#privacy-proxy-password','busy proxy poison');pendingProxy.resolve(clone(reachable));pendingProxy=null;await settle();assert.equal(q('#privacy-proxy-password').value,proxy.password);
 pendingPreview=hold();q('#privacy-browser-preview').click();await settle();assert.equal(q('#privacy-proxy-password').disabled,true);
 const beforeBusyEdit=proxyCalls().length;edit('#privacy-proxy-password','busy browser poison');choose('#privacy-browser-route','ultra');
 pendingPreview.resolve({config:clone(config),hostAvailable:true});pendingPreview=null;await settle();
 assert.equal(q('#privacy-proxy-password').value,proxy.password);assert.equal(q('#privacy-browser-route').value,'proxy');assert.equal(proxyCalls().length,beforeBusyEdit);
 assert.equal(calls.filter(call=>call[0]==='preview').length,1);assert.ok(calls.every(call=>['proxy','preview'].includes(call[0])),'only explicit tests and the explicit mocked configuration preview are allowed');assert.deepEqual(errors,[]);
 console.log(JSON.stringify({scope:'isolated proxy draft DOM',successCaptionHiddenOnEdit:true,failedErrorHiddenOnRestore:true,focusAndSelectionRetained:true,credentialsAndRouteUnchanged:true,detachedAndBusyInputsIgnored:true,noImplicitRequestOrSave:true,noProductBrowserNetworkOrClipboard:true}));
}finally{dom.window.close()}}
run().catch(error=>{console.error(error);process.exitCode=1});
