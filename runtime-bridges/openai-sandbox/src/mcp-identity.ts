import {BridgeError,type Request} from './protocol.js';

export function canonical(value:unknown):string {
  if(Array.isArray(value))return '['+value.map(canonical).join(',')+']';
  if(value && typeof value==='object')return '{'+Object.entries(value).filter(([,v])=>v!==undefined).sort(([a],[b])=>a.localeCompare(b)).map(([k,v])=>JSON.stringify(k)+':'+canonical(v)).join(',')+'}';
  return JSON.stringify(value)??'null';
}
// Only the daemon's explicit plugin-hook binding permits rotating a loopback
// transport. All stable policy/unknown configuration fields remain pinned.
export function stableMCP(request:Request):unknown {
  return Object.fromEntries(Object.entries(request.mcp?.mcpServers??{}).map(([name,cfg])=>{
    if(cfg.disabled!==undefined && typeof cfg.disabled!=='boolean')throw new BridgeError('MCP_CONFIG');
    if(cfg.disabled)return [name,{disabled:true}];
    const stable={...cfg};
    if(cfg.type==='http' && cfg.url) {
      const url=new URL(cfg.url);
      if(cfg.multicaBinding) {
        if(name!=='multica-plugins'||cfg.multicaBinding.kind!=='plugin-hook'||!Array.isArray(cfg.multicaBinding.tools)||url.protocol!=='http:'||url.hostname!=='127.0.0.1'||!url.port||!/^\/[a-f0-9]{48}$/.test(url.pathname)||url.username||url.password||url.search||url.hash)throw new BridgeError('MCP_CONFIG');
        stable.url='http://127.0.0.1/{plugin-hook-transport}';
      }
      // Authorization values are per-turn credentials, not policy. Other headers
      // remain bound; switching auth presence or schemes still invalidates state.
      stable.headers=Object.fromEntries(Object.entries(cfg.headers??{}).map(([key,value])=>[key.toLowerCase(),key.toLowerCase()==='authorization'?value.split(' ')[0]+' {credential}':value]));
    }
    return [name,stable];
  }));
}
