# AH-13 / DA-2 verification

**BLOCKED — implementation candidate delivered, full acceptance not passed.**

Revision R1 resolves the slow/unconsumed message and blocked-stdin cancellation
defects found after this initial report. See `ah-13-da-2-r1.md` for the new test
commands, results, MCP boundary evidence and diagnostic behavior. Default platform
MCP and global user MCP mapping are explicitly unsupported; do not interpret
the original UI/config affordance or comments as evidence of their availability.
The initial thin adapter is implemented and locally tested. Two acceptance gaps
remain: no provider credentials were supplied for prebuilt dcode model E2E, and
upstream dcode does not fail closed on every configured MCP connection failure.
The latter is reproduced below; a green observation test is not a PASS for that
product contract. DA-1 architecture report is unchanged. No OpenAI SDK work was
started and no work was delegated.

## Revision and scope

Base `e5ef79093`; branch `feat/ah-13-da-2-deepagents`, origin
`https://github.com/CM-BF/multica-runtime-lab.git`. Root AGENTS.md and CLAUDE.md read.
The implementation commit containing this report includes:

- `server/pkg/agent/deepagents.go`: official dcode ACP subprocess, strict argument
  guards at all three layers, private stdio MCP JSON, capability-gated load,
  replay suppression, restored model selection, terminal classification and
  bounded cancellation/process cleanup. Existing Hermes transport is reused.
- Factory/header/model discovery, daemon executable probe and display name;
  workspace/runtime/agent/conversation state shard; strict fresh-retry policy.
- AGENTS and `.deepagents/skills` preparation, explicit non-import of global MCP
  inventory, migration 459, frontend family tuple/MCP affordance/display/catalog.
- Default contract tests, opt-in real tests, installation/usage guide
  `docs/deepagents-runtime.md`, builtin runtime reference, dependency snapshot.

Only this fork was modified. No running server/daemon/global CLI/runtime config,
existing database, service port, or account credentials were used. Migration 459
was not applied. Temporary Python/Go tools were installed inside `.da2-tools/`;
Go's normal dependency/toolchain cache was populated. No service was started.

## Toolchain and commands

Date: 2026-09-10. Host: Darwin arm64. Commands below are from the fork root unless
prefixed `server/`. `GO` denotes the downloaded `.da2-tools/go/bin/go`; in server/
its automatic toolchain selection used **Go 1.26.6** from go.mod. The downloaded
bootstrap itself reports 1.26.1. Python 3.13.3; Node v23.11.0; pnpm 9.15.4;
Vitest 4.1.0. The repository requests pnpm 10.28.2 / Node >=22; the installed pnpm
9 completed frozen-lockfile installation and the checks below without changing
the lockfile. A CI toolchain run remains useful.

| Command | Exit | Result |
| --- | --- | --- |
| `git remote -v`, `git status --short`, `cat AGENTS.md CLAUDE.md`, `git show e5ef79093:docs/superpowers/specs/2026-09-10-ah-13-da-1-deepagents-acp.md` | 0 | Correct fork, initially clean, rules/report read |
| `git switch -c feat/ah-13-da-2-deepagents e5ef79093` | 0 | Work branch from required input |
| `uv venv .da2-tools/venv --python 3.13` | 0 | Separate venv |
| `uv pip install --python .da2-tools/venv/bin/python 'deepagents-code==0.1.68' 'deepagents-acp==0.0.11'` | 0 | Installed fixed top-level versions |
| `uv pip freeze --python .da2-tools/venv/bin/python` | 0 | Full resolved snapshot in `ah-13-da-2-python-requirements.txt` |
| `pnpm install --frozen-lockfile --ignore-scripts` | 0 | Installed local frontend dependencies; lockfile unchanged |
| server/: `GO test -race ./pkg/agent -run 'DeepAgents\|ACP\|SupportedTypes\|OnlyLaunchGo' -count=1` | 0 | 11.476s, ACP regressions/new contracts; no real dcode invoked |
| server/: `GO test ./internal/daemon/execenv ./internal/daemon -run DeepAgents -count=1` | 0 | Private state/skills, discovery and fresh retry tests |
| server/: `GO test -race ./pkg/agent -run '^TestDeepAgentsCancel$' -count=1 -v` | 0 | Cooperative and unresponsive agent paths; parent and spawned child absent when Result received |
| `pnpm --filter @multica/views exec vitest run runtimes/components/deepagents-catalog.test.ts runtimes/components/runtime-profile-catalog.test.ts` | 0 | 2 files / 11 tests passed |
| `pnpm --filter @multica/core exec tsc --noEmit` | 0 | Typecheck passed |
| `pnpm --filter @multica/views exec tsc --noEmit` | 0 | Typecheck passed after final UI fix |
| `git diff --check` | 0 | No whitespace errors |

The cancellation descendant assertions were added after the combined ACP run and
verified with the dedicated race-enabled cancellation command. Other later edits
were docs and real-smoke observation assertions. Go test includes its standard
vet checks; standalone `go vet ./...` and full repository suites were not run.

Exploratory failures: initial `gofmt` lookup exited 127 (no ambient Go), resolved
using the local toolchain. One edit script ran from server/ with root-relative
paths and exited 1 before any write, then succeeded from the root. Searches for
nonexistent guessed filenames were replaced with `rg` discovery. The first real
import check exited 1 because `deepagents_acp` exports no top-level
`AgentServerACP`; the correct import is `deepagents_acp.server.AgentServerACP`.
That import failure was corrected and retried once, successfully. No repeated
failure was retried more than once.

