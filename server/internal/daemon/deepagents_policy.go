package daemon

import (
	"encoding/json"
	"github.com/multica-ai/multica/server/pkg/remotemcp"
)

// Policy follows the actual winning overlay, including optional plugin hooks.
func deepAgentsMCPPolicy(provider string, final, overlay, plugin json.RawMessage, connections []remotemcp.Connection) map[string]bool {
	if provider != "deepagents" {
		return nil
	}
	names := func(raw json.RawMessage) map[string]json.RawMessage {
		var c struct {
			Servers map[string]json.RawMessage `json:"mcpServers"`
		}
		_ = json.Unmarshal(raw, &c)
		return c.Servers
	}
	result := map[string]bool{}
	finalNames, overlayNames := names(final), names(overlay)
	for _, c := range connections {
		name := remoteMCPServerName(c)
		if finalNames[name] != nil && overlayNames[name] != nil {
			result[name] = c.FailurePolicy != "optional"
		}
	}
	for name := range names(plugin) {
		if finalNames[name] != nil && overlayNames[name] != nil {
			result[name] = false
		}
	}
	return result
}

func deepAgentsRequiredTools(provider string, final, overlay, plugin json.RawMessage, connections []remotemcp.Connection) map[string]map[string]json.RawMessage {
	policy := deepAgentsMCPPolicy(provider, final, overlay, plugin, connections)
	result := map[string]map[string]json.RawMessage{}
	for _, c := range connections {
		name := remoteMCPServerName(c)
		if !policy[name] {
			continue
		}
		result[name] = map[string]json.RawMessage{}
		for _, tool := range c.ApprovedTools {
			result[name][tool.Name] = tool.InputSchema
		}
	}
	return result
}
