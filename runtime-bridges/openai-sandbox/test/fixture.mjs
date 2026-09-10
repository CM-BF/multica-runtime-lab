// Test-only driver: real SDK Unix sandbox and RunState, simulated model stream.
import fs from 'node:fs/promises';
import {RunState,RunContext} from '@openai/agents';
import {execute} from '../dist/engine.js';
export async function driver(agent,input,options) {
  const session=options.sandbox.session;
  const resume=input.length>1;
  const cmd=resume?'test "$(cat private-canary)" = kept && printf second > artifacts/result.txt && rm artifacts/delete.txt':'mkdir -p artifacts && printf first > artifacts/result.txt && printf delete > artifacts/delete.txt && printf kept > private-canary';
  const run=await session.exec({cmd,login:false,yieldTimeMs:1000});
  if(run.exitCode!==0)throw new Error('real shell failed');
  if(agent.instructions!=='context-canary')throw new Error('missing context');
  const text=resume?'second':'first';
  return {async *[Symbol.asyncIterator](){yield {type:'raw_model_stream_event',data:{type:'output_text_delta',delta:text}};yield {type:'run_item_stream_event',name:'tool_called',item:{rawItem:{callId:'shell-1',name:'shell'}}};yield {type:'run_item_stream_event',name:'tool_output',item:{rawItem:{callId:'shell-1'},output:'exit 0'}};},completed:Promise.resolve(),history:[...input,{type:'message',role:'assistant',status:'completed',content:[{type:'output_text',text}]} ],state:new RunState(new RunContext({}),input,agent,20),finalOutput:text};
}
if(process.argv[2]) {
  const request=JSON.parse(await fs.readFile(process.argv[2],'utf8'));
  await execute(request,async f=>process.stdout.write(JSON.stringify(f)+'\n'),new AbortController().signal,driver);
}
