// Pure jsdom fixtures: mocked native picker/adapter, no product, file picker,
// clipboard, listener, DNS, HTTP request or third-party program is executed.
const assert = require('node:assert/strict');
const fs = require('node:fs'), path = require('node:path'), vm = require('node:vm');
const { JSDOM } = require(process.argv[2] || 'jsdom');
const root = path.resolve(__dirname, '../../..');
const source = name => fs.readFileSync(path.join(root, 'web/dist', name), 'utf8');
const dom = new JSDOM(source('index.html').replace(/<script[\s\S]*?<\/script>/g, ''), {runScripts:'outside-only', url:'http://example.invalid/', pretendToBeVisual:true});
const w=dom.window, calls=[], errors=[];
const sourceHash='a'.repeat(64),copyHash='b'.repeat(64),token='c'.repeat(32);
const fields=[{id:'exif-gps',label:'GPS'},{id:'artist',label:'作者'}];
const sensitive='fixture-only-23.456 <img src=x onerror=bad()>', failure='fixture-private-path-and-secret';
let copyMode='success', deferred=null, chooserMode='selected';
const state={platform:'windows',service:true,svcState:'running',version:'fixture',lang:'zh',theme:'light',view:{profiles:[],nodes:[],settings:{},state:{status:'disconnected'}}};
const api={
 GetState:async()=>state,
 PrivacyReport:async()=>({platform:'windows',controls:[],capabilities:[],warnings:[]}),
 PrivacyFirewallReport:async()=>({supported:true,rules:[],warnings:[]}),
 PrivacyHelperStatus:async()=>({state:'not_requested'}),
 PrivacyTimeReport:async()=>({supported:false,choices:[],status:'unsupported'}),
 BrowserHostReport:async()=>({supported:true,engines:[]}),
 BrowserProfileDefaults:async()=>null,
 MetadataToolStatus:async()=>{calls.push(['status']);return {backend:{available:true},fields};},
 MetadataPreviewImage:async()=>{calls.push(['select']);return chooserMode==='cancel'?null:{previewId:token,fields,preview:{sourceSha256:sourceHash,format:'png',width:1,height:1,bytes:100,tags:{'GPS:Latitude':{field:'exif-gps',jsonBytes:16}}}};},
 MetadataReadValues:async(...args)=>{calls.push(['values',...args]);if(deferred)return deferred.promise;return {'GPS:Latitude':sensitive};},
 MetadataCreateCopy:async(...args)=>{calls.push(['copy',...args]);if(copyMode==='cancel')return null;if(copyMode==='failure')throw Error(failure);if(copyMode==='invalid')return {};return {sourceSha256:sourceHash,copySha256:copyHash,selected:args[1],removed:['exif-gps'],verified:{sourceSha256:copyHash}};}
};
w.go={main:{App:new Proxy(api,{get:(object,key)=>object[key]||(()=>Promise.resolve())})}};
w.runtime={EventsOn:()=>{}};
w.fetch=()=>{throw Error('network forbidden')};
w.HTMLElement.prototype.scrollIntoView=function(){};
Object.defineProperty(w.navigator,'clipboard',{value:{writeText(){throw Error('clipboard forbidden')}}});
w.document.execCommand=()=>{throw Error('clipboard forbidden')};
w.addEventListener('error',event=>errors.push(String(event.error||event.message)));
const context=dom.getInternalVMContext();
for(const name of ['i18n.js','flags.js','privacy.js','app.js'])vm.runInContext(source(name),context,{filename:name});
const q=s=>w.document.querySelector(s), settle=()=>new Promise(resolve=>setTimeout(resolve,20));
const click=async selector=>{assert.ok(q(selector),selector);q(selector).click();await settle()};
const tab=async name=>click('[data-privacy-tab="'+name+'"]');
const choose=()=>{const input=q('[data-metadata-field="exif-gps"]');input.checked=true;input.dispatchEvent(new w.Event('change',{bubbles:true}));};
const hold=()=>{let resolve;const promise=new Promise(r=>resolve=r);return {promise,resolve}};
async function run(){try{
 await settle();vm.runInContext("nav('privacy')",context);await settle();await tab('data');
 assert.equal(calls.length,0,'opening data page must not inspect any file');
 assert.equal(q('#privacy-metadata-select').disabled,true);
 await click('#privacy-metadata-status');await click('#privacy-metadata-select');
 assert.equal(w.document.body.textContent.includes(sensitive),false,'preview never reveals values');
 assert.equal(q('#privacy-metadata-values').disabled,true);assert.equal(q('#privacy-metadata-copy').disabled,true);
 const count=calls.length;choose();assert.equal(calls.length,count,'selection never reveals or writes');
 await click('#privacy-metadata-values');assert.ok(q('.privacy-metadata-values').textContent.includes(sensitive));assert.equal(q('.privacy-metadata-values img'),null,'values must be escaped text');
 const valueCall=calls.at(-1);assert.equal(valueCall[1],token);assert.deepEqual(Array.from(valueCall[2]),['exif-gps']);
 await click('#privacy-metadata-copy');assert.ok(q('.privacy-metadata-copy'));
 copyMode='cancel';await click('#privacy-metadata-copy');assert.equal(q('.privacy-metadata-copy'),null,'cancel clears prior success receipt');
 copyMode='success';await click('#privacy-metadata-copy');assert.ok(q('.privacy-metadata-copy'));
 copyMode='failure';await click('#privacy-metadata-copy');assert.equal(q('.privacy-metadata-copy'),null);assert.ok(q('.privacy-error'));assert.equal(w.document.body.textContent.includes(failure),false);
 copyMode='invalid';await click('#privacy-metadata-copy');assert.equal(q('.privacy-metadata-copy'),null,'invalid receipt cannot report success');
 const old=q('#privacy-metadata-copy');await tab('system');const before=calls.length;old.click();await settle();assert.equal(calls.length,before,'detached control cannot invoke backend');
 await tab('data');deferred=hold();const pending=deferred;const oldValues=q('#privacy-metadata-values');oldValues.click();await settle();await tab('system');pending.resolve({'GPS:Latitude':sensitive});await settle();deferred=null;await tab('data');assert.equal(q('.privacy-metadata-values'),null,'stale tab result is discarded');
 deferred=hold();const pagePending=deferred;q('#privacy-metadata-values').click();await settle();vm.runInContext("nav('home')",context);await settle();pagePending.resolve({'GPS:Latitude':sensitive});await settle();deferred=null;const after=calls.length;oldValues.click();await settle();assert.equal(calls.length,after,'detached page cannot invoke backend');assert.equal(w.document.body.textContent.includes(sensitive),false);
 vm.runInContext("nav('privacy')",context);await settle();await tab('data');await click('#privacy-metadata-status');chooserMode='cancel';await click('#privacy-metadata-select');assert.equal(q('#privacy-metadata-values'),null,'native picker cancel has no fabricated selection');
 for(const call of calls){assert.ok(['status','select','values','copy'].includes(call[0]),'metadata never calls diagnostic export');if(call[0]==='values'||call[0]==='copy'){assert.equal(call.length,3);assert.equal(call[1],token);assert.ok(Array.isArray(call[2]));assert.equal(JSON.stringify(call).includes('inputPath'),false);}}
 assert.deepEqual(errors,[]);console.log('metadata DOM: default privacy, explicit fields, escaped values, native tokens, cancel/error receipt and stale-page checks passed');
}finally{dom.window.close()}}
run().catch(error=>{console.error(error);process.exitCode=1});
