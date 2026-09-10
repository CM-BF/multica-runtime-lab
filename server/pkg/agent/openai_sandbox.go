package agent

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type openAISandboxBackend struct{ cfg Config }
type sandboxFrame struct {
	Type      string `json:"type"`
	Protocol  int    `json:"protocol"`
	SDK       string `json:"sdk"`
	RequestID string `json:"requestId"`
	Seq       int    `json:"seq"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	SessionID string `json:"sessionId"`
	Output    string `json:"output"`
	Error     string `json:"error"`
}

func sandboxPaths(raw string, defaults []string) ([]string, error) {
	if raw == "" {
		return defaults, nil
	}
	var paths []string
	if json.Unmarshal([]byte(raw), &paths) != nil {
		return nil, errors.New("openai-sandbox: INVALID_PATHS")
	}
	for _, p := range paths {
		if p == "" || filepath.IsAbs(p) || strings.Contains(p, "\\") {
			return nil, errors.New("openai-sandbox: INVALID_PATHS")
		}
		for _, part := range strings.Split(p, "/") {
			if part == ".." || part == "." || part == "" {
				return nil, errors.New("openai-sandbox: INVALID_PATHS")
			}
		}
	}
	return paths, nil
}
func (b *openAISandboxBackend) Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error) {
	if len(opts.ExtraArgs) > 0 || len(opts.CustomArgs) > 0 || opts.ThinkingLevel != "" || opts.ServiceTier != "" {
		return nil, errors.New("openai-sandbox: UNSUPPORTED_OPTION")
	}
	for _, arg := range b.cfg.LaunchPrefix {
		if strings.HasPrefix(arg, "-") {
			return nil, errors.New("openai-sandbox: INVALID_LAUNCH_PREFIX")
		}
	}
	cwd, err := filepath.Abs(opts.Cwd)
	if err != nil {
		return nil, err
	}
	state := b.cfg.Env["OPENAI_SANDBOX_STATE"]
	if !filepath.IsAbs(state) {
		return nil, errors.New("openai-sandbox: private absolute OPENAI_SANDBOX_STATE required")
	}
	if err = os.MkdirAll(state, 0700); err != nil {
		return nil, err
	}
	turn, err := os.MkdirTemp(state, "turn-")
	if err != nil {
		return nil, err
	}
	cleanup := func() { _ = os.RemoveAll(turn) }
	inputs, err := sandboxPaths(b.cfg.Env["OPENAI_SANDBOX_INPUTS"], []string{"AGENTS.md"})
	if err != nil {
		cleanup()
		return nil, err
	}
	artifacts, err := sandboxPaths(b.cfg.Env["OPENAI_SANDBOX_ARTIFACTS"], []string{"artifacts"})
	if err != nil {
		cleanup()
		return nil, err
	}
	requestIDBytes := make([]byte, 16)
	if _, err = rand.Read(requestIDBytes); err != nil {
		cleanup()
		return nil, err
	}
	requestID := hex.EncodeToString(requestIDBytes)
	request := map[string]any{"type": "execute", "requestId": requestID, "prompt": prompt, "instructions": opts.SystemPrompt, "model": opts.Model, "cwd": cwd, "stateRoot": state, "sessionId": opts.ResumeSessionID, "maxTurns": opts.MaxTurns, "inputs": inputs, "artifacts": artifacts}
	if len(opts.McpConfig) > 0 {
		if !json.Valid(opts.McpConfig) {
			cleanup()
			return nil, errors.New("openai-sandbox: MCP_CONFIG")
		}
		request["mcp"] = opts.McpConfig
	}
	raw, err := json.Marshal(request)
	if err != nil || len(raw) > 16*1024*1024 {
		cleanup()
		return nil, errors.New("openai-sandbox: REQUEST_LIMIT")
	}
	executable := b.cfg.ExecutablePath
	if executable == "" {
		executable = "multica-openai-sandbox"
	}
	procCtx, stopProc := context.WithCancel(context.Background())
	cmd := b.cfg.commandAt(executable).exec(procCtx)
	cmd.Dir = cwd
	cmd.WaitDelay = 2 * time.Second
	cmd.Stderr = io.Discard
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + turn, "USERPROFILE=" + turn, "TMPDIR=" + turn, "XDG_CONFIG_HOME=" + turn, "XDG_CACHE_HOME=" + turn, "XDG_DATA_HOME=" + turn, "OPENAI_AGENTS_SANDBOX_SNAPSHOT_DIR=" + turn, "OPENAI_AGENTS_DISABLE_TRACING=1"}
	// Only caller-supplied model credentials; never inherit daemon account variables.
	for _, key := range []string{"OPENAI_API_KEY"} {
		if value := b.cfg.Env[key]; value != "" {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		stopProc()
		cleanup()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		stopProc()
		cleanup()
		return nil, err
	}
	if err = startOwnedProcessTree(cmd, b.cfg.Logger); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		stopProc()
		cleanup()
		return nil, err
	}
	var observed atomic.Bool
	messages := make(chan Message, 256)
	results := make(chan Result, 1)
	runCtx, cancel := runContext(ctx, opts.Timeout)
	readCtx, stopRead := context.WithCancel(context.Background())
	frames := make(chan sandboxFrame, 64)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		defer close(frames)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 2*1024*1024)
		for scanner.Scan() {
			var frame sandboxFrame
			if json.Unmarshal(scanner.Bytes(), &frame) != nil {
				frame.Type = "invalid"
			}
			select {
			case frames <- frame:
			case <-readCtx.Done():
				return
			}
		}
		if scanner.Err() != nil {
			select {
			case frames <- sandboxFrame{Type: "invalid"}:
			case <-readCtx.Done():
			}
		}
	}()
	var writers sync.WaitGroup
	var writeMu sync.Mutex
	write := func(bytes []byte) {
		writers.Add(1)
		go func() {
			defer writers.Done()
			writeMu.Lock()
			defer writeMu.Unlock()
			_, _ = stdin.Write(append(bytes, '\n'))
		}()
	}
	go func() {
		start := time.Now()
		r := Result{Status: "failed", Error: "openai-sandbox: EARLY_EOF", SessionID: opts.ResumeSessionID}
		ready, terminal, eof := false, false, false
		seq := 0
		defer func() {
			stopRead()
			_ = stdin.Close()
			if !eof {
				signalProcessGroup(cmd, syscall.SIGKILL)
				stopProc()
			}
			wait := make(chan error, 1)
			go func() { wait <- cmd.Wait() }()
			select {
			case err := <-wait:
				if err != nil && r.Status == "completed" {
					r.Status = "failed"
					r.Error = "openai-sandbox: PROCESS_EXIT"
				}
			case <-time.After(2 * time.Second):
				signalProcessGroup(cmd, syscall.SIGKILL)
				stopProc()
				<-wait
				if r.Status == "completed" {
					r.Status = "failed"
					r.Error = "openai-sandbox: CLEANUP_TIMEOUT"
				}
			}
			signalProcessGroup(cmd, syscall.SIGKILL)
			stopProc()
			releaseProcessGroup(cmd)
			_ = stdout.Close()
			<-readDone
			writers.Wait()
			cleanup()
			cancel()
			r.DurationMs = time.Since(start).Milliseconds()
			close(messages)
			results <- r
			close(results)
		}()
		handshake := opts.HandshakeTimeout
		if handshake <= 0 {
			handshake = 20 * time.Second
		}
		timer := time.NewTimer(handshake)
		defer timer.Stop()
		interrupted := false
		var grace <-chan time.Time
		var graceTimer *time.Timer
		defer func() {
			if graceTimer != nil {
				graceTimer.Stop()
			}
		}()
		runDone := runCtx.Done()
		for {
			select {
			case <-runDone:
				interrupted = true
				runDone = nil
				r.Status = "aborted"
				r.Error = "openai-sandbox: CANCELLED"
				if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
					r.Status = "timeout"
					r.Error = "openai-sandbox: TIMEOUT"
				}
				timeout := opts.TurnInterruptTimeout
				if timeout <= 0 {
					timeout = 500 * time.Millisecond
				}
				graceTimer = time.NewTimer(timeout)
				grace = graceTimer.C
				line, _ := json.Marshal(map[string]any{"type": "cancel", "requestId": requestID})
				write(line)
			case <-grace:
				return
			case <-timer.C:
				if terminal {
					r.Status = "failed"
					r.Error = "openai-sandbox: CLEANUP_TIMEOUT"
					return
				}
				r.Error = "openai-sandbox: HANDSHAKE_TIMEOUT"
				return
			case frame, ok := <-frames:
				if !ok {
					eof = true
					return
				}
				if !ready {
					if frame.Type != "ready" || frame.Protocol != 1 || frame.SDK != "0.17.2" {
						r.Error = "openai-sandbox: BRIDGE_VERSION"
						return
					}
					ready = true
					timer.Stop()
					write(raw)
					continue
				}
				if frame.RequestID != requestID || frame.Seq != seq+1 || terminal {
					r.Status = "failed"
					r.Error = "openai-sandbox: INVALID_PROTOCOL"
					return
				}
				seq = frame.Seq
				if frame.SessionID != "" {
					r.SessionID = frame.SessionID
				}
				if frame.Type == "result" {
					terminal = true
					observed.Store(true)
					timer.Reset(2 * time.Second)
					if !interrupted {
						switch frame.Status {
						case "completed", "failed", "aborted":
							r.Status = frame.Status
						default:
							r.Error = "openai-sandbox: INVALID_TERMINAL"
							return
						}
						r.Output = frame.Output
						if frame.Error != "" {
							r.Error = "openai-sandbox: " + sandboxErrorCode(frame.Error)
						} else {
							r.Error = ""
						}
					}
					continue
				}
				if frame.Type != "event" {
					r.Error = "openai-sandbox: INVALID_PROTOCOL"
					return
				}
				var message Message
				switch frame.Kind {
				case "text":
					message = Message{Type: MessageText, Content: frame.Text}
				case "tool_use":
					message = Message{Type: MessageToolUse, Tool: frame.Name, CallID: frame.ID}
				case "tool_result":
					message = Message{Type: MessageToolResult, Tool: frame.Name, CallID: frame.ID, Output: frame.Text}
				case "status":
					message = Message{Type: MessageStatus, Status: frame.Status, SessionID: frame.SessionID}
				default:
					r.Error = "openai-sandbox: INVALID_EVENT"
					return
				}
				if interrupted {
					continue
				}
				stall := time.NewTimer(2 * time.Second)
				select {
				case messages <- message:
					stall.Stop()
				case <-runCtx.Done():
					stall.Stop()
				case <-stall.C:
					r.Error = "openai-sandbox: MESSAGE_CONSUMER_STALLED"
					return
				}
			}
		}
	}()
	return &Session{Messages: messages, Result: results, TerminalObserved: observed.Load}, nil
}
func sandboxErrorCode(code string) string {
	switch code {
	case "ARTIFACT_TYPE_CHANGE", "MCP_POLICY_CHANGED", "INVALID_REQUEST", "INVALID_SESSION", "STATE_BUSY", "DIRTY_SESSION", "CHECKPOINT_INVALID", "ARTIFACT_CONFLICT", "UNSAFE_PATH", "FILE_LIMIT", "MCP_CONFIG", "MCP_UNREADY", "CANCELLED", "MODEL_CREDENTIALS", "MODEL_RATE_LIMIT", "MAX_TURNS", "APPROVAL_REQUIRED", "STATE_SECRET", "SDK_FAILURE", "BRIDGE_FAILURE":
		return code
	default:
		return "SDK_FAILURE"
	}
}
