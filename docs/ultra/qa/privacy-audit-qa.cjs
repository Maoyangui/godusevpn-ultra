// Pixel/interaction QA with fixed mocks. Pass existing runtime/browser paths;
// this script never installs dependencies, starts Wails or calls product APIs.
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
if (process.argv.length < 4) { console.log('Usage: node privacy-audit-qa.cjs <Playwright module> <browser executable> [screenshots directory]'); process.exit(2); }
const { chromium } = require(process.argv[2]);
const root = path.resolve(__dirname, '../../..');
// The only executed Go fixture emits the static help catalog. It cannot start
// Wails, inspect Windows, access product state or call any native backend.
const guides=JSON.parse(require('node:child_process').execFileSync('go',['run','./tools/qa/center-guides-fixture'],{cwd:root,encoding:'utf8',env:{...process.env,GOTOOLCHAIN:'local',GOPROXY:'off',GOSUMDB:'off'}}));
const out = process.argv[4] ? path.resolve(process.argv[4]) : path.join(process.env.USERPROFILE || root, '.codex/tmp/qa8-ui');
const extension = require(path.join(root, 'browser-extension/core.js'));
fs.mkdirSync(out, { recursive: true });
const report = { platform:'windows',admin:true,observerElevated:false,backupHealthy:true,warnings:['固定 mock：没有读取用户真实设置。','配置回读不是实际隔离证明。'],capabilities:[],controls:[
 {id:'fixture-windows',title:'示例系统策略',category:'windows',description:'通过支持的固定策略限制指定信息。',scope:'支持的 Windows 版本',impact:'可能影响依赖此功能的应用。',status:'available',readStatus:'different',canApply:true,managed:false,requiresAdmin:true},
 {id:'fixture-browser',title:'Chrome：网页定位策略',category:'browser',description:'默认阻止网页定位请求。',scope:'当前用户 / Chrome',impact:'需要定位的站点可能无法工作；IP 和账号仍可关联位置。',status:'configured',readStatus:'match',canApply:true,managed:true,requiresAdmin:false,restart:true}
] };
const detection = {generatedAt:'2026-10-19T16:00:00Z',observer:'固定 mock：普通权限观察进程',admin:false,helperAuthorized:true,warnings:['纯 mock；不代表本机状态。'],findings:[
 {id:'locale',title:'示例地区格式',audience:'app',category:'环境信息',status:'observed',summary:'固定合成环境字段。',evidence:'本地示例',scope:'仅模拟来源',source:'facts',detectedAt:'2026-10-19T15:59:58Z'},
 {id:'browser-fixture',title:'浏览器真实环境尚未检查',audience:'website',category:'网页指纹',status:'unknown',summary:'请在实际浏览器配置检查。',evidence:'没有把 WebView 当作真实网站',scope:'所选浏览器',settingsId:'browser'},
 {id:'policy-fixture',title:'示例固定策略',audience:'app',category:'系统隐私',status:'unknown',summary:'配置证据不代表运行时行为。',evidence:'固定 mock',scope:'有限范围',controlId:'fixture-windows'}
]};
const fw = {supported:true,elevated:true,loopback:'unprotected',scope:'仅指定程序；回环、代理和转发另行判断。',warnings:['定向广播、单播发现、蓝牙和系统代发未覆盖。'],rules:[{rule:{id:'fixture-rule',program:'C:\\Fixture\\application.exe',mode:'block_discovery'},state:'filters_installed',installedFilters:3,expectedFilters:3,detail:'安装条件证据，未做流量测试。'}]};
const environmentPresets=[
 {config:{mode:'browser-emulation',os:'windows',devicePresetId:'windows-desktop',osVersion:'10.0.0'},recommendedViewport:{width:1280,height:720}},
 {config:{mode:'browser-emulation',os:'macos',devicePresetId:'macos-desktop',osVersion:'13.0.0'},recommendedViewport:{width:1280,height:720}},
 {config:{mode:'browser-emulation',os:'android',devicePresetId:'pixel-7',osVersion:'13.0.0'},model:'Pixel 7',recommendedViewport:{width:412,height:915}},
 {config:{mode:'browser-emulation',os:'ios',devicePresetId:'iphone-14',osVersion:'16.0.0'},model:'iPhone',recommendedViewport:{width:390,height:844}}
];
const metadataFields=[{id:'artist',label:'EXIF 作者'},{id:'copyright',label:'EXIF 版权'},{id:'description',label:'EXIF 描述'},{id:'software',label:'EXIF 软件'},{id:'camera',label:'EXIF 相机、镜头与序列号'},{id:'capture-time',label:'EXIF 拍摄与修改时间'},{id:'exif-gps',label:'EXIF GPS 整组（不含 XMP 位置）'}];
async function run() {
 const browser = await chromium.launch({executablePath:process.argv[3],headless:true,args:['--disable-background-networking','--disable-component-update','--disable-sync','--no-first-run','--no-default-browser-check']});
 const result={scope:'fixed local UI mocks only',screenshots:[],errors:[],blockedRequests:[],layoutIssues:[],checks:[]};
 try {
  for(const width of [420,900]) for(const theme of ['light','dark']) {
   const context=await browser.newContext({viewport:{width,height:width===420?760:900},colorScheme:theme,serviceWorkers:'block'});
   await context.route('**/*',route=>{
    const url=new URL(route.request().url());
    if(url.origin!=='http://127.0.0.1:48737') {result.blockedRequests.push(url.origin);return route.abort();}
    const isExtension=url.pathname.startsWith('/extension/'), base=path.join(root,isExtension?'browser-extension':'web/dist');
    const name=decodeURIComponent(isExtension?url.pathname.slice('/extension/'.length):url.pathname==='/'?'index.html':url.pathname.slice(1));
    const file=path.resolve(base,name);
    if(!file.startsWith(base+path.sep)||!fs.existsSync(file))return route.fulfill({status:404,body:''});
    return route.fulfill({status:200,contentType:file.endsWith('.html')?'text/html; charset=utf-8':file.endsWith('.css')?'text/css; charset=utf-8':'text/javascript; charset=utf-8',body:fs.readFileSync(file)});
   });
   await context.addInitScript(({report,detection,fw,theme,extensionRecords,environmentPresets,metadataFields,guides})=>{
    window.__qaCalls=[];
    const state={platform:'windows',service:true,version:'fixed mock',lang:'zh',theme,view:{profiles:[],nodes:[],state:{status:'disconnected'}}};
    const api={GetState:async()=>state,PrivacyReport:async()=>report,PrivacyHelperStatus:async()=>({state:'connected'}),PrivacyFirewallReport:async()=>fw,PrivacyTimeReport:async()=>({supported:true,currentId:'UTC',choices:[{id:'UTC',label:'(UTC) 协调世界时'},{id:'China Standard Time',label:'(UTC+08:00) 北京'}],managed:false,status:'available',canApply:true,detail:'固定 mock；未修改系统。',daylightSavingDisabled:false}),PrivacyDetectionSnapshot:async()=>({snapshotId:'mock-source',expiresAt:new Date(Date.now()+600000).toISOString(),report:detection}),PrivacyPreviewDetectionExport:async selection=>{const findings=detection.findings.filter(f=>(selection.scope==='all'||selection.findingIds.includes(f.id))&&(selection.includeEnvironment||!['locale','timezone'].includes(f.id)));return {previewId:'mock-preview',expiresAt:new Date(Date.now()+600000).toISOString(),count:findings.length,content:JSON.stringify({scope:selection.scope,findings},null,2)};} };
    api.PrivacyModuleGuides=async()=>guides;
    let reviewedExport=null;const originalPreview=api.PrivacyPreviewDetectionExport;api.PrivacyPreviewDetectionExport=async selection=>(reviewedExport=await originalPreview(selection));
    api.PrivacyDetectionTextPreview=async previewId=>({previewId,content:'Ultra 本地检测摘要\n范围：当前筛选 · 固定 mock\n无法确认不表示安全。\n仅是界面示例，没有读取系统或复制剪贴板。',sha256:'a'.repeat(64),count:reviewedExport.count,expiresAt:reviewedExport.expiresAt});
    api.PrivacyDetectionTextForCopy=async()=>{throw Error('Visual QA cannot copy a real clipboard.');};
    let maximized=false;api.IsMaximized=async()=>maximized;api.ToggleMaximize=async()=>{maximized=!maximized;return maximized;};
    const region={presetId:'fixture-region',language:'zh-CN',locale:'zh-CN',timezone:'Asia/Shanghai',acceptLanguages:['zh-CN','zh']};
    const browserConfig={schemaVersion:1,engineId:'webview2',profileId:'private',identity:{mode:'generated'},region,viewport:{width:1280,height:720},webrtc:'disable-non-proxied-udp',webgl:'runtime-default',webgpu:'runtime-default',permissions:{camera:'block',microphone:'block',geolocation:'block',notifications:'block',clipboardRead:'block'},cookies:'runtime-default',exitCleanup:'retain-profile'};
    const plannedIdentity=c=>{const env=c.environment,version=env&&env.osVersion||'10.0.0',os=env&&env.os||'windows',token={windows:'Windows NT 10.0; Win64; x64',macos:'Macintosh; Intel Mac OS X 10_15_7',android:'Linux; Android 10; K',ios:'iPhone; CPU iPhone OS '+version.replace(/\./g,'_')+' like Mac OS X'}[os];return {mode:c.identity.mode,status:'planned',userAgent:c.identity.mode==='manual'?c.identity.userAgent:c.identity.mode==='native'?'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36':'Mozilla/5.0 ('+token+') AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0'+(['ios','android'].includes(os)?' Mobile':'')+' Safari/537.36',applied:false,requiresRuntimeReadback:true,exactChromiumVersionKnown:false};};
    api.BrowserHostReport=async()=>({supported:true,recommended:'webview',engines:[{id:'webview',name:'WebView2',version:'fixture',available:true,detail:'固定 mock，不启动真实主机。'}]});
    const graphicsPresets=[{mode:'webgl-metadata',presetId:'fixture-gpu',vendor:'Google Inc. (NVIDIA)',renderer:'ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0, D3D11)'}];
    api.BrowserProfileDefaults=async()=>({config:JSON.parse(JSON.stringify(browserConfig)),regions:[region],graphicsPresets,environmentPresets,identityPreview:plannedIdentity(browserConfig),hostAvailable:true,canLaunch:true,status:'available'});
    api.BrowserProfileIdentity=async text=>plannedIdentity(JSON.parse(text));
    api.BrowserProfilePreview=async text=>({config:JSON.parse(text),identityPreview:plannedIdentity(JSON.parse(text)),hostAvailable:true,applied:false,canLaunch:true,needsRuntimeReadback:true});
    api.BrowserProfileLaunch=async()=>{window.__qaCalls.push({kind:'browser-launch'});throw Error('固定 mock：未启动产品浏览器。');};
    api.BrowserProfileLaunchProxy=async()=>{window.__qaCalls.push({kind:'browser-launch-proxy'});throw Error('固定 mock：未启动产品浏览器。');};
    api.BrowserProfileClose=async()=>{window.__qaCalls.push({kind:'browser-close'});};
    api.BrowserProxyTest=async text=>{const proxy=JSON.parse(text);window.__qaCalls.push({kind:'proxy-test',type:proxy.type,host:proxy.host});return {status:'reachable',scope:'profile_proxy_probe_only',provider:'country.is',ip:'9.9.9.9',country:'US',subdivision:'California',city:'Fixture City',latencyMs:42,checkedAt:'2026-10-26T00:00:00Z (fixed mock)'};};
    const releases=[{engineId:'chrome-for-testing',version:'130.0.6723.58',channel:'Stable'},{engineId:'chrome-for-testing',version:'131.0.6778.85',channel:'Stable fixture'},{engineId:'firefox',version:'133.0.3',channel:'Gecko release fixture'}];
    let versions=[{engineId:'chrome-for-testing',version:'130.0.6723.58',treeVerified:true,versionVerified:true,canLaunch:true,fileCount:275},{engineId:'firefox',version:'133.0.3',treeVerified:true,versionVerified:true,canLaunch:true,fileCount:143}];
    api.BrowserEngineCatalog=async proxyJSON=>{window.__qaCalls.push({kind:'engine-catalog',route:proxyJSON?'proxy':'ultra'});return {releases,fetchedAt:'2026-10-26 · fixed mock'};};
    api.BrowserEngineVersions=async()=>{window.__qaCalls.push({kind:'engine-inventory'});return versions;};
    api.BrowserEngineDownload=async text=>{const request=JSON.parse(text);window.__qaCalls.push({kind:'engine-download',engineId:request.engineId,version:request.version});const entry={engineId:request.engineId,version:request.version,treeVerified:true,versionVerified:true,canLaunch:true,fileCount:275};versions=versions.filter(v=>v.engineId!==entry.engineId||v.version!==entry.version).concat([entry]);return entry;};
    api.BrowserEngineProfileLaunch=async()=>{window.__qaCalls.push({kind:'engine-launch'});throw Error('固定 mock：原生协议与进程保护尚未通过本次验收；未启动，也不改用直连。');};
    api.BrowserEngineProfileClose=async()=>{window.__qaCalls.push({kind:'engine-close'});};
    const sourceHash='a'.repeat(64),copyHash='b'.repeat(64),previewId='c'.repeat(32);
    const tagNames=['IFD0:Artist','IFD0:Copyright','IFD0:ImageDescription','IFD0:Software','ExifIFD:SerialNumber','ExifIFD:DateTimeOriginal','GPS:Coordinates'];
    const values=['固定 mock 作者','Fixture copyright 2026','固定 mock 描述：<img src=x onerror=bad()> 仅作为文字显示。','Fixture editor 1.0','SYNTHETIC-CAMERA-0000','2026:10:26 12:34:56 +08:00','0.0000 N, 0.0000 E (synthetic)'];
    const tags=Object.fromEntries(metadataFields.map((field,index)=>[tagNames[index],{field:field.id,jsonBytes:values[index].length}]));
    api.MetadataToolStatus=async()=>{window.__qaCalls.push({kind:'metadata-status'});return {backend:{available:true,version:'13.59 (fixed mock)',vmValidated:false},fields:metadataFields};};
    api.MetadataPreviewImage=async()=>{window.__qaCalls.push({kind:'metadata-preview'});return {previewId,fields:metadataFields,preview:{sourceSha256:sourceHash,format:'jpeg',width:1920,height:1080,bytes:245760,tags}};};
    api.MetadataReadValues=async(id,fields)=>{window.__qaCalls.push({kind:'metadata-values',id,fields});return Object.fromEntries(metadataFields.flatMap((field,index)=>fields.includes(field.id)?[[tagNames[index],values[index]]]:[]));};
    api.MetadataCreateCopy=async(id,fields)=>{window.__qaCalls.push({kind:'metadata-copy',id,fields});return {sourceSha256:sourceHash,copySha256:copyHash,selected:fields,removed:fields,verified:{sourceSha256:copyHash,format:'jpeg',width:1920,height:1080,bytes:241000,tags:{}}};};
    window.go={main:{App:new Proxy(api,{get:(object,key)=>object[key]||(()=>Promise.resolve(null))})}};
    window.runtime={EventsOn:()=>{}};
    window.chrome={runtime:{sendMessage:(message,callback)=>{window.__qaCalls.push(message);callback(message.action==='report'?{ok:true,result:extensionRecords}:{ok:false,error:'QA never mutates browser settings'});}},storage:{onChanged:{addListener:()=>{}}}};
   },{report,detection,fw,theme,environmentPresets,metadataFields,guides,extensionRecords:extension.catalog.map(item=>Object.assign({},item,{supported:true,writable:true,value:item.desired,managed:false,checkedAt:'fixed mock',sample:item.type==='content'}))});
   const page=await context.newPage();page.on('pageerror',error=>result.errors.push(error.message));
   const shot=async(name,selector,anchor)=>{
    await page.evaluate(()=>Promise.all(document.getAnimations().map(a=>a.finished.catch(()=>{}))));
    const file='audit-fixed-'+name+'-'+width+'-'+theme+'.png',destination=path.join(out,file);
    if(selector){
     const size=await page.locator(selector).boundingBox();
     if(size && size.height<=page.viewportSize().height-64)await page.locator(selector).screenshot({path:destination});
     else{
      // A tall element lives inside the app's scroll container. Element clips
      // cannot reveal off-screen overflow; capture an actual scrolled viewport.
      await page.locator(anchor||selector).evaluate(el=>(el.closest('label')||el).scrollIntoView({block:'start'}));
      await page.screenshot({path:destination});
     }
    }else await page.screenshot({path:destination});
    result.screenshots.push(file);
   };
   const noOverflow=async(stage='existing')=>{const issues=await page.evaluate(()=>Array.from(document.querySelectorAll('.view')).filter(v=>v.scrollWidth>v.clientWidth+1).map(v=>({className:v.className,scrollWidth:v.scrollWidth,clientWidth:v.clientWidth})));if(issues.length)result.layoutIssues.push({width,theme,stage,issues});};
   await page.goto('http://127.0.0.1:48737/');await page.locator('#ob-privacy').click();await page.locator('[data-privacy-apply="fixture-windows"]').waitFor();assert.equal(await page.locator('#max-btn').isVisible(),true);await noOverflow();await shot('system');
   await page.locator('[data-privacy-tab="browser"]').click();await page.locator('#privacy-browser-preview').waitFor();await noOverflow();await shot('browser');
   await page.locator('[data-browser-pane="fingerprint"]').click();
   await page.locator('#privacy-browser-graphics').selectOption('fixture-gpu');
   await page.locator('#privacy-browser-gpu-renderer').fill('ANGLE (Manual GPU)');
   await page.locator('#privacy-browser-preview').click();await page.locator('#privacy-browser-launch:not([disabled])').waitFor();await noOverflow();
   await page.locator('.privacy-browser-profile').evaluate(el=>el.scrollIntoView({block:'start'}));await shot('browser-profile-top');
   await page.locator('#privacy-browser-launch').scrollIntoViewIfNeeded();await shot('browser-profile-gpu');
   assert.equal(await page.locator('#privacy-browser-environment option').count(),5);
   await page.locator('[data-browser-pane="environment"]').click();
   for(const preset of environmentPresets){
    await page.locator('[data-browser-os="'+preset.config.devicePresetId+'"]').click();
    await page.locator('#privacy-browser-os-version').fill(preset.config.os==='ios'?'17.2':preset.config.os==='windows'?'11.0':'14.1');
    const mobile=['android','ios'].includes(preset.config.os);
    await page.locator('#privacy-browser-width').fill(mobile?'390':'1440');await page.locator('#privacy-browser-height').fill(mobile?'844':'900');
    await page.locator('#privacy-browser-preview').click();await page.locator('#privacy-browser-launch:not([disabled])').waitFor();
    assert.equal(await page.locator('#privacy-browser-graphics option[value="fixture-gpu"]').count(),preset.config.os==='windows'?1:0);
    await noOverflow('environment-'+preset.config.os);await shot('browser-environment-'+preset.config.os,'.privacy-browser-profile','#privacy-browser-environment');
   }
   await page.locator('[data-browser-pane="proxy"]').click();
   await page.locator('#privacy-proxy-type').selectOption('socks5');await page.locator('#privacy-proxy-host').fill('proxy.fixture.invalid');await page.locator('#privacy-proxy-port').fill('1080');await page.locator('#privacy-proxy-username').fill('synthetic-user');await page.locator('#privacy-proxy-password').fill('synthetic-fixture-password');
   assert.equal((await page.evaluate(()=>window.__qaCalls)).filter(c=>c.kind==='proxy-test').length,0);
   await page.locator('#privacy-proxy-test').click();await page.locator('.privacy-browser-proxy .privacy-probe').waitFor();
   assert.match(await page.locator('.privacy-browser-proxy .privacy-probe').textContent(),/9\.9\.9\.9/);assert.doesNotMatch(await page.locator('.privacy-browser-proxy .privacy-probe').textContent(),/synthetic-fixture-password/);
   await noOverflow('proxy-geoip');await shot('browser-proxy-geoip','.privacy-browser-proxy');
   assert.equal((await page.evaluate(()=>window.__qaCalls)).filter(c=>c.kind==='engine-catalog'||c.kind==='engine-download'||c.kind==='engine-inventory').length,0);
   await page.locator('[data-browser-pane="versions"]').click();
   await page.locator('#privacy-engine-route').selectOption('proxy');await page.locator('#privacy-engine-refresh').click();await page.locator('#privacy-engine-options option').first().waitFor({state:'attached'});
   await page.locator('#privacy-engine-version').fill('131.0.6778.85');await page.locator('#privacy-engine-download').click();await page.locator('.privacy-engine-manager [role="status"]').waitFor();await page.locator('#privacy-engine-local').click();await page.locator('.privacy-engine-manager .privacy-probe').waitFor();
   await noOverflow('engine-catalog-cache');await shot('browser-engine-catalog-cache','.privacy-engine-manager');
   await page.locator('[data-browser-pane="environment"]').click();await page.locator('[data-browser-os="native"]').click();await page.locator('[data-browser-pane="fingerprint"]').click();await page.locator('#privacy-browser-graphics').selectOption('native');await page.locator('[data-browser-pane="versions"]').click();await page.locator('#privacy-engine-launch').click();await page.locator('.privacy-engine-manager [role="alert"]').waitFor();assert.equal((await page.evaluate(()=>window.__qaCalls)).filter(c=>c.kind==='engine-launch').length,1);
   await noOverflow('engine-refusal');await shot('browser-engine-refused','.privacy-engine-manager');
   await page.locator('[data-privacy-tab="data"]').click();assert.equal((await page.evaluate(()=>window.__qaCalls)).filter(c=>c.kind&&c.kind.startsWith('metadata-')).length,0);
   await page.locator('#privacy-metadata-status').click();await page.locator('#privacy-metadata-select:not([disabled])').waitFor();await shot('metadata-component','.privacy-metadata-tool');
   await page.locator('#privacy-metadata-select').click();await page.locator('[data-metadata-field]').first().waitFor();assert.equal(await page.locator('[data-metadata-field]').count(),7);assert.equal(await page.locator('[data-metadata-field]:checked').count(),0);assert.equal(await page.locator('#privacy-metadata-values').isDisabled(),true);assert.equal(await page.locator('.privacy-metadata-values').count(),0);
   for(const field of metadataFields)await page.locator('[data-metadata-field="'+field.id+'"]').check();
   assert.equal((await page.evaluate(()=>window.__qaCalls)).filter(c=>c.kind==='metadata-values'||c.kind==='metadata-copy').length,0);
   await noOverflow('metadata-seven-fields');await shot('metadata-seven-fields','.privacy-metadata-tool','.privacy-metadata-fields');
   await page.locator('#privacy-metadata-values').click();await page.locator('.privacy-metadata-values').waitFor();assert.equal(await page.locator('.privacy-metadata-values img').count(),0);assert.match(await page.locator('.privacy-metadata-values').textContent(),/SYNTHETIC-CAMERA-0000/);
   await noOverflow('metadata-explicit-values');await shot('metadata-explicit-values','.privacy-metadata-tool','.privacy-metadata-values');
   await page.locator('#privacy-metadata-copy').click();await page.locator('.privacy-metadata-copy').waitFor();await page.locator('.privacy-metadata-copy summary').click();assert.equal((await page.evaluate(()=>window.__qaCalls)).filter(c=>c.kind==='metadata-copy').at(-1).fields.length,7);
   await noOverflow('metadata-copy-receipt');await shot('metadata-copy-receipt','.privacy-metadata-tool','.privacy-metadata-copy');
   await page.locator('[data-privacy-tab="network"]').click();await shot('network');
   await page.locator('.privacy-module-guide-group > summary').click();const networkGuide=page.locator('[data-privacy-guide="program-network"]');await networkGuide.locator('summary').click();await networkGuide.scrollIntoViewIfNeeded();await noOverflow('network-guide');await shot('network-guide');
   await page.locator('[data-privacy-tab="boundaries"]').click();await page.locator('.privacy-module-guide-group > summary').click();const providerGuide=page.locator('[data-privacy-guide="location-provider"]');await providerGuide.locator('summary').click();await providerGuide.scrollIntoViewIfNeeded();await noOverflow('location-boundary');await shot('location-boundary');
   await page.evaluate(()=>nav('detection'));await page.locator('#detection-export:not([disabled])').waitFor();await shot('detection');
   await page.locator('#detection-export').click();await page.locator('#detection-preview').click();await page.locator('#detection-save').waitFor();await page.locator('#detection-export-preview').scrollIntoViewIfNeeded();await noOverflow();await shot('export');
   assert.ok(!await page.locator('#detection-export-environment').isChecked());
   await page.locator('#detection-text-preview').click();await page.locator('#detection-text-content').waitFor();await page.locator('#detection-text-content').scrollIntoViewIfNeeded();await noOverflow('text-preview');await shot('text-preview');
   await page.goto('http://127.0.0.1:48737/extension/panel.html');await page.locator('[data-request="prediction"]:not([disabled])').waitFor();await shot('extension');
   await page.locator('[data-mode="content-camera"]').selectOption('allow');await page.locator('[data-origin="content-camera"]').fill('https://fixture.invalid');await page.locator('[data-request="content-camera"]').click();await page.locator('[data-confirm="content-camera"]').scrollIntoViewIfNeeded();await shot('permission-choice');
   assert.equal((await page.evaluate(()=>window.__qaCalls)).filter(c=>c.action==='apply'||c.action==='restore').length,0);
   result.checks.push({width,theme,previewFields:true,fourEnvironmentTemplates:true,independentProxyGeoIP:true,explicitEngineCatalogAndCache:true,nativeEngineRefusal:true,metadataComponentAndSevenFields:true,metadataValuesExplicitOnly:true,metadataCopyReceipt:true,permissionDraftOnly:true,noHorizontalOverflow:!result.layoutIssues.some(issue=>issue.width===width&&issue.theme===theme)});await context.close();
  }
  fs.writeFileSync(path.join(out,'privacy-audit-qa-results.json'),JSON.stringify(result,null,2));
  console.log(JSON.stringify({screenshots:result.screenshots.length,profiles:result.checks.length,errors:result.errors,blockedRequests:result.blockedRequests,layoutIssues:result.layoutIssues}));
  assert.deepEqual(result.errors,[]);assert.deepEqual(result.blockedRequests,[]);assert.deepEqual(result.layoutIssues,[]);
 } finally {await browser.close();}
}
run().catch(error=>{console.error(error);process.exitCode=1;});
