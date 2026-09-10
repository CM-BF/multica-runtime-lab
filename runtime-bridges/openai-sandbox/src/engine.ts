import * as fs from 'node:fs/promises';
import path from 'node:path';
import {Runner,OpenAIProvider,MCPServerStdio,MCPServerStreamableHttp,type MCPServer} from '@openai/agents';
import {Manifest,SandboxAgent,file,shell} from '@openai/agents/sandbox';
import {UnixLocalSandboxClient} from '@openai/agents/sandbox/local';
import {BridgeError,mapEvent,safeError,type Request,type Send} from './protocol.js';
import {Store,inventory,inside,readBounded,privateDir,exportArtifacts,atomic} from './storage.js';
export async function connectMCP(request:Request):Promise<MCPServer[]> {
  const servers:MCPServer[]=[];
  try {
    for(const [name,cfg] of Object.entries(request.mcp?.mcpServers??{})) {
      let server:MCPServer;
      if((!cfg.type || cfg.type==='stdio') && cfg.command && !cfg.url) server=new MCPServerStdio({name,command:cfg.command,args:cfg.args??[],env:cfg.env,cwd:request.cwd,clientSessionTimeoutSeconds:10});
      else if(cfg.type==='http' && cfg.url && !cfg.command) {
        const url=new URL(cfg.url);if(!['http:','https:'].includes(url.protocol)||url.username||url.password)throw new BridgeError('MCP_CONFIG');
        server=new MCPServerStreamableHttp({name,url:cfg.url,requestInit:{headers:cfg.headers??{}},clientSessionTimeoutSeconds:10});
      } else throw new BridgeError('MCP_CONFIG');
      servers.push(server);await server.connect();await server.listTools();
    }
    return servers;
  } catch {
    await Promise.allSettled(servers.map(s=>s.close()));
    throw new BridgeError('MCP_UNREADY');
  }
}
export type Driver = (agent:SandboxAgent, input:any, options:any)=>Promise<any>;
// The only driver in the production entry is the official Runner.run.
export async function execute(request:Request,send:Send,signal:AbortSignal,driver?:Driver) {
  const store=new Store(request);let session:Awaited<ReturnType<UnixLocalSandboxClient['create']>>|undefined;
  let servers:MCPServer[]=[];let closed=false,began=false;let result:Record<string,unknown>;
  const apiKey=process.env.OPENAI_API_KEY;
  delete process.env.OPENAI_API_KEY;
  const provider=new OpenAIProvider({apiKey});
  const runner=new Runner({modelProvider:provider,tracingDisabled:true});
  const cleanup=async()=>{
    if(closed)return;
    const outcomes=await Promise.allSettled([session?.close(),...servers.map(s=>s.close()),provider.close()]);
    closed=true;
    if(outcomes.some(o=>o.status==='rejected'))throw new BridgeError('SDK_FAILURE');
  };
  const abort=()=>{void session?.stop().catch(()=>{});};signal.addEventListener('abort',abort);
  try {
    if(!request.model || !request.requestId || !Array.isArray(request.inputs)||!Array.isArray(request.artifacts))throw new BridgeError('INVALID_REQUEST');
    await store.open();const restored=await store.load();
    const baseline=await inventory(request.cwd,request.artifacts);
    if(request.sessionId && JSON.stringify(Object.entries(baseline).sort())!==JSON.stringify(Object.entries(store.previous).sort()))throw new BridgeError('ARTIFACT_CONFLICT');
    const entries:Record<string,ReturnType<typeof file>>={};
    const inputs=await inventory(request.cwd,request.inputs);
    for(const name of new Set([...Object.keys(inputs),...Object.keys(baseline)])) entries[name]=file({content:await readBounded(await inside(request.cwd,name))});
    const manifest=new Manifest({entries});
    await store.begin();began=true;
    await send({type:'event',kind:'status',status:'session_started',sessionId:store.token});
    const workspaceBase=path.join(store.root,'workspaces');await privateDir(workspaceBase);
    // Explicit noop prevents SDK default snapshots outside the private state root.
    const client=new UnixLocalSandboxClient({workspaceBaseDir:workspaceBase,snapshot:{type:'noop'},defaultShell:'/bin/sh',archiveLimits:{maxInputBytes:64*1024*1024,maxExtractedBytes:64*1024*1024,maxMembers:2048}});
    session=await client.create({manifest});
    if(restored.archive) {await session.hydrateWorkspace(restored.archive);for(const name of Object.keys(inputs))await session.materializeEntry({path:name,entry:entries[name]});}
    if(signal.aborted)throw new BridgeError('CANCELLED');
    servers=await connectMCP(request);
    const agent=new SandboxAgent({name:'Multica local sandbox',model:request.model,instructions:request.instructions,defaultManifest:manifest,capabilities:[shell()],mcpServers:servers});
    const input=[...restored.history,{role:'user' as const,content:request.prompt}];
    const stream=await (driver??((a,i,o)=>runner.run(a,i,o)))(agent,input,{stream:true,signal,maxTurns:request.maxTurns||20,sandbox:{client,session}});
    for await(const event of stream) {if(signal.aborted)break;const mapped=mapEvent(event);if(mapped)await send({type:'event',...mapped});}
    await stream.completed;
    if(signal.aborted)throw new BridgeError('CANCELLED');
    if(stream.error)throw stream.error;
    if(stream.interruptions?.length)throw new BridgeError('APPROVAL_REQUIRED');
    const archive=await session.persistWorkspace();
    const sandboxState=await client.serializeSessionState(session.state);
    const runState=stream.state.toString({includeTracingApiKey:false});
    const secrets=[apiKey,...Object.values(request.mcp?.mcpServers??{}).flatMap(c=>[...Object.values(c.env??{}),...Object.values(c.headers??{})])].filter((v):v is string=>!!v&&v.length>=8);
    const serialized=JSON.stringify({history:stream.history,runState,sandboxState});
    if(secrets.some(secret=>serialized.includes(secret)))throw new BridgeError('STATE_SECRET');
    const artifacts=await exportArtifacts(session.state.workspaceRootPath,request.cwd,request.artifacts,baseline);
    await store.checkpoint({history:stream.history,runState,sandboxState,archive,artifacts:artifacts.hashes},cleanup);
    result={type:'result',status:'completed',output:String(stream.finalOutput??''),sessionId:store.token,artifacts:artifacts.changes};
  } catch(error) {
    const code=signal.aborted?'CANCELLED':safeError(error);
    // A failed/aborted turn stays dirty. Preserve recoverable bytes before close.
    let recovered=!session;
    if(session && began) {try{await atomic(path.join(store.root,'recovery.tar'),await session.persistWorkspace());recovered=true;}catch{await session.stop().catch(()=>{});}}
    if(recovered)await cleanup().catch(()=>{});
    else {await Promise.allSettled(servers.map(s=>s.close()));await provider.close().catch(()=>{});}
    result={type:'result',status:signal.aborted?'aborted':'failed',error:code,sessionId:store.token};
  } finally {
    signal.removeEventListener('abort',abort);
    await store.close();
  }
  await send(result!);
}
