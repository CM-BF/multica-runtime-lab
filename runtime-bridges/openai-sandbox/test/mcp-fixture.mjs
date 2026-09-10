import {createInterface} from 'node:readline';
for await (const line of createInterface({input:process.stdin})) {
 const r=JSON.parse(line);if(r.id===undefined)continue;
 let result={};
 if(r.method==='initialize')result={protocolVersion:r.params.protocolVersion,capabilities:{tools:{}},serverInfo:{name:'local-fixture',version:'1'}};
 if(r.method==='tools/list')result={tools:[{name:'echo',description:'Local test handler',inputSchema:{type:'object',properties:{text:{type:'string'}}}}]};
 if(r.method==='tools/call')result={content:[{type:'text',text:r.params.arguments.text}]};
 process.stdout.write(JSON.stringify({jsonrpc:'2.0',id:r.id,result})+'\n');
}
