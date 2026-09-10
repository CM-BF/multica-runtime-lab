package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// deepagentsBackend delegates the agent loop to the official dcode ACP entry.
type deepagentsBackend struct{ cfg Config }

func validateDeepAgentsArgs(args []string) error {
	for _, arg := range args {
		flag := strings.SplitN(unshellQuoteArg(arg), "=", 2)[0]
		if len(flag) > 2 && flag[0] == '-' && strings.ContainsRune("Mramnqshp", rune(flag[1])) {
			return errors.New("deepagents: attached short arguments are unsupported")
		}
		switch flag {
		case "-M", "-a", "-n", "-q", "-s", "--message", "--skill", "--startup-cmd", "--default-model", "--clear-default-model", "--profile-override", "--stdin", "--json", "--quiet", "--no-stream", "--goal", "--rubric", "--max-turns", "--timeout", "--sandbox", "--", "--acp", "--model", "-m", "--mcp-config", "--no-mcp", "--resume", "-r", "--profile", "-p", "--agent", "--headless", "--non-interactive", "--shell-allow-list", "--trust-project", "--help", "-h", "--version":
			return fmt.Errorf("deepagents: adapter-owned or unsupported argument %s", flag)
		}
	}
	return nil
}

// Only explicit stdio entries are supported until remote transports are tested.
func prepareDeepAgentsMCP(raw json.RawMessage) (string, func(), error) {
	noop := func() {}
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return "", noop, nil
	}
	var config struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(raw, &config) != nil || config.Servers == nil {
		return "", noop, errors.New("deepagents: expected mcpServers object")
	}
	for _, entry := range config.Servers {
		var s struct {
			Command string            `json:"command"`
			Type    string            `json:"type"`
			URL     string            `json:"url"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		}
		if json.Unmarshal(entry, &s) != nil || strings.TrimSpace(s.Command) == "" || s.URL != "" || (s.Type != "" && s.Type != "stdio") {
			return "", noop, errors.New("deepagents: invalid or unsupported MCP entry; explicit stdio command required")
		}
	}
	if len(config.Servers) == 0 {
		return "", noop, nil
	}
	dir, err := os.MkdirTemp("", "multica-deepagents-mcp-")
	if err != nil {
		return "", noop, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "mcp.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		cleanup()
		return "", noop, err
	}
	return path, cleanup, nil
}

func (b *deepagentsBackend) Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error) {
	if opts.ThinkingLevel != "" || opts.ServiceTier != "" || opts.MaxTurns != 0 {
		return nil, errors.New("deepagents: thinking level, service tier and max turns are unsupported")
	}
	for _, args := range [][]string{b.cfg.LaunchPrefix, opts.ExtraArgs, opts.CustomArgs} {
		if err := validateDeepAgentsArgs(args); err != nil {
			return nil, err
		}
	}
	cwd, err := filepath.Abs(opts.Cwd)
	if err != nil {
		return nil, err
	}
	state := b.cfg.Env["DEEPAGENTS_HOME"]
	if !filepath.IsAbs(state) {
		return nil, errors.New("deepagents: absolute private DEEPAGENTS_HOME is required")
	}
	if err = os.MkdirAll(state, 0700); err != nil {
		return nil, err
	}
	mcp, cleanMCP, err := prepareDeepAgentsMCP(opts.McpConfig)
	if err != nil {
		return nil, err
	}
	args := append([]string{}, opts.ExtraArgs...)
	args = append(args, opts.CustomArgs...)
	args = append(args, "--acp")
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if mcp != "" {
		args = append(args, "--mcp-config", mcp)
	}
	path := b.cfg.ExecutablePath
	if path == "" {
		path = "dcode"
	}
	// The execution context controls RPCs, not pipes: cancellation must reach ACP
	// before the independent, bounded process-tree cleanup kills the transport.
	processCtx, stopProcess := context.WithCancel(context.Background())
	cmd := b.cfg.commandAt(path).exec(processCtx, args...)
	cmd.Dir = cwd
	cmd.Env = buildEnv(b.cfg.Env)
	cmd.WaitDelay = 2 * time.Second
	cmd.Stderr = io.Discard // Never expose provider config or credentials from stderr.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		stopProcess()
		cleanMCP()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		stopProcess()
		cleanMCP()
		return nil, err
	}
	if err = startOwnedProcessTree(cmd, b.cfg.Logger); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		stopProcess()
		cleanMCP()
		return nil, err
	}
	b.cfg.logAgentCommand(cmd, newAgentCommandLogArgs(args))
	messages := make(chan Message, 256)
	results := make(chan Result, 1)
	var terminal, active atomic.Bool
	runCtx, cancel := runContext(ctx, opts.Timeout)
	c := &hermesClient{cfg: b.cfg, stdin: stdin, pending: make(map[int]*pendingRPC)}
	var output acpDeliverableTracker
	c.acceptNotification = func(string) bool { return active.Load() }
	c.onMessage = func(m Message) {
		output.observe(m)
		select {
		case messages <- m:
		default:
		}
	}
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		scanner := newAgentStreamScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if !json.Valid([]byte(line)) {
				c.closeAllPending(errors.New("deepagents: malformed ACP stdout"))
				return
			}
			c.handleLine(line)
		}
		c.closeAllPending(io.EOF)
	}()
	go func() {
		started := time.Now()
		r := Result{Status: "failed", SessionID: opts.ResumeSessionID}
		defer func() {
			active.Store(false)
			_ = stdin.Close()
			signalProcessGroup(cmd, syscall.SIGKILL)
			stopProcess()
			_ = stdout.Close()
			_ = cmd.Wait()
			releaseProcessGroup(cmd)
			<-readerDone
			cleanMCP()
			cancel()
			r.Output, _ = output.result()
			r.DurationMs = time.Since(started).Milliseconds()
			close(messages)
			results <- r
			close(results)
		}()
		fail := func(stage string, e error) {
			r.Error = "deepagents: " + stage + " failed"
			if errors.Is(e, context.DeadlineExceeded) || errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				r.Status = "timeout"
			} else if errors.Is(e, context.Canceled) || errors.Is(runCtx.Err(), context.Canceled) {
				r.Status = "aborted"
			}
		}
		handshake := opts.HandshakeTimeout
		if handshake <= 0 {
			handshake = 40 * time.Second
		}
		setupCtx, stopSetup := context.WithTimeout(runCtx, handshake)
		defer stopSetup()
		init, e := c.request(setupCtx, "initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}, "clientInfo": map[string]string{"name": "multica", "version": "1"}})
		if e != nil {
			fail("initialize", e)
			return
		}
		var capabilities struct {
			Version int `json:"protocolVersion"`
			Agent   struct {
				Load bool `json:"loadSession"`
			} `json:"agentCapabilities"`
		}
		if json.Unmarshal(init, &capabilities) != nil || capabilities.Version != 1 {
			r.Error = "deepagents: incompatible ACP protocol"
			return
		}
		method := "session/new"
		params := map[string]any{"cwd": cwd, "mcpServers": []any{}}
		if opts.ResumeSessionID != "" {
			if !capabilities.Agent.Load {
				r.Error = "deepagents: runtime does not advertise session/load"
				return
			}
			method = "session/load"
			params["sessionId"] = opts.ResumeSessionID
		}
		session, e := c.request(setupCtx, method, params)
		if e != nil {
			fail(method, e)
			r.ResumeRejected = deepAgentsMissingSession(e, opts.ResumeSessionID)
			return
		}
		var info struct {
			ID      string `json:"sessionId"`
			Options []struct {
				ID       string `json:"id"`
				Category string `json:"category"`
				Value    string `json:"currentValue"`
			} `json:"configOptions"`
		}
		if json.Unmarshal(session, &info) != nil {
			r.Error = "deepagents: invalid session response"
			return
		}
		id := info.ID
		if method == "session/load" {
			id = opts.ResumeSessionID
		}
		if id == "" {
			r.Error = "deepagents: missing session ID"
			return
		}
		if method == "session/load" && opts.Model != "" {
			found := false
			for _, option := range info.Options {
				if option.Category == "model" {
					found = true
					if option.Value != opts.Model {
						_, e = c.request(setupCtx, "session/set_config_option", map[string]any{"sessionId": id, "configId": option.ID, "value": opts.Model})
						if e != nil {
							fail("model selection", e)
							return
						}
					}
					break
				}
			}
			if !found {
				r.Error = "deepagents: restored session has no model selector"
				return
			}
		}
		stopSetup()
		r.SessionID = id
		c.sessionID = id
		active.Store(true)
		c.onMessage(Message{Type: MessageStatus, Status: "running", SessionID: id})
		if opts.SystemPrompt != "" {
			prompt = opts.SystemPrompt + "\n\n" + prompt
		}
		if opts.ResumeExpected && opts.ResumeSessionID == "" && opts.ResumeContinuityNotice != "" {
			prompt = opts.ResumeContinuityNotice + "\n\n" + prompt
		}
		// Keep the prompt request pending during graceful cancellation.
		promptCtx, stopPrompt := context.WithCancel(context.Background())
		defer stopPrompt()
		done := make(chan rpcResult, 1)
		go func() {
			value, e := c.request(promptCtx, "session/prompt", map[string]any{"sessionId": id, "prompt": []any{map[string]any{"type": "text", "text": prompt}}})
			done <- rpcResult{result: value, err: e}
		}()
		var reply rpcResult
		select {
		case reply = <-done:
		case <-readerDone:
			stopPrompt()
			<-done
			fail("ACP transport closed", io.EOF)
			return
		case <-runCtx.Done():
			data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]any{"sessionId": id}})
			_ = c.writeLine(append(data, '\n'))
			grace := opts.TurnInterruptTimeout
			if grace <= 0 {
				grace = 2 * time.Second
			}
			timer := time.NewTimer(grace)
			select {
			case <-done:
			case <-timer.C:
				stopPrompt()
				<-done
			}
			timer.Stop()
			fail("cancelled", runCtx.Err())
			return
		}
		if reply.err != nil {
			fail("session/prompt", reply.err)
			return
		}
		var end struct {
			Reason string `json:"stopReason"`
		}
		if json.Unmarshal(reply.result, &end) != nil {
			r.Error = "deepagents: invalid prompt response"
			return
		}
		terminal.Store(true)
		// Bound the final pipe drain for runtimes that flush trailing updates after the response.
		time.Sleep(25 * time.Millisecond)
		switch end.Reason {
		case "end_turn":
			r.Status = "completed"
		case "cancelled":
			r.Status = "aborted"
		default:
			r.Error = "deepagents: non-completion stopReason " + end.Reason
		}
	}()
	return &Session{Messages: messages, Result: results, TerminalObserved: terminal.Load}, nil
}

func deepAgentsMissingSession(err error, requested string) bool {
	var rpc *acpRPCError
	if !errors.As(err, &rpc) || rpc.Method != "session/load" || rpc.Code != -32002 || rpc.Message != "Resource not found" {
		return false
	}
	var data struct {
		URI string `json:"uri"`
	}
	return json.Unmarshal([]byte(rpc.Data), &data) == nil && requested != "" && data.URI == requested
}
