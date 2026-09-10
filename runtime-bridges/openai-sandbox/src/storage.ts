import * as fs from 'node:fs/promises';
import path from 'node:path';
import {createHash,randomBytes} from 'node:crypto';
import {constants} from 'node:fs';
import {BridgeError,SDK_VERSION,type Request} from './protocol.js';
export const LIMIT=16*1024*1024;
export const digest=(data:string|Uint8Array)=>createHash('sha256').update(data).digest('hex');
export function relative(value:string):string {
  if (!value || path.isAbsolute(value) || value.includes('\\') || value.split('/').some(p=>!p || p==='.' || p==='..') || value.includes('\0')) throw new BridgeError('UNSAFE_PATH');
  return value;
}
export async function privateDir(p:string) {await fs.mkdir(p,{recursive:true,mode:0o700});if ((await fs.lstat(p)).isSymbolicLink()) throw new BridgeError('UNSAFE_PATH');await fs.chmod(p,0o700);}
export async function atomic(p:string,data:string|Uint8Array) {const tmp=p+'.'+randomBytes(8).toString('hex');await fs.writeFile(tmp,data,{mode:0o600,flag:'wx'});await fs.rename(tmp,p);}
export async function inside(root:string,rel:string):Promise<string> {
  relative(rel);let current=root;
  for (const part of rel.split('/')) {current=path.join(current,part);try {if ((await fs.lstat(current)).isSymbolicLink()) throw new BridgeError('UNSAFE_PATH');} catch(e:any) {if(e.code!=='ENOENT') throw e;}}
  return current;
}
export async function readBounded(p:string,limit=LIMIT):Promise<Buffer> {
  const file=await fs.open(p,constants.O_RDONLY|constants.O_NOFOLLOW);
  try {const stat=await file.stat();if(!stat.isFile() || stat.size>limit) throw new BridgeError('FILE_LIMIT');return await file.readFile();} finally {await file.close();}
}
export async function inventory(root:string,paths:string[]):Promise<Record<string,string>> {
  const result:Record<string,string>={};let bytes=0;
  const visit=async(rel:string):Promise<void>=>{
    const target=await inside(root,rel);let info;try{info=await fs.lstat(target);}catch(e:any){if(e.code==='ENOENT') return;throw e;}
    if(info.isDirectory()) {for(const name of await fs.readdir(target)) await visit(rel+'/'+name);}
    else {const data=await readBounded(target);bytes+=data.length;if(bytes>LIMIT || Object.keys(result).length>=256)throw new BridgeError('FILE_LIMIT');result[rel]=digest(data);}
  };
  for(const rel of paths) await visit(relative(rel));
  return result;
}
export class Store {
  token:string;root:string;generation?:string;previous:Record<string,string>={};identity:string;
  private lock?:fs.FileHandle;
  constructor(public request:Request) {
    this.token=request.sessionId||randomBytes(16).toString('hex');
    if(!/^[a-f0-9]{32}$/.test(this.token))throw new BridgeError('INVALID_SESSION');
    this.root=path.join(request.stateRoot,this.token);
    this.identity=digest(JSON.stringify({sdk:SDK_VERSION,cwd:request.cwd,model:request.model,mcp:request.mcp,inputs:request.inputs,artifacts:request.artifacts}));
  }
  async open() {
    if(!path.isAbsolute(this.request.stateRoot)||!path.isAbsolute(this.request.cwd))throw new BridgeError('UNSAFE_PATH');
    await privateDir(this.request.stateRoot);await privateDir(this.root);
    try {this.lock=await fs.open(path.join(this.root,'lock'),'wx',0o600);} catch {throw new BridgeError('STATE_BUSY');}
    try {
      await fs.access(path.join(this.root,'in_flight'));throw new BridgeError('DIRTY_SESSION');
    } catch(e:any){if(e.code!=='ENOENT')throw e;}
  }
  async load():Promise<{history:any[];archive?:Buffer}> {
    if(!this.request.sessionId)return {history:[]};
    try {
      const pointer=JSON.parse((await readBounded(path.join(this.root,'current.json'))).toString());
      if(!/^[a-f0-9]{32}$/.test(pointer.generation))throw new Error();
      const gen=path.join(this.root,pointer.generation);
      const metadataRaw=await readBounded(path.join(gen,'checkpoint.json'));
      if(digest(metadataRaw)!==pointer.sha256)throw new Error();
      const metadata=JSON.parse(metadataRaw.toString());
      if(metadata.identity!==this.identity || metadata.sdk!==SDK_VERSION)throw new Error();
      const files:Record<string,Buffer>={};
      for(const name of ['history.json','run-state.json','sandbox-state.json','workspace.tar']) {
        files[name]=await readBounded(path.join(gen,name),64*1024*1024);
        if(digest(files[name])!==metadata.hashes[name])throw new Error();
      }
      this.previous=metadata.artifacts;this.generation=pointer.generation;
      return {history:JSON.parse(files['history.json'].toString()),archive:files['workspace.tar']};
    } catch {throw new BridgeError('CHECKPOINT_INVALID');}
  }
  async begin() {await atomic(path.join(this.root,'in_flight'),JSON.stringify({requestId:this.request.requestId}));}
  async checkpoint(data:{history:any[];runState:string;sandboxState:unknown;archive:Uint8Array;artifacts:Record<string,string>}, beforeCommit:()=>Promise<void> = async()=>{}) {
    const generation=randomBytes(16).toString('hex'),dir=path.join(this.root,generation);await privateDir(dir);
    const files:Record<string,string|Uint8Array>={'history.json':JSON.stringify(data.history),'run-state.json':data.runState,'sandbox-state.json':JSON.stringify(data.sandboxState),'workspace.tar':data.archive};
    const hashes:Record<string,string>={};
    for(const [name,body] of Object.entries(files)){hashes[name]=digest(body);await atomic(path.join(dir,name),body);}
    const meta=JSON.stringify({schema:1,sdk:SDK_VERSION,identity:this.identity,hashes,artifacts:data.artifacts});
    await atomic(path.join(dir,'checkpoint.json'),meta);
    await beforeCommit();
    await atomic(path.join(this.root,'current.json'),JSON.stringify({generation,sha256:digest(meta)}));
    await fs.unlink(path.join(this.root,'in_flight'));
    // Generations are immutable. No automatic GC of the last good archive.
  }
  async close() {if(this.lock){await this.lock.close();this.lock=undefined;await fs.unlink(path.join(this.root,'lock'));}}
}
export async function exportArtifacts(workspace:string,cwd:string,paths:string[],baseline:Record<string,string>) {
  const output=await inventory(workspace,paths),current=await inventory(cwd,paths);
  if(JSON.stringify(Object.entries(current).sort())!==JSON.stringify(Object.entries(baseline).sort()))throw new BridgeError('ARTIFACT_CONFLICT');
  const changes:{path:string;kind:string;sha256?:string;bytes?:number}[]=[];
  // Validate every destination before performing the first write.
  for(const rel of new Set([...Object.keys(output),...Object.keys(baseline)]))await inside(cwd,rel);
  for(const [rel,hash] of Object.entries(output)) {
    if(baseline[rel]===hash)continue;
    const bytes=await readBounded(await inside(workspace,rel));const target=await inside(cwd,rel);
    await fs.mkdir(path.dirname(target),{recursive:true,mode:0o700});await atomic(target,bytes);
    changes.push({path:rel,kind:baseline[rel]?'modified':'created',sha256:hash,bytes:bytes.length});
  }
  for(const rel of Object.keys(baseline)) if(!output[rel]) {await fs.unlink(await inside(cwd,rel));changes.push({path:rel,kind:'deleted'});}
  return {hashes:output,changes};
}
