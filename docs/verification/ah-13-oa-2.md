# AH-13 / OA-2 implementation handoff

**BLOCKED overall; targeted implementation checks PASS.** This is a reviewable
candidate, not full runtime/model acceptance. Branch
`feat/ah-13-da-2-deepagents`, baseline `2699971b5af3c84f6bbdcd6d03af59ce13de4083`,
origin `CM-BF/multica-runtime-lab`. Final implementation commit is supplied in the
issue reply. No parent status change, delegation, merge or deployment.

## Changes

- New Go backend and separate TypeScript bridge, pinned official `@openai/agents`
  0.17.2. Production uses SDK Runner.run; test driver injection has no production
  CLI selector. No copied/reimplemented agent loop.
- Strict JSONL version/sequence/request correlation, bounded delivery and process
  supervision, cancellation independent of blocked stdin, safe error codes,
  private child environment and explicit model credentials.
- Separate opaque Multica session, immutable hashed SDK RunState/history/sandbox
  metadata and workspace archive generations. Clean continuation only; dirty
  state never silently replays. Artifact create/modify/delete with conflict checks.
- Explicit SDK stdio/HTTP MCP mapping, required connection/list check before run;
  shell capability and system context. Remote broker provider gate is not enabled.
- Additive factory/discovery/command inventory, UI family/display/MCP support,
  conversation state preparer, model manual-entry explanation, migration 460,
  installation and builtin runtime documentation. No historical migration edited
  and no database migration executed.
- Deep Agents implementation/test/bridge files unchanged. Shared Go maps have
  gofmt alignment changes and additive OA entries only.

## Versions and isolation

Node 23.11.0; npm 10.9.2; Go 1.26.6 darwin/arm64; TypeScript 5.9.3;
`@openai/agents`, agents-core, agents-openai and agents-realtime 0.17.2.
`oa-2/versions.log` records the complete installed dependency tree; npm lockfile
is committed. Official SDK constraints were checked against OA-1 and the locally
installed pinned SDK, with official guide
https://developers.openai.com/api/docs/guides/agents/sandboxes .

Commands below ran from the fork. `python3 .oa2-tools/run.py LOG COMMAND...`
creates a private temporary HOME/TMPDIR/XDG tree and a whitelist environment;
source is copied to `oa-2/run-isolated.py`. Only PATH and Go module/build caches
are reused; no account/runtime credentials or Multica config roots are inherited.
Real Go smoke additionally requires environment opt-in and verifies fork origin.
All real processes are owned test children, no fixed/service port or database.
Unix-local remains host execution, not OS security isolation.

## Final checks

Full raw output and exit codes are in `docs/verification/oa-2/`.

| Command (inside isolation wrapper) | Result | Log |
| --- | --- | --- |
| `npm --prefix runtime-bridges/openai-sandbox run build` | PASS, exit 0 | `build-cleanup.log` |
| `.da2-tools/go/bin/go -C server test ./pkg/agent ./internal/daemon ./internal/daemon/execenv -run 'Test(OpenAISandbox\|DeepAgents\|ShouldRetryWithFreshSession_UnresumableHistoryIsBackendAgnostic)' -count=1 -timeout=90s -v` | PASS, exit 0; 31 top-level tests plus subtests | `go-final.log` |
| `node --test --test-name-pattern 'real SDK explicit\|required MCP\|real Unix cancellation\|artifact conflict\|events' runtime-bridges/openai-sandbox/test/bridge.test.mjs` | PASS, exit 0; 5 tests; excludes the unresolved cross-process case | `bridge-final-directed.log` |
| `env MULTICA_RUN_REAL_AGENT_SMOKE=1 .da2-tools/go/bin/go -C server test -tags=agentintegration ./pkg/agent -run '^TestOpenAISandboxReal' -count=1 -timeout=40s -v` | PASS, exit 0; 2 tests | `real-entry-final.log` |
| `pnpm --filter @multica/views exec vitest run runtimes/components/openai-sandbox-catalog.test.ts runtimes/components/deepagents-catalog.test.ts runtimes/components/runtime-profile-catalog.test.ts` | PASS, exit 0; 3 files / 12 tests | `ui.log` |
| `npm pack ./runtime-bridges/openai-sandbox --pack-destination <fork>/.oa2-tools` | PASS, exit 0; 6 files, no test/debug code | `pack-final.log` |
| `npm install --prefix <fork>/.oa2-tools/installed --ignore-scripts --no-audit --no-fund <fork>/.oa2-tools/multica-openai-sandbox-0.1.0.tgz` with empty private npmrc/cache | PASS, exit 0; 26 dependencies installed | `install-entry.log` |
| `.oa2-tools/installed/node_modules/.bin/multica-openai-sandbox --probe` | PASS, exit 0; protocol 1 / SDK 0.17.2 | `installed-probe.log` |
| `.da2-tools/go/bin/go -C server vet ./pkg/agent ./internal/daemon ./internal/daemon/execenv` | PASS, exit 0 | `vet.log` |
| Source-only `git diff --check` and deliverable exclusions | PASS, exit 0; no private/build artifacts in deliverable list | `audit.log` |

