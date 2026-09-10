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
		calls <- task + "/" + install + "/" + hook + "/" + string(input)
		return json.RawMessage(`{"value":"platform-canary"}`), nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	script := filepath.Join(root, "loader.py")
	code := `import asyncio,json,os,sys,traceback
from deepagents_code.mcp_tools import resolve_and_load_mcp_tools
async def main():
 path=sys.argv[sys.argv.index('--mcp-config')+1]
 assert os.stat(path).st_mode & 0o777 == 0o600
 config=json.load(open(path))
 assert config['mcpServers']['multica-plugins']['type']=='http'
 tools,manager,infos=await resolve_and_load_mcp_tools(explicit_config_path=path)
 names=[t.name for t in tools]
 print('loader tools/list: '+repr(names),file=sys.stderr,flush=True)
 tool=next(t for t in tools if 'platform_echo' in t.name)
 try:
  while True:
   line=await asyncio.to_thread(sys.stdin.readline)
   if not line: break
   q=json.loads(line); method=q['method']; result={}
   if method=='initialize': result={'protocolVersion':1,'agentCapabilities':{}}
   elif method=='session/new': result={'sessionId':'plugin-test'}
   elif method=='session/prompt':
    output=await tool.ainvoke({'value':'platform-canary'})
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
	backend, err := agent.New("deepagents", agent.Config{ExecutablePath: "/usr/bin/env", LaunchPrefix: []string{"-i", "HOME=" + root, "PATH=" + filepath.Dir(python) + ":/usr/bin:/bin", "DEEPAGENTS_HOME=" + profile, "XDG_CONFIG_HOME=" + root, "XDG_CACHE_HOME=" + root, "XDG_DATA_HOME=" + root, "DO_NOT_TRACK=1", python, script}, Env: map[string]string{"DEEPAGENTS_HOME": profile}})
	if err != nil {
		t.Fatal(err)
	}
	session, err := backend.Execute(ctx, "invoke platform echo", agent.ExecOptions{Cwd: root, McpConfig: raw, HandshakeTimeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	for range session.Messages {
	}
	result := <-session.Result
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
	t.Log("generated HTTP config -> adapter private 0600 config -> installed dcode loader -> loopback daemon handler -> completed ACP result; isolated env has no model credentials")
}
