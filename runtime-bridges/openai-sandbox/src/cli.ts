#!/usr/bin/env node
import {createInterface} from 'node:readline';
import {once} from 'node:events';
import {createRequire} from 'node:module';
import * as fs from 'node:fs';
import path from 'node:path';
import {execute} from './engine.js';
import {SDK_VERSION,PROTOCOL,type Frame,type Request} from './protocol.js';
process.umask(0o077);
const pkg=JSON.parse(fs.readFileSync(path.resolve(path.dirname(createRequire(import.meta.url).resolve('@openai/agents')),'..','package.json'),'utf8'));
if(pkg.version!==SDK_VERSION || process.platform==='win32')process.exit(1);
const ready={type:'ready',protocol:PROTOCOL,sdk:SDK_VERSION,capabilities:['unix-local','shell','mcp-stdio','mcp-http','artifacts','clean-checkpoint']};
if(process.argv.slice(2).some(a=>!['--probe','--version'].includes(a)))process.exit(1);
if(process.argv.length>2){process.stdout.write(JSON.stringify(ready)+'\n');process.exit(0);}
async function write(frame:Frame){const line=JSON.stringify(frame)+'\n';if(Buffer.byteLength(line)>1024*1024)throw new Error('FRAME_LIMIT');if(!process.stdout.write(line))await once(process.stdout,'drain');}
await write(ready);
let active:Promise<void>|undefined,requestId='',seq=0;
const controller=new AbortController();
const lines=createInterface({input:process.stdin,crlfDelay:Infinity});
for await(const line of lines) {
  try {
    if(Buffer.byteLength(line)>16*1024*1024)throw new Error();
    const frame=JSON.parse(line);
    if(frame.type==='cancel' && frame.requestId===requestId){controller.abort();continue;}
    if(frame.type!=='execute'||active)throw new Error();
    requestId=frame.requestId;
    active=execute(frame as Request,f=>write({...f,requestId,seq:++seq}),controller.signal).catch(async()=>{await write({type:'result',requestId,seq:++seq,status:'failed',error:'BRIDGE_FAILURE'});}).finally(()=>{lines.close();});
  }catch{controller.abort();process.exitCode=1;break;}
}
if(active){controller.abort();await active;}
process.stdin.pause();