The Go tests verify 400 text/tool-use/tool-result triplets with slow consumption,
nonconsumption failure, cancellation while stdin is blocked, descendant reaping,
invalid version/EOF/hanging terminal, discovery and private state identities.
Real production entry imports the SDK and reports `MODEL_CREDENTIALS` without a
key; this verifies safe failure, **not a model E2E PASS**. A separate test-only
entry starts an actual Unix SDK shell descendant under the Go process group and
proves bounded cancellation/reaping. SDK stdio MCP list/call uses a real local
JSON-RPC handler; model events in bridge unit tests are simulated and labeled.

## Preserved failures and acceptance gaps

1. **Snapshot/recovery final acceptance BLOCKED.** Both full Node test attempts
   exited 1 (`bridge-tests-1.log`, `bridge-tests-2.log`), each 5/6 passed. The
   cross-process case reached the second real shell execution, but its simulated
   assistant history used a string instead of the SDK's assistant message schema;
   RunState serialization rejected it. Test history is now corrected to a
   completed message with output_text content. Under the same-failure retry cap,
   it was **not rerun a third time**. Do not infer full recovery/export/corruption
   acceptance from the passing directed subset. QA must verify the corrected
   full test suite and real model continuation separately.
2. First cancellation fixture demonstrated SDK `session.stop()` waiting about
   60 seconds for a shell descendant holding pipes. Direct SDK test now covers a
   single exec process and completes in milliseconds; actual descendant safety
   is separately proven by the Go-supervised real SDK cancellation test. Standalone
   engine use has no equivalent process group supervision guarantee.
3. Initial `npm --prefix ... pack` targeted the repository cwd and failed with
   ENOENT during temporary test cleanup, exit 254 (`pack.log`). It was corrected
   once to explicit `npm pack ./runtime-bridges/openai-sandbox`; install/probe pass.
   No root-repository package was delivered.
4. **Model E2E BLOCKED:** no explicitly available credentials. Real model streaming,
   shell/tool choice, output generation and continuation remain unexecuted. Test
   streams/driver fixtures do not substitute for those checks.
5. Platform plugin HTTP handler and full remote broker round trips are unexecuted
   for OA; broker gate intentionally remains unsupported. Explicit stdio MCP
   round trip is verified; HTTP configuration uses the SDK class but does not yet
   have a real handler integration test. Skills/whole-checkout import, live
   approval RunState resume, interrupted-run automatic recovery and relocation
   are unsupported and documented.
6. Full Go suite, deployed service startup/TLS, DB migration execution and browser
   interaction were not run. Earlier DA full-suite state-root/agent timeout
   failures remain in the prior DA evidence and are not relabeled green by this
   directed regression. Installation probe and catalog tests are not a deployed
   service acceptance claim.

Private `.oa2-tools/`, `.da2-tools/`, node_modules, dist, Python egg-info and npm
archives are excluded from source delivery. Source package, lockfile, tests and
raw verification logs are committed. Write ownership is released for QA/reviewer;
no automatic further implementation round is started.

Raw failure logs preserve the test runner output, including four trailing-space
lines. The all-files staged whitespace check flagged those log lines; the
source-only check passes. Logs were not normalized to hide the original output.
