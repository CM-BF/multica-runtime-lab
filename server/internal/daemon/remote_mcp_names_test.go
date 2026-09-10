package daemon

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/pkg/remotemcp"
	"github.com/multica-ai/multica/server/pkg/remotemcp/remotemcptest"
)

func TestRemoteMCPNamesPreserveInstallationsAndPolicy(t *testing.T) {
	fixture := remotemcptest.NewServer()
	defer fixture.Close()
	t.Setenv(remotemcp.DevOriginsEnv, fixture.URL)
	tools := discoveredTools(t, fixture)
	a := startupConnection(fixture, []remotemcp.Tool{tools["fixture.read"]})
	a.ContributionID = "plugin:01a08abc-1111-7111-8111-111111111111:toolbox"
	b := startupConnection(fixture, []remotemcp.Tool{tools["fixture.write"]})
	b.ContributionID = "plugin:01b09def-2222-7222-8222-222222222222:toolbox"
	b.FailurePolicy = "optional"
	first, second := remoteMCPServerName(a), remoteMCPServerName(b)
	if first == second {
		t.Fatal("distinct installations collided")
	}
	for _, c := range []remotemcp.Connection{a, b} {
		name := remoteMCPServerName(c)
		if !regexp.MustCompile(`^plugin-toolbox-[0-9a-f]{32}$`).MatchString(name) || strings.Contains(name, c.ContributionID) {
			t.Fatal("invalid or exposed identity", name)
		}
		changed := c
		changed.Endpoint = "https://rotated.invalid"
		changed.FailurePolicy = "different"
		if remoteMCPServerName(changed) != name {
			t.Fatal("transport/policy changed identity")
		}
	}
	for _, connections := range [][]remotemcp.Connection{{a, b}, {b, a}} {
		ctx, cancel := context.WithCancel(context.Background())
		raw, diagnostics, set, err := startTaskRemoteMCPBrokers(ctx, ctx, "names", "deepagents", connections, fixtureCredential, nil)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		var config struct {
			Servers map[string]json.RawMessage `json:"mcpServers"`
		}
		err = json.Unmarshal(raw, &config)
		if err != nil || len(diagnostics) != 0 || len(config.Servers) != 2 || config.Servers[first] == nil || config.Servers[second] == nil {
			t.Fatal("lost configuration", string(raw), err)
		}
		if string(config.Servers[first]) == string(config.Servers[second]) {
			t.Fatal("brokers share transport")
		}
		policy := deepAgentsMCPPolicy("deepagents", raw, raw, nil, connections)
		if len(policy) != 2 || !policy[first] || policy[second] {
			t.Fatal("lost policy", policy)
		}
		required := deepAgentsRequiredTools("deepagents", raw, raw, nil, connections)
		if len(required) != 1 || len(required[first]) != 1 || string(required[first]["fixture.read"]) != string(tools["fixture.read"].InputSchema) {
			t.Fatal("wrong required tool mapping", required)
		}
		set.Close()
		cancel()
	}
	// Duplicate identities must fail even when optional, never silently replace.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	duplicate := a
	duplicate.FailurePolicy = "optional"
	raw, _, set, err := startTaskRemoteMCPBrokers(ctx, ctx, "duplicate", "deepagents", []remotemcp.Connection{a, duplicate}, fixtureCredential, nil)
	if set != nil {
		set.Close()
	}
	if err == nil || err.Error() != "Remote MCP server name collision" || raw != nil || set != nil {
		t.Fatalf("collision not rejected: %s %v", raw, err)
	}
}
