//go:build agentintegration

package daemon

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenAISandboxPluginHandlerRebinding(t *testing.T) {
	if os.Getenv("MULTICA_RUN_REAL_AGENT_SMOKE") != "1" {
		t.Skip("explicit real smoke opt-in required")
	}
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	origin, err := exec.Command("git", "-C", repo, "remote", "get-url", "origin").Output()
	if err != nil || !strings.Contains(string(origin), "CM-BF/multica-runtime-lab") {
		t.Fatal("fork gate")
	}
	bridge := os.Getenv("MULTICA_OPENAI_SANDBOX_TEST_BRIDGE")
	if !filepath.IsAbs(bridge) {
		t.Fatal("explicit private bridge copy required")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cwd := filepath.Join(root, "cwd")
	if err = os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tools := []PluginHookTool{{Name: "platform_echo", InstallationID: "install", HookKey: "hook", InputSchema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"]}`)}}
	start := func(policy []PluginHookTool) (json.RawMessage, *pluginHookMCPSet, *atomic.Int32) {
		calls := new(atomic.Int32)
		raw, set, e := startTaskPluginHookMCP(ctx, "task", policy, func(_ context.Context, task, installation, hook string, input json.RawMessage) (json.RawMessage, error) {
			calls.Add(1)
			if task != "task" || installation != "install" || hook != "hook" {
				t.Error("wrong stable handler routing")
			}
			var v struct {
				Value string `json:"value"`
			}
			if e := json.Unmarshal(input, &v); e != nil {
				return nil, e
			}
			return json.Marshal(map[string]string{"value": "handler-" + v.Value})
		}, nil)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(set.Close)
		raw, e = bindOpenAISandboxPluginMCP(raw, policy)
		if e != nil {
			t.Fatal(e)
		}
		raw, e = mergeTaskRemoteMCPConfig(nil, raw)
		if e != nil {
			t.Fatal(e)
		}
		return raw, set, calls
	}
	run := func(raw json.RawMessage, token, prompt string) map[string]any {
		request := map[string]any{"type": "execute", "requestId": prompt, "prompt": prompt, "instructions": "context-canary", "model": "simulated", "cwd": cwd, "stateRoot": filepath.Join(root, "state"), "inputs": []string{}, "artifacts": []string{"artifacts"}, "mcp": raw, "sessionId": token}
		data, e := json.Marshal(request)
		if e != nil {
			t.Fatal(e)
		}
		file := filepath.Join(root, "request.json")
		if e = os.WriteFile(file, data, 0600); e != nil {
			t.Fatal(e)
		}
		cmd := exec.CommandContext(ctx, node, filepath.Join(bridge, "test/mcp-roundtrip-entry.mjs"), file)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "USERPROFILE=" + root, "TMPDIR=" + root, "XDG_CONFIG_HOME=" + root, "XDG_CACHE_HOME=" + root, "XDG_DATA_HOME=" + root, "OPENAI_AGENTS_DISABLE_TRACING=1"}
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("node: %v %s", e, out)
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		var result map[string]any
		if e = json.Unmarshal([]byte(lines[len(lines)-1]), &result); e != nil {
			t.Fatalf("%v %s", e, out)
		}
		return result
	}
	first, old, oldCalls := start(tools)
	a := run(first, "", "one")
	if a["status"] != "completed" {
		t.Fatalf("first: %+v", a)
	}
	token := a["sessionId"].(string)
	old.Close()
	second, current, currentCalls := start(tools)
	if string(first) == string(second) {
		t.Fatal("transport did not rotate")
	}
	b := run(second, token, "two")
	if b["status"] != "completed" || b["sessionId"] != token {
		t.Fatalf("second: %+v", b)
	}
	if oldCalls.Load() != 1 || currentCalls.Load() != 1 {
		t.Fatal("wrong handler/call count")
	}
	current.Close()
	// A changed stable hook route must fail even before model/tool execution.
	policyTools := append([]PluginHookTool(nil), tools...)
	policyTools[0].HookKey = "other-hook"
	policyRaw, policyServer, policyCalls := start(policyTools)
	policyResult := run(policyRaw, token, "policy-change")
	if policyResult["error"] != "CHECKPOINT_INVALID" || policyCalls.Load() != 0 {
		t.Fatalf("stable policy: %+v calls=%d", policyResult, policyCalls.Load())
	}
	policyServer.Close()
	// Rebinding must inspect tools/list before allowing even one model/tool call.
	changedTools := append([]PluginHookTool(nil), tools...)
	changedTools[0].InputSchema = json.RawMessage(`{"type":"object","properties":{"changed":{"type":"string"}}}`)
	third, changed, changedCalls := start(changedTools)
	// Keep the claimed binding unchanged to exercise actual advertised-tool verification.
	var config map[string]any
	_ = json.Unmarshal(third, &config)
	config["mcpServers"].(map[string]any)["multica-plugins"].(map[string]any)["multicaBinding"] = map[string]any{"kind": "plugin-hook", "tools": tools}
	third, _ = json.Marshal(config)
	c := run(third, token, "three")
	if c["error"] != "MCP_POLICY_CHANGED" || changedCalls.Load() != 0 {
		t.Fatalf("tool policy: %+v calls=%d", c, changedCalls.Load())
	}
	changed.Close()
	t.Log("Two distinct real daemon handlers + official SDK HTTP list/call + Unix checkpoint continuation passed; changed tools rejected before driver, zero calls")
}
