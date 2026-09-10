# AH-13 DA-6: TLS broker / official loader coverage

DA-6 scoped result: PASS. Model E2E remains BLOCKED. Base: db7ae60962ba9beb07f61af3051f863fe50cef0d; fork branch: feat/ah-13-da-2-deepagents.

## Change and regression found

The first real loader run failed (exit 1; `evidence/da6/tls.log`). The broker server name included a colon from the real `plugin:` contribution prefix, rejected by dcode's server-name validation before connecting. The minimal production change replaces colons with underscores in the existing name suffix; names without colons are unchanged. No provider gate, agent loop, OpenAI bridge, deployment or service configuration changed.

`server/internal/daemon/deepagents_tls_integration_test.go` creates a new private CA and TLS MCP server per run, uses the actual remote broker and reviewed tool digest, rejects missing CA before launching a loader, then passes the broker-generated config to the fork wrapper guard and installed official dcode resolver. The wrapper source is copied to a private directory; the installed entry is separately checked with `--version`. `runtime-bridges/deepagents/tests/tls_loader.py` invokes the official resolver once, validates READY, observes only the approved tool and calls its actual SDK tool object. It also attempts a forbidden call directly against the broker. No replacement agent loop is used.

The single permitted retry passed: upstream initialize=4, tools/list=4, allowed calls=1, forbidden calls=0, no writes. The official manager is cleaned up. TLS verification remains enabled; the broker terminates TLS trust and fixture credentials, so neither the CA nor upstream Authorization is passed to Python. Both environment opt-in and fork-origin gates apply. HOME/XDG/TMP and runtime state are private; no account/model credentials or MULTICA configuration are inherited. All servers use newly allocated loopback ports and are closed at test completion.

## Reproduction and evidence

Versions: Go 1.26.6 darwin/arm64 (server module), Python 3.13.3, multica-dcode-acp 0.1.0, deepagents-code 0.1.68, deepagents-acp 0.0.11, agent-client-protocol 0.12.1. Install the pinned bridge/dependencies as described in `docs/deepagents-runtime.md`. Use an isolated virtualenv and temporary HOME/XDG/TMP with a whitelist environment. The exact local whitelist runner is preserved in `evidence/da6/run.py`; its cache paths are machine-specific, not service state. Commands below were prefixed with `python3 .oa2-tools/run.py <log>` from the fork root. GO is `.da2-tools/go/bin/go`; PYTHON is the absolute `.da2-tools/venv/bin/python` path.

| Command after runner | Exit | Evidence |
| --- | --- | --- |
| `env MULTICA_RUN_REAL_AGENT_SMOKE=1 MULTICA_DEEPAGENTS_TEST_PYTHON=$PYTHON $GO -C server test -race -tags=agentintegration ./internal/daemon -run '^TestDeepAgentsRealTLSBrokerLoader$' -count=1 -timeout=90s -v` | first 1; retry 0 | tls.log; tls-retry.log |
| `$GO -C server test -race ./pkg/remotemcp ./internal/daemon ./pkg/agent -run 'RemoteMCP\|DeepAgents\|ShouldRetryWithFreshSession_UnresumableHistoryIsBackendAgnostic' -count=1 -timeout=300s -v` (regex uses literal pipe characters, without backslashes) | 0; 37 top-level tests | regression.log |
| `env MULTICA_RUN_REAL_AGENT_SMOKE=1 MULTICA_DEEPAGENTS_TEST_PYTHON=$PYTHON $GO -C server test -race -tags=agentintegration ./internal/daemon -run '^TestDeepAgentsRealPluginMCP$' -count=1 -timeout=120s -v` | 0; 8 scenarios | plugin.log |
| `env MULTICA_RUN_REAL_AGENT_SMOKE=1 MULTICA_DEEPAGENTS_TEST_ENTRY=<absolute venv>/bin/multica-dcode-acp $PYTHON runtime-bridges/deepagents/tests/real_entry.py` | script 0; production --acp 1 | entry.log |
| `$GO -C server vet -tags=agentintegration ./internal/daemon ./pkg/agent ./pkg/remotemcp` | 0 | vet.log |
| `env PYTHONDONTWRITEBYTECODE=1 PYTHONPATH=<private bridge copy> $PYTHON -m unittest discover -s <private bridge copy>/tests -p test_guard.py -v` | 0; 3 tests | guard.log |

All log names above are under `docs/verification/evidence/da6/`. Gofmt and git diff --check passed. Build/virtualenv directories are excluded from the commit.

## Acceptance boundary

This is real TLS remote MCP transport through the real broker and official loader, with policy enforcement. It is not a credentialed model turn or a successful production CLI ACP session. The production entry rejects absent model credentials before loader execution (exit 1; no READY); model E2E remains BLOCKED. No external broker infrastructure, account credential refresh, or existing runtime configuration audit was exercised. No full repository suite was rerun in DA-6; targeted related regressions above are the new evidence. DA-5's broader results and historical limitations are not reset by this test. Independent QA/review remains outstanding.
