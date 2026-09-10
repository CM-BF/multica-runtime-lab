package agent

import (
	"context"
	"encoding/json"
	"fmt"
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
import sys,json,os,subprocess,time
mode=os.environ['DA_MODE']
record=os.environ['DA_RECORD']
with open(record+'.pid','w') as f:f.write(str(os.getpid()))
if mode in ('credentials','dependency'):
 print(('Error: No credentials configured' if mode=='credentials' else 'ModuleNotFoundError: missing runtime dependency')+' private-value-do-not-leak',file=sys.stderr,flush=True)
 sys.exit(1)
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
  if mode=='burst':
   for i in range(400):
    update('text-%04d;'%i)
    id='tool-%04d'%i
    send({'jsonrpc':'2.0','method':'session/update','params':{'sessionId':'s1','update':{'sessionUpdate':'tool_call','toolCallId':id,'title':'Read file','kind':'read','status':'in_progress','rawInput':{'path':'test'}}}})
    send({'jsonrpc':'2.0','method':'session/update','params':{'sessionId':'s1','update':{'sessionUpdate':'tool_call_update','toolCallId':id,'status':'completed','content':[{'type':'content','content':{'type':'text','text':'result-%04d'%i}}]}}})
  if mode=='tools':
   update('BEFORE')
   for id in ('tool1','tool2'):
    send({'jsonrpc':'2.0','method':'session/update','params':{'sessionId':'s1','update':{'sessionUpdate':'tool_call','toolCallId':id,'title':'Read file','kind':'read','status':'in_progress','rawInput':{'path':'test'}}}})
    send({'jsonrpc':'2.0','method':'session/update','params':{'sessionId':'s1','update':{'sessionUpdate':'tool_call_update','toolCallId':id,'status':'completed','content':[{'type':'content','content':{'type':'text','text':'bytes'}}]}}})
  update('CURRENT')
  result={'stopReason':mode if mode in ('max_tokens','max_turn_requests','refusal','cancelled','unknown') else 'end_turn'}
  if mode=='no-stop':result={}
 send({'jsonrpc':'2.0','id':q['id'],'result':result})
 if mode=='no-read' and method=='session/new':
  import fcntl,termios,array
  child=subprocess.Popen([sys.executable,'-c','import time;time.sleep(30)'])
  with open(record+'.child','w') as f:f.write(str(child.pid))
  while True:
   queued=array.array('i',[0])
   fcntl.ioctl(sys.stdin.fileno(),termios.FIONREAD,queued,True)
   if queued[0]>=4096:break
   time.sleep(.001)
  with open(record+'.full','w') as f:f.write(str(queued[0]))
  time.sleep(30)
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

func TestDeepAgentsSlowConsumerLossless(t *testing.T) {
	b, o, _ := deepAgentsFake(t, "burst")
	o.Timeout = 10 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, e := b.Execute(ctx, "prompt", o)
	if e != nil {
		t.Fatal(e)
	}
	// Observe a full queue, independent of Python startup speed.
	deepAgentsWaitFullQueue(t, s)
	time.Sleep(20 * time.Millisecond)
	var text strings.Builder
	starts, ends := map[string]int{}, map[string]int{}
	outputs := map[string]string{}
	for m := range s.Messages {
		switch m.Type {
		case MessageText:
			text.WriteString(m.Content)
		case MessageToolUse:
			starts[m.CallID]++
		case MessageToolResult:
			ends[m.CallID]++
			outputs[m.CallID] = m.Output
		}
		time.Sleep(time.Millisecond)
	}
	r := <-s.Result
	if r.Status != "completed" {
		t.Fatalf("%+v", r)
	}
	var want strings.Builder
	for i := 0; i < 400; i++ {
		want.WriteString(fmt.Sprintf("text-%04d;", i))
		id := fmt.Sprintf("tool-%04d", i)
		if starts[id] != 1 || ends[id] != 1 || !strings.Contains(outputs[id], fmt.Sprintf("result-%04d", i)) {
			t.Fatalf("%s starts=%d ends=%d", id, starts[id], ends[id])
		}
	}
	want.WriteString("CURRENT")
	if text.String() != want.String() || r.Output != "CURRENT" {
		t.Fatalf("stream bytes=%d want=%d final=%q", text.Len(), want.Len(), r.Output)
	}
	t.Logf("delivered 401 text chunks, %d tool starts, %d tool completions; result=%s", len(starts), len(ends), r.Status)
}

func TestDeepAgentsUnconsumedMessages(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelRun), func(t *testing.T) {
			b, o, _ := deepAgentsFake(t, "burst")
			o.Timeout = 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s, e := b.Execute(ctx, "prompt", o)
			if e != nil {
				t.Fatal(e)
			}
			if cancelRun {
				deepAgentsWaitFullQueue(t, s)
				time.Sleep(20 * time.Millisecond)
				cancel()
			}
			select {
			case r := <-s.Result:
				want := "failed"
				if cancelRun {
					want = "aborted"
				}
				if r.Status != want || !strings.Contains(r.Error, "message") {
					t.Fatalf("silent stream loss: %+v", r)
				}
				n := 0
				for range s.Messages {
					n++
				}
				if n != 256 {
					t.Fatalf("buffered events=%d", n)
				}
				t.Logf("no consumer: %s, %q; %d queued events retained", r.Status, r.Error, n)
			case <-time.After(4 * time.Second):
				t.Fatal("Result blocked by unconsumed Messages")
			}
		})
	}
}

