# AH-13 / DA-2 R2 follow-up — recovery and production readiness

PASS for the requested history retry correction and the readiness contracts
listed below. Overall DA-2 remains BLOCKED: credentialed model E2E and complete
remote broker TLS startup E2E have not passed. This continues R2, not a third
repair round. Base: `71f4ce9421025ae588312ff58114ce31c3fdad1f`; same fork branch
`feat/ah-13-da-2-deepagents`. DA-1A was downloaded via CLI (17604 bytes) and read;
its older base was compared with the HTTP/R1 candidate. Architecture report and
R1 backpressure/cancellation changes are retained.

## Recovery correction

`daemon.shouldRetryWithFreshSession` previously returned false for Deep Agents
before reaching the common unresumable-history branch. It now distinguishes:

- Common preflight/poisoned-history failure: shared retry rule applies when a
  prior session exists, status is failed and no tool was observed.
- Live `session/load` error: the adapter sets `Result.ResumeLoadFailed`. Only an
  exact missing resource sets `ResumeRejected`; auth, DB, ambiguous failures or
  even poisoned-history wording in a live load error do not fall through into
  the common text-based retry.
- `session/prompt` poisoned-history error: normalize the recognized class into
  safe canonical wording for the shared rule. Raw provider details are discarded.

The exact GH6066/GH5760 generic test passes for Deep Agents and all other listed
backends. Adapter tests verify live-load versus prompt classification and secret
sentinel redaction; decision tests verify prior-session and zero-tool gates.

## Production changes

- `runtime-bridges/deepagents/`: installable `multica-dcode-acp` 0.1.0, schema 1,
  pinned deepagents-code 0.1.68 / deepagents-acp 0.0.11 / ACP 0.12.1. The console
  entry calls official `cli_main`; only its dynamic MCP loader import is guarded.
  Full dependency snapshot included. No replacement model/agent loop.
- The same official loader is called once with original arguments. Required
  metadata must be unique, present, ok and error-free. Required original tool
  names and reviewed schemas are matched through real dcode tool metadata, not
  guessed prefixes. Failure cleans the returned manager with a bounded attempt;
  the Go process owner provides the final hard cleanup bound. Success returns
  the original tuple/tools/manager. No readiness tool calls.
- The Go backend supplies private config/policy/request files and checks atomic
  READY against schema, random nonce, config/policy SHA256, child PID and exactly
  one loader call. Matching READY precedes initialize; initialize precedes
  new/load/prompt. Missing/old/wrong/FAIL receipts, exit or timeout cannot prompt.
  Config/policy/request directory cleanup is covered; receipts expose safe
  categories/counts, not upstream URL/header/env/error text.
- Managed entries default required. Remote policy follows the winning overlay
  and original FailurePolicy; plugin hooks remain optional. Disabled managed
  entries are omitted; explicit required/disabled conflicts fail. The existing
  remote provider gate now includes Deep Agents, preserving credential resolution,
  approved tools/schema validation, filtering and lifecycle.
- Executable probing, command inventory, installation docs and model discovery
  use `multica-dcode-acp`; bare dcode cannot bypass READY. ACP model discovery
  also creates a fresh gate. Manual model entry remains available without creds.
  Global user MCP inventory import remains unsupported.

## Commands, versions and results

Go 1.26.6 darwin/arm64; Python 3.13.3; deepagents SDK 0.7.13; MCP Python 1.30.0.
Commands run from the authorized fork. `run-isolated-tests.py` is included in the
attached evidence: it invokes `.da2-tools/go/bin/go -C server test`, uses a temporary
HOME/USERPROFILE/XDG and a PATH/LANG/TMPDIR whitelist, sets only Go cache paths,
and inherits no MULTICA variables or account credentials. For real tests it adds
only the smoke gate and absolute private Python path. Cache locations in that
reproduction helper reflect this machine and must be adjusted on another host.

```sh
python3 .da2-tools/run-isolated-tests.py .da2-tools/da2-r2-accepted-go.log -race ./pkg/agent ./internal/daemon -run 'TestDeepAgents|TestShouldRetryWithFreshSession_UnresumableHistoryIsBackendAgnostic|TestRemoteMCP' -count=1 -v -timeout 60s
```

Exit 0; agent 10.158s, daemon 1.454s. GH6066/GH5760 Deep Agents retry cases PASS.
R1 slow-consumer (1201 mixed events), unconsumed failure/cancel and no-read-stdin
regressions PASS. No-read cancellation: 52.129792ms against 50ms + bounded cleanup;
parent and child reaped. Existing remote credential/filter tests, required
credential-resolver rejection and optional preparation degradation PASS.

