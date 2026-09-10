//go:build agentintegration

package daemon

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/pkg/remotemcp"
	"github.com/multica-ai/multica/server/pkg/remotemcp/remotemcptest"
)

// This is the real installed guarded loader and real broker/TLS transport, not
// credentialed cli_main/ACP/model execution. No agent loop is mocked or copied.
func TestDeepAgentsRealTLSBrokerLoader(t *testing.T) {
	if os.Getenv("MULTICA_RUN_REAL_AGENT_SMOKE") != "1" {
		t.Skip("explicit real smoke opt-in required")
	}
	python := os.Getenv("MULTICA_DEEPAGENTS_TEST_PYTHON")
	if !filepath.IsAbs(python) {
		t.Fatal("explicit isolated Python required")
	}
	repo, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	origin, e := exec.Command("git", "-C", repo, "remote", "get-url", "origin").Output()
	if e != nil || !strings.Contains(string(origin), "CM-BF/multica-runtime-lab") {
		t.Fatal("fork gate")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	fixture := remotemcptest.NewServer()
	defer fixture.Close()
	var initialized, listed, readCalls, writeCalls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var rpc struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.Unmarshal(body, &rpc)
		switch rpc.Method {
		case "initialize":
			initialized.Add(1)
		case "tools/list":
			listed.Add(1)
		case "tools/call":
			if rpc.Params.Name == "fixture.read" {
				readCalls.Add(1)
			} else {
				writeCalls.Add(1)
			}
		}
		fixture.Config.Handler.ServeHTTP(w, r)
	})
	// Generate a fresh private trust root per test; never disable certificate checks.
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if e != nil {
		t.Fatal(e)
	}
	cert := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "DA-6 private test CA"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, e := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	upstream := httptest.NewUnstartedServer(handler)
	upstream.Config.ErrorLog = log.New(io.Discard, "", 0)
	upstream.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	upstream.StartTLS()
	defer upstream.Close()
	ca := filepath.Join(root, "ca.pem")
	if e = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv(remotemcp.DevOriginsEnv, upstream.URL)
	t.Setenv(remotemcp.DevCAEnv, ca)
	headers, _ := fixtureCredential(ctx, "")
	discovered, _, e := remotemcp.Discover(ctx, upstream.URL, []string{"127.0.0.1"}, fixtureProtocolVersions, headers)
	if e != nil {
		t.Fatal(e)
	}
	var approved []remotemcp.Tool
	for _, tool := range discovered {
		if tool.Name == "fixture.read" {
			approved = append(approved, tool)
		}
	}
	if len(discovered) != 2 || len(approved) != 1 {
		t.Fatal("fixture discovery contract")
	}
	connection := startupConnection(fixture, approved)
	connection.Endpoint = upstream.URL
	t.Setenv(remotemcp.DevCAEnv, "")
	_, _, bad, e := startTaskRemoteMCPBrokers(ctx, ctx, "tls-loader", "deepagents", []remotemcp.Connection{connection}, fixtureCredential, nil)
	if bad != nil {
		bad.Close()
	}
	if e == nil || !strings.Contains(e.Error(), "certificate") {
		t.Fatalf("expected certificate rejection, got %v", e)
	}
	if readCalls.Load() != 0 || writeCalls.Load() != 0 {
		t.Fatal("untrusted startup invoked a tool")
	}
	t.Log("PASS: missing private CA rejects broker startup with certificate verification error; loader not launched")
	t.Setenv(remotemcp.DevCAEnv, ca)
	raw, diagnostics, brokers, e := startTaskRemoteMCPBrokers(ctx, ctx, "tls-loader", "deepagents", []remotemcp.Connection{connection}, fixtureCredential, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer brokers.Close()
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	var config struct {
		Servers map[string]any `json:"mcpServers"`
	}
	if e = json.Unmarshal(raw, &config); e != nil || len(config.Servers) != 1 {
		t.Fatal("broker config", e)
	}
	required := map[string]bool{}
	for name := range config.Servers {
		required[name] = true
	}
	policy, _ := json.Marshal(map[string]any{"required": required, "tools": map[string]any{}})
	configPath, policyPath, requestPath := filepath.Join(root, "mcp.json"), filepath.Join(root, "policy.json"), filepath.Join(root, "request.json")
	request, _ := json.Marshal(map[string]any{"schema": "1", "nonce": "da6-isolated-loader", "config": configPath, "policy": policyPath, "receipt": filepath.Join(root, "ready.json"), "config_digest": fmt.Sprintf("%x", sha256.Sum256(raw)), "policy_digest": fmt.Sprintf("%x", sha256.Sum256(policy))})
	for file, data := range map[string][]byte{configPath: raw, policyPath: policy, requestPath: request} {
		if e = os.WriteFile(file, data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	// Copy only our wrapper and test driver; installed SDK dependencies are read-only.
	module := filepath.Join(root, "multica_dcode_acp")
	if e = os.Mkdir(module, 0700); e != nil {
		t.Fatal(e)
	}
	for source, destination := range map[string]string{"multica_dcode_acp/__init__.py": filepath.Join(module, "__init__.py"), "tests/tls_loader.py": filepath.Join(root, "loader.py")} {
		data, err := os.ReadFile(filepath.Join(repo, "runtime-bridges/deepagents", source))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(destination, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{"PATH=" + filepath.Dir(python) + ":/usr/bin:/bin", "HOME=" + root, "USERPROFILE=" + root, "TMPDIR=" + root, "XDG_CONFIG_HOME=" + root, "XDG_CACHE_HOME=" + root, "XDG_DATA_HOME=" + root, "DEEPAGENTS_HOME=" + filepath.Join(root, "profile"), "PYTHONDONTWRITEBYTECODE=1", "DO_NOT_TRACK=1", "LANGSMITH_TRACING=false", "MULTICA_DEEPAGENTS_REQUEST=" + requestPath}
	entry := exec.CommandContext(ctx, filepath.Join(filepath.Dir(python), "multica-dcode-acp"), "--version")
	entry.Dir = root
	entry.Env = env
	version, err := entry.CombinedOutput()
	if err != nil {
		t.Fatalf("installed entry version: %v %s", err, version)
	}
	t.Log(strings.TrimSpace(string(version)))
	cmd := exec.CommandContext(ctx, python, filepath.Join(root, "loader.py"), configPath)
	cmd.Dir = root
	cmd.Env = env
	cmd.WaitDelay = 2 * time.Second
	output, err := cmd.CombinedOutput()
	t.Log(strings.TrimSpace(string(output)))
	if err != nil {
		t.Fatalf("official guarded loader exit: %v", err)
	}
	if initialized.Load() < 3 || listed.Load() < 3 || readCalls.Load() != 1 || writeCalls.Load() != 0 || len(fixture.Writes()) != 0 {
		t.Fatalf("TLS upstream initialize=%d list=%d read=%d forbidden=%d writes=%v", initialized.Load(), listed.Load(), readCalls.Load(), writeCalls.Load(), fixture.Writes())
	}
	t.Logf("PASS: real TLS upstream initialize=%d tools/list=%d allowed calls=%d forbidden calls=%d; no model credentials/agent loop", initialized.Load(), listed.Load(), readCalls.Load(), writeCalls.Load())
}
