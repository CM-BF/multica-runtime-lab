"""Real guarded dcode loader/broker test; no model or replacement agent loop."""
import asyncio
import json
import os
from pathlib import Path
import sys
import urllib.request

from multica_dcode_acp import install_guard

guard = install_guard()
from deepagents_code.mcp_tools import resolve_and_load_mcp_tools


async def main():
    config_path = sys.argv[1]
    assert Path(config_path).stat().st_mode & 0o777 == 0o600
    config = json.loads(Path(config_path).read_text())
    assert len(config["mcpServers"]) == 1
    server_name, entry = next(iter(config["mcpServers"].items()))
    assert entry["type"] == "http" and entry["url"].startswith("http://127.0.0.1:")
    # The TLS trust and upstream credential terminate at the Go broker.
    assert "Authorization" not in json.dumps(config)
    assert not any(k in os.environ for k in ("OPENAI_API_KEY", "ANTHROPIC_API_KEY", "MULTICA_TASK_CONFIG_ROOT"))
    tools, manager, infos = await resolve_and_load_mcp_tools(explicit_config_path=config_path)
    try:
        assert guard.calls == 1
        receipt = json.loads(Path(guard.request["receipt"]).read_text())
        assert receipt["state"] == "READY" and receipt["pid"] == os.getpid()
        assert receipt["nonce"] == guard.request["nonce"] and receipt["loader_calls"] == 1
        originals = [(tool.metadata or {}).get("_deepagents_code_mcp_tool") for tool in tools]
        assert originals == ["fixture.read"], originals
        assert len(infos) == 1 and infos[0].status == "ok"
        assert (tools[0].metadata or {}).get("_deepagents_code_mcp_server") == server_name
        print("PASS: installed wrapper guard READY, official loader initialized once; tools/list exposes only fixture.read", flush=True)
        result = await tools[0].ainvoke({})
        assert "fixture-value" in str(result), str(result)
        print("PASS: official loader tool ainvoke -> broker -> trusted TLS upstream -> fixture-value", flush=True)
        # An adversarial call bypassing the filtered catalog must also be denied.
        request = urllib.request.Request(entry["url"], data=json.dumps({"jsonrpc": "2.0", "id": 999, "method": "tools/call", "params": {"name": "fixture.write", "arguments": {"value": "must-not-write"}}}).encode(), headers={"Content-Type": "application/json"})
        with urllib.request.urlopen(request, timeout=5) as response:
            denied = json.load(response)
        assert denied["error"]["code"] == -32602, denied
        print("PASS: direct forbidden tools/call rejected by broker policy", flush=True)
    finally:
        if manager:
            await manager.cleanup()
        print("PASS: official manager cleanup completed", flush=True)


asyncio.run(main())
