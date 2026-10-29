// Pure jsdom interaction checks. Every backend result is synthetic. The real
// copyText is replaced in memory; no clipboard, network, browser or product runs.
'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {JSDOM}=require(process.argv[2]||'jsdom');
const root=path.resolve(__dirname,'../../..'),source=name=>fs.readFileSync(path.join(root,'web/dist',name),'utf8');
const clone=value=>JSON.parse(JSON.stringify(value));
const future=()=>new Date(Date.now()+600000).toISOString();
const wait=()=>new Promise(resolve=>setTimeout(resolve,25));
const hold=()=>{let resolve;const promise=new Promise(r=>resolve=r);return {promise,resolve}};
const guide=(id,title,actions=[])=>({id,title,summary:'明确的模块短说明。',capability:'settings',scope:'固定合成范围',verification:'检查结果仍以实际读回为准。',recovery:'只在明确操作后恢复。',limitations:['有限范围。'],pending:['下一实现范围。'],backend:{read:['NeverDispatchThis'],write:['NeverWriteThis']},actions});
const guides={schemaVersion:1,kind:'help_only_not_live_evidence',modules:[
 guide('system-language-region','语言与地区',[{kind:'settings',target:'language',label:'Windows 语言与地区'}]),
 guide('app-permissions','应用权限',[{kind:'settings',target:'camera',label:'摄像头权限'}]),
 guide('credential-storage','凭据存储'),guide('location-provider','系统位置模拟'),
 guide('detection-observations','检测快照',[{kind:'app-page',target:'detection',label:'打开检测中心'}]),
 guide('recovery','恢复',[{kind:'settings',target:'file:///private-command',label:'不可执行入口'},{kind:'app-page',target:'javascript:bad',label:'不可执行页面'}])
]};
const report={platform:'windows',admin:true,backupHealthy:true,observerElevated:false,warnings:[],capabilities:[{id:'hardware',title:'合成能力说明',status:'planned',description:'没有统一修改能力。',guide:'只读范围说明。'}],controls:[
 {id:'fixed-available',title:'可应用项目',category:'windows',status:'available',readStatus:'different',canApply:true,requiresAdmin:false,description:'固定策略简述。'},
 {id:'fixed-unknown',title:'未知项目',category:'windows',status:'unknown',readStatus:'unknown',canApply:false,requiresAdmin:true,description:'尚未确认。'},
 {id:'guidance-only',title:'合成能力说明',category:'guidance',status:'manual',readStatus:'unknown',canApply:false,description:'仅功能说明，不是可检查策略。'}
]};
const findings=[
 {id:'owned-browser-session',title:'专用浏览器会话',category:'专用浏览器',audience:'app',status:'unknown',summary:'浏览器收尾尚未确认；请先检查会话，暂勿重复启动。',evidence:'只读管理器内存，没有重新测网页。',scope:'先前启动记录不是当前页面实测。',source:'browser-session',detectedAt:'2026-10-27T01:02:03Z',settingsId:'browser'},
 {id:'external-browser',title:'外部浏览器',category:'网页环境',audience:'website',status:'unknown',summary:'仍需在实际浏览器中检查。',evidence:'未启动外部浏览器。',scope:'其他浏览器',settingsId:'browser'},
 {id:'observed-fixture',title:'合成观察项',category:'系统',audience:'app',status:'observed',summary:'读取到固定合成值。',evidence:'固定输入。',scope:'仅 mock',source:'facts',detectedAt:'2026-10-27T01:02:03Z'}
];
function fixture({oldBridge=false}={}){
 const dom=new JSDOM(source('index.html').replace(/<script[\s\S]*?<\/script>/g,''),{runScripts:'outside-only',url:'http://example.invalid/',pretendToBeVisual:true});
 const w=dom.window,calls=[],errors=[],copied=[],toasts=[];let textHook=null,copyHook=null;
 const text={previewId:'reviewed-preview',content:'精简检测报告\n固定合成观察；没有配置名、UA 或凭据。\n',sha256:'a'.repeat(64),expiresAt:future(),count:3};
 const invoke=(name,fn)=>(...args)=>{calls.push([name,...args]);return fn(...args)};
 const api={
  GetState:async()=>({platform:'windows',service:true,lang:'zh',theme:'light',view:{profiles:[],nodes:[],settings:{},state:{status:'disconnected'}}}),
  PrivacyReport:async()=>clone(report),PrivacyFirewallReport:async()=>({supported:true,rules:[],warnings:[]}),
  PrivacyHelperStatus:async()=>({state:'connected',browserGuard:{available:false,diagnostic:{stage:'owner_token_open',nativeCode:5}}}),
  PrivacyTimeReport:async()=>({supported:false,choices:[],status:'unsupported'}),
  PrivacyOpenSettings:invoke('settings',async()=>{}),
  PrivacyDetectionSnapshot:invoke('snapshot',async()=>({snapshotId:'snapshot-fixed',expiresAt:future(),report:{generatedAt:'2026-10-27T01:02:05Z',observer:'合成普通用户',helperAuthorized:true,warnings:[],findings:clone(findings)}})),
  PrivacyPreviewDetectionExport:invoke('json-preview',async selection=>({previewId:'reviewed-preview',content:JSON.stringify({selection,findings:clone(findings)}),expiresAt:future(),count:3})),
  PrivacyExportDetection:invoke('save',async()=>false)
 };
 if(!oldBridge){api.PrivacyModuleGuides=invoke('guides',async()=>clone(guides));api.PrivacyDetectionTextPreview=invoke('text-preview',async id=>textHook?textHook(id):clone(text));api.PrivacyDetectionTextForCopy=invoke('text-copy',async(id,sha)=>copyHook?copyHook(id,sha):text.content);}
 w.go={main:{App:new Proxy(api,{get:(object,key)=>object[key]||(()=>{throw Error('unexpected native API '+String(key))})})}};
 w.runtime={EventsOn(){}};w.fetch=()=>{throw Error('network forbidden')};w.XMLHttpRequest=function(){throw Error('network forbidden')};
 w.HTMLElement.prototype.scrollIntoView=function(){};Object.defineProperty(w.navigator,'clipboard',{value:{writeText(){throw Error('real clipboard forbidden')}}});w.document.execCommand=()=>{throw Error('real clipboard forbidden')};
 w.addEventListener('error',event=>errors.push(String(event.error||event.message)));
 const context=dom.getInternalVMContext();for(const name of ['i18n.js','flags.js','privacy.js','app.js'])vm.runInContext(source(name),context,{filename:name});
 w.__copied=copied;w.__toasts=toasts;vm.runInContext('copyText=async text=>{window.__copied.push(text);return true;};toast=(message)=>window.__toasts.push(message);',context);
 const q=selector=>w.document.querySelector(selector),count=name=>calls.filter(c=>c[0]===name).length;
 const nav=async(page,arg)=>{vm.runInContext(`nav(${JSON.stringify(page)},${JSON.stringify(arg)})`,context);await wait()};
 const click=async selector=>{assert.ok(q(selector),selector);q(selector).click();await wait()};
 const edit=(selector,value,event='change')=>{const field=q(selector);assert.ok(field,selector);if(typeof value==='boolean')field.checked=value;else field.value=value;field.dispatchEvent(new w.Event(event,{bubbles:true}));};
 return {w,context,q,calls,count,copied,toasts,errors,text,nav,click,edit,wait,onText:fn=>{textHook=fn},onCopy:fn=>{copyHook=fn},dispose:()=>w.close()};
}
async function openJSON(f){await f.click('#detection-export');await f.click('#detection-preview');assert.ok(f.q('#detection-save'));}
async function run(){
 const f=fixture();try{
  await wait();await f.nav('privacy');
  assert.ok(f.q('.privacy-summary'),'privacy center needs a compact factual overview');
  assert.equal(f.q('.privacy-summary-item strong').textContent,'2','help-only guidance is not counted as a policy');assert.equal(f.q('[data-privacy-control="guidance-only"]'),null,'system cards must not duplicate capability guidance');
  assert.match(f.q('.privacy-summary').textContent,/待确认/);assert.doesNotMatch(f.q('.privacy-summary').textContent,/评分|安全率|保护率/);
  assert.equal(f.count('guides'),1);assert.ok(f.q('[data-privacy-guide="system-language-region"] details'));
  assert.equal(f.q('.privacy-module-guide-group').open,false,'the module catalog starts as one compact closed group');await f.click('.privacy-module-guide-group > summary');
  assert.equal(f.q('[data-privacy-guide="system-language-region"] details').open,false,'long guidance starts collapsed');
  assert.match(f.q('#privacy-browser-guard-diagnostic').textContent,/owner_token_open.*5/s);assert.match(f.q('#privacy-browser-guard-diagnostic').textContent,/未就绪/);
  assert.equal(f.calls.some(c=>/Never|Apply|Restore/.test(c[0])),false,'guide metadata never dispatches methods');
  await f.click('[data-privacy-guide="system-language-region"] details > summary');await f.click('[data-privacy-guide="system-language-region"] [data-privacy-guide-action]');assert.deepEqual(f.calls.filter(c=>c[0]==='settings'),[['settings','language']]);
  await f.click('[data-privacy-tab="boundaries"]');assert.match(f.q('#privacy-panel').textContent,/合成能力说明/,'capability guidance remains on its own page');assert.equal(f.q('[data-privacy-guide="recovery"] [data-privacy-guide-action]'),null,'unrecognized catalog targets are not executable');
  await f.nav('detection');
  assert.match(f.q('.privacy-summary').textContent,/3.*观察项.*2.*待确认/s);assert.match(f.q('[data-detection-finding="owned-browser-session"]').textContent,/专用浏览器管理器/);
  assert.match(f.q('#detection-browser').textContent,/专用浏览器/);assert.ok(f.q('[data-detection-finding="external-browser"]'),'external browser unknown remains separate');
  assert.equal(f.q('[data-detection-finding="owned-browser-session"] details').open,false);
  await openJSON(f);assert.equal(f.q('#detection-export-environment').checked,false);assert.equal(f.q('#detection-export-observer').checked,false);assert.equal(f.calls.find(c=>c[0]==='json-preview')[1].includeEnvironment,false);assert.equal(f.calls.find(c=>c[0]==='json-preview')[1].includeObserver,false);
  assert.equal(f.count('text-preview'),0);assert.equal(f.count('text-copy'),0);assert.equal(f.q('#detection-text-copy'),null);
  await f.click('#detection-text-preview');assert.equal(f.q('#detection-text-content').textContent,f.text.content);assert.equal(f.count('snapshot'),1,'text derives reviewed JSON, never re-detects');assert.equal(f.count('json-preview'),1);assert.equal(f.count('text-copy'),0);
  assert.deepEqual(f.calls.filter(c=>c[0]==='text-preview'),[['text-preview','reviewed-preview']]);
  await f.click('#detection-text-copy');assert.deepEqual(f.copied,[f.text.content]);assert.deepEqual(f.calls.filter(c=>c[0]==='text-copy'),[['text-copy','reviewed-preview','a'.repeat(64)]]);
  // A changed export choice revokes both text and JSON; detached buttons are inert.
  const oldCopy=f.q('#detection-text-copy'),oldText=f.q('#detection-text-preview');f.edit('#detection-export-environment',true);await wait();
  assert.equal(f.q('#detection-text-content'),null);oldCopy.click();oldText.click();await wait();assert.equal(f.count('text-copy'),1);assert.equal(f.count('text-preview'),1);
  await f.click('#detection-preview');await f.click('#detection-text-preview');f.onCopy(()=> 'different backend bytes');await f.click('#detection-text-copy');assert.equal(f.copied.length,1,'backend mismatch never reaches clipboard adapter');assert.match(f.q('.privacy-error').textContent,/复制未完成/);
  // Visible text must itself still equal the reviewed content.
  f.onCopy(null);f.q('#detection-text-content').textContent='tampered visible content';const copies=f.count('text-copy');await f.click('#detection-text-copy');assert.equal(f.count('text-copy'),copies);assert.equal(f.copied.length,1);
  await f.click('#detection-text-preview');const pending=hold();f.onCopy(()=>pending.promise);f.q('#detection-text-copy').click();await wait();
  vm.runInContext('privacyEvidenceChanged()',f.context);pending.resolve(f.text.content);await wait();assert.equal(f.copied.length,1,'privacy epoch changes discard an in-flight copy');assert.equal(f.q('#detection-text-content'),null);
  await f.click('#detection-refresh');await openJSON(f);await f.click('#detection-text-preview');
  const searchCopy=f.q('#detection-text-copy'),beforeSearch=f.count('text-copy');f.edit('#detection-search','no matching synthetic finding','input');await wait();searchCopy.click();await wait();assert.equal(f.q('#detection-text-content'),null);assert.equal(f.count('text-copy'),beforeSearch,'search changes revoke the visible text');
  f.edit('#detection-search','','input');await f.click('#detection-preview');await f.click('#detection-text-preview');
  const beforeExpiry=f.count('text-copy'),nativeNow=f.w.Date.now;f.w.Date.now=()=>Date.now()+3600000;await f.click('#detection-text-copy');assert.equal(f.count('text-copy'),beforeExpiry);assert.equal(f.q('#detection-text-content'),null,'an expired snapshot is removed before a copy RPC');f.w.Date.now=nativeNow;
  await f.click('#detection-refresh');await openJSON(f);const pendingText=hold();f.onText(()=>pendingText.promise);f.q('#detection-text-preview').click();await wait();await f.nav('home');pendingText.resolve(clone(f.text));await wait();assert.equal(f.q('#detection-text-content'),null,'late preview cannot reattach an outgoing page');assert.equal(f.copied.length,1);
  assert.deepEqual(f.errors,[]);
 }finally{f.dispose()}
 const old=fixture({oldBridge:true});try{await wait();await old.nav('privacy');assert.ok(old.q('#privacy-refresh'));assert.equal(old.count('guides'),0);await old.nav('detection');await openJSON(old);assert.equal(old.q('#detection-text-preview'),null,'older bridge has no fake copy feature');assert.deepEqual(old.errors,[]);}finally{old.dispose()}
 console.log(JSON.stringify({scope:'pure synthetic center UI',factualOverview:true,collapsedHelpAndFixedActions:true,guardFactoryFailureDistinct:true,ownedAndExternalBrowserSeparated:true,reviewedTextOnlyExplicitCopy:true,byteEqualityRequired:true,choicesEpochDepartureAndOldDOMInvalidate:true,oldBridgeFallback:true,noNativeClipboardNetworkBrowserOrProduct:true}));
}
run().catch(error=>{console.error(error);process.exitCode=1});