## Default contract evidence

`deepagents_test.go` drives a test-created absolute executable with spaces in its
path. It captures wire JSON and private MCP metadata. Assertions cover:

- initialize/new/prompt shape, absolute cwd, empty session MCP array, model argv;
  0600 MCP file content and deletion after cleanup, malformed config rejection;
  fixed/extra/custom argument conflict rejection.
- end_turn, cancelled, token/turn limits, refusal, missing/unknown stop reason,
  incompatible protocol, missing session ID, malformed stdout, early EOF;
  one Result then channel close; terminal observation before Result.
- two tool IDs and final answer selection after tool narration; loadSession
  capability gate, session/load (never session/resume), replay omission, model
  reapplication; missing-resource fallback vs database failure preservation.
- cooperative cancel and unresponsive prompt, no JSON-RPC id on cancel,
  deadline-bound cleanup, both owned parent and child no longer alive on Result.

Daemon tests establish that executable discovery registers the Deep Agents family
without executing the fake dcode; auth/general failure cannot request a fresh
session; state shards differ across workspace/runtime/agent and persist across
same-conversation tasks. Static whitelist tests cover Go/TS/new migration/CLI
command inventory. Migration execution and live profile creation/picker binding
were not exercised against a server/database.

## Real checks (double-gated, no credentials)

Each command ran from server/ with:

```sh
MULTICA_RUN_REAL_AGENT_SMOKE=1 \
MULTICA_DEEPAGENTS_TEST_PYTHON=/absolute/fork/.da2-tools/venv/bin/python \
GO test -tags=agentintegration ./pkg/agent -run '<test-name>' -count=1 -v
```

The actual Python executable was the venv under this fork. Each subprocess had a
new HOME, DEEPAGENTS_HOME and cwd, a minimal explicit environment and no provider
keys. No network model request was attempted. Each subprocess was foreground,
bounded, and reaped. Test source preserves the exact runner and environment.

`^TestDeepAgentsRealStartup$`: **exit 0 for the observation test** (9.612s).
Observed package versions: deepagents-code 0.1.68, deepagents-acp 0.0.11,
agent-client-protocol 0.12.1, Deep Agents SDK 0.7.13. Real `dcode --version`
**exit 0**. Actual `dcode --acp` **exit 1**, no ACP stdout, stderr:

```text
Error: No credentials configured. Please set one of: ANTHROPIC_API_KEY, OPENAI_API_KEY, or GOOGLE_API_KEY
```

This is a real failed startup/configuration observation, not a successful model
or handshake E2E. `_paths.PATHS.profile.state_dir` resolved to the fresh profile's
`.state`. The ACP SDK serialized missing session as:

```json
{"code":-32002,"message":"Resource not found","data":{"uri":"s1"}}
```

`^TestDeepAgentsRealMCP$`: **exit 0 for observation test** (2.029s).
The actual dcode `resolve_and_load_mcp_tools` loaded a separate FastMCP stdio
server, discovered `local_echo`, called it and returned `DA2-local-canary`.
Managers were cleaned up. A second config pointing at a nonexistent command
returned an empty tools list and `status='error'` server metadata, without raising.
It printed:

```text
MCP_FAIL_CLOSED=BLOCKED: dcode loader returns error metadata without raising for a missing command
```

Source inspection of installed `deepagents_code/main.py` ACP startup confirms it
passes this metadata into the agent factory and only aborts on loader exceptions.
The standard ACP initialize/new response has no managed MCP readiness contract.
The current adapter does not claim this missing fail-closed guarantee. A required
follow-up must introduce an authoritative startup health surface or a separately
reviewed narrow entry wrapper; parsing arbitrary model text is not acceptable.

`^TestDeepAgentsRealSDKCheckpoint$`: **exit 0** (3.921s).
Real AgentServerACP + AsyncSqliteSaver + a deterministic LangGraph graph ran in
two separate Python processes using the same temporary cwd/SQLite database:

```text
phase new SDK checkpoint messages=1 end_turn
real SDK replay observed
phase load SDK checkpoint messages=3 end_turn
```

Artifact bytes were asserted after each run. This verifies actual SDK checkpoint,
replay and local artifact behavior, without pretending the deterministic graph is
the prebuilt dcode model loop or model context ingestion.

## Outstanding acceptance and handoff

1. **BLOCKED: MCP fail-closed startup guarantee**, reproducible above. Current
   direct `dcode --acp` adapter cannot observe all required server readiness.
2. **BLOCKED: prebuilt dcode model E2E**, no credential configuration supplied.
   Includes model-driven MCP use after new/load, AGENTS/skills canary ingestion,
   real long-running tool cancellation, model-driven artifact generation and
   prebuilt dcode cross-process continuation. No credentials were searched for.
3. **Not run:** live database migration/profile creation, browser picker binding,
   online attachment upload, remote MCP (explicitly unsupported), full repository
   checks and independent DA-3/DA-4 validation.

Implementation and test files are released to the leader after this commit and
report. Parent issue status remains in_progress. No merge or deployment is
requested. The leader should resolve the MCP startup contract before marking
DA-2 complete; this report is not a request to start the OpenAI phase.
