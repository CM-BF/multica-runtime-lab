// Protocol fixture only: real SDK shell in the Go-owned process group, no model.
import {createInterface} from 'node:readline';
import fs from 'node:fs/promises';
import {Manifest} from '@openai/agents/sandbox';
import {UnixLocalSandboxClient} from '@openai/agents/sandbox/local';
process.stdout.write(JSON.stringify({type:'ready',protocol:1,sdk:'0.17.2'})+'\n');
let session;
for await(const line of createInterface({input:process.stdin})) {
 const r=JSON.parse(line);
 if(r.type==='cancel'){await session.stop();continue;}
 const client=new UnixLocalSandboxClient({workspaceBaseDir:r.stateRoot,snapshot:{type:'noop'},defaultShell:'/bin/sh'});
 session=await client.create({manifest:new Manifest()});
 const output=await session.exec({cmd:'sleep 60 & echo $!; wait',login:false,yieldTimeMs:10});
 await fs.writeFile(r.cwd+'/child.pid',output.stdout.trim());
}
