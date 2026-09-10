import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import http from 'node:http';
import {connectMCP} from '../dist/engine.js';
import {Store,inventory,exportArtifacts} from '../dist/storage.js';
const binding={kind:'plugin-hook',tools:[{installation_id:'install',hook_key:'hook',name:'echo',input_schema:{type:'object'}}]};
async function setup(t){const root=await fs.mkdtemp(path.join(os.tmpdir(),'oa4-'));t.after(()=>fs.rm(root,{recursive:true,force:true}));return root;}
function request(root,entry){return {cwd:root,stateRoot:path.join(root,'state'),model:'test',inputs:[],artifacts:['artifacts'],mcp:{mcpServers:{'multica-plugins':entry}}};}
test('disabled stdio and HTTP cause zero child starts and zero connections',async t=>{
 const root=await setup(t),marker=path.join(root,'spawned');let connections=0;
 const server=http.createServer((q,r)=>{r.end();});server.on('connection',()=>connections++);await new Promise(r=>server.listen(0,'127.0.0.1',r));t.after(()=>new Promise(r=>server.close(r)));
 const req=request(root,{});req.mcp.mcpServers={stdio:{disabled:true,command:process.execPath,args:['-e',`require('fs').writeFileSync(${JSON.stringify(marker)},'started')`]},http:{disabled:true,type:'http',url:`http://127.0.0.1:${server.address().port}/mcp`}};
 assert.deepEqual(await connectMCP(req),[]);assert.equal(connections,0);await assert.rejects(fs.access(marker));
 // Validate every disabled flag before any earlier enabled entry can start.
 req.mcp.mcpServers.stdio.disabled=false;req.mcp.mcpServers.http.disabled='false';await assert.rejects(connectMCP(req),/MCP_CONFIG/);await assert.rejects(fs.access(marker));assert.equal(connections,0);
});
test('stable binding permits only bounded transport/credential rotation; substantive policy remains bound',async t=>{
 const root=await setup(t);const entry={type:'http',url:'http://127.0.0.1:30001/'+ 'a'.repeat(48),headers:{Authorization:'Bearer first'},multicaBinding:binding};
 const id=e=>new Store(request(root,e)).identity;
 assert.equal(id(entry),id({...entry,url:'http://127.0.0.1:30002/'+'b'.repeat(48),headers:{Authorization:'Bearer second'}}));
 assert.notEqual(id(entry),id({...entry,multicaBinding:{...binding,tools:[{...binding.tools[0],hook_key:'different'}]}}));
 assert.notEqual(id(entry),id({...entry,headers:{Authorization:'Bearer second','X-Policy':'different'}}));
 assert.notEqual(id(entry),id({...entry,disabled:true}));
 assert.throws(()=>id({...entry,url:'http://example.invalid:30001/'+'a'.repeat(48)}),/MCP_CONFIG/);
 const plain={...entry};delete plain.multicaBinding;assert.notEqual(id(plain),id({...plain,url:'http://127.0.0.1:30002/'+'b'.repeat(48)}));
});
for(const direction of ['file-to-directory','directory-to-file','empty-directory-to-file'])test(`artifact ${direction} rejects before unrelated export writes`,async t=>{
 const root=await setup(t),cwd=path.join(root,'cwd'),workspace=path.join(root,'workspace');for(const dir of [cwd,workspace])await fs.mkdir(path.join(dir,'artifacts'),{recursive:true});
 await fs.writeFile(path.join(cwd,'artifacts/aaa'),'human');await fs.writeFile(path.join(workspace,'artifacts/aaa'),'new output');
 const old=path.join(cwd,'artifacts/entry'),next=path.join(workspace,'artifacts/entry');
 if(direction==='file-to-directory'){await fs.writeFile(old,'old file');await fs.mkdir(next);await fs.writeFile(path.join(next,'new'),'new child');}
 else {await fs.mkdir(old);if(direction==='directory-to-file')await fs.writeFile(path.join(old,'old'),'old child');await fs.writeFile(next,'new file');}
 const before=await inventory(cwd,['artifacts']);await assert.rejects(exportArtifacts(workspace,cwd,['artifacts'],before),/ARTIFACT_TYPE_CHANGE/);
 assert.deepEqual(await inventory(cwd,['artifacts']),before);assert.equal(await fs.readFile(path.join(cwd,'artifacts/aaa'),'utf8'),'human');assert.equal((await fs.stat(old)).isDirectory(),direction!=='file-to-directory');
});
