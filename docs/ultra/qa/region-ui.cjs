// Only isolated jsdom and synthetic current-user APIs. No system settings,
// clipboard, real browser, service, product or external network is invoked.
'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {JSDOM}=require(process.argv[2]||'jsdom');
const root=path.resolve(__dirname,'../../..'),source=fs.readFileSync(path.join(root,'web/dist/privacy-region.js'),'utf8');
const clone=value=>JSON.parse(JSON.stringify(value));
const wait=()=>new Promise(resolve=>setTimeout(resolve,5));
const held=()=>{let resolve,reject;const promise=new Promise((yes,no)=>{resolve=yes;reject=no});return {promise,resolve,reject}};
const base=()=>({supported:true,status:'available',currentCode:'US',originalCode:'',managed:false,canApply:true,canRestore:false,choices:[{code:'US',label:'United States (US)'},{code:'DE',label:'Germany (DE)'},{code:'001',label:'World (001)'}],observedAt:'2026-10-27T01:00:00Z',scope:'仅当前用户国家地区',detail:'合成系统读回',atomicExternalCAS:false,warnings:[]});
function fixture(initial=base()) {
 const dom=new JSDOM('<!doctype html><main><div id="slot"></div></main>',{runScripts:'outside-only',url:'https://example.invalid/'}),w=dom.window;
 const calls=[],errors=[],draft={code:'',touched:false},confirmations=[];
 let report=clone(initial),alive=true,busy=false,slot=w.document.querySelector('#slot'),cleanup,readHook,applyHook,restoreHook,confirmHook;
 w.fetch=()=>{throw Error('network forbidden')};w.XMLHttpRequest=function(){throw Error('network forbidden')};
 Object.defineProperty(w.navigator,'clipboard',{value:{writeText(){throw Error('clipboard forbidden')}}});w.document.execCommand=()=>{throw Error('clipboard forbidden')};
 w.addEventListener('error',event=>errors.push(String(event.error||event.message)));
 vm.runInContext(source,dom.getInternalVMContext(),{filename:'privacy-region.js'});
 const facade={draft,alive:()=>alive,busy:()=>busy,
  Report:async()=>{calls.push(['report']);return readHook?readHook():clone(report)},
  Apply:async code=>{calls.push(['apply',code]);if(applyHook)return applyHook(code);const original=report.originalCode||report.currentCode;report={...report,status:'configured',currentCode:code,originalCode:original,managed:true,canRestore:true};return {status:'applied',currentCode:code,changed:true,verified:true}},
  Restore:async()=>{calls.push(['restore']);if(restoreHook)return restoreHook();report={...report,status:'available',currentCode:report.originalCode,managed:false,canRestore:false};return {status:'restored',currentCode:report.currentCode,changed:true,verified:true}},
  confirm:async(text,options)=>{confirmations.push({text,options:clone(options)});return confirmHook?confirmHook():true},toast:()=>{throw Error('unexpected detached toast')}
 };
 const mount=()=>{cleanup=w.PrivacyRegionUI.mount(facade,slot)};
 const q=selector=>slot.querySelector(selector),count=name=>calls.filter(call=>call[0]===name).length;
 const click=async selector=>{assert.ok(q(selector),selector);q(selector).click();await wait()};
 const select=code=>{const input=q('#privacy-region-choice');assert.ok(input);input.value=code;input.dispatchEvent(new w.Event('change',{bubbles:true}))};
 mount();
 return {w,q,facade,calls,count,errors,draft,confirmations,wait,click,select,get report(){return report},set report(value){report=clone(value)},setRead:fn=>{readHook=fn},setApply:fn=>{applyHook=fn},setRestore:fn=>{restoreHook=fn},setConfirm:fn=>{confirmHook=fn},setBusy:value=>{busy=value},leave:()=>{alive=false;slot.remove()},cleanup:()=>cleanup(),remount:()=>{cleanup();slot.remove();slot=w.document.createElement('div');slot.id='slot';w.document.querySelector('main').append(slot);mount()},dispose:()=>{cleanup();w.close()}};
}
async function caseRun(name,body){await body();console.log('PASS '+name)}
async function run(){
 await caseRun('explicit system catalog selection / confirmation / fresh apply and restore',async()=>{
  const f=fixture();try{await wait();assert.equal(f.count('apply'),0);assert.equal(f.q('#privacy-region-choice').value,'US');assert.equal(f.q('#privacy-region-apply').disabled,true);assert.equal(f.q('#privacy-region-restore').disabled,true);
   assert.match(f.q('#privacy-region-card').textContent,/不更改 GPS、IP、显示语言、键盘、时区或浏览器环境/);assert.match(f.q('#privacy-region-card').textContent,/没有原子比较替换/);
   f.select('DE');assert.equal(f.count('apply'),0);assert.equal(f.q('#privacy-region-apply').disabled,false);await f.click('#privacy-region-apply');
   assert.deepEqual(f.calls.filter(c=>c[0]==='apply'),[['apply','DE']]);assert.equal(f.count('report'),3);assert.match(f.confirmations[0].text,/Germany.*DE/);assert.match(f.confirmations[0].text,/不更改 GPS/);assert.match(f.q('[role="status"]').textContent,/已应用并读回/);
   assert.equal(f.q('#privacy-region-restore').disabled,false);await f.click('#privacy-region-restore');assert.equal(f.count('restore'),1);assert.equal(f.report.currentCode,'US');assert.match(f.q('[role="status"]').textContent,/已恢复并读回/);assert.equal(f.q('#privacy-region-restore').disabled,true);assert.deepEqual(f.errors,[]);
  }finally{f.dispose()}
 });
 await caseRun('confirmation cancel and duplicate/held DOM cannot submit',async()=>{
  const f=fixture();try{await wait();f.select('DE');f.setConfirm(()=>false);await f.click('#privacy-region-apply');assert.equal(f.count('apply'),0);assert.equal(f.count('report'),1);assert.equal(f.q('#privacy-region-apply').disabled,false);
   const confirm=held();f.setConfirm(()=>confirm.promise);const old=f.q('#privacy-region-apply');old.click();old.dispatchEvent(new f.w.MouseEvent('click',{bubbles:true}));await wait();assert.equal(f.confirmations.length,2);assert.equal(f.q('#privacy-region-choice').disabled,true);assert.equal(f.q('#privacy-region-refresh').disabled,true);
   confirm.resolve(true);await wait();assert.equal(f.count('apply'),1);old.dispatchEvent(new f.w.MouseEvent('click',{bubbles:true}));await wait();assert.equal(f.count('apply'),1);
  }finally{f.dispose()}
 });
 await caseRun('external change during confirm or before setter is preserved',async()=>{
  const f=fixture();try{await wait();f.select('DE');const confirm=held();f.setConfirm(()=>confirm.promise);f.q('#privacy-region-apply').click();await wait();f.report={...f.report,currentCode:'001'};confirm.resolve(true);await wait();assert.equal(f.count('apply'),0);assert.match(f.q('[role="alert"]').textContent,/确认期间.*已改变/);
   f.setConfirm(()=>true);f.setApply(()=>{throw Error('REGION_EXTERNAL_CHANGE_PRESERVED')});await f.click('#privacy-region-apply');assert.equal(f.count('apply'),1);assert.match(f.q('[role="alert"]').textContent,/检测到外部更改/);assert.equal(f.q('[role="status"]'),null);
  }finally{f.dispose()}
 });
 await caseRun('pending recovery may restore / backup external unknown cannot write',async()=>{
  for(const status of ['external_change','backup_error','unknown','unsupported']){
   const f=fixture({...base(),status,supported:status!=='unsupported',managed:true,originalCode:'001',canRestore:true});try{await wait();f.select('DE');assert.equal(f.q('#privacy-region-apply').disabled,true);assert.equal(f.q('#privacy-region-restore').disabled,true);f.q('#privacy-region-restore').dispatchEvent(new f.w.MouseEvent('click'));await wait();assert.equal(f.count('restore'),0)}finally{f.dispose()}
  }
  const f=fixture({...base(),status:'recovery_pending',currentCode:'DE',originalCode:'001',managed:true,canApply:false,canRestore:true});try{await wait();assert.equal(f.q('#privacy-region-apply').disabled,true);assert.equal(f.q('#privacy-region-restore').disabled,false);await f.click('#privacy-region-restore');assert.equal(f.report.currentCode,'001');assert.equal(f.count('restore'),1)}finally{f.dispose()}
 });
 await caseRun('failure and malformed catalog clear prior evidence / no raw error exposure',async()=>{
  const f=fixture();try{await wait();f.select('DE');f.setRead(()=>{throw Error('SYNTHETIC_PRIVATE_PATH_OR_SID')});await f.click('#privacy-region-refresh');assert.equal(f.q('#privacy-region-choice').disabled,true);assert.equal(f.q('#privacy-region-apply').disabled,true);assert.doesNotMatch(f.q('#privacy-region-card').textContent,/SYNTHETIC_PRIVATE|United States/);
   f.setRead(()=>({...base(),choices:[{code:'US',label:'one'},{code:'US',label:'two'}]}));await f.click('#privacy-region-refresh');assert.equal(f.q('#privacy-region-apply').disabled,true);
   f.setRead(()=>({...base(),observedAt:'not-a-time'}));await f.click('#privacy-region-refresh');assert.equal(f.q('#privacy-region-apply').disabled,true);
   f.setRead(()=>({...base(),atomicExternalCAS:true}));await f.click('#privacy-region-refresh');assert.equal(f.q('#privacy-region-apply').disabled,true);assert.equal(f.count('apply'),0);
  }finally{f.dispose()}
 });
 await caseRun('only exact catalog code is submitted / no DOM label injection',async()=>{
  const report=base();report.choices[1].label='<img src=x onerror="BAD"> Germany';const f=fixture(report);try{await wait();assert.equal(f.q('img'),null);f.select('DE');
   const input=f.q('#privacy-region-choice'),option=f.w.document.createElement('option');option.value='../US';input.append(option);input.value='../US';input.dispatchEvent(new f.w.Event('change',{bubbles:true}));await f.click('#privacy-region-apply');assert.deepEqual(f.calls.filter(c=>c[0]==='apply'),[['apply','DE']]);assert.equal(f.q('img'),null);assert.deepEqual(f.errors,[]);
  }finally{f.dispose()}
 });
 await caseRun('write in flight is single / requires final readback / recovery shown honestly',async()=>{
  const f=fixture();try{await wait();f.select('DE');const write=held();f.setApply(()=>write.promise);f.q('#privacy-region-apply').click();await wait();assert.equal(f.count('apply'),1);assert.equal(f.q('#privacy-region-choice').disabled,true);f.q('#privacy-region-restore').dispatchEvent(new f.w.MouseEvent('click'));assert.equal(f.count('restore'),0);
   f.report={...base(),status:'recovery_pending',currentCode:'DE',originalCode:'US',managed:true,canApply:false,canRestore:true};write.resolve({status:'applied',currentCode:'DE',changed:true,verified:false});await wait();assert.equal(f.q('[role="status"]'),null);assert.match(f.q('[role="alert"]').textContent,/操作未完成/);assert.equal(f.q('#privacy-region-apply').disabled,true);assert.equal(f.q('#privacy-region-restore').disabled,false);
  }finally{f.dispose()}
  const g=fixture();try{await wait();g.select('DE');let reads=0;g.setRead(()=>{reads++;if(reads===2)throw Error('PRIVATE_READ_FAILURE');return clone(g.report)});await g.click('#privacy-region-apply');assert.equal(g.count('apply'),1);assert.match(g.q('[role="alert"]').textContent,/操作后重新读取失败/);assert.equal(g.q('[role="status"]'),null);assert.equal(g.q('#privacy-region-apply').disabled,true)}finally{g.dispose()}
  for(const result of [{status:'restored',currentCode:'DE',changed:true,verified:true},{status:'unchanged',currentCode:'DE',changed:true,verified:true},{status:'applied',currentCode:'DE',changed:true,verified:true}]){
   const h=fixture();try{await wait();h.select('DE');h.setApply(()=>{h.report={...base(),currentCode:'DE'};return result});await h.click('#privacy-region-apply');assert.equal(h.q('[role="status"]'),null,'wrong action/ownership result must not report success');assert.ok(h.q('[role="alert"]'));}finally{h.dispose()}
  }
 });
 await caseRun('draft persists parent redraw / detached confirmation and late read do not update replacement',async()=>{
  const f=fixture();try{await wait();f.select('DE');f.remount();await wait();assert.equal(f.q('#privacy-region-choice').value,'DE');assert.equal(f.count('apply'),0);
   const confirm=held();f.setConfirm(()=>confirm.promise);f.q('#privacy-region-apply').click();await wait();f.remount();await wait();confirm.resolve(true);await wait();assert.equal(f.count('apply'),0);assert.equal(f.q('#privacy-region-choice').value,'DE');
   const read=held();f.setRead(()=>read.promise);f.q('#privacy-region-refresh').click();f.setRead(null);f.remount();await wait();read.resolve({...base(),currentCode:'001'});await wait();assert.match(f.q('#privacy-region-card').textContent,/United States/);assert.equal(f.q('#privacy-region-card').getAttribute('aria-busy'),'false');
   const confirm2=held();f.setConfirm(()=>confirm2.promise);f.q('#privacy-region-apply').click();await wait();f.leave();confirm2.resolve(true);await wait();assert.equal(f.count('apply'),0);assert.deepEqual(f.errors,[]);
  }finally{f.dispose()}
 });
 await caseRun('global busy forbids write and help stays local',async()=>{
  const f=fixture();try{await wait();f.select('DE');f.setBusy(true);f.q('#privacy-region-apply').dispatchEvent(new f.w.MouseEvent('click'));await wait();assert.equal(f.confirmations.length,0);assert.equal(f.count('apply'),0);f.setBusy(false);await f.click('[data-region-help="restore"]');assert.match(f.q('#privacy-region-help').textContent,/原子比较替换/);assert.equal(f.count('apply'),0);assert.equal(f.count('report'),1);f.cleanup();f.q('#privacy-region-apply').dispatchEvent(new f.w.MouseEvent('click'));await wait();assert.equal(f.count('apply'),0)}finally{f.dispose()}
 });
 await caseRun('production tooltip and action grouping survive asynchronous redraw',async()=>{
  const f=fixture();try{
   f.w.esc=value=>String(value==null?'':value).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
   f.w.eval(fs.readFileSync(path.join(root,'web/dist/privacy.js'),'utf8'));
   f.facade.help=f.w.privacyHelp;f.facade.decorate=()=>f.w.privacyDecorateHelp(f.q('#privacy-region-card'));
   await wait();f.select('DE');await f.click('#privacy-region-refresh');
   assert.equal(f.q('#privacy-region-apply').parentElement.className,'privacy-action-option','action and help remain one group after each read');
   assert.equal(f.q('[data-region-help]'),null,'production uses the existing delegated tooltip');
   const summary=f.q('summary'),details=summary.parentElement,helpButton=summary.querySelector('.privacy-help');assert.ok(helpButton);summary.click();assert.equal(details.open,true);
   helpButton.click();assert.equal(details.open,true,'help does not toggle the disclosure');assert.ok(f.w.document.querySelector('#privacy-tooltip'));
   f.w.document.dispatchEvent(new f.w.KeyboardEvent('keydown',{key:'Escape',bubbles:true}));assert.equal(f.w.document.querySelector('#privacy-tooltip'),null);
   helpButton.focus();assert.ok(f.w.document.querySelector('#privacy-tooltip'));f.w.document.dispatchEvent(new f.w.KeyboardEvent('keydown',{key:'Escape',bubbles:true}));assert.equal(f.w.document.activeElement,helpButton);
   assert.equal(f.count('apply'),0);assert.equal(f.count('restore'),0);assert.deepEqual(f.errors,[]);
  }finally{f.dispose()}
 });
 console.log(JSON.stringify({scope:'isolated region module DOM with fake Report/Apply/Restore only',cases:10,noSystemSettings:true,noNativeRuntimeOrNetwork:true,explicitConfirmation:true,freshPreflightAndReadback:true,staleDOMBlocked:true,draftPreserved:true,externalChangePreserved:true,delegatedHelpAfterAsyncRedraw:true}));
}
run().catch(error=>{console.error(error);process.exitCode=1});
