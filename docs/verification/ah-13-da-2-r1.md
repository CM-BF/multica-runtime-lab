# AH-13 / DA-2 R1 — delivery and cancellation fixes

**PASS for the two requested blocking fixes and their regression tests. Overall
DA-2 remains BLOCKED for the previously recorded MCP readiness/model E2E gaps.**
Default platform MCP/global user MCP mapping is explicitly unsupported; the
negative production-path evidence below replaces any implied support in comments.

Base: `7d6ef1d5af21fc7777083d5937cb55a11524054e`. Same exclusive checkout and branch
`feat/ah-13-da-2-deepagents`, origin `CM-BF/multica-runtime-lab`. No delegation,
branch switch, service/CLI/daemon/runtime configuration changes, existing DB or
existing port access. `.da2-tools/` stays ignored and is excluded from the commit.

## Fixes

`server/pkg/agent/deepagents.go` now uses a bounded 256-entry Messages channel and
cancellable send backpressure, with a two-second stalled-send deadline independent
of the execution timeout. There is no default-drop case. Slow consumers retain
text and tool start/completion events. A consumer that never drains the channel
causes an explicit failed Result rather than silent successful truncation;
explicit cancellation returns aborted with an interrupted-message indication.
Already buffered messages remain readable after Result. Successful callers must
drain Messages; unlimited retention for an absent consumer is not promised.

All potentially blocking setup requests run in tracked workers. Prompt and cancel
writes are also tracked. The cancellation timer starts before the cancel writer
is launched; the session owner does not synchronously wait on the write mutex or
join a stuck prompt before closing the pipe. Cleanup cancels delivery, closes
stdin/stdout, kills/waits for the owned process tree and joins the reader and all
RPC workers before closing Messages and returning Result. This also covers setup
writes whose context cannot interrupt the underlying synchronous pipe Write.

Stderr recognition retains at most a 4096-byte tail and outputs only fixed hints
for missing credentials or missing Python dependencies. Raw stderr and exception
values are never surfaced. Both hints have subprocess tests with a secret canary
that must not appear in Result.Error.

## Reproduction and complete test results

Host: Darwin arm64. Effective Go toolchain: **go1.26.6** (`server/go.mod`); test
fixtures use Python 3.13.3. `GO` below is the existing fork-local
`../.da2-tools/go/bin/go`, run from `server/`. No installed agent CLI is executed.

Before the implementation fix, the new regression command exited **1**:

```sh
GO test ./pkg/agent \
  -run '^TestDeepAgents(SlowConsumerLossless|UnconsumedMessages|CancelNoReadStdin)$' \
  -count=1 -v -timeout 20s
```

Observed old defects: `tool-0085 starts=1 ends=0`; an unconsumed stream returned
`completed`; and cancellation timed out after 50ms + 2s with stdin holding 65,536
unread bytes while an 8 MiB prompt was being written. The test's cleanup killed
only its recorded fixture processes, so the negative run did not orphan work.

The first post-fix run exposed a test timing assumption: cancelling after 100ms
sometimes happened during Python initialization, before the message queue filled.
The test now waits for the actual 256-entry queue occupancy before cancelling.
The corrected focused race run passed, exit **0** (5.525s).

Final broader verification, both commands exited **0**:

```sh
GO test -race ./pkg/agent \
  -run 'DeepAgents|ACP|SupportedTypes|OnlyLaunchGo' \
  -count=1 -v -timeout 120s
GO test -race ./internal/daemon/execenv ./internal/daemon \
  -run DeepAgents -count=1 -v -timeout 60s
```

Complete stdout/stderr is delivered as issue attachments, not truncated excerpts:

| Attachment | Lines | SHA256 | Final result |
| --- | --- | --- | --- |
| `da2-r1-agent-full.txt` | 1487 | `e84db5086b37a8caabe078af80870074ead822915fbdf6364232db7ea835610e` | PASS; pkg/agent 16.592s |
| `da2-r1-daemon-full.txt` | 15 | `ea8f87b3af2661bf800b5f2b137ffec0fc4fb2ccc95f0a9656ddf98f46523702` | PASS; execenv 1.294s, daemon 1.379s |
| `da2-r1-before-fix.txt` | negative run | `3875eda59fe9d027682ef2393f09f17c81d2e3b186fbdae22d145eb5a9937c98` | Expected FAIL reproducing old defects |

Final assertions/results:

- `TestDeepAgentsSlowConsumerLossless`: queue fills before consumption; consumer
  sleeps 1ms per message. All **401 text chunks, 400 tool starts and 400 tool
  completions** arrive exactly once. Concatenated streamed text matches expected
  bytes; every completion carries its expected result text; final Output is
  CURRENT and Result is completed. 1,201 semantic events exceed the old buffer.
