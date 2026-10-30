// Pure DOM fixtures. All profile persistence and native APIs are mocked; no
// browser, product directory, Windows API, proxy, clipboard or network is used.
const assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),crypto=require('node:crypto');
const {JSDOM}=require(process.argv[2]||'jsdom');
const root=path.resolve(__dirname,'../../..'),source=name=>fs.readFileSync(path.join(root,'web/dist',name),'utf8');
const dom=new JSDOM(source('index.html').replace(/<script[\s\S]*?<\/script>/g,''),{runScripts:'outside-only',url:'http://example.invalid/',pretendToBeVisual:true});
const w=dom.window,clone=value=>JSON.parse(JSON.stringify(value)),calls=[],errors=[],records=new Map();
const region={presetId:'en-us-pacific',language:'en-US',locale:'en-US',timezone:'America/Los_Angeles',acceptLanguages:['en-US','en']};
const config={schemaVersion:1,engineId:'webview2',profileId:'private',identity:{mode:'generated'},region,viewport:{width:1280,height:720},webrtc:'disable-non-proxied-udp',webgl:'runtime-default',webgpu:'runtime-default',permissions:{camera:'block',microphone:'block',geolocation:'block',notifications:'block',clipboardRead:'block'},cookies:'runtime-default',exitCleanup:'retain-profile'};
const state={platform:'windows',service:true,svcState:'running',version:'fixture',lang:'zh',theme:'light',view:{profiles:[],nodes:[],settings:{},state:{status:'disconnected'}}};
const privateProxy={host:'private-proxy.invalid',username:'fixture-user',password:'fixture-do-not-persist-password'};
let serial=0,saveMode='ok',deferred=null;
const hash=value=>crypto.createHash('sha256').update(value).digest('hex');
function makeRecord(request){
 const id=request.id||'p-'+(++serial).toString(16).padStart(32,'0');
 const result={schemaVersion:request.schemaVersion,id,name:request.name,meta:clone(request.meta||{}),extras:clone(request.extras||{}),revision:hash(JSON.stringify(request)+':'+(++serial)),config:{...clone(request.config),profileId:id},engine:clone(request.engine),updatedAt:'2026-10-26T00:00:00Z',applied:false,requiresLaunchValidation:true,recoveryNeeded:false};
 assert.equal(Object.hasOwn(result.config,'binding'),false,'binding must be stripped before save');
 records.set(id,clone(result));return result;
}
const api={
 GetState:async()=>state,
 PrivacyReport:async()=>({platform:'windows',controls:[],capabilities:[],warnings:[]}),
 PrivacyFirewallReport:async()=>({supported:true,rules:[],warnings:[]}),
 PrivacyHelperStatus:async()=>({state:'not_requested'}),
 PrivacyTimeReport:async()=>({supported:false,choices:[],status:'unsupported'}),
 BrowserHostReport:async()=>({supported:true,recommended:'webview',engines:[]}),
 BrowserProfileDefaults:async()=>({config:clone(config),regions:[clone(region)],graphicsPresets:[],environmentPresets:[],hostAvailable:true,status:'available'}),
 BrowserProfileIdentity:async text=>({mode:JSON.parse(text).identity.mode,status:'planned',userAgent:'Mozilla/5.0 Chrome/143.0.0.0',exactChromiumVersionKnown:false,applied:false}),
 BrowserProfilePreview:async text=>{const draft=JSON.parse(text);calls.push(['preview',clone(draft)]);return {config:clone(draft),hostAvailable:true}},
 BrowserProfilesList:async()=>{calls.push(['list']);return {records:Array.from(records.values(),clone),applied:false}},
 BrowserProfileSave:async text=>{
  const request=JSON.parse(text);calls.push(['save',clone(request)]);
  const serialized=JSON.stringify(request);
  for(const value of Object.values(privateProxy))assert.equal(serialized.includes(value),false,'proxy fields must stay in page memory');
  const allowed=request.schemaVersion===2?['config','engine','extras','meta','name','schemaVersion']:['config','engine','name','schemaVersion'];
  if(request.id)allowed.push('expectedRevision','id');
  assert.deepEqual(Object.keys(request).sort(),allowed.sort());
  assert.equal(Object.hasOwn(request.config,'binding'),false);
  if(saveMode==='conflict')throw Error('browser_profile_revision_conflict');
  if(request.id&&records.get(request.id)?.revision!==request.expectedRevision)throw Error('browser_profile_revision_conflict');
  if(deferred)return deferred.promise;
  return clone(makeRecord(request));
 },
 BrowserProfileLoad:async id=>{calls.push(['load',id]);const record=records.get(id);if(!record)throw Error('browser_profile_not_found');return clone(record)},
 BrowserProfileDelete:async(id,revision)=>{calls.push(['delete',id,revision]);if(records.get(id)?.revision!==revision)throw Error('browser_profile_revision_conflict');records.delete(id);return {id,deleted:true}},
 BrowserEngineVersions:async()=>[],
};
w.go={main:{App:new Proxy(api,{get:(object,key)=>object[key]||(()=>{throw Error('unexpected native API '+String(key))})})}};
w.runtime={EventsOn:()=>{}};w.fetch=()=>{throw Error('network forbidden')};
w.HTMLElement.prototype.scrollIntoView=function(){};
Object.defineProperty(w.navigator,'clipboard',{value:{writeText(){throw Error('clipboard forbidden')}}});
w.document.execCommand=()=>{throw Error('clipboard forbidden')};
w.addEventListener('error',event=>errors.push(String(event.error||event.message)));
const context=dom.getInternalVMContext();
for(const name of ['i18n.js','flags.js','privacy.js','app.js'])vm.runInContext(source(name),context,{filename:name});
const q=selector=>w.document.querySelector(selector),settle=()=>new Promise(resolve=>setTimeout(resolve,30));
const click=async selector=>{assert.ok(q(selector),selector);q(selector).click();await settle()};
const edit=(selector,value)=>{const element=q(selector);assert.ok(element,selector);element.value=value;element.dispatchEvent(new w.Event('input',{bubbles:true}))};
const choose=(selector,value)=>{const element=q(selector);assert.ok(element,selector);element.value=value;element.dispatchEvent(new w.Event('change',{bubbles:true}))};
const savedCalls=()=>calls.filter(call=>call[0]==='save');
const savedStatus=()=>q('.privacy-profile-library > p.muted');
const visible=element=>{if(!element||!element.isConnected)return false;for(let node=element;node;node=node.parentElement)if(node.hidden)return false;return true};
const hold=()=>{let resolve;const promise=new Promise(r=>resolve=r);return {promise,resolve}};
async function confirm(action){await click(action);assert.ok(q('#dialog-yes'),'destructive/replace confirmation');await click('#dialog-yes')}

