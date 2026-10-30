// Isolated native-engine form fixtures: fixed in-memory Wails replies only.
// No browser process, clipboard, service, Windows API, or external requests.
'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {JSDOM}=require(process.argv[2]||'jsdom');
const root=path.resolve(__dirname,'../../..'),source=name=>fs.readFileSync(path.join(root,'web/dist',name),'utf8');
const dom=new JSDOM(source('index.html').replace(/<script[\s\S]*?<\/script>/g,''),{runScripts:'outside-only',url:'http://example.invalid/',pretendToBeVisual:true});
const w=dom.window,calls=[],errors=[],clone=value=>JSON.parse(JSON.stringify(value));
const regions=[{presetId:'en-us',language:'en-US',locale:'en-US',timezone:'America/Los_Angeles',acceptLanguages:['en-US','en']},{presetId:'zh-cn',language:'zh-CN',locale:'zh-CN',timezone:'Asia/Shanghai',acceptLanguages:['zh-CN','zh']}];
const environments=[['windows','windows-desktop','10.0.0',1280,720],['macos','macos-desktop','13.0.0',1280,720],['android','pixel-7','13.0.0',412,915],['ios','iphone-14','16.0.0',390,844]].map(([os,id,osVersion,width,height])=>({config:{mode:'browser-emulation',os,devicePresetId:id,osVersion},recommendedViewport:{width,height}}));
const config={schemaVersion:1,engineId:'webview2',profileId:'private',identity:{mode:'generated'},region:regions[0],viewport:{width:1280,height:720},webrtc:'disable-non-proxied-udp',webgl:'runtime-default',webgpu:'runtime-default',permissions:{camera:'block',microphone:'block',geolocation:'block',notifications:'block',clipboardRead:'block'},cookies:'runtime-default',exitCleanup:'retain-profile'};
const inventory=[['chrome-for-testing','156.0.8078.47',true,true],['chrome-for-testing','155.0.8000.9',true,true],['firefox','147.0.1',true,true],['firefox','146.0.2',true,true],['firefox','145.0.0',false,true],['chrome-for-testing','154.0.1.1',true,false]].map(([engineId,version,treeVerified,versionVerified])=>({engineId,version,treeVerified,versionVerified,canLaunch:true,fileCount:17}));
let heldIdentity=null,heldInventory=null,identityRefused=false;
function planned(c,native){
  const identity=c.identity||{mode:'generated'},env=c.environment;
  const version=native?c.version:'150.0.0.0',engine=native?c.engineId:'webview2';
  const major=version.split('.')[0],os=env?env.os:'windows',parts=(env?env.osVersion:'10.0.0').split('.');while(parts.length<3)parts.push('0');
  const mobile=os==='android'||os==='ios';
  let token='Windows NT 10.0; Win64; x64',generated;
  if(engine==='firefox'){
    if(os==='macos')token='Macintosh; Intel Mac OS X 10.15';
    else if(os==='android'){const release=parts.slice();while(release.length>1&&release.at(-1)==='0')release.pop();const android=Number(parts[0])<10?'10':Number(major)>=158?parts[0]:release.join('.');token='Android '+android;}
    else if(os==='ios')token='iPhone; CPU iPhone OS '+parts.join('_')+' like Mac OS X';
    generated='Mozilla/5.0 ('+token+(mobile?'; Mobile':'')+'; rv:'+major+'.0) Gecko/'+(mobile?major+'.0':'20100101')+' Firefox/'+major+'.0';
  }else{
    if(os==='macos')token='Macintosh; Intel Mac OS X 10_15_7';
    else if(os==='android')token='Linux; Android 10; K';
    else if(os==='ios')token='iPhone; CPU iPhone OS '+parts.join('_')+' like Mac OS X';
    generated='Mozilla/5.0 ('+token+') AppleWebKit/537.36 (KHTML, like Gecko) Chrome/'+major+'.0.0.0'+(mobile?' Mobile':'')+' Safari/537.36';
  }
  return {mode:identity.mode,status:'planned',userAgent:identity.mode==='manual'?identity.userAgent:identity.mode==='native'?'':generated,userAgentTemplate:identity.mode==='native'?'{NativeUserAgent}':'',runtimeProductVersion:version,versionSource:'selected-official-version-not-running',exactChromiumVersionKnown:false,requiresRuntimeReadback:true,applied:false,warnings:os==='ios'?['合成iOS环境，不是零售iOS浏览器或Safari']:[]};
}
const api={
  GetState:async()=>({platform:'windows',service:true,lang:'zh',theme:'light',view:{profiles:[],nodes:[],settings:{},state:{status:'disconnected'}}}),
  PrivacyReport:async()=>({platform:'windows',controls:[],capabilities:[],warnings:[]}),PrivacyFirewallReport:async()=>({supported:true,rules:[]}),PrivacyHelperStatus:async()=>({state:'not_requested'}),PrivacyTimeReport:async()=>({supported:false,choices:[],status:'unsupported'}),BrowserHostReport:async()=>({supported:true,engines:[]}),
  BrowserProfileDefaults:async()=>({config:clone(config),regions:clone(regions),environmentPresets:clone(environments),graphicsPresets:[],identityPreview:planned(config,false),hostAvailable:true}),
  BrowserProfileIdentity:async text=>{const c=JSON.parse(text);calls.push(['webview-identity',c]);return planned(c,false);},
  BrowserProfilePreview:async text=>{const c=JSON.parse(text);calls.push(['webview-preview',c]);return {config:c,identityPreview:planned(c,false),hostAvailable:true};},
  BrowserProfileLaunch:async text=>{calls.push(['webview-launch',JSON.parse(text)]);throw Error('UNEXPECTED_WEBVIEW_LAUNCH');},
  BrowserEngineVersions:async()=>{calls.push(['inventory']);return heldInventory?heldInventory.promise:clone(inventory);},
  BrowserEngineIdentity:async text=>{const c=JSON.parse(text);calls.push(['native-identity',c]);if(identityRefused)throw Error('fixture identity refused');return heldIdentity?heldIdentity.promise:planned(c,true);},
  BrowserEngineProfileLaunch:async(text,proxyJSON)=>{calls.push(['native-launch',JSON.parse(text),JSON.parse(proxyJSON)]);throw Error('NATIVE_VM_VALIDATION_PENDING');},
  BrowserEngineProfileClose:async()=>{calls.push(['native-close']);},
  BrowserProxyTest:async()=>{calls.push(['proxy-test']);throw Error('unexpected proxy probe');},
};
w.go={main:{App:new Proxy(api,{get:(object,key)=>object[key]||(()=>Promise.resolve())})}};w.runtime={EventsOn:()=>{}};
w.fetch=()=>{throw Error('network forbidden in native DOM fixture');};w.HTMLElement.prototype.scrollIntoView=function(){};
w.addEventListener('error',event=>errors.push(String(event.error||event.message)));
const context=dom.getInternalVMContext();for(const name of ['i18n.js','flags.js','privacy.js','app.js'])vm.runInContext(source(name),context,{filename:name});
const q=selector=>w.document.querySelector(selector),alertText=()=>Array.from(w.document.querySelectorAll('[role="alert"]')).map(node=>node.textContent||'').join('\n'),wait=(ms=30)=>new Promise(resolve=>setTimeout(resolve,ms));
async function until(predicate,label){const end=Date.now()+3000;while(!predicate()&&Date.now()<end)await wait(20);assert.ok(predicate(),label);}
const edit=(id,value)=>{const input=q(id);assert.ok(input,'missing '+id);input.value=value;input.dispatchEvent(new w.Event('input',{bubbles:true}));};
const editSelector=(selector,value)=>{const input=q(selector);assert.ok(input,'missing '+selector);input.value=value;input.dispatchEvent(new w.Event('input',{bubbles:true}));};
const choose=(id,value)=>{const input=q(id);assert.ok(input,'missing '+id);input.value=value;input.dispatchEvent(new w.Event('change',{bubbles:true}));};
const count=kind=>calls.filter(call=>call[0]===kind).length,last=kind=>calls.filter(call=>call[0]===kind).at(-1);
const proxy={type:'socks5',host:'fixture.proxy.invalid',port:1080,username:'fixture-user',password:'fixture-secret'};
async function selectEngine(engine){const before=count('inventory'),beforeIdentity=count('native-identity'),mode=q('#privacy-browser-identity-mode').value;choose('#privacy-browser-engine',engine);await until(()=>count('inventory')===before+1&&!q('#privacy-browser-engine').disabled,'native selection completes installed inventory read');if(mode!=='manual')await until(()=>{if(count('native-identity')<=beforeIdentity)return false;const request=last('native-identity')[1],version=q('#privacy-browser-engine-version').value;if(request.engineId!==engine||request.version!==version)return false;return mode==='native'?q('#privacy-browser-ua').value===''&&q('#privacy-browser-ua').placeholder==='{NativeUserAgent}':q('#privacy-browser-ua').value.includes((engine==='firefox'?'Firefox/':'Chrome/')+version.split('.')[0]+'.0');},'selected engine produces the reduced UA or preserves the native runtime placeholder');assert.equal(count('inventory'),before+1,'native selection explicitly reads installed inventory');assert.equal(q('#privacy-browser-route').value,'proxy');}

