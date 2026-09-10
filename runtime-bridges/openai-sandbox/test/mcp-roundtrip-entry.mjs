// Test-only simulated model driver; actual SDK HTTP tools, Unix workspace and checkpoint.
import fs from 'node:fs/promises';
import {execute} from '../dist/engine.js';
import {driver} from './fixture.mjs';
const req=JSON.parse(await fs.readFile(process.argv[2],'utf8'));
await execute(req,async f=>process.stdout.write(JSON.stringify(f)+'\n'),new AbortController().signal,async(a,i,o)=>{
 const out=await a.mcpServers[0].callTool('platform_echo',{value:req.prompt});
 if(!JSON.stringify(out).includes('handler-'+req.prompt))throw new Error('wrong handler reply');
 return driver(a,i,o);
});