async function run(){try{
 await settle();vm.runInContext("nav('privacy',{tab:'browser'})",context);await settle();
 assert.equal(calls.length,0,'opening browser editor never saves or enumerates records');
 assert.equal(q('#privacy-browser-profile-name').value,'');assert.ok(q('#privacy-browser-profile-save'));
	 // BrowserProfile v2 is a single editable contract: all persisted sections are
	 // present, cookies stay in memory, and URL validation happens before save.
	 assert.ok(q('#privacy-browser-environment-editor'),'v2 editor is visible');
	 const v2Fields=Array.from(w.document.querySelectorAll('[data-browser-v2]'));
	 assert.ok(v2Fields.length>=43,`expected at least 43 v2 fields, got ${v2Fields.length}`);
	 for(const path of ['meta.environmentName','meta.group','meta.tags','meta.remark','storage.cookieFormat','storage.merge','tabs.urls','tabs.startupMode','fingerprint.webrtc','fingerprint.timezone','fingerprint.geo','fingerprint.language','fingerprint.uiLanguage','fingerprint.resolution','fingerprint.fonts','fingerprint.noise.canvas','fingerprint.noise.webglImage','fingerprint.noise.audio','fingerprint.noise.media','fingerprint.noise.clientRects','fingerprint.noise.speechVoices','fingerprint.webglMetadata','fingerprint.webglVendor','fingerprint.webglRenderer','fingerprint.webgpu','advanced.ram','advanced.deviceName','advanced.mac','advanced.dnt','advanced.portScan','advanced.allowedPorts','advanced.hardwareAcceleration','advanced.tls','advanced.teamApps','advanced.dataSync','advanced.browserSettings','advanced.randomFingerprint']) assert.ok(q(`[data-browser-v2="${path}"]`),`missing v2 field ${path}`);
	 edit('[data-browser-v2="meta.environmentName"]','工作环境');
	 edit('[data-browser-v2-cookie]','[{"name":"sid","value":"memory-only"}]');
	 edit('[data-browser-v2="tabs.urls"]','https://example.com');
	 await click('#privacy-browser-v2-validate');
	 assert.match(q('#privacy-browser-environment-editor').textContent,/Cookie 已解析|配置校验通过/);
	 edit('[data-browser-v2="tabs.urls"]','file:///not-allowed');await click('#privacy-browser-v2-validate');
	 assert.match(q('#privacy-browser-environment-editor').textContent,/启动网址必须是完整/);
	 edit('[data-browser-v2="tabs.urls"]','https://example.com');await click('#privacy-browser-v2-validate');
 // The shared save footer works while a proxy draft is present, but that draft
 // is excluded from the persistence request and survives ordinary saves.
 edit('#privacy-proxy-host',privateProxy.host);edit('#privacy-proxy-username',privateProxy.username);edit('#privacy-proxy-password',privateProxy.password);
 edit('#privacy-browser-profile-name','中文 工作环境');edit('#privacy-browser-ua','  User-Chosen/1.0  ');
 await click('#privacy-browser-profile-save');
 const first=Array.from(records.values())[0],firstSave=savedCalls().at(-1)[1];
 assert.equal(firstSave.name,'中文 工作环境');assert.equal(Object.hasOwn(firstSave,'id'),false);assert.equal(Object.hasOwn(firstSave,'expectedRevision'),false);
 assert.deepEqual(firstSave.engine,{engineId:'webview2',version:''});assert.deepEqual(firstSave.config.identity,{mode:'manual',userAgent:'  User-Chosen/1.0  '});
 assert.match(first.id,/^p-[a-f0-9]{32}$/);assert.equal(q('#privacy-browser-id').value,first.id);assert.equal(q('#privacy-browser-profile-save').textContent,'保存修改');
 assert.equal(q('#privacy-proxy-password').value,privateProxy.password);assert.match(q('#privacy-browser-message').textContent,/配置已保存/);
 assert.match(savedStatus().textContent,/配置已保存/);
 const nameInput=q('#privacy-browser-profile-name'),beforeRenameSaveCount=savedCalls().length;
 nameInput.focus();nameInput.value='改名：办公室';nameInput.setSelectionRange(2,4);nameInput.dispatchEvent(new w.Event('input',{bubbles:true}));
 assert.match(savedStatus().textContent,/有未保存的修改/,'editing only the name must immediately replace the saved status');
 assert.equal(visible(q('#privacy-browser-message')),false,'the old saved success must disappear on name edit');
 assert.equal(q('#privacy-browser-profile-name'),nameInput,'typing must not redraw and replace the input');
 assert.equal(w.document.activeElement,nameInput);assert.equal(nameInput.selectionStart,2);assert.equal(nameInput.selectionEnd,4);
 assert.equal(savedCalls().length,beforeRenameSaveCount,'typing never saves automatically');
 await click('#privacy-browser-profile-save');assert.match(savedStatus().textContent,/配置已保存/);
 const renamed=clone(records.get(first.id)),renameRequest=savedCalls().at(-1)[1];assert.equal(renameRequest.id,first.id);assert.equal(renameRequest.expectedRevision,first.revision);assert.equal(renamed.name,'改名：办公室');assert.notEqual(renamed.revision,first.revision);
 // A conflict retains the current draft and the old expected revision.
 edit('#privacy-browser-profile-name','冲突时保留我的草稿');edit('#privacy-browser-width','1401');saveMode='conflict';await click('#privacy-browser-profile-save');
 assert.equal(q('#privacy-browser-profile-name').value,'冲突时保留我的草稿');assert.equal(q('#privacy-browser-width').value,'1401');assert.ok(q('.privacy-browser-profile .privacy-error'));assert.equal(q('#privacy-browser-message'),null);assert.equal(records.get(first.id).revision,renamed.revision);
 assert.match(savedStatus().textContent,/有未保存的修改/);
 const failedSave=savedCalls().length;edit('#privacy-browser-width','1401');
 assert.equal(visible(q('.privacy-browser-profile .privacy-error')),false,'editing after a failure must hide the stale error immediately');
 assert.equal(savedCalls().length,failedSave,'editing a rejected draft never retries the save');
 saveMode='ok';await click('#privacy-browser-profile-new');
 const newRequest=savedCalls().at(-1)[1],second=Array.from(records.values()).find(record=>record.id!==first.id);assert.equal(Object.hasOwn(newRequest,'id'),false);assert.equal(Object.hasOwn(newRequest,'expectedRevision'),false);assert.ok(second);assert.equal(second.name,'冲突时保留我的草稿');assert.equal(second.config.viewport.width,1401);
 await click('#privacy-browser-profile-list');assert.equal(q('#privacy-browser-profile-select').options.length,3);
 choose('#privacy-browser-profile-select',first.id);await click('#privacy-browser-profile-load');
 assert.equal(q('#privacy-browser-profile-name').value,renamed.name);assert.equal(q('#privacy-browser-id').value,first.id);assert.equal(q('#privacy-browser-width').value,'1280');assert.equal(q('#privacy-browser-ua').value,'  User-Chosen/1.0  ');
 // A name changes saved metadata only; it must not invalidate a checked set of
 // launch parameters. Editing an actual parameter invalidates the entire old
 // preview, including its explanatory caption, without an implicit API call.
 await click('#privacy-browser-preview');assert.equal(visible(q('#privacy-browser-preview-result')),true);
 const readyText=q('#privacy-browser-launch').textContent,beforeDraftEdits=calls.length;
 edit('#privacy-browser-profile-name','仅修改名称');
 assert.match(savedStatus().textContent,/有未保存的修改/);assert.equal(visible(q('#privacy-browser-preview-result')),true);
 assert.equal(q('#privacy-browser-launch').textContent,readyText,'name metadata does not invalidate launch configuration');
 const caption=Array.from(q('.privacy-browser-profile').querySelectorAll('p')).find(p=>p.textContent.includes('预览未应用设置'));
 assert.equal(visible(caption),true);edit('#privacy-browser-width','1281');
 assert.equal(q('#privacy-browser-preview-result').hidden,true,'parameter edit invalidates the checked preview');
 assert.equal(visible(caption),false,'the old preview caption disappears with the result');
 assert.equal(q('#privacy-browser-launch').textContent,'检查并打开');assert.equal(calls.length,beforeDraftEdits,'draft edits never invoke preview, save or launch');
 edit('#privacy-browser-width','1280');
 // Display names are rendered as text, including HTML-like ordinary names.
 edit('#privacy-browser-profile-name','<img src=x onerror=bad()> 中文');await click('#privacy-browser-profile-save');await click('#privacy-browser-profile-list');assert.equal(q('.privacy-profile-library img'),null);assert.ok(q('#privacy-browser-profile-select').textContent.includes('<img src=x onerror=bad()> 中文'));
 const beforeCancel=calls.filter(call=>call[0]==='delete').length;await click('#privacy-browser-profile-delete');await click('#dialog-no');assert.equal(calls.filter(call=>call[0]==='delete').length,beforeCancel);assert.ok(records.has(first.id));
 const expected=records.get(first.id).revision;await confirm('#privacy-browser-profile-delete');assert.equal(records.has(first.id),false);assert.equal(records.has(second.id),true);assert.deepEqual(calls.filter(call=>call[0]==='delete').at(-1),['delete',first.id,expected]);assert.equal(q('#privacy-browser-profile-name').value,'<img src=x onerror=bad()> 中文','deleting record preserves current draft');
 // Replacing an unsaved draft requires explicit confirmation; cancellation is
 // a UI decision and never invokes a backend load.
 choose('#privacy-browser-profile-select',second.id);const beforeLoad=calls.filter(call=>call[0]==='load').length;await click('#privacy-browser-profile-load');await click('#dialog-no');assert.equal(calls.filter(call=>call[0]==='load').length,beforeLoad);await confirm('#privacy-browser-profile-load');assert.equal(q('#privacy-browser-id').value,second.id);
 // A detached old control from a redraw cannot reissue its write action.
 const detached=q('#privacy-browser-profile-save'),detachedName=q('#privacy-browser-profile-name'),retainedName=detachedName.value;await click('#privacy-browser-profile-list');const beforeDetached=calls.length;detached.click();await settle();assert.equal(calls.length,beforeDetached,'detached save control must not call backend');
 detachedName.value='detached name must not replace current draft';detachedName.dispatchEvent(new w.Event('input',{bubbles:true}));
 await click('#privacy-browser-profile-list');assert.equal(q('#privacy-browser-profile-name').value,retainedName,'detached old name input cannot mutate the current draft');
 const departed=q('#privacy-browser-profile-save');await click('[data-privacy-tab="system"]');const beforeTab=calls.length;departed.click();await settle();assert.equal(calls.length,beforeTab,'browser controls cannot write from a different privacy tab');await click('[data-privacy-tab="browser"]');
 // Results from a request that outlives a tab departure must not replace the
 // draft, even if the user returned before the old promise settled.
 edit('#privacy-browser-profile-name','异步期间的草稿');deferred=hold();const pending=deferred;const currentRequest={schemaVersion:1,name:'过期返回不应覆盖',config:clone(config),engine:{engineId:'webview2',version:''}};
 q('#privacy-browser-profile-save').click();await settle();await click('[data-privacy-tab="system"]');await click('[data-privacy-tab="browser"]');pending.resolve(clone(makeRecord(currentRequest)));deferred=null;await settle();assert.equal(q('#privacy-browser-profile-name').value,'异步期间的草稿','stale successful reply must not replace draft');
 const pageOld=q('#privacy-browser-profile-save');vm.runInContext("nav('home')",context);await settle();const beforePage=calls.length;pageOld.click();await settle();assert.equal(calls.length,beforePage,'departed page must not call backend');
 for(const call of savedCalls()){const serialized=JSON.stringify(call[1]);for(const value of Object.values(privateProxy))assert.equal(serialized.includes(value),false);assert.equal(Object.hasOwn(call[1].config,'binding'),false)}
 assert.ok(calls.every(call=>['list','save','load','delete','preview'].includes(call[0])),'profile persistence never launches, tests a proxy or exports diagnostics');assert.equal(calls.filter(call=>call[0]==='preview').length,1,'only the explicit preview click invokes the mocked preview');assert.deepEqual(errors,[]);
 console.log('saved browser profiles DOM: create/load/rename/clone/delete, immediate dirty state, focus retention, stale success/error/preview removal, metadata-only rename, CAS draft retention, no proxy persistence, cancellation and stale-result guards passed');
}finally{dom.window.close()}}
run().catch(error=>{console.error(error);process.exitCode=1});
