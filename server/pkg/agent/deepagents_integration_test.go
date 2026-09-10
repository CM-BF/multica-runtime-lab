//go:build agentintegration

package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDeepAgentsRealStartup is opt-in and uses an explicit isolated venv.
// It never forwards the parent environment or queries an existing account.
func TestDeepAgentsRealStartup(t *testing.T) {
	if os.Getenv("MULTICA_RUN_REAL_AGENT_SMOKE") != "1" {
		t.Skip("real smoke disabled")
	}
	python := os.Getenv("MULTICA_DEEPAGENTS_TEST_PYTHON")
	if !filepath.IsAbs(python) {
		t.Fatal("set absolute MULTICA_DEEPAGENTS_TEST_PYTHON to the isolated venv")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	script := `
import json,os,subprocess,sys,importlib.metadata,pathlib
from acp import RequestError
from deepagents_acp.server import AgentServerACP
from deepagents_code._paths import PATHS
print('versions', json.dumps({p:importlib.metadata.version(p) for p in ('deepagents-code','deepagents-acp','agent-client-protocol')}),flush=True)
print('missing-session-fixture',json.dumps(RequestError.resource_not_found('s1').to_error_obj()),flush=True)
assert str(PATHS.profile.state_dir)==os.path.join(os.environ['DEEPAGENTS_HOME'],'.state')
print('state-path',PATHS.profile.state_dir,flush=True)
dcode=str(pathlib.Path(sys.executable).parent/'dcode')
v=subprocess.run([dcode,'--version'],capture_output=True,text=True,timeout=20)
print('version-exit',v.returncode,'stdout',v.stdout.strip(),flush=True)
assert v.returncode==0
q=json.dumps({'jsonrpc':'2.0','id':1,'method':'initialize','params':{'protocolVersion':1,'clientCapabilities':{}}})+'\n'
p=subprocess.run([dcode,'--acp'],input=q,capture_output=True,text=True,timeout=25)
print('dcode-start-exit',p.returncode,flush=True)
print('dcode-stdout',p.stdout[:4000],flush=True)
print('dcode-stderr',p.stderr[-4000:],flush=True)
print('MODEL_E2E=BLOCKED: no provider credentials supplied',flush=True)
`
	cmd := Command{Path: python}.exec(ctx, "-c", script)
	cmd.Dir = root
	cmd.Env = []string{"HOME=" + root, "PATH=" + filepath.Dir(python) + ":/usr/bin:/bin", "DEEPAGENTS_HOME=" + filepath.Join(root, "profile"), "XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "XDG_CACHE_HOME=" + filepath.Join(root, "cache"), "XDG_DATA_HOME=" + filepath.Join(root, "data"), "LANG=C.UTF-8", "DO_NOT_TRACK=1"}
	output, err := combinedOutputOwned(cmd, nil)
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeepAgentsRealMCP(t *testing.T) {
	if os.Getenv("MULTICA_RUN_REAL_AGENT_SMOKE") != "1" {
		t.Skip("real smoke disabled")
	}
	python := os.Getenv("MULTICA_DEEPAGENTS_TEST_PYTHON")
	if !filepath.IsAbs(python) {
		t.Fatal("isolated venv python required")
	}
	root := t.TempDir()
	server := filepath.Join(root, "server.py")
	if err := os.WriteFile(server, []byte("from mcp.server.fastmcp import FastMCP\nm=FastMCP('local-test')\n@m.tool()\ndef echo(value: str) -> str:\n return value\nm.run(transport='stdio')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	script := `
import asyncio,json,sys,os,pathlib
from deepagents_code.mcp_tools import resolve_and_load_mcp_tools
from deepagents_code._paths import get_project_skills_dir
root=pathlib.Path.cwd()
assert get_project_skills_dir(root)==root/'.deepagents'/'skills'
config=root/'mcp.json'
config.write_text(json.dumps({'mcpServers':{'local':{'command':sys.executable,'args':[str(root/'server.py')]}}}))
async def main():
 tools,manager,infos=await resolve_and_load_mcp_tools(explicit_config_path=str(config))
 try:
  print('real-dcode-MCP-tools', [t.name for t in tools],flush=True)
  tool=next(t for t in tools if t.name.endswith('echo'))
  value=await tool.ainvoke({'value':'DA2-local-canary'})
  print('real-dcode-MCP-result',value,flush=True)
  assert 'DA2-local-canary' in str(value)
 finally:
  if manager: await manager.cleanup()
 config.write_text(json.dumps({'mcpServers':{'missing':{'command':str(root/'does-not-exist')}}}))
 tools,manager,infos=await resolve_and_load_mcp_tools(explicit_config_path=str(config))
 try:
  assert not tools and any(i.status=='error' for i in infos)
  print('MCP_FAIL_CLOSED=BLOCKED: dcode loader returns error metadata without raising for a missing command',flush=True)
 finally:
  if manager: await manager.cleanup()
asyncio.run(main())
`
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := Command{Path: python}.exec(ctx, "-c", script)
	cmd.Dir = root
	cmd.Env = []string{"HOME=" + root, "PATH=" + filepath.Dir(python) + ":/usr/bin:/bin", "DEEPAGENTS_HOME=" + filepath.Join(root, "profile"), "XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "XDG_CACHE_HOME=" + filepath.Join(root, "cache"), "XDG_DATA_HOME=" + filepath.Join(root, "data"), "LANG=C.UTF-8", "DO_NOT_TRACK=1"}
	output, err := combinedOutputOwned(cmd, nil)
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}

// This tests the real ACP SDK with a deterministic graph, not the dcode model loop.
func TestDeepAgentsRealSDKCheckpoint(t *testing.T) {
	if os.Getenv("MULTICA_RUN_REAL_AGENT_SMOKE") != "1" {
		t.Skip("real smoke disabled")
	}
	python := os.Getenv("MULTICA_DEEPAGENTS_TEST_PYTHON")
	if !filepath.IsAbs(python) {
		t.Fatal("isolated venv python required")
	}
	root := t.TempDir()
	script := `
import asyncio,json,pathlib,sys
from acp import text_block
from deepagents_acp.server import AgentServerACP
from langgraph.graph import StateGraph,MessagesState,START,END
from langgraph.checkpoint.sqlite.aio import AsyncSqliteSaver
from langchain_core.messages import AIMessage
class Sink:
 def __init__(self):self.events=[]
 async def session_update(self,**kwargs):self.events.append(str(kwargs))
async def main():
 root=pathlib.Path.cwd()
 async with AsyncSqliteSaver.from_conn_string(str(root/'sessions.db')) as saver:
  await saver.setup()
  graph=StateGraph(MessagesState)
  def answer(state):
   text='SDK checkpoint messages='+str(len(state['messages']))
   (root/'artifact.txt').write_text(text)
   return {'messages':[AIMessage(content=text)]}
  graph.add_node('answer',answer);graph.add_edge(START,'answer');graph.add_edge('answer',END)
  server=AgentServerACP(graph.compile(checkpointer=saver),load_sessions=True)
  sink=Sink();server.on_connect(sink)
  if sys.argv[1]=='new':
   session=await server.new_session(cwd=str(root),mcp_servers=[])
   sid=session.session_id;(root/'sid').write_text(sid)
  else:
   sid=(root/'sid').read_text()
   await server.load_session(cwd=str(root),session_id=sid,mcp_servers=[])
   assert any('SDK checkpoint messages=1' in e for e in sink.events),sink.events
   print('real SDK replay observed',flush=True)
  reply=await server.prompt(prompt=[text_block('canary')],session_id=sid)
  assert reply.stop_reason=='end_turn',reply
  expected='SDK checkpoint messages='+('1' if sys.argv[1]=='new' else '3')
  assert (root/'artifact.txt').read_text()==expected
  print('phase',sys.argv[1],expected,'end_turn',flush=True)
asyncio.run(main())
`
	for _, phase := range []string{"new", "load"} {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		cmd := Command{Path: python}.exec(ctx, "-c", script, phase)
		cmd.Dir = root
		cmd.Env = []string{"HOME=" + root, "PATH=" + filepath.Dir(python) + ":/usr/bin:/bin", "DEEPAGENTS_HOME=" + filepath.Join(root, "profile"), "LANG=C.UTF-8", "DO_NOT_TRACK=1"}
		output, err := combinedOutputOwned(cmd, nil)
		cancel()
		t.Log(string(output))
		if err != nil {
			t.Fatal(err)
		}
	}
}
