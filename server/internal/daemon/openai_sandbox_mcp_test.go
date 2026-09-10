package daemon

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenAISandboxBindingSources(t *testing.T) {
	runtime := json.RawMessage(`{"mcpServers":{"multica-plugins":{"type":"http","url":"http://127.0.0.1:10001/a","multicaBinding":{"kind":"plugin-hook","tools":[]}},"runtime-only":{"command":"runtime","multicaBinding":{}}}}`)
	user := json.RawMessage(`{"mcpServers":{"multica-plugins":{"type":"http","url":"http://127.0.0.1:10002/b","multicaBinding":{"kind":"plugin-hook","tools":[]}}}}`)
	plugin := json.RawMessage(`{"mcpServers":{"multica-plugins":{"type":"http","url":"http://127.0.0.1:10003/c"}}}`)
	for _, tc := range []struct {
		name                  string
		runtime, user, plugin json.RawMessage
		url                   string
		bound                 bool
	}{
		{"runtime-forgery", runtime, nil, nil, "10001/a", false},
		{"user-forgery", nil, user, nil, "10002/b", false},
		{"user-wins-without-trust", runtime, user, nil, "10002/b", false},
		{"real-plugin-wins-last", runtime, user, plugin, "10003/c", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, e := assembleOpenAISandboxMCP(tc.runtime, tc.user, tc.plugin, []PluginHookTool{{Name: "real", InstallationID: "install", HookKey: "hook"}})
			if e != nil {
				t.Fatal(e)
			}
			var doc struct {
				Servers map[string]map[string]any `json:"mcpServers"`
			}
			if e = json.Unmarshal(raw, &doc); e != nil {
				t.Fatal(e)
			}
			entry := doc.Servers["multica-plugins"]
			_, bound := entry["multicaBinding"]
			if bound != tc.bound || !strings.Contains(entry["url"].(string), tc.url) {
				t.Fatalf("%s", raw)
			}
			if _, ok := doc.Servers["runtime-only"]["multicaBinding"]; ok {
				t.Fatal("runtime field survived")
			}
		})
	}
	merged, e := mergeRuntimeAndAgentMcpConfig("openai-sandbox", user)
	if e != nil || strings.Contains(string(merged), "multicaBinding") {
		t.Fatalf("ordinary provider entry: %s %v", merged, e)
	}
	if _, e = stripOpenAISandboxBindings(json.RawMessage(`{"mcpServers":{"bad":"malformed"}}`)); e == nil {
		t.Fatal("malformed input accepted")
	}
}
