//go:build agentintegration

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func realOpenAISandbox(t *testing.T, cancellation bool) (Backend, ExecOptions) {
	t.Helper()
	if os.Getenv("MULTICA_RUN_REAL_AGENT_SMOKE") != "1" {
		t.Skip("requires explicit real smoke opt-in")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	origin, err := exec.Command("git", "-C", root, "remote", "get-url", "origin").Output()
	if err != nil || !strings.Contains(string(origin), "CM-BF/multica-runtime-lab") {
		t.Fatal("fork smoke gate")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(root, "runtime-bridges/openai-sandbox/dist/cli.js")
	if cancellation {
		entry = filepath.Join(root, "runtime-bridges/openai-sandbox/test/cancel-entry.mjs")
	}
	home := t.TempDir()
	b, err := New("openai-sandbox", Config{ExecutablePath: node, LaunchPrefix: []string{entry}, Env: map[string]string{"OPENAI_SANDBOX_STATE": filepath.Join(home, "state")}})
	if err != nil {
		t.Fatal(err)
	}
	return b, ExecOptions{Cwd: home, Model: "gpt-4.1-mini", Timeout: 10 * time.Second, TurnInterruptTimeout: 100 * time.Millisecond}
}
func TestOpenAISandboxRealEntryNoCredentials(t *testing.T) {
	b, o := realOpenAISandbox(t, false)
	s, e := b.Execute(context.Background(), "Do not call any network endpoint without credentials.", o)
	if e != nil {
		t.Fatal(e)
	}
	for range s.Messages {
	}
	r := <-s.Result
	t.Logf("real SDK entry: status=%s error=%s", r.Status, r.Error)
	if r.Status != "failed" || r.Error != "openai-sandbox: MODEL_CREDENTIALS" {
		t.Fatalf("%+v", r)
	}
	if !s.TerminalObserved() {
		t.Fatal("missing terminal")
	}
}
func TestOpenAISandboxRealSDKDescendantCancellation(t *testing.T) {
	b, o := realOpenAISandbox(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, e := b.Execute(ctx, "cancel fixture", o)
	if e != nil {
		t.Fatal(e)
	}
	var pid int
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		raw, _ := os.ReadFile(filepath.Join(o.Cwd, "child.pid"))
		pid, _ = strconv.Atoi(string(raw))
		if pid > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("real SDK child did not start")
	}
	start := time.Now()
	cancel()
	r := <-s.Result
	if r.Status != "aborted" || time.Since(start) > 3*time.Second {
		t.Fatalf("%+v", r)
	}
	if syscall.Kill(pid, 0) == nil {
		t.Fatalf("SDK descendant %d survived", pid)
	}
}
