const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const {chromium} = require(process.argv[2]);
const executablePath = process.argv[3];
const root = path.resolve(__dirname, '../../..');
const out = path.join(root, 'docs/ultra/screenshots');
const errors = [], outbound = [], checks = [];
async function main() {
  const browser = await chromium.launch({executablePath,headless:true,args:['--disable-background-networking','--disable-component-update','--disable-sync','--no-first-run','--no-default-browser-check']});
  try {
    for (const width of [320,420]) for(const theme of ['light','dark']) for(const dpr of width===420?[1,2]:[1]) {
      const context = await browser.newContext({viewport:{width,height:760},deviceScaleFactor:dpr,colorScheme:theme});
      await context.route('**/*', async route=> {
        const url=new URL(route.request().url());
        if(url.origin!=='http://127.0.0.1:48731'){outbound.push(url.origin);return route.abort();}
        const name=decodeURIComponent(url.pathname==='/'?'index.html':url.pathname.slice(1));
        const file=path.resolve(root,'web/dist',name);
        if(!file.startsWith(path.join(root,'web/dist')+path.sep)||!fs.existsSync(file))return route.fulfill({status:404,body:''});
        return route.fulfill({contentType:name.endsWith('.html')?'text/html; charset=utf-8':name.endsWith('.css')?'text/css; charset=utf-8':'text/javascript; charset=utf-8',body:fs.readFileSync(file)});
      });
      await context.addInitScript(({theme})=>{
        const fixture={platform:'windows',service:true,version:'Ultra mock QA',lang:'zh',theme,view:{profiles:[],nodes:[],state:{status:'disconnected'}}};
        window.go={main:{App:new Proxy({GetState:async()=>fixture,
          PrivacyReport:async()=>({platform:'windows',controls:[],capabilities:[],warnings:[],backupHealthy:true}),
          PrivacyFirewallReport:async()=>({supported:false,rules:[],warnings:[]}),
          PrivacyTimeReport:async()=>({supported:false,choices:[],status:'unsupported'})
        },{get:(o,k)=>o[k]||(()=>Promise.resolve())})}};
        window.__qaEvents={};window.runtime={EventsOn:(name,callback)=>{window.__qaEvents[name]=callback;}};
      },{theme});
      const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
      await page.goto('http://127.0.0.1:48731/');
      const logo=page.locator('.ultra-logo');await logo.waitFor();
      await page.evaluate(()=>Promise.all(document.getAnimations().filter(a=>a.effect.getComputedTiming().iterations!==Infinity).map(a=>a.finished.catch(()=>{}))));
      const geometry=await logo.evaluate(el=>{
        const box=el.getBoundingClientRect(), svg=el.querySelector('svg'), body=svg.querySelector(':scope > g');
        const center=new DOMPoint(209,169).matrixTransform(body.getCTM());
        const mark=svg.querySelector('.ultra-signature').getBoundingClientRect();
        return {width:box.width,height:box.height,center:box.left+center.x,glyphY:center.y,
          markWidth:mark.width,markHeight:mark.height,
          markInside:mark.left>=box.left&&mark.right<=box.right&&mark.top>=box.top&&mark.bottom<=box.bottom,
          signatureStroke:svg.querySelector('.ultra-signature').getAttribute('stroke-width'),
          background:getComputedStyle(el).backgroundColor,border:getComputedStyle(el).borderWidth};
      });
      assert.equal(geometry.width,geometry.height);assert.equal(geometry.width,80);
      assert.ok(Math.abs(geometry.center-width/2)<.1,JSON.stringify({width,geometry}));assert.ok(Math.abs(geometry.glyphY-40)<.1);
      assert.equal(geometry.markInside,true);assert.ok(geometry.markWidth>=30&&geometry.markHeight>=12,JSON.stringify({width,geometry}));assert.equal(geometry.signatureStroke,'8');
      assert.equal(geometry.background,'rgba(0, 0, 0, 0)');assert.equal(geometry.border,'0px');
      if(width===420){
        await page.screenshot({path:path.join(out,`ultra-brand-square-logo-${theme}-${dpr}x.png`)});
        await logo.screenshot({path:path.join(out,`ultra-brand-square-detail-${theme}-${dpr}x.png`)});
      }
      // The Wails host can navigate to home; fixture data stays entirely local.
      await page.evaluate(()=>window.__qaEvents.nav('home'));
      await page.locator('#menu-btn').click();
      const drawer=await page.locator('.drawer-brand-title > .ultra-mark').evaluate(el=>({text:el.textContent,font:getComputedStyle(el).fontFamily,weight:getComputedStyle(el).fontWeight,size:getComputedStyle(el).fontSize}));
      assert.equal(drawer.text,'ultra');assert.equal(drawer.weight,'300');assert.equal(drawer.size,'9.5px');
      if(width===420)await page.screenshot({path:path.join(out,`ultra-brand-square-drawer-${theme}-${dpr}x.png`)});
      checks.push({width,theme,dpr,geometry,drawer});await context.close();
    }
    assert.deepEqual(errors,[]);assert.deepEqual(outbound,[]);
    fs.writeFileSync(path.join(out,'ultra-brand-square-results.json'),JSON.stringify({scope:'headless original frontend with fixed Wails mock; no real settings or account data',checks,errors,outbound},null,2));
    console.log(JSON.stringify({profiles:checks.length,errors,outbound,square:true,centered:true}));
  } finally {await browser.close();}
}
main().catch(e=>{console.error(e);process.exitCode=1;});