func TestDeepAgentsCancelNoReadStdin(t *testing.T) {
	b, o, path := deepAgentsFake(t, "no-read")
	o.Timeout = 0
	o.TurnInterruptTimeout = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Cleanup(func() {
		for _, suffix := range []string{".child", ".pid"} {
			data, _ := os.ReadFile(path + suffix)
			pid, _ := strconv.Atoi(string(data))
			if pid > 0 {
				p, e := os.FindProcess(pid)
				if e == nil {
					_ = p.Kill()
				}
			}
		}
	})
	s, e := b.Execute(ctx, strings.Repeat("x", 8<<20), o)
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if data, e := os.ReadFile(path + ".full"); e == nil {
			t.Logf("child does not read stdin; pipe has %s unread bytes, prompt=8 MiB", data)
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture did not fill stdin")
		}
		time.Sleep(time.Millisecond)
	}
	started := time.Now()
	cancel()
	select {
	case r := <-s.Result:
		elapsed := time.Since(started)
		if r.Status != "aborted" || elapsed > o.TurnInterruptTimeout+2*time.Second {
			t.Fatalf("elapsed=%s result=%+v", elapsed, r)
		}
		for _, suffix := range []string{".child", ".pid"} {
			data, e := os.ReadFile(path + suffix)
			if e != nil {
				t.Fatal(e)
			}
			pid, _ := strconv.Atoi(string(data))
			p, _ := os.FindProcess(pid)
			deadline := time.Now().Add(time.Second)
			for p.Signal(syscall.Signal(0)) == nil {
				if time.Now().After(deadline) {
					t.Fatalf("process %d survived", pid)
				}
				time.Sleep(time.Millisecond)
			}
		}
		t.Logf("cancel Result after %s (budget %s + 2s cleanup); parent/child reaped", elapsed, o.TurnInterruptTimeout)
	case <-time.After(o.TurnInterruptTimeout + 2*time.Second):
		t.Fatal("cancel blocked on full stdin/write mutex")
	}
}

func deepAgentsWaitFullQueue(t *testing.T, s *Session) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for len(s.Messages) != 256 {
		if time.Now().After(deadline) {
			t.Fatal("message queue did not fill")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDeepAgentsSafeStartupDiagnostics(t *testing.T) {
	for _, mode := range []string{"credentials", "dependency"} {
		t.Run(mode, func(t *testing.T) {
			b, o, _ := deepAgentsFake(t, mode)
			s, e := b.Execute(context.Background(), "prompt", o)
			if e != nil {
				t.Fatal(e)
			}
			r := deepAgentsResult(t, s)
			want := "configure provider credentials"
			if mode == "dependency" {
				want = "install the documented deepagents-code"
			}
			if r.Status != "failed" || !strings.Contains(r.Error, want) || strings.Contains(r.Error, "private-value-do-not-leak") {
				t.Fatalf("%+v", r)
			}
			t.Log(r.Error)
		})
	}
}
