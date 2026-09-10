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
