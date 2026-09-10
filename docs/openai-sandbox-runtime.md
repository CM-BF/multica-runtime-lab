# OpenAI Sandbox runtime

Status: OA-2 candidate, awaiting independent QA. Model E2E is blocked without
explicit model credentials. See `docs/verification/ah-13-oa-2.md` for the exact
checks and outstanding snapshot test verification.

The `openai-sandbox` family runs this fork's `multica-openai-sandbox` entry. It
wraps official `@openai/agents` **0.17.2**, `Runner.run`, `SandboxAgent` and
`UnixLocalSandboxClient`; it does not implement another agent loop. This is
OpenAI Sandbox Agent SDK, not Rivet and not ACP. Source/API reference:
https://developers.openai.com/api/docs/guides/agents/sandboxes

## Isolated installation

Requires Unix, Node >=22 and npm. Run from this fork, using a new private directory
for HOME, npm cache, configuration and installation prefix. Do not install over a
running Multica installation or use its configuration/database.

```sh
mkdir -m 700 .oa-install
: > .oa-install/npmrc
# Resolve these paths in your own checkout before using env -i.
env -i PATH="$PATH" HOME="$PWD/.oa-install" TMPDIR="$PWD/.oa-install" \
  npm_config_userconfig="$PWD/.oa-install/npmrc" \
  npm_config_globalconfig="$PWD/.oa-install/npmrc-global" \
  npm_config_cache="$PWD/.oa-install/cache" \
  npm --prefix runtime-bridges/openai-sandbox ci --ignore-scripts --no-audit --no-fund
env -i PATH="$PATH" HOME="$PWD/.oa-install" \
  npm --prefix runtime-bridges/openai-sandbox run build
env -i PATH="$PATH" HOME="$PWD/.oa-install" \
  npm pack ./runtime-bridges/openai-sandbox --pack-destination "$PWD/.oa-install"
env -i PATH="$PATH" HOME="$PWD/.oa-install" \
  npm_config_cache="$PWD/.oa-install/cache" \
  npm install --prefix "$PWD/.oa-install/runtime" --ignore-scripts --no-audit --no-fund \
  "$PWD/.oa-install/multica-openai-sandbox-0.1.0.tgz"
env -i PATH="$PATH" HOME="$PWD/.oa-install" \
  "$PWD/.oa-install/runtime/node_modules/.bin/multica-openai-sandbox" --probe
```

The ready record reports protocol 1 and SDK 0.17.2. Configure a runtime profile
with family `openai-sandbox` and the absolute installed executable. Auto-discovery
recognizes `multica-openai-sandbox` on PATH or `MULTICA_OPENAI_SANDBOX_PATH`, with
optional default `MULTICA_OPENAI_SANDBOX_MODEL`. Both direct PATH discovery and
the login-shell command inventory include this name. Choose an explicit model ID;
no credential-free model catalog is claimed. UI family/catalog and migration 460
are additive; the migration has not been applied to any existing database.

## Execution and credentials

The daemon prepares `openai-sandbox-state/<identity hash>` under its selected
profile, scoped by workspace/runtime/agent/conversation. It sets
`OPENAI_SANDBOX_STATE` after custom environment layering. Direct Backend callers
must supply an absolute private state root in Config.Env.

The Go child environment starts empty except for PATH and private HOME, TMPDIR,
XDG and snapshot directories. It never inherits account tokens, Multica task
configuration or provider credentials. Only an explicitly configured agent
`OPENAI_API_KEY` is passed to the model provider; the bridge removes it from
process.env before creating shell or MCP processes. Tracing is disabled. Do not
put keys in arguments, prompts, input files or artifact directories. Raw SDK
errors and stderr are not returned as user-visible errors.

Unix-local is host execution, **not a security sandbox/chroot/VM**. Filesystem APIs
restrict workspace paths, but shell commands can access paths permitted to the
host user. Use a dedicated execution account/environment. The bridge does not
claim to protect other host files from deliberately executed code.

## Context, tools and outputs

System instructions are passed as SandboxAgent instructions. By default only
`AGENTS.md` is copied from the task cwd into the SDK manifest. Custom agent env
`OPENAI_SANDBOX_INPUTS` is a JSON array of relative files/directories to copy;
`OPENAI_SANDBOX_ARTIFACTS` similarly selects output paths, default `["artifacts"]`.
Inputs/outputs reject traversal and symlinks and are limited to 256 files/16 MiB.
Skills and arbitrary checkout files are not automatically imported.

The SDK owns shell tool execution. Explicit `ExecOptions.McpConfig.mcpServers`
stdio (`command`, `args`, `env`) and HTTP (`type:"http"`, `url`, `headers`) map to
official SDK MCP servers. Each must connect and list tools before Runner.run;
all explicit servers are required. No platform/user account MCP configuration is
imported. Remote broker provider gating has **not** been enabled for this family;
platform plugin HTTP and full remote broker end-to-end acceptance remain unproven.
A real local stdio handler list/call round trip is covered by tests.

On a successful turn, selected artifact files are copied back with hashes and
create/modify/delete metadata in the bridge result. Go consumers see the files in
the task cwd and existing task artifact handling. Concurrent host edits cause
ARTIFACT_CONFLICT before export. Multi-file export is not a filesystem transaction;
a crash mid-export leaves the session dirty and requires manual reconciliation.

Text/tool events use a bounded, cancelable delivery path. A consumer stalled for
2 seconds fails visibly rather than silently dropping events. Terminal frames
must be followed by process exit within 2 seconds. Cancellation starts its timer
before writing cancel to stdin; the Go owner forcibly reaps the process group
when TurnInterruptTimeout expires. This supervision is necessary because the
SDK's Unix stop alone may wait on descendants holding pipes open.

## State and recovery

Multica's session ID is an opaque 32-hex token, never an SDK RunState or a host
path. An immutable generation stores separate history, SDK RunState JSON,
serialized sandbox state and SDK workspace archive bytes (the file is named `workspace.tar`; the SDK bytes are actually its JSON
archive format).
A hashed manifest binds the generation to SDK, model, cwd, explicit MCP and path
selection. All persisted files are private. Snapshot storage uses explicit SDK
`noop` plus `persistWorkspace`/`hydrateWorkspace`; it never selects the SDK's
ambient default snapshot directory.

Only a clean generation may continue: hydrate workspace in a new SDK session,
then run saved history plus the new user prompt. SDK RunState is preserved for
audit; it is not treated as a Multica token or automatically resumed as an active
run. Approval interruptions, failed and canceled turns retain a dirty marker and
best-effort recovery archive. Missing/corrupt/dirty state fails closed, without
fresh-session replay. Live approval resume, automatic interrupted-run recovery,
and workspace relocation are not implemented. If archive persistence fails, the
workspace is retained for manual recovery. Go forced termination may also leave a
lock/dirty workspace; no automatic stale-lock deletion is attempted.

Model E2E, real model tool selection and real model continuation require explicit
credentials and remain BLOCKED. Tests with simulated model streams do not prove
these behaviors.