- `TestDeepAgentsUnconsumedMessages`: no execution timeout, no Messages consumer.
  Stalled delivery returns failed explicitly; the cancellation variant returns
  aborted explicitly. Both preserve all 256 buffered events and Result is reached
  without draining Messages first.
- `TestDeepAgentsCancelNoReadStdin`: the child completes session/new, spawns its
  own child, then **never reads stdin again**. FIONREAD confirms **65,536 unread
  bytes** while an **8 MiB prompt** is in flight. With TurnInterruptTimeout=50ms,
  final run returned aborted after **51.942334ms**, within 50ms + 2s cleanup budget.
  Recorded parent and child PIDs are verified gone; workers are joined before
  Result. This is not the old fixture that merely declines to answer cancel.
- Existing cooperative/unresponsive cancellation, ACP compatibility, resume,
  terminal, factory, private MCP and race tests also pass in the complete log.
- `TestDeepAgentsSafeStartupDiagnostics`: actionable credential/dependency hints;
  the injected private-value canary is absent from Result.Error.
- `git diff --check`: exit 0. No frontend code changed in R1; no frontend rerun.

One final version lookup used a server-relative tool path from the root and
exited 127; rerunning it from server/ returned go1.26.6. This did not affect tests.
No real model/SDK smoke was rerun; the original DA-2 report remains the record of
those earlier checks, including the credential-free dcode startup failure.

## MCP: actual code path and supported boundary

The adapter consumes **only `opts.McpConfig` canonical explicit stdio MCP** at
`server/pkg/agent/deepagents.go:101` (`prepareDeepAgentsMCP`). It does not provide a
new platform/default/global-user-MCP mapping. Proof is in code and tests:

1. `server/internal/daemon/daemon.go:7513` takes `task.Agent.McpConfig`; :7515
   calls `mergeRuntimeAndAgentMcpConfig`; :7524 merges any task broker config;
   :8229 selects the effective config; :8277 assigns `ExecOptions.McpConfig`.
2. `server/internal/daemon/runtime_mcp.go:40` preserves explicit agent bytes when
   runtime import is unsupported. `loadRuntimeMcpServerConfigs` at :266 has no
   Deep Agents importer. A nil agent config remains nil: there is no implicit
   default MCP server synthesized for this family.
3. `server/internal/daemon/remote_mcp_broker.go:55` applies
   `providerSupportsRemoteMCPBroker` (:159). Deep Agents is absent. A required
   connection fails before credential resolution/network discovery; optional
   connections are degraded by the existing policy. **Remote MCP unsupported.**
4. `server/internal/daemon/plugin_hook_mcp.go:74` is the actual platform plugin
   config generator. It emits `multica-plugins` with `type: http`. The new test
   starts it on its own `127.0.0.1:0` listener, merges its real config with explicit
   stdio via `mergeTaskRemoteMCPConfig`, and calls the Deep Agents backend. The
   adapter rejects this before process startup. The listener is closed/cancelled;
   no external plugin invocation occurs. **Platform plugin MCP unsupported.**

`TestDeepAgentsMCPConfigurationBoundary` in
`server/internal/daemon/deepagents_test.go:49` verifies all these boundaries:
explicit bytes unchanged, nil remains nil, no ambient `.deepagents/mcp.json`
import, required broker rejection, and rejection of actual generated plugin HTTP
configuration. Existing `TestDeepAgentsWireAndPrivateMCP` proves explicit stdio
bytes reach the 0600 private file passed to dcode. These are linked configuration
boundary tests, not a claim of a live full-daemon/model/platform MCP E2E.

The task's AGENTS/CLI brief is separate from MCP. No claim is made that preparing
that brief proves the model consumed it or that default platform tools work.
The installation guide now states these limitations prominently.

## Remaining scope

- **BLOCKED:** no provider credentials, so prebuilt dcode model E2E remains unrun.
- **BLOCKED:** dcode can expose failed MCP connections only as metadata; no
  authoritative managed-MCP readiness contract has been added in this fix.
- **Unsupported:** default platform/Remote/plugin MCP and automatic global user
  MCP mapping; this revision proves the negative boundary rather than adding
  HTTP/SSE support.
- No database migration execution, live platform/picker/attachment E2E, full-repo
  checks or OpenAI implementation. Parent issue status is unchanged.

The same fork branch and checkout are retained. This R1 commit contains the
requested two fixes and evidence; it does not mark overall DA-2 complete or
start another phase.
