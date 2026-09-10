# AH-13 / OA-6 (OA-2-R2) binding source repair

**PASS for the assigned source-boundary repair and related checks; overall OA
acceptance remains BLOCKED on credentialed model E2E and previously untested
remote/deployment boundaries.** This is OA repair round 2/2, awaiting independent
QA/review; no automatic third round is authorized.

Baseline `f820e342c98cda7507474c5aece634ccd752fe3c`; same fork branch
`feat/ah-13-da-2-deepagents`, origin `CM-BF/multica-runtime-lab`.
The issue, mandatory thread scan, OA-5 review and downloaded
`oa5-review-probe.mjs` were read. That probe bypasses the daemon and shows why
shape/tool checks alone cannot establish input provenance.

## Source boundary

The accepted daemon-sanitization option is implemented, without relaxing bridge
identity or tools/list checks:

- `stripOpenAISandboxBindings` removes reserved `multicaBinding` from every
  ordinary MCP server entry. It returns an error on malformed input; the task
  does not fall back to unclean raw input.
- `runTask` cleans the agent document before assigning the ordinary merge/fallback
  value. `mergeRuntimeAndAgentMcpConfig` also sanitizes the OA ordinary entry
  path, which currently has no ambient runtime MCP import.
- `assembleOpenAISandboxMCP` sanitizes both ordinary layers before merging
  runtime→user. The production call sanitizes the resulting effective config
  again, then binds and overlays only the raw result of this turn's real
  `startTaskPluginHookMCP`. Plugin wins last; no ordinary config is merged later.
  The earlier pre-merge binding injection was removed.
- No real plugin result means no binding, even if a user names an endpoint
  `multica-plugins` and copies the previous metadata. The bridge then pins its
  ordinary URL and rejects the prior bound checkpoint identity.

The trusted boundary is daemon configuration assembly. This does not turn the
internal JSONL protocol/Store constructor into a cryptographically authenticated
API for arbitrary local callers. A direct caller bypassing daemon assembly must
apply equivalent source separation. Same-account hostile host code remains
outside Unix-local isolation claims.

R1 disabled-client prechecks, artifact type preflight, stable routing/policy,
actual tools/list fingerprint and SDK-owned agent loop remain unchanged. DA
implementation and remote broker gate are unchanged. No parent status write,
service/CLI/runtime configuration change, database operation, delegation, merge
or deployment occurred.

## Verification

Raw results: `docs/verification/oa-6/`. Node 23.11.0, npm 10.9.2, TypeScript 5.9.3,
Go 1.26.6 darwin/arm64, official SDK packages 0.17.2; lockfile unchanged.
Commands ran with `python3 .oa2-tools/run.py LOG COMMAND...`; runner source is
copied with evidence. Environment starts from a whitelist with fresh private
HOME/USERPROFILE/TMPDIR/XDG; no model/account credentials or MULTICA config are
inherited. Source was copied to 0700 `.oa2-tools/oa6-bridge`, dependencies reused
through a read-only-use symlink, and all bridge build/runtime output stayed in
that private copy/temp state. Go reads fork source and reuses build/module caches.
Real checks additionally require explicit smoke opt-in and verify fork origin.

| Command after isolation wrapper | Result |
| --- | --- |
| `npm --prefix .oa2-tools/oa6-bridge run build` | exit 0, `oa6-build.log` |
| `npm --prefix .oa2-tools/oa6-bridge test` | complete 11/11, no skips, exit 0, `oa6-bridge.log` |
| `.da2-tools/go/bin/go -C server test ./pkg/agent ./internal/daemon ./internal/daemon/execenv -run 'Test(OpenAISandbox\|DeepAgents\|ShouldRetryWithFreshSession_UnresumableHistoryIsBackendAgnostic)' -count=1 -timeout=90s -v` | 32 top-level tests plus subtests, exit 0, `oa6-go.log` |
| `env MULTICA_RUN_REAL_AGENT_SMOKE=1 MULTICA_OPENAI_SANDBOX_TEST_BRIDGE=<fork>/.oa2-tools/oa6-bridge .da2-tools/go/bin/go -C server test -tags=agentintegration ./pkg/agent ./internal/daemon -run '^TestOpenAISandbox(Real\|PluginHandler\|UntrustedBinding)' -count=1 -timeout=60s -v` | 4 top-level tests plus 3 provenance subtests, exit 0, `oa6-real.log` |
| `.da2-tools/go/bin/go -C server vet ./pkg/agent ./internal/daemon ./internal/daemon/execenv` | exit 0, `oa6-vet.log` |

Specific evidence:

1. `TestOpenAISandboxBindingSources` covers runtime forgery, user forgery,
   runtime/user collision precedence, real plugin winning last, cleanup of other
   server entries, ordinary provider entry and malformed-input rejection.
2. `TestOpenAISandboxUntrustedBindingCannotResume` feeds forged user, runtime and
   runtime→user replacements through the actual Go final assembler and into the
   TypeScript Store. Each pair has different identity and fails clean checkpoint
   load with CHECKPOINT_INVALID. These are isolated negative Store probes, not
   real model/sandbox execution; they preserve the review probe's same-tools
   premise while exercising the source boundary it bypassed.
3. The real plugin-handler test still completes two successive actual daemon
   HTTP handler / official SDK / Unix workspace turns. It then copies a
   replacement handler's binding into ordinary user configuration with no real
   hook overlay. Final assembly strips it; the previously real clean checkpoint
   is rejected and replacement handler tool calls remain zero. Stable hook and
   actual schema rejection cases continue passing as well.
4. Full bridge tests preserve disabled stdio/HTTP zero starts/connections,
   artifact no-partial-write rejection, cross-process recovery, MCP and cleanup.
   Go real entry still returns MODEL_CREDENTIALS without a key; real SDK descendant
   cancellation passes. Simulated model driver/events remain explicitly fixtures.

All executed checks passed on first attempt; no failed-test retry was needed.
Full repository/race/UI/browser tests, fresh installation, DB migrations,
deployed service/TLS, full remote broker and credentialed model E2E were not run
this round. Existing DA and OA historical evidence is not rewritten or promoted
to green. Write ownership is released following commit/push for QA/reviewer.
