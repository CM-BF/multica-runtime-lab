package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAISandboxDiscovery(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "multica-openai-sandbox")
	if err := os.WriteFile(entry, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	original := resolveAgentsViaLoginShell
	t.Cleanup(func() { resolveAgentsViaLoginShell = original })
	resolveAgentsViaLoginShell = func([]string) map[string]string { return map[string]string{} }
	resetShellResolveCacheForTest(t)
	t.Setenv("HOME", root)
	t.Setenv("PATH", root)
	t.Setenv("MULTICA_OPENAI_SANDBOX_PATH", "")
	t.Setenv("MULTICA_OPENAI_SANDBOX_MODEL", "explicit-model")
	got, ok := probeAgentCLIs()["openai-sandbox"]
	expected, _ := filepath.EvalSymlinks(entry)
	if !ok || got.Path != expected || got.Model != "explicit-model" {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(strings.Join(defaultAgentCommandNames, "|"), "multica-openai-sandbox") || !providerNeedsInlineSystemPrompt("openai-sandbox") {
		t.Fatal("discovery/context gate missing")
	}
}
