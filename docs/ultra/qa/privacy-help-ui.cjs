// Pure jsdom tooltip/grouping checks; no product, clipboard, browser or network.
'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {JSDOM}=require(process.argv[2]||'jsdom');
const root=path.resolve(__dirname,'../../..');
const dom=new JSDOM(`<!doctype html><html><body><section id="scope" class="privacy">
 <div class="privacy-toolbar"><button id="toolbar-action">检查</button><button id="toolbar-existing">已有帮助</button><button id="toolbar-help" type="button" class="privacy-help" data-privacy-help="固定已有帮助">?</button></div>
 <div class="privacy-actions"><button id="row-action">执行</button><input id="row-input"><span id="status">尚未操作</span></div>
 <details id="plain"><summary id="plain-summary">范围</summary><p>说明</p></details>
 <article class="privacy-module-guide"><details id="guide"><summary id="guide-summary"><strong>模块标题</strong><span class="privacy-badge">部分范围</span><span class="privacy-guide-summary">简短说明</span></summary><button id="guide-existing-help" type="button" class="privacy-help" data-privacy-help="模块已有帮助">?</button><p>说明</p></details></article>
 <details id="existing"><summary id="existing-summary">已有内部帮助<button id="inside-help" type="button" class="privacy-help" data-privacy-help="内部已有帮助">?</button></summary><p>说明</p></details>
 <div class="privacy-editor-tabs"><button id="segment-one">环境</button><button id="segment-two">代理</button></div>
 <div class="privacy-browser-fields"><label id="input-label">名称<input id="field-input"></label><label id="select-label">选择<select id="field-select"><option>固定值</option></select></label></div>
 </section></body></html>`,{runScripts:'outside-only',url:'http://example.invalid/',pretendToBeVisual:true});
const w=dom.window,context=dom.getInternalVMContext(),q=selector=>w.document.querySelector(selector),errors=[];
w.fetch=()=>{throw Error('network forbidden')};w.XMLHttpRequest=function(){throw Error('network forbidden')};w.addEventListener('error',event=>errors.push(String(event.error||event.message)));
vm.runInContext(`function esc(value){return String(value == null ? '' : value).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));}`,context);
vm.runInContext(fs.readFileSync(path.join(root,'web/dist/privacy.js'),'utf8'),context,{filename:'privacy.js'});
try{
 const original={toolbar:q('#toolbar-action'),row:q('#row-action'),toolbarHelp:q('#toolbar-help'),guideHelp:q('#guide-existing-help'),insideHelp:q('#inside-help'),input:q('#field-input'),inputParent:q('#field-input').parentElement,select:q('#field-select'),selectParent:q('#field-select').parentElement,segments:q('.privacy-editor-tabs').innerHTML};
 let actions=0;original.row.addEventListener('click',()=>actions++);
 vm.runInContext("privacyDecorateHelp(document.querySelector('#scope'))",context);
 assert.ok(q('#plain-summary > .privacy-help'),'summary help belongs on the disclosure line');
 assert.equal(q('#plain-summary').nextElementSibling.tagName,'P','expanded content must not begin with a stray help button');
 assert.equal(q('#guide-summary > strong + .privacy-help'),original.guideHelp,'a guide keeps its existing help immediately after the title');
 assert.equal(q('#existing-summary').querySelectorAll('.privacy-help').length,1);assert.equal(q('#inside-help'),original.insideHelp);assert.equal(q('#existing-summary').nextElementSibling.tagName,'P');
 for(const id of ['toolbar-action','toolbar-existing','row-action']){const button=q('#'+id),group=button.parentElement;assert.equal(group.className,'privacy-action-option');assert.equal(group.children.length,2);assert.equal(group.children[0],button);assert.ok(group.children[1].classList.contains('privacy-help'));}
 assert.equal(q('#toolbar-action'),original.toolbar);assert.equal(q('#row-action'),original.row);assert.equal(q('#toolbar-existing').nextElementSibling,original.toolbarHelp);
 assert.equal(q('#field-input'),original.input);assert.equal(q('#field-input').parentElement,original.inputParent);assert.equal(q('#field-select').parentElement,original.selectParent);assert.equal(q('.privacy-editor-tabs').querySelector('.privacy-action-option'),null,'segment buttons keep their structure');
 assert.equal(q('#segment-one').parentElement.className,'privacy-editor-tabs');assert.equal(q('#segment-two').parentElement.className,'privacy-editor-tabs');
 const helpCount=q('#scope').querySelectorAll('.privacy-help').length,groupCount=q('#scope').querySelectorAll('.privacy-action-option').length;
 vm.runInContext("privacyDecorateHelp(document.querySelector('#scope'))",context);
 assert.equal(q('#scope').querySelectorAll('.privacy-help').length,helpCount,'decoration is idempotent');assert.equal(q('#scope').querySelectorAll('.privacy-action-option').length,groupCount);assert.equal(q('#scope').querySelector('.privacy-action-option .privacy-action-option'),null);
 original.row.click();assert.equal(actions,1,'wrapping preserves action listeners');
 const summary=q('#plain-summary'),details=q('#plain'),help=summary.querySelector('.privacy-help');
 summary.click();assert.equal(details.open,true,'native disclosure click still toggles');
 let prevented=false;help.addEventListener('click',event=>queueMicrotask(()=>{prevented=event.defaultPrevented}));help.click();
 assert.equal(details.open,true,'tooltip click must not toggle its enclosing disclosure');assert.ok(q('#privacy-tooltip'));assert.equal(q('#privacy-tooltip').getAttribute('role'),'tooltip');
 assert.equal(w.document.activeElement===help,false,'pointer tooltip does not take focus');
 q('#field-input').focus();help.dispatchEvent(new w.MouseEvent('mouseover',{bubbles:true}));assert.equal(w.document.activeElement,original.input,'hover help preserves input focus');
 w.document.dispatchEvent(new w.KeyboardEvent('keydown',{key:'Escape',bubbles:true}));assert.equal(q('#privacy-tooltip'),null);
 help.focus();assert.ok(q('#privacy-tooltip'),'help remains keyboard-focusable');w.document.dispatchEvent(new w.KeyboardEvent('keydown',{key:'Escape',bubbles:true}));assert.equal(q('#privacy-tooltip'),null);assert.equal(w.document.activeElement,help);
 summary.click();assert.equal(details.open,false);assert.deepEqual(errors,[]);
 // Read the captured click after the document delegate ran, without a timer.
 Promise.resolve().then(()=>{assert.equal(prevented,true,'the existing delegated handler prevents disclosure default');console.log(JSON.stringify({scope:'pure tooltip and help DOM',summaryHelpInline:true,guideTitleThenHelp:true,actionPairsGrouped:true,existingNodeAndListenerIdentity:true,inputAndSegmentParentsUnchanged:true,idempotent:true,nativeDisclosureAndTooltipClickDistinct:true,focusAndEscapePreserved:true,noNativeBrowserOrNetwork:true}));}).catch(error=>{console.error(error);process.exitCode=1}).finally(()=>w.close());
}catch(error){w.close();throw error}
