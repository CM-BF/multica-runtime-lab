# AH-13 / DA-2 R2 — plugin hook streamable-HTTP

PASS for the requested plugin hook transport and real loader/handler round trip.
Overall DA-2 remains BLOCKED for credentialed dcode model E2E and the separately
recorded required-MCP readiness/fail-closed gap; this is not full-stage acceptance.

Base: `56c1f2465c723290bf403d74942c469ffe9ade80`, continuing the same exclusive
`feat/ah-13-da-2-deepagents` fork branch. No delegation or OpenAI work. No existing
service restarted or database/port intentionally reused. See the full-suite
isolation failure below; that run did inherit the runtime configuration root. `.da2-tools/` excluded.

## Changes and supported boundary

- `server/pkg/agent/deepagents.go`: accept canonical `type:"http"` plus HTTP(S)
  URL and optional string headers, preserving exact config in a 0600 temporary
  file. Reject embedded URL credentials, missing hosts, SSE, and mixed stdio/HTTP
  fields. Existing stdio support and cleanup remain.
- Agent unit tests cover mixed stdio/HTTP, unchanged header/config bytes, private
  permissions, cleanup, malformed URLs/headers and conflicting fields.
- Daemon boundary test starts the production plugin generator, merges its config
  with agent stdio, and verifies adapter validation reaches executable lookup.
- New double-gated `TestDeepAgentsRealPluginMCP` starts the production generator
  on its own loopback ephemeral listener. The adapter passes its actual private
  file to a test ACP harness using the installed dcode loader. Loader discovery
  finds `platform_echo`; actual HTTP `tools/call` traverses the production daemon
  handler and reaches the local callback with exact task, installation, hook and
  input. Returned canary reaches a completed adapter Result. The harness cleans
  the real MCP manager before completing. This is not a fake MCP server/loader,
  and is not a credentialed dcode agent/model turn.
- `docs/deepagents-runtime.md` documents installation/use and the supported path;
  initial/R1 reports identify their older plugin rejection as superseded.

Remote broker support is deliberately not enabled. Inspection of
`startTaskRemoteMCPBrokers` confirms the provider gate precedes credential
resolution and listener creation; the required-connection boundary test proves
Deep Agents is rejected there. Consequently remote broker credential injection,
upstream connection and filtering are not claimed. Global user MCP import also
remains unsupported. Plugin hook support is a distinct, now verified path.

## Reproduction and results (2026-09-10)

Run from the fork root; full output logs accompany the issue reply.
Go 1.26.6 darwin/arm64; Python 3.13.3; deepagents-code 0.1.68;
deepagents-acp 0.0.11; agent-client-protocol 0.12.1; deepagents 0.7.13.
Dependencies are reproducible from `ah-13-da-2-python-requirements.txt`.

```sh
.da2-tools/go/bin/go -C server test -race ./pkg/agent ./internal/daemon -run 'TestDeepAgents|TestPluginHookMCP' -count=1 -v -timeout 120s
```

Exit 0: agent 9.069s, daemon 1.366s. Includes 1201 mixed slow-consumer events,
no-consumer explicit failure/cancellation, and no-read-stdin process-tree cleanup:
52.772125ms against 50ms interruption plus bounded 2s cleanup.

```sh
MULTICA_RUN_REAL_AGENT_SMOKE=1 MULTICA_DEEPAGENTS_TEST_PYTHON=/Users/citrine/Projects/multica-runtime-lab/.da2-tools/venv/bin/python .da2-tools/go/bin/go -C server test -tags=agentintegration ./internal/daemon -run '^TestDeepAgentsRealPluginMCP$' -count=1 -v -timeout 60s
```

Exit 0: test 0.77s, package 1.051s. Actual callback evidence:
`task-canary/install-canary/hook-canary/{"value":"platform-canary"}`.
The child uses `env -i`, temporary HOME/DEEPAGENTS_HOME/XDG directories and an
explicit isolated Python executable; no model credentials, database or shared
ports. Only the production generator's fresh loopback listener is used.

```sh
.da2-tools/go/bin/go -C server test -tags=agentintegration ./internal/daemon -run '^TestDeepAgentsRealPluginMCP$' -count=1 -v -timeout 60s
```

Exit 0, test SKIP without environment gate. Default builds exclude the test.

```sh
.da2-tools/go/bin/go -C server test -race ./pkg/agent ./internal/daemon -count=1 -timeout 120s
```

Exit 1 (FAIL), no retry: agent package hit the 120s suite timeout while
`TestRunCollectLeavesNoGoroutines` was running. Daemon package failed inherited
config-root tests (`TestLoadConfig_BackendOverrides_BackwardCompat_NoConfigFile`,
Reasonix/Dsh state-home tests and daemon identity tests) and the two Deep Agents
cases in `TestShouldRetryWithFreshSession_UnresumableHistoryIsBackendAgnostic`.
The latter expect broad fresh retries, conflicting with the adapter's narrow
missing-session policy. These failures are not fixed or claimed as passing.

Isolation incident: the broad suite inherited the runtime's configuration root,
which overrides tests' temporary HOME. Identity/state tests therefore accessed
that root and may have created identity/state files there. No further broad run
or configuration cleanup was attempted; no service was restarted. Future full
suite execution must first remove ambient Multica path overrides and verify all
state paths are temporary. The new real loader test itself uses `env -i` and
its isolated production plugin listener as described above.
Full output is attached as `da2-r2-full.log`.

No credentialed model E2E was attempted in R2: BLOCKED, no credentials supplied.
Required-MCP readiness failure handling remains the separately reported DA-1A
architecture follow-up; HTTP transport acceptance does not solve that gap.