async function run(){try{
  await wait();vm.runInContext("nav('privacy',{tab:'browser'})",context);await wait(270);
  assert.equal(q('#privacy-browser-engine').value,'webview2');assert.equal(q('#privacy-browser-engine-version').disabled,true);
  assert.equal(count('inventory'),0,'opening the form does not request engine inventory');assert.equal(count('native-launch'),0);
  await selectEngine('firefox');
  assert.deepEqual(Array.from(q('#privacy-browser-engine-version').options,option=>option.value),['','147.0.1','146.0.2'],'only versions with both integrity checks are selectable');
  assert.equal(q('#privacy-browser-engine-version').value,'147.0.1');assert.match(q('#privacy-browser-ua').value,/Firefox\/147\.0$/,JSON.stringify({calls:calls.map(c=>c[0]),status:q('#privacy-browser-ua-status').textContent}));
  assert.equal(last('native-identity')[1].engineId,'firefox');assert.equal(last('native-identity')[1].version,'147.0.1');
  choose('#privacy-browser-engine-version','146.0.2');await wait(260);assert.match(q('#privacy-browser-ua').value,/Firefox\/146\.0$/,'Firefox UA uses selected actual major while the catalog retains its full version');
  await selectEngine('chrome-for-testing');assert.deepEqual(Array.from(q('#privacy-browser-engine-version').options,option=>option.value),['','156.0.8078.47','155.0.8000.9']);assert.match(q('#privacy-browser-ua').value,/Chrome\/156\.0\.0\.0 /);
  choose('#privacy-browser-engine-version','155.0.8000.9');await wait(260);assert.match(q('#privacy-browser-ua').value,/Chrome\/155\.0\.0\.0 /,'Chrome UA uses reduced version while the full selected binary version is retained');
  // Golden mock replies exercise the UI's complete text handling; they are not
  // observations of a running engine or proof of browser impersonation.
  const generatedSamples={
    'chrome-for-testing':[
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36',
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36',
      'Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Mobile Safari/537.36',
      'Mozilla/5.0 (iPhone; CPU iPhone OS 16_0_0 like Mac OS X) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Mobile Safari/537.36',
    ],
    firefox:[
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:147.0) Gecko/20100101 Firefox/147.0',
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:147.0) Gecko/20100101 Firefox/147.0',
      'Mozilla/5.0 (Android 13; Mobile; rv:147.0) Gecko/147.0 Firefox/147.0',
      'Mozilla/5.0 (iPhone; CPU iPhone OS 16_0_0 like Mac OS X; Mobile; rv:147.0) Gecko/147.0 Firefox/147.0',
    ],
  };
  for(const engine of ['chrome-for-testing','firefox']){
    await selectEngine(engine);
    const fullVersion=q('#privacy-browser-engine-version').value;
    for(const [index,preset]of environments.entries()){
      const before=count('native-identity');choose('#privacy-browser-environment',preset.config.devicePresetId);
      await until(()=>count('native-identity')>before&&q('#privacy-browser-ua').value===generatedSamples[engine][index],'generated '+engine+' '+preset.config.os+' UA is displayed exactly');
      const request=last('native-identity')[1];assert.deepEqual(request.identity,{mode:'generated'});assert.deepEqual(request.environment,preset.config);assert.equal(request.version,fullVersion,'UA reduction never rewrites the selected engine binary version');
      assert.equal(q('#privacy-browser-ua').value.includes(fullVersion),false,'full product patch version is not inserted into the reduced UA');
      if(preset.config.os==='ios'){assert.equal(q('#privacy-browser-ua').value.includes('CriOS/'),false);assert.equal(q('#privacy-browser-ua').value.includes('FxiOS/'),false);assert.match(q('#privacy-browser-environment').closest('label').innerHTML,/合成环境/,'iOS controls disclose synthetic environment');}
    }
  }
  const manual=' Exact manually edited UA/17 "quoted" \\path ';
  edit('#privacy-browser-ua',manual);assert.equal(q('#privacy-browser-identity-mode').value,'manual');const previewsBeforeManual=count('native-identity');
  await selectEngine('firefox');choose('#privacy-browser-engine-version','146.0.2');await wait(260);assert.equal(q('#privacy-browser-ua').value,manual);assert.equal(count('native-identity'),previewsBeforeManual,'changing engine/version never regenerates a manual UA');
  edit('#privacy-proxy-host',proxy.host);edit('#privacy-proxy-username',proxy.username);edit('#privacy-proxy-password',proxy.password);edit('#privacy-browser-id','native-private');choose('#privacy-browser-region','zh-cn');
  editSelector('[data-browser-v2="tabs.urls"]','https://startup.invalid/');choose('[data-browser-v2="tabs.startupMode"]','urls');await wait();q('#privacy-browser-launch').click();await wait();assert.equal(last('native-launch')[1].startupURL,'https://startup.invalid/','one startup URL reaches native request');
  const startupBefore=count('native-launch');editSelector('[data-browser-v2="tabs.urls"]','https://startup.invalid/\nhttps://second.invalid/');await wait();q('#privacy-browser-launch').click();await wait();assert.equal(count('native-launch'),startupBefore,'multiple startup URLs remain blocked');assert.match(alertText(),/最多支持 1 个 URL/);editSelector('[data-browser-v2="tabs.urls"]','');choose('[data-browser-v2="tabs.startupMode"]','restore');await wait();
  for(const [engine,version]of [['firefox','146.0.2'],['chrome-for-testing','155.0.8000.9']]){
   await selectEngine(engine);choose('#privacy-browser-engine-version',version);
   for(const [index,preset] of environments.entries()){
    choose('#privacy-browser-environment',preset.config.devicePresetId);edit('#privacy-browser-os-version',String(21+index)+'.2');const mobile=['android','ios'].includes(preset.config.os);edit('#privacy-browser-width',mobile?400+index:1400+index);edit('#privacy-browser-height',mobile?800+index:900+index);
    const expected={engineId:engine,version,profileId:'native-private',language:'zh-CN',timezone:'Asia/Shanghai',width:mobile?400+index:1400+index,height:mobile?800+index:900+index,identity:{mode:'manual',userAgent:manual},environment:{...preset.config,osVersion:String(21+index)+'.2'},exitCleanup:'retain-profile'};
    const previewsBefore=count('native-identity');q('#privacy-browser-preview').click();await wait();assert.equal(count('native-identity'),previewsBefore+1);assert.deepEqual(last('native-identity')[1],expected,'preview forwards all identity and environment fields');assert.equal(q('#privacy-browser-ua').value,manual);
    q('#privacy-browser-launch').click();await wait();assert.deepEqual(last('native-launch')[1],expected,'native launch preserves the complete edited form');assert.deepEqual(last('native-launch')[2],proxy);assert.match(q('.privacy-browser-profile [role="alert"]').textContent,/NATIVE_VM_VALIDATION_PENDING/);
    assert.equal(Object.hasOwn(last('native-launch')[1],'binding'),false);assert.equal(Object.hasOwn(last('native-launch')[1].identity,'clientHints'),false);assert.equal(q('#privacy-browser-ua').value,manual);
   }
  }
  await selectEngine('chrome-for-testing');choose('#privacy-browser-engine-version','155.0.8000.9');q('#privacy-browser-launch').click();await wait();const chromeLaunch=last('native-launch')[1];assert.deepEqual(chromeLaunch,{...last('native-identity')[1],engineId:'chrome-for-testing',version:'155.0.8000.9'},'main launch uses the selected Chrome binary/version while preserving every environment and manual-UA field');
  q('#privacy-browser-close').click();await wait();assert.equal(count('native-close'),1,'unified Close uses the selected native lifecycle');await selectEngine('firefox');choose('#privacy-browser-engine-version','146.0.2');
  choose('#privacy-engine-kind','firefox');edit('#privacy-engine-version','146.0.2');choose('#privacy-engine-route','proxy');q('#privacy-engine-launch').click();await wait();assert.deepEqual(last('native-launch')[1],{...last('native-identity')[1],engineId:'firefox',version:'146.0.2'},'legacy manager launch forwards the same complete edited form');
  choose('#privacy-browser-identity-mode','native');
  for(const [engine,version]of [['chrome-for-testing','155.0.8000.9'],['firefox','146.0.2']]){
    await selectEngine(engine);choose('#privacy-browser-engine-version',version);await wait(260);
    for(const preset of environments){
      choose('#privacy-browser-environment',preset.config.devicePresetId);await wait(260);
      q('#privacy-browser-preview').click();await wait();
      await until(()=>q('#privacy-browser-ua').value===''&&q('#privacy-browser-ua').placeholder==='{NativeUserAgent}','native preview settles without synthesizing a UA');
      const request=last('native-identity')[1];assert.equal(request.engineId,engine);assert.equal(request.version,version);assert.deepEqual(request.environment,preset.config);assert.deepEqual(request.identity,{mode:'native'},'native UA mode survives all four independent environment selections');
      assert.equal(q('#privacy-browser-ua').value,'','UI does not invent a native UA before runtime readback');assert.equal(q('#privacy-browser-ua').placeholder,'{NativeUserAgent}');
      q('#privacy-browser-launch').click();await wait();assert.deepEqual(last('native-launch')[1],request,'native launch keeps the mode and independent environment unchanged');assert.equal(Object.hasOwn(last('native-launch')[1].identity,'userAgent'),false);assert.equal(Object.hasOwn(last('native-launch')[1].identity,'clientHints'),false);
    }
  }
  for(const kind of ['graphics','webgpu']){
    assert.equal(q('#privacy-browser-engine').value,'firefox');
    if(kind==='graphics'){choose('#privacy-browser-graphics','manual');edit('#privacy-browser-gpu-vendor','Manual GPU vendor');edit('#privacy-browser-gpu-renderer','Manual GPU renderer');}else{choose('#privacy-browser-graphics','native');choose('#privacy-browser-webgpu','disabled');}
    assert.match(q('#privacy-browser-graphics-capability').textContent,/Gecko 原生偏好/);assert.match(q('#privacy-browser-graphics-capability').textContent,/没有 Worker API 运行读回/);
    const supported=count('native-launch');q('#privacy-browser-launch').click();await wait();assert.equal(count('native-launch'),supported+1,'Firefox '+kind+' reaches its native host');assert.equal(last('native-launch')[1].webgpu,'disabled');
    if(kind==='graphics')assert.deepEqual(last('native-launch')[1].graphics,{mode:'webgl-metadata',presetId:'manual',vendor:'Manual GPU vendor',renderer:'Manual GPU renderer'});
    else assert.equal(Object.hasOwn(last('native-launch')[1],'graphics'),false,'WebGPU-only request does not insert a name override');
    choose('#privacy-engine-kind','firefox');edit('#privacy-engine-version','146.0.2');q('#privacy-engine-launch').click();await wait();assert.equal(count('native-launch'),supported+2,'legacy Firefox also forwards its supported graphics request');assert.equal(last('native-launch')[1].webgpu,'disabled');
    await selectEngine('chrome-for-testing');
    choose('#privacy-engine-kind','chrome-for-testing');edit('#privacy-engine-version','156.0.8078.47');
    assert.match(q('#privacy-browser-graphics-capability').textContent,/仅支持原生 GPU/);assert.equal(q('#privacy-browser-graphics option[value="manual"]').disabled,true);assert.equal(q('#privacy-browser-webgpu option[value="disabled"]').disabled,true);
    const before=count('native-launch');q('#privacy-browser-launch').click();await wait();assert.equal(count('native-launch'),before,'unsupported '+kind+' cannot be silently dropped by main launch');assert.match(alertText(),/不会忽略这些参数|宿主尚未提供/);
    q('#privacy-engine-launch').click();await wait();assert.equal(count('native-launch'),before,'unsupported '+kind+' cannot be silently dropped by legacy launch');assert.match(alertText(),/不会忽略这些参数|宿主尚未提供/);
    await selectEngine('firefox');choose('#privacy-browser-engine-version','146.0.2');await wait(260);
    if(kind==='graphics')assert.equal(q('#privacy-browser-gpu-renderer').value,'Manual GPU renderer','unsupported engine selection preserves the metadata draft');
  }
  choose('#privacy-browser-webgpu','runtime-default');
  let resolve;heldIdentity={promise:new Promise(done=>{resolve=done;})};const pendingManual=count('native-identity');q('#privacy-browser-ua-generate').click();await until(()=>count('native-identity')===pendingManual+1,'native preview pending before manual edit');edit('#privacy-browser-ua','manual changed while native preview pending');resolve({...planned(last('native-identity')[1],true),userAgent:'STALE_NATIVE_UA_MUST_NOT_APPEAR'});await wait();heldIdentity=null;assert.equal(q('#privacy-browser-ua').value,'manual changed while native preview pending');
  q('#privacy-browser-ua-generate').click();await wait(260);assert.match(q('#privacy-browser-ua').value,/Firefox\/146\.0$/);
  identityRefused=true;q('#privacy-browser-ua-generate').click();await until(()=>q('#privacy-browser-ua-status').textContent.includes('暂未生成'),'identity refusal is shown as incomplete');assert.equal(q('#privacy-browser-ua').value,'');assert.equal(w.document.body.textContent.includes('fixture identity refused'),false,'internal preview failure details are not displayed');identityRefused=false;q('#privacy-browser-ua-generate').click();await until(()=>q('#privacy-browser-ua').value.endsWith('Firefox/146.0'),'identity preview recovers only after explicit retry');
  let resolveOld;heldIdentity={promise:new Promise(done=>{resolveOld=done;})};const pendingVersion=count('native-identity');choose('#privacy-browser-engine-version','147.0.1');await until(()=>count('native-identity')===pendingVersion+1,'old-version native preview pending');heldIdentity=null;choose('#privacy-browser-engine-version','146.0.2');await until(()=>q('#privacy-browser-ua').value.endsWith('Firefox/146.0'),'new-version UA completed first');const currentUA=q('#privacy-browser-ua').value;resolveOld({...planned(last('native-identity')[1],true),userAgent:'STALE_WRONG_VERSION_UA'});await wait();assert.equal(q('#privacy-browser-ua').value,currentUA,'late old-version preview cannot overwrite selected-version UA');
  let resolvePage;heldIdentity={promise:new Promise(done=>{resolvePage=done;})};const pendingPage=count('native-identity');q('#privacy-browser-ua-generate').click();await until(()=>count('native-identity')===pendingPage+1,'outgoing-page native preview pending');const outgoing=q('.privacy-browser-profile'),outgoingText=outgoing.textContent;vm.runInContext("nav('home')",context);resolvePage({...planned(config,false),userAgent:'STALE_OUTGOING_PAGE_UA'});await wait();heldIdentity=null;assert.equal(outgoing.textContent,outgoingText);assert.equal(w.document.body.textContent.includes('STALE_OUTGOING_PAGE_UA'),false);
  assert.equal(count('proxy-test'),0,'engine selection, previews and launches do not treat an IP probe as authorization');assert.equal(count('webview-launch'),0);assert.equal(w.document.body.textContent.includes(proxy.password),false);assert.deepEqual(errors,[]);
  console.log(JSON.stringify({scope:'isolated native engine DOM',verifiedVersionSelection:true,modernReducedChromeAndFirefoxUAMocks:true,fourOSGeneratedUADisplayedExactly:true,fourOSManualUAPreservedBothEngines:true,fourOSNativeModePreservedWithoutInventedUA:true,fullCatalogVersionRetained:true,fourEnvironmentFieldsForwarded:true,legacyManagerFieldsForwarded:true,firefoxGraphicsAndWebGPUForwarded:true,chromeGraphicsCapabilityShownBeforeEditing:true,unsupportedGPUExplicitlyRefused:true,workerRuntimeProbeNotClaimed:true,staleManualVersionAndPagePreviewBlocked:true,noRuntimeOrNetwork:true}));
}finally{w.close();}}
run().catch(error=>{console.error(error);process.exitCode=1;});
