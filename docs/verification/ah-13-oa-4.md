# AH-13 / OA-4 (OA-2-R1) minimal repair

**PASS for the three assigned repairs and local related regression. Overall OA
acceptance remains BLOCKED on credentialed model E2E and the previously declared
untested deployment/remote-broker boundaries.** Independent QA/review is pending.
Baseline `82ce5f11ada3eb5e330985072ba420139eb2ce66`, unchanged fork branch
`feat/ah-13-da-2-deepagents`, origin `CM-BF/multica-runtime-lab`.

The issue, thread scan, review comment and downloaded `oa3-review-probes.mjs` were
read before implementation. No delegation, parent state write, deployment,
service/CLI/runtime configuration edit or database access. Remote broker gate
remains closed. DA implementation and bridge files are unchanged.

## Repairs

1. **Disabled MCP:** typed boolean flag; all flags are validated before starting
   the first client. Disabled entries are skipped before constructing stdio/HTTP
   clients. A real listening HTTP server receives zero connections; a stdio
   command with a marker-file side effect never starts. An invalid flag later in
   a mixed configuration also prevents the earlier enabled process from starting.
2. **Stable policy versus per-turn transport:** new OA-only daemon mapper annotates
   plugin config with installation/hook/tool policy before the existing overlay
   merge. The bridge canonicalizes this stable policy separately from the bound
   loopback endpoint's port and 48-hex path token. It does not normalize arbitrary
   URLs or ignore the entire MCP configuration. HTTP Authorization values can
   rotate within a fixed auth scheme; non-auth headers and stdio env/command/args
   remain pinned. Newly connected servers are listed, and the complete sorted
   tool definitions are fingerprinted against the clean checkpoint before Runner
   or tool execution. Both changed stable hook route and changed advertised tool
   schema are tested as rejection cases with zero tool calls.
3. **Artifact type replacement:** choose the explicitly authorized fail-before-
   write option. Preflight inventories file/directory kinds on both sides,
   including empty directories, before the first export mutation. File→directory,
   directory→file and empty-directory→file all return ARTIFACT_TYPE_CHANGE while
   preserving an unrelated file that would otherwise be overwritten first.

Checkpoint schema is now 2. Old schema 1 snapshots are rejected rather than
silently upgraded to weaker identity semantics. Multica token, SDK RunState,
history and sandbox archive remain distinct. The SDK still owns the agent loop;
no new model loop or remote provider support was introduced.

## Execution evidence

Raw logs: `docs/verification/oa-4/`. Go 1.26.6 darwin/arm64, Node 23.11.0,
npm 10.9.2, TypeScript 5.9.3, official agents SDK packages 0.17.2; dependency lock
unchanged. `run-isolated.py` is copied alongside the logs for reproducibility.
Commands run from the fork via `python3 .oa2-tools/run.py LOG COMMAND...`, which
uses a fresh private HOME/USERPROFILE/TMPDIR/XDG and whitelist environment. No
model credentials or MULTICA configuration variables are inherited. Go caches
and installed npm dependency files are reused, not account configuration.

The initial build/5-test check wrote ignored fork-local dist, then final build,
complete Node suite, package and real integrations used the independent 0700
`.oa2-tools/oa4-bridge` source copy with a read-only-use dependency symlink. Test
state is disposable/private; real HTTP handlers listen on loopback port 0. Real
Go checks require explicit opt-in plus verified fork origin. The SDK shell's
host-user boundary is unchanged; this is not a VM/chroot security claim.

| Command after isolation wrapper | Result / log |
| --- | --- |
| `npm --prefix .oa2-tools/oa4-bridge run build` | exit 0, `oa4-private-build.log` |
| `node --test runtime-bridges/openai-sandbox/test/review-fixes.test.mjs` | 5/5 PASS, exit 0, `oa4-targeted.log` |
| `npm --prefix .oa2-tools/oa4-bridge test` | complete 11/11 PASS, no skips, exit 0, `oa4-full-bridge.log` |
| `.da2-tools/go/bin/go -C server test ./pkg/agent ./internal/daemon ./internal/daemon/execenv -run 'Test(OpenAISandbox\|DeepAgents\|ShouldRetryWithFreshSession_UnresumableHistoryIsBackendAgnostic)' -count=1 -timeout=90s -v` | 31 top-level tests plus subtests PASS, exit 0, `oa4-go-regression.log` |
| `env MULTICA_RUN_REAL_AGENT_SMOKE=1 MULTICA_OPENAI_SANDBOX_TEST_BRIDGE=<fork>/.oa2-tools/oa4-bridge .da2-tools/go/bin/go -C server test -tags=agentintegration ./pkg/agent ./internal/daemon -run '^TestOpenAISandbox(Real\|PluginHandler)' -count=1 -timeout=60s -v` | 3 top-level tests PASS, exit 0, `oa4-real-final.log` |
| `.da2-tools/go/bin/go -C server vet ./pkg/agent ./internal/daemon ./internal/daemon/execenv` | exit 0, `oa4-vet.log` |
| `npm pack <fork>/.oa2-tools/oa4-bridge --pack-destination <fork>/.oa2-tools/oa4-package` | exit 0, 7 files including new identity module, `oa4-pack.log` |
| `npm install --prefix <fork>/.oa2-tools/oa4-installed --ignore-scripts --no-audit --no-fund <fork>/.oa2-tools/oa4-package/multica-openai-sandbox-0.1.0.tgz` with private npmrc/cache | exit 0, `oa4-install.log` |
| `.oa2-tools/oa4-installed/node_modules/.bin/multica-openai-sandbox --probe` | exit 0, protocol 1 / SDK 0.17.2, `oa4-installed-probe.log` |

The handler integration starts with **actual startTaskPluginHookMCP output**, adds
OA policy metadata and passes it through the real overlay merge. Two Node
processes connect through official SDK HTTP clients, invoke distinct successive
local daemon handlers, and persist/restore a real Unix workspace under the same
Multica token. Old/current handler call counts are exactly one each. Model input
and events use a labeled test driver; no credentialed model success is claimed.
A changed stable hook route fails CHECKPOINT_INVALID before execution; a changed
advertised schema with unchanged claimed policy fails MCP_POLICY_CHANGED before
the driver, with zero tool calls. SDK and Go cancellation/no-credential entry
checks remain passing.

## Remaining boundaries

- Credentialed model streaming/tool choice/continuation E2E: **BLOCKED**, no key.
  MODEL_CREDENTIALS remains the expected negative real-entry result.
- Full remote broker, deployed service/TLS startup, full repository Go suite,
  browser acceptance and real DB migration: **not executed** this repair round.
  Neither handler-level HTTP success nor related tests imply these are green.
- Live RunState approval/interrupted resume, automatic stale-lock recovery,
  relocation and file/directory artifact replacement remain unsupported. Type
  replacement now rejects deterministically before mutation. Multi-file export
  is still not a filesystem transaction or fsync durability guarantee.
- Prior OA-2 two full-suite failures and DA full-package failures remain in their
  original evidence. OA-3 QA independently passed the corrected original 6 tests;
  this round's complete 11-test suite adds the review scenarios and also passes.
  No failure was hidden by a filtered final Node run.

All executed checks in this repair round passed; no failed-test retry was needed.
Write ownership is released after commit/push for independent QA/reviewer.
