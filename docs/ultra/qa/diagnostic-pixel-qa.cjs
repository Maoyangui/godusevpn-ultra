// Existing runtime/browser only. Fixed mocks; never loads Wails, service, picker,
// real diagnostics, registry or browser extension settings.
'use strict';
const fs = require('node:fs'), path = require('node:path'), assert = require('node:assert/strict');
if (process.argv.length < 4) { console.log('Usage: node diagnostic-pixel-qa.cjs <Playwright module> <browser executable> [output directory]'); process.exit(2); }
const {chromium} = require(process.argv[2]);
const root = path.resolve(__dirname, '../../..');
const sourceRoot = path.join(root, 'web/dist');
const out = process.argv[4] ? path.resolve(process.argv[4]) : path.join(root, 'docs/ultra/screenshots/diagnostic-modals');
fs.mkdirSync(out, {recursive:true});
const longText = Array.from({length:75}, (_,i) => 'FIXTURE ' + String(i+1).padStart(2,'0') + '：' + '这是固定合成诊断内容，用于检查长文本滚动和换行。'.repeat(5)).join('\n');
const files = [{name:'summary.json',content:'{"fixture":"fixed mock only","status":"unknown"}',bytes:52}, {name:'selected-details.txt',content:longText,bytes:Buffer.byteLength(longText)}, {name:'scope.json',content:'{"scope":"fixed mock; no real device data"}',bytes:45}];
async function main() {
 const browser = await chromium.launch({executablePath:process.argv[3],headless:true,args:['--disable-background-networking','--disable-component-update','--disable-sync','--no-first-run','--no-default-browser-check']});
 const result = {scope:'isolated fixed diagnostic modals only',screenshots:[],checks:[],errors:[],blockedRequests:[]};
 try {
  for (const width of [420,900]) for (const theme of ['light','dark']) {
   const height = width===420?760:900;
   const context = await browser.newContext({viewport:{width,height},colorScheme:theme});
   await context.route('**/*', route => {
    const url = new URL(route.request().url());
    if (url.origin !== 'http://127.0.0.1:48751') { result.blockedRequests.push(url.origin); return route.abort(); }
    const name = decodeURIComponent(url.pathname==='/'?'index.html':url.pathname.slice(1));
    const file = path.resolve(sourceRoot,name);
    if (!file.startsWith(sourceRoot+path.sep) || !fs.existsSync(file)) return route.fulfill({status:404,body:''});
    return route.fulfill({status:200,contentType:file.endsWith('.html')?'text/html; charset=utf-8':file.endsWith('.css')?'text/css; charset=utf-8':file.endsWith('.svg')?'image/svg+xml':'text/javascript; charset=utf-8',body:fs.readFileSync(file)});
   });
   await context.addInitScript(({theme,files}) => {
    window.__diagnosticQACalls=[];
    const state={platform:'windows',service:true,svcState:'running',version:'fixed mock',lang:'zh',theme,view:{profiles:[],nodes:[],settings:{},state:{status:'disconnected'}}};
    const api={GetState:async()=>state,TrialStatus:async()=>({enabled:false,status:'disabled',plan:'free',purchaseUrl:'https://godusevpn.org/subscription/'}),GetLogs:async()=>[],PreviewDiag:async options=>{
     window.__diagnosticQACalls.push(['preview',options]);
     return {id:'fixed-diagnostic-preview',expiresAt:new Date(Date.now()+300000).toISOString(),sha256:'a'.repeat(64),selection:options,files,warnings:['固定 mock：没有读取本机日志、配置、网络或设备信息。','详细内容可能包含身份信息。本次仅以合成文字演示；审阅后再决定是否保存。']};
    },ExportDiagPreview:async()=>{window.__diagnosticQACalls.push(['FORBIDDEN_SAVE']);throw new Error('Pixel QA never exports or opens a file picker');}};
    window.go={main:{App:new Proxy(api,{get:(obj,key)=>obj[key]||(()=>{window.__diagnosticQACalls.push(['unexpected',String(key)]);return Promise.reject(new Error('Unexpected mock method'));})})}};
    window.runtime={EventsOn:()=>{}};
   },{theme,files});
   const page=await context.newPage();page.on('pageerror',error=>result.errors.push(error.message));
   await page.goto('http://127.0.0.1:48751/');await page.locator('#ob-privacy').waitFor();
   await page.evaluate(()=>{nav('logs');void exportDiag();});
   const card=page.locator('#dialog .dialog-card');
   await page.locator('#diag-ack').waitFor();
   const shot=async name=>{await page.evaluate(()=>Promise.all(document.getAnimations().map(a=>a.finished.catch(()=>{}))));const file='diagnostic-'+name+'-'+width+'-'+theme+'.png';await page.screenshot({path:path.join(out,file)});result.screenshots.push(file);};
   const geometry=async()=>page.locator('#dialog .dialog-card').evaluate(el=>{const b=el.getBoundingClientRect();return {x:b.x,y:b.y,right:b.right,bottom:b.bottom,scrollWidth:el.scrollWidth,clientWidth:el.clientWidth};});
   assert.equal(await page.locator('#dialog [data-privacy-help]').count(),7);
   assert.equal(await page.locator('#dialog [data-diag-option]:checked').count(),0);
   assert.equal(await page.locator('#dialog').getAttribute('role'),'dialog');
   let g=await geometry();assert.ok(g.x>=0&&g.y>=0&&g.right<=width+1&&g.bottom<=height+1&&g.scrollWidth<=g.clientWidth+1,'selection modal bounds');
   await shot('selection');
   await page.locator('[data-diag-option="network"]').check();assert.ok(await page.locator('#dialog-yes').isDisabled());
   await page.locator('#diag-ack').check();assert.ok(await page.locator('#dialog-yes').isEnabled());
   await page.locator('#dialog-yes').focus();await page.keyboard.press('Tab');assert.ok(await page.evaluate(()=>document.activeElement.hasAttribute('data-privacy-help')),'selection submit help keyboard access');
   await page.mouse.move(0,0);await page.locator('#dialog-yes').click();
   await page.locator('.diag-file').first().waitFor();assert.equal(await page.locator('.diag-file').count(),3);
   for(const id of ['dialog-no','dialog-yes']) assert.ok(await page.locator('#'+id+' + .privacy-help').count());
   await page.locator('.diag-file').nth(1).locator('summary').click();
   const text=page.locator('.diag-file').nth(1).locator('.diag-text');await text.waitFor();
   assert.ok(await text.evaluate(el=>el.scrollHeight>el.clientHeight),'long diagnostic text scrolls');
   await text.evaluate(el=>{el.scrollTop=el.scrollHeight;});assert.ok(await text.evaluate(el=>el.scrollTop>0));
   await card.evaluate(el=>{el.scrollTop=0;});g=await geometry();assert.ok(g.scrollWidth<=g.clientWidth+1,'review modal has no horizontal overflow');await shot('review');
   await page.locator('.diag-file').evaluateAll(nodes=>nodes.forEach(el=>{el.open=true;}));
   if(width===420) assert.ok(await card.evaluate(el=>el.scrollHeight>el.clientHeight),'expanded narrow review scrolls as a whole');
   await page.locator('#dialog-yes').scrollIntoViewIfNeeded();
   const footer=await page.locator('#dialog-yes').boundingBox();assert.ok(footer.y>=0&&footer.y+footer.height<=height+1,'save footer reachable within viewport');
   await page.locator('#dialog-yes').focus();await page.keyboard.press('Tab');assert.ok(await page.evaluate(()=>document.activeElement.hasAttribute('data-privacy-help')),'save help keyboard access');
   await page.locator('#dialog-no').focus();await shot('review-footer');
   await page.locator('#dialog-no').click();assert.equal(await page.locator('#dialog').count(),0);
   const calls=await page.evaluate(()=>window.__diagnosticQACalls);assert.equal(calls.length,1);assert.equal(calls[0][0],'preview');assert.equal(calls[0][1].network,true);assert.equal(calls[0][1].acknowledgeSensitive,true);
   result.checks.push({width,theme,selectionHelp:7,noHorizontalOverflow:true,detailsRequireAcknowledgement:true,textScroll:true,footerReachable:true,keyboardHelp:true,noExport:true});
   await context.close();
  }
  assert.deepEqual(result.errors,[]);assert.deepEqual(result.blockedRequests,[]);
  fs.writeFileSync(path.join(out,'diagnostic-pixel-qa-results.json'),JSON.stringify(result,null,2));
  console.log(JSON.stringify({screenshots:result.screenshots.length,profiles:result.checks.length,errors:result.errors,blockedRequests:result.blockedRequests}));
 } finally {await browser.close();}
}
main().catch(error=>{console.error(error);process.exitCode=1;});
