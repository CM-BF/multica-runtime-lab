// Negative identity probe: only Store is used, not a model or SDK sandbox run.
import fs from 'node:fs/promises';
import path from 'node:path';
import assert from 'node:assert/strict';
import {Store} from '../dist/storage.js';
const pair=JSON.parse(await fs.readFile(process.argv[2],'utf8'));
const req={cwd:process.env.HOME,stateRoot:path.join(process.env.HOME,'state'),model:'fixture',inputs:[],artifacts:[]};
const first=new Store({...req,mcp:pair.first});await first.open();await first.begin();await first.checkpoint({history:[],runState:'{}',sandboxState:{},archive:Buffer.from('fixture'),artifacts:{},mcpTools:'same-tools'});await first.close();
const next=new Store({...req,mcp:pair.second,sessionId:first.token});assert.notEqual(first.identity,next.identity);await next.open();try{await assert.rejects(next.load(),/CHECKPOINT_INVALID/);}finally{await next.close();}
console.log('PASS: sanitized ordinary configuration cannot preserve identity across endpoint replacement or load the prior clean checkpoint');