```sh
python3 .da2-tools/run-isolated-tests.py .da2-tools/da2-r2-accepted-real.log --real -tags=agentintegration ./internal/daemon -run '^TestDeepAgentsRealPluginMCP$' -count=1 -v -timeout 60s
```

Exit 0; package 5.340s. Eight real loader cases PASS: production plugin generator
and daemon handler round trip; HTTP-required failure; missing stdio executable;
HTTP-optional degradation; missing required tool; real stdio discovery/call;
missing required server metadata; reviewed schema mismatch. Required failures
prevent all ACP RPCs. Loader count is 1, readiness tools/call count 0, explicit
platform invocation count 1 on success. Original names/schema matching is exercised
with real dcode tools. Child clears its environment before importing dependencies;
only temporary state and the generator's own ephemeral loopback port are used.
This uses a test ACP harness around the production guard and real loader, not a
credentialed official model loop.

```sh
PYTHONPATH=runtime-bridges/deepagents .da2-tools/venv/bin/python -m unittest discover -s runtime-bridges/deepagents/tests -v
```

Exit 0, 3 tests (matrix subcases): error/unauthenticated/reconnect/disabled/unknown,
missing/duplicate metadata, cleanup, optional degradation, tuple identity, second
loader rejection, config mutation/exception and safe receipt. These metadata cases
are explicitly unit fixtures, not real provider/OAuth acceptance.

```sh
python3 .da2-tools/run-isolated-tests.py .da2-tools/da2-r2-gate-models.log -race ./pkg/agent -run 'TestDeepAgentsReadinessGatesAllRPCs|TestDeepAgentsGatePrivateLifecycle|Test.*(ACP|Hermes).*Models|TestListModelsHermes' -count=1 -v -timeout 60s
```

Exit 0, agent 2.828s: receipt digest/lifecycle and existing ACP/Hermes model
checks PASS. Canonical PATH/shell-command inventory discovery is also checked
separately in `da2-r2-discovery-retry.log`. Its initial test failed on macOS
`/tmp` versus `/private/tmp` path equality (discovery itself found the correct
executable); the fixture now compares canonical paths. Retry exit 0, daemon
0.282s. Both logs are retained.

```sh
uv pip install --python .da2-tools/venv/bin/python --no-deps --no-build-isolation ./runtime-bridges/deepagents
MULTICA_RUN_REAL_AGENT_SMOKE=1 MULTICA_DEEPAGENTS_TEST_ENTRY=/Users/citrine/Projects/multica-runtime-lab/.da2-tools/venv/bin/multica-dcode-acp .da2-tools/venv/bin/python runtime-bridges/deepagents/tests/real_entry.py
```

Install exit 0. Installed production `--version` exit 0. Production `--acp` exit 1
with missing model credentials and no READY, as expected from the pinned CLI's
model-before-loader order. Smoke harness exit 0. Model E2E remains BLOCKED.

## Failures retained; not a full PASS

The original R2 full-suite `da2-r2-full.log` remains unchanged, SHA256
`43a8a7b0415182457ee2c7d2cd8fb8ec97e8e69b1055a728525c1c970e7272db`.
It is still exit 1: agent 120s timeout, daemon state-root/identity failures and the
then-failing history cases. Only the history cases have been corrected and
retested here. The earlier ambient runtime config-root access incident remains
recorded in the R2 report; this turn did not clean or alter that configuration.
No full-suite rerun is being represented as green.

A proposed additional TLS proxy fixture failed twice (initial run plus one
correction/retry). It omitted the upstream tools/list contract and reviewed schema
metadata; it was withdrawn from the retained tests. Both raw logs
`da2-r2-final-go.log` and `da2-r2-final-go-retry.log` are attached. They are not
passing TLS evidence. Existing HTTP broker tests, the provider gate and optional
policy pass, but full controlled-TLS `startTaskRemoteMCPBrokers` startup through
public-endpoint validation, loader and invocation remains unverified/BLOCKED.
No production security gate was weakened to make localhost upstream tests pass.

Other unverified items: credentialed official agent/model turn, OAuth, Windows,
cross-network-namespace routing. No DA-3/DA-4/OpenAI work or delegation occurred;
no merge/deployment, existing DB or port reuse. This R2 handoff releases code
ownership for independent verification; parent issue remains in_progress.
