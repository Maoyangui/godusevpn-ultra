// Fixed production read-only probe in Node vm mocks. No browser or network.
'use strict';
const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const input=JSON.parse(fs.readFileSync(0,'utf8')),listeners=new Set(),scopes=[];
let released=0,crossFrames=0;
function scope(origin,secure){
  class GL{getParameter(p){return p===0x9245?input.vendor:p===0x9246?input.renderer:'native-other';}getExtension(name){return name==='WEBGL_debug_renderer_info'?{UNMASKED_VENDOR_WEBGL:0x9245,UNMASKED_RENDERER_WEBGL:0x9246}:name==='WEBGL_lose_context'?{loseContext(){released++;}}:null;}}
  class GL2 extends GL{}
  const getContext=kind=>kind==='webgl'?new GL():kind==='webgl2'?new GL2():null;
  const s={isSecureContext:secure,navigator:Object.freeze({}),location:{origin},URL,Promise,setTimeout,clearTimeout,encodeURIComponent,decodeURIComponent,Worker:class{constructor(){const e=Error('blocked');e.name='NotAllowedError';throw e;}},WebGLRenderingContext:GL,WebGL2RenderingContext:GL2,OffscreenCanvas:class{getContext(kind){return getContext(kind);}}};
  s.document={createElement(kind){if(kind==='canvas')return{getContext};assert.equal(kind,'iframe');return{remove(){}};},body:{appendChild(f){const same=f.srcdoc!==undefined,opaque=!same&&f.src.startsWith('data:');const origin=same?s.location.origin:opaque?'null':new URL(f.src).origin;if(!same&&!opaque){assert.equal(origin,'https://godusevpn-graphics.invalid');crossFrames++;}const child=scope(origin,!opaque);f.contentWindow=child;child.parent={postMessage(value){for(const on of [...listeners])on({source:child,origin,data:value});}};const html=same?f.srcdoc:opaque?decodeURIComponent(f.src.slice('data:text/html,'.length)):input.frameHTML;queueMicrotask(()=>vm.runInContext(html.slice(html.indexOf('<script>')+8,html.lastIndexOf('</script>')),vm.createContext(child)));return f;}}};
  s.addEventListener=(name,fn)=>{assert.equal(name,'message');listeners.add(fn);};s.removeEventListener=(name,fn)=>listeners.delete(fn);
  scopes.push({s,method:GL.prototype.getParameter,descriptor:Object.getOwnPropertyDescriptor(GL.prototype,'getParameter')});return s;
}
(async()=>{
 const top=scope('https://godusevpn.invalid',true),result=JSON.parse(await vm.runInContext(input.expression,vm.createContext(top)));
 assert.equal(result.scopes.length,4);assert.equal(crossFrames,1);assert.equal(listeners.size,0);
 assert.equal(released,input.metadata?16:0);
 for(const {s,method,descriptor}of scopes){assert.equal(s.WebGLRenderingContext.prototype.getParameter,method);assert.deepEqual(Object.getOwnPropertyDescriptor(s.WebGLRenderingContext.prototype,'getParameter'),descriptor);assert.equal(s.WebGLRenderingContext.prototype.getParameter.call({},0x9999),'native-other');assert.equal(Object.hasOwn(s.navigator,'gpu'),false);}
 for(const item of result.scopes){assert.equal(item.workerBlocked,true);assert.equal(item.webgpuPresent,false);assert.equal(item.paths.length,input.metadata?4:0);if(input.metadata)for(const p of item.paths){assert.equal(p.vendor,input.vendor);assert.equal(p.renderer,input.renderer);}}
 process.stdout.write(JSON.stringify({scopes:result.scopes.length,nativeMethodsUnchanged:true,contextsReleased:released,workerExecutionStillBlocked:true,workerAPIRuntimeReadback:false,network:false}));
})().catch(e=>{console.error(e);process.exitCode=1;});
