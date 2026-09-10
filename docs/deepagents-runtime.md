# Deep Agents runtime (fork)

This fork adds the `deepagents` protocol family. Multica owns task preparation,
stream normalization and process cleanup; the official `dcode --acp` process owns
its model loop, tools and subagents. The architecture report remains at
`docs/superpowers/specs/2026-09-10-ah-13-da-1-deepagents-acp.md`.

## Install in a separate environment

Python 3.12 or newer is required. The validated environment used Python 3.13.3,
`deepagents-code==0.1.68`, `deepagents-acp==0.0.11`, ACP Python 0.12.1, and
Deep Agents SDK 0.7.13. The resolved dependency snapshot is committed alongside
`docs/verification/ah-13-da-2.md`.

```sh
uv venv .venv-deepagents --python 3.13
uv pip install --python .venv-deepagents/bin/python \
  -r docs/verification/ah-13-da-2-python-requirements.txt
.venv-deepagents/bin/dcode --version
```

Use the fork in its own environment. Do not run migrations against an existing
Multica database, reuse a running service's ports, update the global CLI, or copy
an existing profile/database into this installation. Migration 459 widens the
runtime family constraint; it has not been applied to any running database.

## Select and configure

In the existing Runtimes management dialog, create a custom runtime with protocol
family **Deep Agents**, using the absolute path to the venv's `dcode` as its
command. A command path containing spaces must be quoted. Leave fixed arguments
empty: the backend adds `--acp`, model and private MCP arguments.

Alternatively an independently configured fork daemon discovers `dcode` on PATH
or through `MULTICA_DEEPAGENTS_PATH`; `MULTICA_DEEPAGENTS_MODEL` supplies its default
model. Bind the runtime to an agent through the existing runtime selector.
Executable discovery proves availability, not authentication.

Set the model explicitly (for example a provider-qualified model ID available to
your account). The model selector accepts manual input if discovery is empty.
Supply only the intended provider credentials through the agent's environment
settings or the independent daemon environment. Never put keys into arguments,
source files, logs or issue comments. Unconfigured dcode currently exits before
ACP initialization with a missing-credentials error; Multica reports a failed
initialize stage with a fixed, actionable credential/dependency hint when recognized.
Raw provider stderr is never copied into task output.

Managed MCP accepts canonical explicit stdio and `type:"http"` streamable-HTTP entries:

```json
{"mcpServers":{"local":{"command":"/absolute/path/to/mcp-server","args":[]}}}
```

The adapter consumes `ExecOptions.McpConfig`. The daemon merges generated
`multica-plugins` streamable-HTTP configuration into that input; plugin hook tools
are supported. `TestDeepAgentsRealPluginMCP` verifies the production generator,
private adapter config, installed dcode loader `tools/list`, and `tools/call`
through the daemon handler in an isolated environment without model credentials.
This loader test uses a test ACP harness, not a dcode model turn.

Remote MCP brokers remain unsupported: their provider gate excludes Deep Agents,
so their credential resolution, broker startup and provider-specific filtering
contracts are not enabled. `TestDeepAgentsMCPConfigurationBoundary` exercises
that rejection separately from plugin hook acceptance. Global user MCP import
into the daemon inventory also remains unsupported. Explicit agent stdio and
HTTP configuration is preserved. A prepared AGENTS/CLI brief is not MCP evidence.

Each execution validates the object, writes it in a private directory (0700) as
a 0600 file, and adds `--mcp-config`. New and loaded ACP sessions receive an empty
`mcpServers` array because dcode loads the explicit file at startup. The file is
removed after process cleanup. HTTP requires an explicit `type:"http"`, an
HTTP(S) URL without embedded credentials, and optional string-valued headers.
SSE and mixed command/HTTP entries are rejected. The daemon does not import global Deep Agents MCP configuration into
its inventory. dcode itself may merge project, profile or plugin configuration
under its own trust rules; explicit config is not an isolation switch. No blanket
project trust is added. A successful handshake does not prove every upstream MCP
server connected: dcode can represent some discovery failures as server metadata.
Inspect and test required tools before relying on a configured runtime.

## State, context and limits

The daemon assigns a private persistent `DEEPAGENTS_HOME` under its own profile,
keyed by workspace, runtime, agent and issue/chat (or task if there is no durable
conversation). User overrides cannot redirect it. Checkpoints live in its
`.state/sessions.db`; no global credentials or database are copied. Keep this
shard and the same absolute workdir to resume. Automatic state retention/GC is
not added in this change.

The prepared workdir contains `AGENTS.md`; skills are staged under
`.deepagents/skills`. Real dcode context consumption and model-driven use of
these files still require credentialed E2E validation. Artifacts remain in the
workdir and use the existing Multica attachment workflow; writing a file does
not upload it.

Resume calls `session/load` only when `loadSession` is advertised. Replay is
suppressed from current transcript/output/tool accounting. A restored model is
changed through its advertised `session/set_config_option` selector. Only a
structured missing resource whose URI matches the requested session permits
fresh-session fallback; auth, cwd, database and capability failures preserve the
pointer. A process killed during tool execution has unknown side effects;
exactly-once execution is not guaranteed.

Messages use a 256-entry bounded queue with cancellable backpressure. A slow
consumer receives every event while it continues draining. A send stalled for
two seconds fails the run explicitly with `message consumer stalled`; abandoning
the stream cannot silently return a truncated success. Cancellation also reports
an interrupted stream when queued delivery cannot finish. Buffered events remain
readable after Result; successful streaming callers must consume Messages.

Cancellation attempts a notification in an owned worker, waits at most two seconds (configurable by
`TurnInterruptTimeout`), then closes the pipes, kills the owned process tree and joins all RPC workers.
A blocked prompt/cancel write cannot keep the session owner from reaching cleanup. `end_turn`
completes; `cancelled` aborts; token limits, refusal, malformed output, EOF and
missing/unknown stop reasons fail. Context deadlines time out. Result is emitted
once after cleanup. Usage remains unknown when not reported; no cost is inferred.
Text input is supported. Explicit thinking level, service tier and MaxTurns are
rejected. Interactive/resume/model/MCP launch overrides are rejected across
fixed, extra and custom arguments.

## Verify without touching an account

Default contract tests create their own fake executable and never resolve dcode:

```sh
cd server
go test -race ./pkg/agent -run 'DeepAgents|ACP|SupportedTypes|OnlyLaunchGo' -count=1
go test ./internal/daemon/execenv ./internal/daemon -run DeepAgents -count=1
```

Real checks require both the build tag and opt-in environment variable, plus the
absolute Python path from an isolated venv. They give subprocesses a fresh HOME,
profile and cwd, and forward no credentials:

```sh
MULTICA_RUN_REAL_AGENT_SMOKE=1 \
MULTICA_DEEPAGENTS_TEST_PYTHON=/absolute/path/.venv-deepagents/bin/python \
go test -tags=agentintegration ./pkg/agent -run '^TestDeepAgentsReal' -count=1 -v
```

`RealStartup` records the real import/version and no-credentials launch outcome.
`RealMCP` exercises the actual dcode MCP loader and a local stdio tool.
`RealSDKCheckpoint` uses the real ACP SDK with a deterministic graph in two
processes, observing SQLite restoration, replay and a local artifact. It is not
the prebuilt dcode model loop. See the verification report for PASS/BLOCKED scope.
