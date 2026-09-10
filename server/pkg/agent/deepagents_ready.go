package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const deepAgentsBridgeVersion = "1"

type deepAgentsReady struct {
	Schema       string `json:"schema"`
	Nonce        string `json:"nonce"`
	ConfigDigest string `json:"config_digest"`
	PolicyDigest string `json:"policy_digest"`
	PID          int    `json:"pid"`
	State        string `json:"state"`
	Code         string `json:"code"`
	LoaderCalls  int    `json:"loader_calls"`
	Degraded     int    `json:"degraded"`
}

type deepAgentsGate struct {
	expected         deepAgentsReady
	request, receipt string
}

func prepareDeepAgentsGate(mcp string, policy map[string]bool, toolPolicies ...map[string]map[string]json.RawMessage) (*deepAgentsGate, func(), error) {
	noop := func() {}
	raw := []byte{}
	if mcp != "" {
		var err error
		raw, err = os.ReadFile(mcp)
		if err != nil {
			return nil, noop, err
		}
	}
	var config struct {
		Servers map[string]struct {
			Disabled bool `json:"disabled"`
		} `json:"mcpServers"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &config); err != nil {
			return nil, noop, err
		}
	}
	effective := map[string]bool{}
	for name, entry := range config.Servers {
		required := !entry.Disabled
		if value, ok := policy[name]; ok {
			required = value
		}
		if required && entry.Disabled {
			return nil, noop, errors.New("deepagents: MCP_CONFIG_INVALID")
		}
		effective[name] = required
	}
	for name, required := range policy {
		if required {
			effective[name] = true
		}
	}
	var tools map[string]map[string]json.RawMessage
	if len(toolPolicies) > 0 {
		tools = toolPolicies[0]
	}
	policyRaw, _ := json.Marshal(struct {
		Required map[string]bool                       `json:"required"`
		Tools    map[string]map[string]json.RawMessage `json:"tools"`
	}{effective, tools})
	dir, err := os.MkdirTemp("", "multica-deepagents-ready-")
	if err != nil {
		return nil, noop, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	nonce := make([]byte, 32)
	if _, err = rand.Read(nonce); err != nil {
		cleanup()
		return nil, noop, err
	}
	digest := func(b []byte) string { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }
	g := &deepAgentsGate{expected: deepAgentsReady{Schema: deepAgentsBridgeVersion, Nonce: hex.EncodeToString(nonce), ConfigDigest: digest(raw), PolicyDigest: digest(policyRaw)}, request: filepath.Join(dir, "request.json"), receipt: filepath.Join(dir, "ready.json")}
	policyPath := filepath.Join(dir, "policy.json")
	if err = os.WriteFile(policyPath, policyRaw, 0600); err != nil {
		cleanup()
		return nil, noop, err
	}
	request := map[string]any{"schema": g.expected.Schema, "nonce": g.expected.Nonce, "config_digest": g.expected.ConfigDigest, "policy_digest": g.expected.PolicyDigest, "config": mcp, "policy": policyPath, "receipt": g.receipt}
	data, _ := json.Marshal(request)
	if err = os.WriteFile(g.request, data, 0600); err != nil {
		cleanup()
		return nil, noop, err
	}
	return g, cleanup, nil
}

func (g *deepAgentsGate) wait(ctx context.Context, readerDone <-chan struct{}, pid int) (int, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(g.receipt)
		if err == nil {
			var got deepAgentsReady
			if len(data) > 16384 || json.Unmarshal(data, &got) != nil || got.Schema != g.expected.Schema || got.Nonce != g.expected.Nonce || got.ConfigDigest != g.expected.ConfigDigest || got.PolicyDigest != g.expected.PolicyDigest || got.PID != pid {
				return 0, errors.New("BRIDGE_READY_INVALID")
			}
			if got.State != "READY" {
				switch got.Code {
				case "MCP_CONFIG_INVALID", "MCP_REQUIRED_UNREADY", "BRIDGE_VERSION", "DEPENDENCY_MISSING":
					return 0, errors.New(got.Code)
				}
				return 0, errors.New("BRIDGE_READY_INVALID")
			}
			if got.LoaderCalls != 1 || got.Degraded < 0 {
				return 0, errors.New("BRIDGE_READY_INVALID")
			}
			return got.Degraded, nil
		}
		if !os.IsNotExist(err) {
			return 0, errors.New("BRIDGE_READY_INVALID")
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-readerDone:
			return 0, errors.New("BRIDGE_READY_MISSING")
		case <-ticker.C:
		}
	}
}
