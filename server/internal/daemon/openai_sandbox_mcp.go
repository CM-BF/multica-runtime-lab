package daemon

import (
	"encoding/json"
	"sort"
)

// Bind only OA plugin transports to their stable routing and tool policy.
// Other providers receive precisely the original plugin configuration.
func bindOpenAISandboxPluginMCP(raw json.RawMessage, tools []PluginHookTool) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	var config struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	stable := append([]PluginHookTool(nil), tools...)
	sort.Slice(stable, func(i, j int) bool { return stable[i].Name < stable[j].Name })
	if entry := config.Servers["multica-plugins"]; entry != nil {
		entry["multicaBinding"] = map[string]any{"kind": "plugin-hook", "tools": stable}
	}
	return json.Marshal(config)
}

// stripOpenAISandboxBindings treats every ordinary MCP document as untrusted.
// The field is protocol metadata, never a user/runtime configuration option.
func stripOpenAISandboxBindings(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return raw, nil
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	if serversRaw, ok := document["mcpServers"]; ok {
		var servers map[string]map[string]json.RawMessage
		if err := json.Unmarshal(serversRaw, &servers); err != nil {
			return nil, err
		}
		for _, entry := range servers {
			delete(entry, "multicaBinding")
		}
		cleaned, err := json.Marshal(servers)
		if err != nil {
			return nil, err
		}
		document["mcpServers"] = cleaned
	}
	return json.Marshal(document)
}

// assembleOpenAISandboxMCP is the final provenance boundary. Runtime and user
// layers can never grant a binding, even if no real hook exists this turn.
// plugin must come directly from this turn's startTaskPluginHookMCP result.
// It wins last; no ordinary overlay is applied after this function.
func assembleOpenAISandboxMCP(runtime, user, plugin json.RawMessage, tools []PluginHookTool) (json.RawMessage, error) {
	cleanRuntime, err := stripOpenAISandboxBindings(runtime)
	if err != nil {
		return nil, err
	}
	cleanUser, err := stripOpenAISandboxBindings(user)
	if err != nil {
		return nil, err
	}
	merged, err := mergeTaskRemoteMCPConfig(cleanRuntime, cleanUser)
	if err != nil {
		return nil, err
	}
	trusted, err := bindOpenAISandboxPluginMCP(plugin, tools)
	if err != nil {
		return nil, err
	}
	return mergeTaskRemoteMCPConfig(merged, trusted)
}
