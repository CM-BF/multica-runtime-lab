"""Version-pinned MCP readiness guard; the official CLI owns the agent loop."""
import asyncio
import hashlib
import importlib.metadata
import inspect
import json
import os
from pathlib import Path
import sys
import tempfile

SCHEMA = "1"
VERSIONS = {"deepagents-code": "0.1.68", "deepagents-acp": "0.0.11", "agent-client-protocol": "0.12.1"}


def verify_versions():
    try:
        if any(importlib.metadata.version(p) != v for p, v in VERSIONS.items()):
            raise RuntimeError("BRIDGE_VERSION")
    except importlib.metadata.PackageNotFoundError:
        raise RuntimeError("DEPENDENCY_MISSING") from None


class Guard:
    def __init__(self, request_path):
        self.request = json.loads(Path(request_path).read_bytes())
        self.calls = 0
        self.policy = None
        if self.request.get("schema") != SCHEMA:
            raise RuntimeError("BRIDGE_VERSION")

    def receipt(self, state, code="", degraded=0):
        # Do not serialize upstream metadata, identifiers, paths or exceptions.
        data = {k: self.request[k] for k in ("schema", "nonce", "config_digest", "policy_digest")}
        data.update(pid=os.getpid(), state=state, code=code, loader_calls=self.calls, degraded=degraded)
        path = Path(self.request["receipt"])
        fd, temporary = tempfile.mkstemp(dir=path.parent)
        try:
            with os.fdopen(fd, "w") as stream:
                json.dump(data, stream)
            os.replace(temporary, path)
        finally:
            if os.path.exists(temporary):
                os.unlink(temporary)

    def validate_input(self, kwargs):
        config_path = self.request["config"]
        if str(kwargs.get("explicit_config_path") or "") != config_path or kwargs.get("no_mcp"):
            raise RuntimeError("MCP_CONFIG_INVALID")
        raw = Path(config_path).read_bytes() if config_path else b""
        policy_raw = Path(self.request["policy"]).read_bytes()
        if hashlib.sha256(raw).hexdigest() != self.request["config_digest"] or hashlib.sha256(policy_raw).hexdigest() != self.request["policy_digest"]:
            raise RuntimeError("MCP_CONFIG_INVALID")
        policy = json.loads(policy_raw)
        if not isinstance(policy, dict) or not isinstance(policy.get("required"),dict) or any(not isinstance(v, bool) for v in policy.get("required",{}).values()):
            raise RuntimeError("MCP_CONFIG_INVALID")
        self.policy = policy["required"]
        self.required_tools = policy.get("tools") or {}

    def wrap(self, original):
        signature = inspect.signature(original)
        if "explicit_config_path" not in signature.parameters:
            raise RuntimeError("BRIDGE_VERSION")

        async def guarded(*args, **kwargs):
            manager = None
            try:
                if self.calls:
                    raise RuntimeError("BRIDGE_VERSION")
                bound = signature.bind(*args, **kwargs)
                self.validate_input(bound.arguments)
                self.calls += 1
                result = await original(*args, **kwargs)
                tools, manager, infos = result
                grouped = {}
                for info in infos or []:
                    grouped.setdefault(info.name, []).append(info)
                if any(getattr(info,"transport",None)=="config" and info.error is not None for info in infos or []):
                    raise RuntimeError("MCP_CONFIG_INVALID")
                for name, required in self.policy.items():
                    entries = grouped.get(name, [])
                    if required and (len(entries) != 1 or entries[0].status != "ok" or entries[0].error is not None):
                        raise RuntimeError("MCP_REQUIRED_UNREADY")
                for server, requirements in self.required_tools.items():
                    for original_name, schema in requirements.items():
                        matches = [tool for tool in tools if (tool.metadata or {}).get("_deepagents_code_mcp_server")==server and (tool.metadata or {}).get("_deepagents_code_mcp_tool")==original_name]
                        if len(matches)!=1:
                            raise RuntimeError("MCP_REQUIRED_UNREADY")
                        metadata = [entry for info in grouped.get(server,[]) for entry in info.tools if entry.name==matches[0].name]
                        if len(metadata)!=1 or metadata[0].input_schema!=schema:
                            raise RuntimeError("MCP_REQUIRED_UNREADY")
                degraded = sum(len(entries) != 1 or entries[0].status != "ok" or entries[0].error is not None for entries in grouped.values())
                degraded += sum(name not in grouped for name, required in self.policy.items() if not required)
                self.receipt("READY", degraded=degraded)
                return result  # Preserve identities and the official manager's ownership.
            except BaseException as exc:
                if manager is not None:
                    try:
                        await asyncio.wait_for(manager.cleanup(), timeout=2)
                    except BaseException:
                        pass  # The Go owner still bounds and reaps the process tree.
                code = str(exc) if isinstance(exc, RuntimeError) and str(exc) in {"MCP_REQUIRED_UNREADY", "BRIDGE_VERSION"} else "MCP_CONFIG_INVALID"
                self.receipt("FAIL", code)
                raise RuntimeError(code) from None
        return guarded


def install_guard():
    guard = Guard(os.environ["MULTICA_DEEPAGENTS_REQUEST"])
    try:
        verify_versions()
        import deepagents_code.mcp_tools as mcp
        mcp.resolve_and_load_mcp_tools = guard.wrap(mcp.resolve_and_load_mcp_tools)
    except Exception as exc:
        code = str(exc) if str(exc) in {"BRIDGE_VERSION", "DEPENDENCY_MISSING"} else "BRIDGE_VERSION"
        guard.receipt("FAIL", code)
        raise RuntimeError(code) from None
    return guard


def main():
    try:
        if sys.argv[1:] == ["--version"]:
            verify_versions()
            print("multica-dcode-acp 0.1.0 schema=1 deepagents-code=0.1.68 deepagents-acp=0.0.11")
            return
        install_guard()
        from deepagents_code.main import cli_main
        cli_main()
    except Exception:
        # Unknown exception messages can contain MCP URLs or credentials.
        print("deepagents: bridge startup failed", file=sys.stderr)
        raise SystemExit(1) from None
