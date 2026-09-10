# AH-13 DA-8: collision-resistant broker names

Scoped PASS; model E2E remains BLOCKED. Base: 3af7beca4148e444457592645501c46c83dd067f. Fork branch: feat/ah-13-da-2-deepagents.

The broker name now uses 128 bits of SHA-256 over the complete ContributionID, encoded as 32 lowercase hex characters. The existing sanitized contribution key remains the readable prefix. Raw installation/contribution IDs are not embedded in the name. Transport and policy changes do not change the name. A cryptographic digest is not an encryption mechanism for guessable IDs.

Broker map assembly explicitly rejects an already-present name before starting the conflicting connection and closes the accumulated broker set. The error contains no ID. This also rejects duplicate identities, including optional ones, instead of silently overwriting. Names change from the previous truncated format; no compatibility alias is emitted. Configuration and Deep Agents policy use the same naming function. No OpenAI files or provider gates changed.

The new TestRemoteMCPNamesPreserveInstallationsAndPolicy uses two legal plugin IDs with the same contribution key, checks distinct/stable names and the strict ASCII/hex shape, starts two actual brokers in both input orders, and verifies separate transport configurations, required/optional policies and required tool schemas. It exercises explicit duplicate rejection. Single-broker TLS and official loader coverage is retained unchanged.

## Evidence

Original DA-7 failing probe, log and report are preserved in evidence/da8/da7_names_test.go, da7-names.log and DA-7-report.md. Their historical FAIL is not rewritten. DA-6 evidence is unchanged.

All commands ran from the fork root through the whitelist runner preserved in evidence/da6/run.py: `python3 .oa2-tools/run.py <log> <command>`. Temporary HOME/USERPROFILE/XDG/TMP were private; installed fork venv dependencies and Go caches were reused. No model credentials or MULTICA configuration were inherited. Loopback servers used fresh allocated ports. Versions remain Go 1.26.6 (server module), Python 3.13.3, dcode 0.1.68, deepagents-acp 0.0.11, agent-client-protocol 0.12.1 and wrapper 0.1.0; wrapper version is also in loader.log.

Commands (GO=.da2-tools/go/bin/go; PYTHON=absolute fork .da2-tools/venv/bin/python):

```sh
$GO -C server test -race ./internal/daemon ./pkg/agent -run 'RemoteMCP|DeepAgents|ShouldRetryWithFreshSession_UnresumableHistoryIsBackendAgnostic' -count=1 -timeout=300s -v
# exit 0, 38 top-level tests; evidence/da8/race.log

env MULTICA_RUN_REAL_AGENT_SMOKE=1 MULTICA_DEEPAGENTS_TEST_PYTHON=$PYTHON $GO -C server test -race -tags=agentintegration ./internal/daemon -run '^TestDeepAgentsReal(TLSBrokerLoader|PluginMCP)$' -count=1 -timeout=120s -v
# exit 0, TLS + eight plugin scenarios; evidence/da8/loader.log

$GO -C server test -race ./pkg/remotemcp/... -count=1 -timeout=120s -v
# exit 0, 8 top-level tests (no name filter); evidence/da8/remotemcp.log

$GO -C server vet -tags=agentintegration ./internal/daemon ./pkg/agent ./pkg/remotemcp/...
# exit 0; evidence/da8/vet.log
```

TLS assertions: missing CA rejects startup; trusted CA yields READY and exactly one official loader call; only approved read is listed and invoked; upstream initialize=4, list=4, allowed call=1, forbidden call=0, writes=0. Manager cleanup completes. All DA-8 commands passed on their first run; gofmt and git diff --check passed.

## Limits

The TLS upstream is a local fixture and the official loader is called directly, not a credentialed production ACP/model turn. Model E2E remains BLOCKED (prior production entry evidence: DA-6 entry.log, missing credentials, exit 1, no READY). External broker infrastructure/credential refresh and existing configuration/history audits remain untested. No full repository suite or OpenAI regression was run in this scoped change. Independent QA/review is pending; no parent status change, merge or deployment was performed.
