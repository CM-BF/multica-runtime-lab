package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/multica-ai/multica/server/pkg/agent"
	"github.com/multica-ai/multica/server/pkg/remotemcp"
	"net/http"
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
	path := filepath.Join(root, "multica-dcode-acp")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	original := resolveAgentsViaLoginShell
	t.Cleanup(func() { resolveAgentsViaLoginShell = original })
	resolveAgentsViaLoginShell = func([]string) map[string]string { return map[string]string{} }
	resetShellResolveCacheForTest(t)
	t.Setenv("HOME", root)
	t.Setenv("PATH", root)
	t.Setenv("MULTICA_DEEPAGENTS_PATH", "")
	if !strings.Contains(strings.Join(defaultAgentCommandNames, "|"), "multica-dcode-acp") {
		t.Fatal("shell resolver command inventory")
	}
	t.Setenv("MULTICA_DEEPAGENTS_MODEL", "provider:model")
	got, ok := probeAgentCLIs()["deepagents"]
	expectedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.Path != expectedPath || got.Model != "provider:model" {
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
	raw, _, brokers, err := startTaskRemoteMCPBrokers(context.Background(), context.Background(), "test", "deepagents", []remotemcp.Connection{{ContributionKey: "required", FailurePolicy: "required", CredentialHeader: "Authorization"}}, nil, nil)
	if err == nil || brokers != nil || len(raw) != 0 || !strings.Contains(err.Error(), "credential resolver is unavailable") {
		t.Fatalf("remote gate: %s %v %v", raw, brokers, err)
	}
	t.Log("provider gate accepted; required broker correctly rejected missing credential resolver")

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

func TestDeepAgentsHistoryPreflightVersusLiveLoad(t *testing.T) {
	poisoned := "Invalid request: the message at position 37 with role 'assistant' must not be empty"
	for _, live := range []bool{false, true} {
		result := agent.Result{Status: "failed", Error: poisoned, ResumeLoadFailed: live}
		if got := shouldRetryWithFreshSession(result, "s1", 0, "deepagents"); got == live {
			t.Fatalf("live=%v retry=%v", live, got)
		}
		if shouldRetryWithFreshSession(result, "s1", 1, "deepagents") || shouldRetryWithFreshSession(result, "", 0, "deepagents") {
			t.Fatal("unsafe retry")
		}
	}
	if !shouldRetryWithFreshSession(agent.Result{Status: "failed", ResumeLoadFailed: true, ResumeRejected: true}, "s1", 0, "deepagents") {
		t.Fatal("exact missing resource fallback")
	}
}

func TestDeepAgentsPolicyFollowsWinningOverlay(t *testing.T) {
	connection := remotemcp.Connection{ContributionKey: "remote", FailurePolicy: "optional"}
	name := remoteMCPServerName(connection)
	overlay := json.RawMessage(`{"mcpServers":{"` + name + `":{"type":"http"},"multica-plugins":{"type":"http"}}}`)
	plugin := json.RawMessage(`{"mcpServers":{"multica-plugins":{"type":"http"}}}`)
	policy := deepAgentsMCPPolicy("deepagents", overlay, overlay, plugin, []remotemcp.Connection{connection})
	if policy[name] || policy["multica-plugins"] || len(policy) != 2 {
		t.Fatal(policy)
	}
	connection.FailurePolicy = "required"
	policy = deepAgentsMCPPolicy("deepagents", overlay, overlay, plugin, []remotemcp.Connection{connection})
	if !policy[name] || policy["multica-plugins"] {
		t.Fatal(policy)
	}
	policy = deepAgentsMCPPolicy("deepagents", overlay, nil, nil, []remotemcp.Connection{connection})
	if len(policy) != 0 {
		t.Fatal("failed overlay must not override base policy", policy)
	}
	if !providerSupportsRemoteMCPBroker("deepagents") {
		t.Fatal("provider gate")
	}
}

func TestDeepAgentsRemoteBrokerOptionalPreparation(t *testing.T) {
	called := false
	raw, diagnostics, set, err := startTaskRemoteMCPBrokers(context.Background(), context.Background(), "test", "deepagents", []remotemcp.Connection{{ContributionKey: "fixture", CredentialHeader: "Authorization", FailurePolicy: "optional"}}, func(context.Context, string) (http.Header, error) {
		called = true
		return nil, errors.New("fixture unavailable")
	}, nil)
	if err != nil || set != nil || len(raw) != 0 || len(diagnostics) != 1 || !called {
		t.Fatalf("optional preparation: %s %v %v %v called=%v", raw, diagnostics, set, err, called)
	}
}
