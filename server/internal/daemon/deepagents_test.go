package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/multica-ai/multica/server/pkg/agent"
	"github.com/multica-ai/multica/server/pkg/remotemcp"
	"os"
	"path/filepath"
	"strings"
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

func TestDeepAgentsMCPConfigurationBoundary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(home, ".deepagents"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".deepagents", "mcp.json"), []byte(`{"mcpServers":{"ambient":{"command":"not-imported"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	explicit := json.RawMessage(`{"mcpServers":{"user":{"command":"explicit-stdio","args":["value"],"env":{"CANARY":"test"}}}}`)
	merged, err := mergeRuntimeAndAgentMcpConfig("deepagents", explicit)
	if err != nil || !bytes.Equal(merged, explicit) {
		t.Fatalf("explicit config changed: %s %v", merged, err)
	}
	defaults, err := mergeRuntimeAndAgentMcpConfig("deepagents", nil)
	if err != nil || len(defaults) != 0 {
		t.Fatalf("unexpected default MCP: %s %v", defaults, err)
	}
	_, supported, err := loadRuntimeMcpServerConfigs("deepagents")
	if err != nil || supported {
		t.Fatal("global import must be unsupported", err)
	}
	t.Log("explicit agent stdio config preserved byte-for-byte; nil stays nil; global runtime MCP import unsupported")

	// Exercise the real broker gate without opening a listener or resolving credentials.
	raw, _, brokers, err := startTaskRemoteMCPBrokers(context.Background(), context.Background(), "test", "deepagents", []remotemcp.Connection{{ContributionKey: "required", FailurePolicy: "required"}}, nil, nil)
	if err == nil || brokers != nil || len(raw) != 0 || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("remote gate: %s %v %v", raw, brokers, err)
	}
	t.Log("required platform Remote MCP broker rejected by provider gate")

	// The production plugin generator uses its own ephemeral loopback listener.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	plugin, set, err := startTaskPluginHookMCP(ctx, "test", []PluginHookTool{{Name: "test-tool"}}, func(context.Context, string, string, string, json.RawMessage) (json.RawMessage, error) {
		t.Error("no external invocation expected")
		return nil, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	merged, err = mergeTaskRemoteMCPConfig(explicit, plugin)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Servers map[string]struct {
			Type string `json:"type"`
		} `json:"mcpServers"`
	}
	if err = json.Unmarshal(merged, &config); err != nil || config.Servers["multica-plugins"].Type != "http" {
		t.Fatal("plugin config transport", err)
	}
	backend, err := agent.New("deepagents", agent.Config{ExecutablePath: filepath.Join(home, "must-not-start"), Env: map[string]string{"DEEPAGENTS_HOME": filepath.Join(home, "state")}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = backend.Execute(ctx, "test", agent.ExecOptions{Cwd: home, McpConfig: merged})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected executable lookup after accepting real plugin HTTP config: %v", err)
	}
	cancel()
	set.Close()
	t.Log("real multica-plugins HTTP config merged with explicit stdio and accepted by adapter validation")
}
