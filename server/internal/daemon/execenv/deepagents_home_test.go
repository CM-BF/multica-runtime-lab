package execenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeepAgentsStateIsolation(t *testing.T) {
	root := t.TempDir()
	task := TaskContextForEnv{IssueID: "issue"}
	a, e := prepareDeepAgentsHomeAt(root, "runtime", "agent", "workspace", "task1", task)
	if e != nil {
		t.Fatal(e)
	}
	b, e := prepareDeepAgentsHomeAt(root, "runtime", "agent", "workspace", "task2", task)
	if e != nil || a != b {
		t.Fatal("followup changed shard", e)
	}
	for _, values := range [][3]string{{"other", "agent", "workspace"}, {"runtime", "other", "workspace"}, {"runtime", "agent", "other"}} {
		b, e = prepareDeepAgentsHomeAt(root, values[0], values[1], values[2], "task1", task)
		if e != nil || a == b {
			t.Fatal("identity collision", e)
		}
	}
	info, e := os.Stat(a)
	if e != nil || info.Mode().Perm() != 0700 {
		t.Fatal("private mode", e)
	}
	if got := skillsDirPath(root, "deepagents"); got != filepath.Join(root, ".deepagents", "skills") {
		t.Fatal(got)
	}
}
