package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func deepAgentsFake(t *testing.T, mode string) (Backend, ExecOptions, string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	script := filepath.Join(root, "fake dcode")
	code := `#!` + python + `
import sys,json,os,subprocess
mode=os.environ['DA_MODE']
record=os.environ['DA_RECORD']
with open(record+'.pid','w') as f:f.write(str(os.getpid()))
def send(x): print(json.dumps(x),flush=True)
def update(text): send({'jsonrpc':'2.0','method':'session/update','params':{'sessionId':'s1','update':{'sessionUpdate':'agent_message_chunk','content':{'type':'text','text':text}}}})
with open(record+'.args','w') as f: json.dump(sys.argv[1:],f)
if '--mcp-config' in sys.argv:
 p=sys.argv[sys.argv.index('--mcp-config')+1]
 with open(record+'.mcp','w') as f: json.dump({'path':p,'mode':os.stat(p).st_mode&511,'content':json.load(open(p))},f)
for line in sys.stdin:
 q=json.loads(line)
 with open(record,'a') as f: f.write(json.dumps(q)+'\n')
 method=q['method']
 if method=='session/cancel':
  if mode=='cancel': send({'jsonrpc':'2.0','id':prompt_id,'result':{'stopReason':'cancelled'}})
  continue
 result={}
 if method=='initialize':result={'protocolVersion':2 if mode=='version' else 1,'agentCapabilities':{'loadSession':mode!='no-load'}}
 elif method in ('session/new','session/load'):
  if method=='session/load':
   update('OLD REPLAY')
   if mode=='missing':
    send({'jsonrpc':'2.0','id':q['id'],'error':{'code':-32002,'message':'Resource not found','data':{'uri':'s1'}}});continue
   if mode=='db':
    send({'jsonrpc':'2.0','id':q['id'],'error':{'code':-32603,'message':'database unavailable'}});continue
  result={'configOptions':[{'id':'model','category':'model','currentValue':'old'}]}
  if method=='session/new' and mode!='no-id':result['sessionId']='s1'
 elif method=='session/prompt':
  prompt_id=q['id']
  if mode in ('cancel','stubborn'):
   child=subprocess.Popen([sys.executable,'-c','import time;time.sleep(30)'])
   with open(record+'.child','w') as f:f.write(str(child.pid))
   continue
  if mode=='eof':sys.exit(0)
  if mode=='malformed':print('not json',flush=True);continue
  if mode=='tools':
   update('BEFORE')
   for id in ('tool1','tool2'):
    send({'jsonrpc':'2.0','method':'session/update','params':{'sessionId':'s1','update':{'sessionUpdate':'tool_call','toolCallId':id,'title':'Read file','kind':'read','status':'in_progress','rawInput':{'path':'test'}}}})
    send({'jsonrpc':'2.0','method':'session/update','params':{'sessionId':'s1','update':{'sessionUpdate':'tool_call_update','toolCallId':id,'status':'completed','content':[{'type':'content','content':{'type':'text','text':'bytes'}}]}}})
  update('CURRENT')
  result={'stopReason':mode if mode in ('max_tokens','max_turn_requests','refusal','cancelled','unknown') else 'end_turn'}
  if mode=='no-stop':result={}
 send({'jsonrpc':'2.0','id':q['id'],'result':result})
`
	if err = os.WriteFile(script, []byte(code), 0700); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(root, "wire.jsonl")
	b, err := New("deepagents", Config{ExecutablePath: script, Env: map[string]string{"DEEPAGENTS_HOME": filepath.Join(root, "state"), "DA_MODE": mode, "DA_RECORD": record}})
	if err != nil {
		t.Fatal(err)
	}
	return b, ExecOptions{Cwd: root, Timeout: 3 * time.Second, TurnInterruptTimeout: 30 * time.Millisecond}, record
}
func deepAgentsResult(t *testing.T, s *Session) Result {
	t.Helper()
	for range s.Messages {
	}
	r := <-s.Result
	if _, ok := <-s.Result; ok {
		t.Fatal("extra result")
	}
	return r
}
func TestDeepAgentsWireAndPrivateMCP(t *testing.T) {
	b, o, record := deepAgentsFake(t, "ok")
	o.Model = "provider:model"
	o.McpConfig = json.RawMessage(`{"mcpServers":{"local":{"command":"test-mcp","env":{"TOKEN":"test-secret"}}}}`)
	s, e := b.Execute(context.Background(), "prompt", o)
	if e != nil {
		t.Fatal(e)
	}
	r := deepAgentsResult(t, s)
	if r.Status != "completed" || r.Output != "CURRENT" || r.SessionID != "s1" || !s.TerminalObserved() {
		t.Fatalf("%+v", r)
	}
	var m struct {
		Path    string
		Mode    int
		Content any
	}
	data, _ := os.ReadFile(record + ".mcp")
	if e = json.Unmarshal(data, &m); e != nil {
		t.Fatal(e)
	}
	if m.Mode != 0600 {
		t.Fatal(m.Mode)
	}
	if _, e = os.Stat(m.Path); !os.IsNotExist(e) {
		t.Fatal("MCP not cleaned")
	}
	data, _ = os.ReadFile(record)
	if !strings.Contains(string(data), `"mcpServers": []`) || !strings.Contains(string(data), `"cwd": "`+o.Cwd+`"`) {
		t.Fatal(string(data))
	}
	data, _ = os.ReadFile(record + ".args")
	if !strings.Contains(string(data), `"--acp", "--model", "provider:model", "--mcp-config"`) {
		t.Fatal(string(data))
	}
}
func TestDeepAgentsTerminalMatrix(t *testing.T) {
	for _, mode := range []string{"max_tokens", "max_turn_requests", "refusal", "unknown", "no-stop", "cancelled", "eof", "malformed", "version", "no-id"} {
		t.Run(mode, func(t *testing.T) {
			b, o, _ := deepAgentsFake(t, mode)
			s, e := b.Execute(context.Background(), "prompt", o)
			if e != nil {
				t.Fatal(e)
			}
			r := deepAgentsResult(t, s)
			want := "failed"
			if mode == "cancelled" {
				want = "aborted"
			}
			if r.Status != want {
				t.Fatalf("%+v", r)
			}
		})
	}
}
func TestDeepAgentsLoadAndReplay(t *testing.T) {
	for _, mode := range []string{"ok", "no-load", "missing", "db"} {
		t.Run(mode, func(t *testing.T) {
			b, o, path := deepAgentsFake(t, mode)
			o.ResumeSessionID = "s1"
			o.Model = "new"
			s, e := b.Execute(context.Background(), "continue", o)
			if e != nil {
				t.Fatal(e)
			}
			r := deepAgentsResult(t, s)
			if strings.Contains(r.Output, "OLD") {
				t.Fatal("replay leaked")
			}
			if r.ResumeRejected != (mode == "missing") {
				t.Fatalf("%+v", r)
			}
			data, _ := os.ReadFile(path)
			wire := string(data)
			if strings.Contains(wire, "session/resume") {
				t.Fatal(wire)
			}
			if mode == "ok" {
				if r.Status != "completed" || !strings.Contains(wire, "session/set_config_option") {
					t.Fatalf("%+v %s", r, wire)
				}
			} else if r.Status != "failed" || strings.Contains(wire, "session/prompt") {
				t.Fatalf("%+v %s", r, wire)
			}
		})
	}
}
func TestDeepAgentsCancel(t *testing.T) {
	for _, mode := range []string{"cancel", "stubborn"} {
		t.Run(mode, func(t *testing.T) {
			b, o, path := deepAgentsFake(t, mode)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s, e := b.Execute(ctx, "prompt", o)
			if e != nil {
				t.Fatal(e)
			}
			deadline := time.Now().Add(time.Second)
			for {
				data, _ := os.ReadFile(path)
				if strings.Contains(string(data), "session/prompt") {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("prompt not received")
				}
				time.Sleep(time.Millisecond)
			}
			cancel()
			r := deepAgentsResult(t, s)
			if r.Status != "aborted" || r.ResumeRejected {
				t.Fatalf("%+v", r)
			}

			for _, suffix := range []string{".pid", ".child"} {
				data, e := os.ReadFile(path + suffix)
				if e != nil {
					t.Fatal(e)
				}
				pid, e := strconv.Atoi(string(data))
				if e != nil {
					t.Fatal(e)
				}
				process, e := os.FindProcess(pid)
				if e != nil {
					continue
				}
				deadline := time.Now().Add(time.Second)
				for process.Signal(syscall.Signal(0)) == nil {
					if time.Now().After(deadline) {
						t.Fatalf("owned process %d survived Result", pid)
					}
					time.Sleep(5 * time.Millisecond)
				}
			}
			data, _ := os.ReadFile(path)
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, "session/cancel") {
					var q map[string]any
					_ = json.Unmarshal([]byte(line), &q)
					if _, ok := q["id"]; ok {
						t.Fatal("cancel was request")
					}
					return
				}
			}
			t.Fatal("cancel not delivered")
		})
	}
}
func TestDeepAgentsRejectArgumentsAndMCP(t *testing.T) {
	for _, args := range [][]string{{"--model=x"}, {"-M", "x"}, {"--resume"}, {"--non-interactive"}, {"--mcp-config=x"}, {"--no-mcp"}, {"--startup-cmd=x"}} {
		if validateDeepAgentsArgs(args) == nil {
			t.Fatal(args)
		}
	}
	for _, raw := range []string{`{`, `{"servers":{}}`, `{"mcpServers":{"x":{}}}`, `{"mcpServers":{"x":{"url":"https://example.test"}}}`} {
		if _, _, e := prepareDeepAgentsMCP(json.RawMessage(raw)); e == nil {
			t.Fatal(raw)
		}
	}
	for i := range 3 {
		b, o, _ := deepAgentsFake(t, "ok")
		switch i {
		case 0:
			b.(*deepagentsBackend).cfg.LaunchPrefix = []string{"--resume"}
		case 1:
			o.ExtraArgs = []string{"--resume"}
		case 2:
			o.CustomArgs = []string{"--resume"}
		}
		if _, e := b.Execute(context.Background(), "p", o); e == nil {
			t.Fatal("argument layer accepted")
		}
	}
}

func TestDeepAgentsFamilyWhitelist(t *testing.T) {
	if !IsSupportedType("deepagents") {
		t.Fatal("factory whitelist")
	}
	for _, path := range []string{"../../migrations/459_runtime_profile_deepagents.up.sql", "../../../packages/core/types/agent.ts", "../../../scripts/agent-cli-command-names.txt"} {
		data, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		want := "deepagents"
		if strings.HasSuffix(path, ".txt") {
			want = "dcode"
		}
		if !strings.Contains(string(data), want) {
			t.Fatal(path)
		}
	}
}

func TestDeepAgentsToolsAndFinalAnswer(t *testing.T) {
	b, o, _ := deepAgentsFake(t, "tools")
	s, e := b.Execute(context.Background(), "prompt", o)
	if e != nil {
		t.Fatal(e)
	}
	calls := map[string]bool{}
	for m := range s.Messages {
		if m.Type == MessageToolUse {
			calls[m.CallID] = true
		}
	}
	r := <-s.Result
	if len(calls) != 2 || r.Output != "CURRENT" || r.Status != "completed" {
		t.Fatalf("calls=%v result=%+v", calls, r)
	}
}
