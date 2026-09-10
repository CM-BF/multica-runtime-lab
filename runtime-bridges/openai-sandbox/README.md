# Multica OpenAI Sandbox bridge

This package wraps the official `@openai/agents` 0.17.2 `Runner` and
`UnixLocalSandboxClient`. It contains no agent loop. The entry is
`multica-openai-sandbox`; transport is versioned JSONL over stdin/stdout, not ACP.

Build with `npm ci --ignore-scripts && npm run build`. Install the built package
in an isolated npm prefix. See `docs/openai-sandbox-runtime.md` in this fork for
complete installation, private state, credentials, recovery and limitations.

`--probe` and `--version` validate the installed SDK version and print protocol
version/capabilities without creating a sandbox or calling a model.

Unix-local shell execution runs as the host user; it is not a chroot or a VM.
Use only on a dedicated execution account/machine suitable for the requested
code. The Go backend supplies private HOME/state and a minimal environment.

Tests use real SDK Unix workspaces and explicit local MCP handlers; simulated
model streams are labeled and do not constitute model E2E acceptance. The
production CLI has no test-driver selector.
