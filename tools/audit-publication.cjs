'use strict';
// Publication hygiene, not a universal secret detector. Only names/line numbers
// are printed on failure; suspected values are never included in its output.
const fs=require('node:fs'),path=require('node:path'),cp=require('node:child_process'),crypto=require('node:crypto');
const root=path.resolve(__dirname,'..');
const listed=cp.execFileSync('git',['ls-files','-z','--cached','--others','--exclude-standard'],{cwd:root,maxBuffer:16<<20}).toString().split('\0').filter(Boolean);
const files=[...new Set(listed)].sort(), findings=[];
const sensitiveName=/(?:^|\/)(?:\.env(?:\..+)?|id_rsa|id_ed25519|credentials\.json|keystore\.jks)$|\.(?:pfx|p12|keystore)$/i;
const detectors=[
  ['private_key',/-----BEGIN (?:RSA |EC |DSA |OPENSSH |ENCRYPTED )?PRIVATE KEY-----/g],
  ['github_token',/\b(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{30,})\b/g],
  ['azure_resource_identity',/\/subscriptions\/[a-f0-9-]{36}\/resourceGroups\//gi]
];
const manifest=[];
for(const name of files){
  const absolute=path.resolve(root,name);
  if(!absolute.startsWith(root+path.sep))throw Error('Git path escaped workspace');
  if(!fs.existsSync(absolute))continue;
  const stat=fs.lstatSync(absolute);
  if(!stat.isFile()||stat.isSymbolicLink()){findings.push({file:name,kind:'non_regular_source'});continue;}
  if(sensitiveName.test(name))findings.push({file:name,kind:'credential_file_name'});
  const bytes=fs.readFileSync(absolute);
  manifest.push(name+'\0'+crypto.createHash('sha256').update(bytes).digest('hex'));
  if(bytes.includes(0)||bytes.length>8<<20)continue;
  const text=bytes.toString('utf8');
  for(const [kind,re] of detectors){
    re.lastIndex=0;let match;
    while((match=re.exec(text)))findings.push({file:name,kind,line:text.slice(0,match.index).split('\n').length});
  }
  const urls=text.matchAll(/https?:\/\/[^\s"'`<>]+/g);
  for(const match of urls){
    try{
      const url=new URL(match[0]);
      const synthetic=/(?:^|\.)(?:example\.(?:com|org|net)|example|test|invalid|localhost)$/.test(url.hostname)||/^(?:127\.|0\.0\.0\.0$|\[::1\]$)/.test(url.hostname);
      if(!synthetic && ((url.username||url.password)||/\/sub\/[^/]+/.test(url.pathname)||/(?:token|access_token|api_key|password)=/i.test(url.search)))findings.push({file:name,kind:'non_fixture_credential_url',line:text.slice(0,match.index).split('\n').length});
    }catch{}
  }
}
const result={scope:'tracked and non-ignored source; heuristic patterns plus required manual review',files:manifest.length,sourceManifestSHA256:crypto.createHash('sha256').update(manifest.join('\n')).digest('hex'),findings};
process.stdout.write(JSON.stringify(result,null,2)+'\n');
if(findings.length)process.exitCode=1;
