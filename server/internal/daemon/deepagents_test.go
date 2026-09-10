package daemon

import (
	"github.com/multica-ai/multica/server/pkg/agent"
	"os"
	"path/filepath"
	"testing"
)

func TestDeepAgentsFreshRetryOnlyMissingSession(t *testing.T) {
	for _, text := range []string{"Could not resolve authentication method", "database unavailable", "session not found"} {
		if shouldRetryWithFreshSession(agent.Result{Status: "failed", Error: text}, "s1", 0, "deepagents") {
			t.Fatal(text)
		}
	}
	if !shouldRetryWithFreshSession(agent.Result{Status: "failed", ResumeRejected: true}, "s1", 0, "deepagents") {
		t.Fatal("missing resource must allow existing fallback")
	}
}

func TestDeepAgentsDiscovery(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "dcode")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	original := resolveAgentsViaLoginShell
	t.Cleanup(func() { resolveAgentsViaLoginShell = original })
	resolveAgentsViaLoginShell = func([]string) map[string]string { return map[string]string{} }
	resetShellResolveCacheForTest(t)
	t.Setenv("HOME", root)
	t.Setenv("PATH", root)
	t.Setenv("MULTICA_DEEPAGENTS_PATH", path)
	t.Setenv("MULTICA_DEEPAGENTS_MODEL", "provider:model")
	got, ok := probeAgentCLIs()["deepagents"]
	if !ok || got.Path != path || got.Model != "provider:model" {
		t.Fatalf("%+v %v", got, ok)
	}
	if providerDisplayName("deepagents") != "Deep Agents" {
		t.Fatal("display name")
	}
}
