//go:build agentintegration

package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/pkg/agent"
)

// This exercises the installed dcode loader, not a credentialed model turn.
func TestDeepAgentsRealPluginMCP(t *testing.T) {
	for _, mode := range []string{"success", "http-required", "stdio-required", "http-optional", "missing-tool", "stdio-success", "missing-metadata", "schema-mismatch"} {
		t.Run(mode, func(t *testing.T) { testDeepAgentsRealPluginMCP(t, mode) })
	}
}
func testDeepAgentsRealPluginMCP(t *testing.T, mode string) {
	if os.Getenv("MULTICA_RUN_REAL_AGENT_SMOKE") != "1" {
		t.Skip("real agent smoke disabled")
	}
	python := os.Getenv("MULTICA_DEEPAGENTS_TEST_PYTHON")
	if !filepath.IsAbs(python) {
		t.Fatal("explicit isolated Python path required")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	calls := make(chan string, 2)
	raw, set, err := startTaskPluginHookMCP(ctx, "task-canary", []PluginHookTool{{Name: "platform_echo", InstallationID: "install-canary", HookKey: "hook-canary", InputSchema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`)}}, func(_ context.Context, task, install, hook string, input json.RawMessage) (json.RawMessage, error) {
		stage, _ := os.ReadFile(filepath.Join(root, "rpc-stage"))
		if string(stage) != "session/prompt" {
			t.Error("tool called before prompt")
		}
		calls <- task + "/" + install + "/" + hook + "/" + string(input)
		return json.RawMessage(`{"value":"platform-canary"}`), nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	var config struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	if err = json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(mode, "http-") {
		config.Servers["multica-plugins"]["url"] = config.Servers["multica-plugins"]["url"].(string) + "/wrong"
	}
	if mode == "stdio-required" {
		config.Servers["missing-stdio"] = map[string]any{"command": filepath.Join(root, "missing-command")}
	}
	if mode == "stdio-success" {
		stdioScript := filepath.Join(root, "stdio.py")
		code := `from mcp.server.fastmcp import FastMCP
from pathlib import Path
mcp=FastMCP("fixture")
@mcp.tool()
def stdio_echo(value:str)->str:
 Path("stdio-called").write_text(value)
 return value
mcp.run(transport="stdio")
`
		if err = os.WriteFile(stdioScript, []byte(code), 0600); err != nil {
			t.Fatal(err)
		}
		config.Servers["stdio-fixture"] = map[string]any{"command": python, "args": []string{stdioScript}}
	}
	raw, _ = json.Marshal(config)
	requiredTools := map[string]map[string]json.RawMessage{}
	if mode == "success" {
		requiredTools["multica-plugins"] = map[string]json.RawMessage{"platform_echo": json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`)}
	}
	if mode == "missing-tool" {
		requiredTools["multica-plugins"] = map[string]json.RawMessage{"missing": json.RawMessage(`{}`)}
	}
	if mode == "schema-mismatch" {
		requiredTools["multica-plugins"] = map[string]json.RawMessage{"platform_echo": json.RawMessage(`{"type":"array"}`)}
	}
	policy := map[string]bool{}
	if mode == "missing-metadata" {
		policy["missing-server"] = true
	}
	if mode == "http-optional" {
		policy["multica-plugins"] = false
	}

	script := filepath.Join(root, "loader.py")
	bridge, err := filepath.Abs("../../../runtime-bridges/deepagents")
	if err != nil {
		t.Fatal(err)
	}
	code := `import os,sys
request=os.environ['MULTICA_DEEPAGENTS_REQUEST']
bridge=os.environ['DA_TEST_BRIDGE']
root=os.getcwd()
os.environ.clear()
os.environ.update(HOME=root,PATH=os.path.dirname(sys.executable)+':/usr/bin:/bin',DEEPAGENTS_HOME=root+'/profile',XDG_CONFIG_HOME=root,XDG_DATA_HOME=root,XDG_CACHE_HOME=root,MULTICA_DEEPAGENTS_REQUEST=request,DO_NOT_TRACK='1')
sys.path.insert(0,bridge)
import asyncio,json,traceback
from multica_dcode_acp import install_guard
guard=install_guard()
from deepagents_code.mcp_tools import resolve_and_load_mcp_tools
async def main():
 path=sys.argv[sys.argv.index('--mcp-config')+1]
 assert os.stat(path).st_mode & 0o777 == 0o600
 config=json.load(open(path))
 assert config['mcpServers']['multica-plugins']['type']=='http'
 tools,manager,infos=await resolve_and_load_mcp_tools(explicit_config_path=path)
 assert guard.calls==1
 names=[t.name for t in tools]
 print('loader tools/list: '+repr(names),file=sys.stderr,flush=True)
 tool=next((t for t in tools if 'platform_echo' in t.name),None)
 try:
  while True:
   line=await asyncio.to_thread(sys.stdin.readline)
   if not line: break
   q=json.loads(line); method=q['method']; result={}
   open('rpc-stage','w').write(method)
   if method=='initialize':
    assert not os.path.exists('stdio-called')
    result={'protocolVersion':1,'agentCapabilities':{}}
   elif method=='session/new': result={'sessionId':'plugin-test'}
   elif method=='session/prompt':
    output=await tool.ainvoke({'value':'platform-canary'}) if tool else 'platform-canary optional-degraded'
    stdio_tool=next((t for t in tools if 'stdio_echo' in t.name),None)
    if stdio_tool: assert 'stdio-canary' in str(await stdio_tool.ainvoke({'value':'stdio-canary'}))
    assert 'platform-canary' in str(output),repr(output)
    print(json.dumps({'jsonrpc':'2.0','method':'session/update','params':{'sessionId':'plugin-test','update':{'sessionUpdate':'agent_message_chunk','content':{'type':'text','text':str(output)}}}}),flush=True)
    if manager: await manager.cleanup()
    manager=None
    result={'stopReason':'end_turn'}
   if 'id' in q: print(json.dumps({'jsonrpc':'2.0','id':q['id'],'result':result}),flush=True)
 finally:
  if manager: await manager.cleanup()
try: asyncio.run(main())
except Exception:
 open('loader-error.txt','w').write(traceback.format_exc())
 raise
`
	if err = os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(root, "profile")
	backend, err := agent.New("deepagents", agent.Config{ExecutablePath: python, LaunchPrefix: []string{script}, Env: map[string]string{"DEEPAGENTS_HOME": profile, "DA_TEST_BRIDGE": bridge}})
	if err != nil {
		t.Fatal(err)
	}
	session, err := backend.Execute(ctx, "invoke platform echo", agent.ExecOptions{Cwd: root, McpConfig: raw, McpServerRequired: policy, McpRequiredTools: requiredTools, HandshakeTimeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	for range session.Messages {
	}
	result := <-session.Result
	if mode == "http-required" || mode == "stdio-required" || mode == "missing-tool" || mode == "missing-metadata" || mode == "schema-mismatch" {
		if result.Status != "failed" || !strings.Contains(result.Error, "MCP_REQUIRED_UNREADY") {
			t.Fatalf("expected required failure: %+v", result)
		}
		if _, err = os.Stat(filepath.Join(root, "rpc-stage")); !os.IsNotExist(err) {
			t.Fatal("RPC reached before readiness", err)
		}
		if len(calls) != 0 {
			t.Fatal("readiness invoked tool")
		}
		t.Log(mode, "real loader failure blocked every RPC and tool call", result.Error)
		return
	}
	if mode == "http-optional" {
		if result.Status != "completed" || len(calls) != 0 {
			t.Fatalf("optional degradation: %+v calls=%d", result, len(calls))
		}
		t.Log("optional real HTTP failure degraded; zero tool calls; completed")
		return
	}

	if result.Status != "completed" || !strings.Contains(result.Output, "platform-canary") {
		diagnostic, _ := os.ReadFile(filepath.Join(root, "loader-error.txt"))
		t.Fatalf("result=%+v loader=%s", result, diagnostic)
	}
	select {
	case call := <-calls:
		if call != `task-canary/install-canary/hook-canary/{"value":"platform-canary"}` {
			t.Fatalf("handler arguments: %s", call)
		}
		t.Log("real dcode tools/list discovered platform_echo; tools/call traversed daemon handler:", call)
	default:
		t.Fatal("daemon handler was not invoked")
	}
	if len(calls) != 0 {
		t.Fatal("unexpected extra handler invocation")
	}
	if mode == "stdio-success" {
		data, err := os.ReadFile(filepath.Join(root, "stdio-called"))
		if err != nil || string(data) != "stdio-canary" {
			t.Fatal("real stdio invocation", string(data), err)
		}
		t.Log("real stdio tool round trip; zero readiness invocations")
	}
	t.Log("generated HTTP config -> adapter private 0600 config -> installed dcode loader -> loopback daemon handler -> completed ACP result; isolated env has no model credentials")
}
