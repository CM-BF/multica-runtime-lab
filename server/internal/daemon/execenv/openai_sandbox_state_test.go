package execenv

import (
	"os"
	"testing"
)

func TestOpenAISandboxStateIsolation(t *testing.T) {
	root := t.TempDir()
	a, e := prepareOpenAISandboxStateAt(root, "runtime", "agent", "workspace", "task", TaskContextForEnv{})
	if e != nil {
		t.Fatal(e)
	}
	b, e := prepareOpenAISandboxStateAt(root, "other", "agent", "workspace", "task", TaskContextForEnv{})
	if e != nil || a == b {
		t.Fatalf("%s %s %v", a, b, e)
	}
	info, e := os.Stat(a)
	if e != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("%v %v", info, e)
	}
	again, e := prepareOpenAISandboxStateAt(root, "runtime", "agent", "workspace", "task", TaskContextForEnv{})
	if e != nil || again != a {
		t.Fatal("unstable state identity")
	}
}
