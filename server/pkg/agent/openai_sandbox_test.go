package agent

import (
	"context"
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

func sandboxFixture(t *testing.T, mode string) (Backend, ExecOptions, string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	script := filepath.Join(root, "bridge.py")
	record := filepath.Join(root, "record")
	code := fmt.Sprintf(`import os,sys,json,time,subprocess
mode=%q
record=%q
assert not os.environ.get('MULTICA_TASK_CONFIG_ROOT')
assert not os.environ.get('OPENAI_API_KEY')
assert os.environ['HOME'] != %q
print(json.dumps({'type':'ready','protocol':1,'sdk':'wrong' if mode=='version' else '0.17.2'}),flush=True)
if mode=='no-read':
 child=subprocess.Popen([sys.executable,'-c','import time;time.sleep(30)'])
 open(record,'w').write(str(child.pid))
 time.sleep(30)
 sys.exit(0)
r=json.loads(sys.stdin.readline());seq=0
def send(**kw):
 global seq
 seq+=1
 print(json.dumps(dict(requestId=r['requestId'],seq=seq,**kw)),flush=True)
if mode=='eof':sys.exit(0)
for i in range(400):
 send(type='event',kind='text',text=str(i)+';')
 send(type='event',kind='tool_use',id=str(i),name='shell')
 send(type='event',kind='tool_result',id=str(i),text='done')
send(type='result',status='completed',output='done',sessionId='a'*32)
if mode=='terminal-hang':time.sleep(30)
`, mode, record, os.Getenv("HOME"))
	if err = os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := New("openai-sandbox", Config{ExecutablePath: python, LaunchPrefix: []string{script}, Env: map[string]string{"OPENAI_SANDBOX_STATE": filepath.Join(root, "state")}})
	if err != nil {
		t.Fatal(err)
	}
	return b, ExecOptions{Cwd: root, Model: "test", Timeout: 8 * time.Second, TurnInterruptTimeout: 30 * time.Millisecond}, record
}
func TestOpenAISandboxSlowMixedDelivery(t *testing.T) {
	b, o, _ := sandboxFixture(t, "burst")
	s, e := b.Execute(context.Background(), "prompt", o)
	if e != nil {
		t.Fatal(e)
	}
	text := strings.Builder{}
	calls, done := map[string]bool{}, map[string]bool{}
	for m := range s.Messages {
		time.Sleep(time.Millisecond)
		switch m.Type {
		case MessageText:
			text.WriteString(m.Content)
		case MessageToolUse:
			calls[m.CallID] = true
		case MessageToolResult:
			done[m.CallID] = true
		}
	}
	r := <-s.Result
	var expected strings.Builder
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&expected, "%d;", i)
		if !calls[strconv.Itoa(i)] || !done[strconv.Itoa(i)] {
			t.Fatalf("missing %d", i)
		}
	}
	if text.String() != expected.String() || r.Status != "completed" || r.Output != "done" {
		t.Fatalf("text length=%d result=%+v", text.Len(), r)
	}
}
func TestOpenAISandboxCancelBlockedStdinReapsChild(t *testing.T) {
	b, o, record := sandboxFixture(t, "no-read")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, e := b.Execute(ctx, strings.Repeat("x", 8*1024*1024), o)
	if e != nil {
		t.Fatal(e)
	}
	var raw []byte
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		raw, _ = os.ReadFile(record)
		if len(raw) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	pid, e := strconv.Atoi(string(raw))
	if e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	cancel()
	select {
	case r := <-s.Result:
		if r.Status != "aborted" {
			t.Fatalf("%+v", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel blocked")
	}
	if time.Since(start) > o.TurnInterruptTimeout+2500*time.Millisecond {
		t.Fatal("cleanup exceeded bound")
	}
	if syscall.Kill(pid, 0) == nil {
		t.Fatalf("child %d survived", pid)
	}
}
func TestOpenAISandboxProtocolFailures(t *testing.T) {
	for _, mode := range []string{"version", "eof", "terminal-hang"} {
		t.Run(mode, func(t *testing.T) {
			b, o, _ := sandboxFixture(t, mode)
			s, e := b.Execute(context.Background(), "p", o)
			if e != nil {
				t.Fatal(e)
			}
			for range s.Messages {
			}
			r := <-s.Result
			if r.Status != "failed" {
				t.Fatalf("%+v", r)
			}
		})
	}
}
func TestOpenAISandboxUnconsumedMessagesBounded(t *testing.T) {
	b, o, _ := sandboxFixture(t, "burst")
	s, e := b.Execute(context.Background(), "p", o)
	if e != nil {
		t.Fatal(e)
	}
	select {
	case r := <-s.Result:
		if !strings.Contains(r.Error, "MESSAGE_CONSUMER_STALLED") {
			t.Fatalf("%+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stalled consumer hung")
	}
}
