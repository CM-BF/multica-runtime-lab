import hashlib
import json
import os
from pathlib import Path
import tempfile
import types
import unittest
from multica_dcode_acp import Guard

class Manager:
    def __init__(self): self.cleaned = 0
    async def cleanup(self): self.cleaned += 1

class GuardTests(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def make(self, policy):
        config = self.root / 'mcp.json'
        config.write_text('{"mcpServers":{"required":{"command":"fixture"}}}')
        p = self.root / 'policy.json'; p.write_text(json.dumps({"required":policy,"tools":{}}))
        req = dict(schema='1', nonce='test-nonce', config=str(config), policy=str(p), receipt=str(self.root/'ready.json'), config_digest=hashlib.sha256(config.read_bytes()).hexdigest(), policy_digest=hashlib.sha256(p.read_bytes()).hexdigest())
        path=self.root/'request.json';path.write_text(json.dumps(req))
        return Guard(path),config

    async def test_required_matrix_cleanup_and_safe_receipt(self):
        for status in ('error','unauthenticated','awaiting_reconnect','disabled','unknown','missing','duplicate'):
            with self.subTest(status=status):
                guard,config=self.make({'required':True})
                manager=Manager()
                info=types.SimpleNamespace(name='required',status=status,error='SECRET_URL_AUTH_ENV_SENTINEL')
                infos=[] if status=='missing' else [info,info] if status=='duplicate' else [info]
                calls=0
                async def original(explicit_config_path=None):
                    nonlocal calls
                    calls+=1
                    return [],manager,infos
                with self.assertRaisesRegex(RuntimeError,'MCP_REQUIRED_UNREADY'):
                    await guard.wrap(original)(explicit_config_path=str(config))
                self.assertEqual(calls,1);self.assertEqual(manager.cleaned,1)
                raw=(self.root/'ready.json').read_text()
                self.assertNotIn('SENTINEL',raw)
                self.assertEqual(json.loads(raw)['state'],'FAIL')

    async def test_optional_and_object_identity_single_loader(self):
        guard,config=self.make({'required':True,'optional':False})
        manager=Manager();tools=[object()]
        infos=[types.SimpleNamespace(name='required',status='ok',error=None),types.SimpleNamespace(name='optional',status='error',error='secret')]
        original_result=(tools,manager,infos)
        async def original(explicit_config_path=None, **kwargs):
            self.assertEqual(kwargs,{'trust_project_mcp':None})
            return original_result
        wrapped=guard.wrap(original)
        result=await wrapped(explicit_config_path=str(config),trust_project_mcp=None)
        self.assertIs(result,original_result);self.assertEqual(manager.cleaned,0)
        self.assertEqual(json.loads((self.root/'ready.json').read_text())['degraded'],1)
        with self.assertRaisesRegex(RuntimeError,'BRIDGE_VERSION'):
            await wrapped(explicit_config_path=str(config))
        self.assertEqual(guard.calls,1)

    async def test_config_mutation_and_loader_exception_are_safe(self):
        for mutate in (False,True):
            guard,config=self.make({'required':True})
            if mutate: config.write_text('SECRET_MUTATION')
            async def original(explicit_config_path=None):raise ValueError('SECRET_EXCEPTION')
            with self.assertRaisesRegex(RuntimeError,'MCP_CONFIG_INVALID'):
                await guard.wrap(original)(explicit_config_path=str(config))
            self.assertNotIn('SECRET',(self.root/'ready.json').read_text())

if __name__ == '__main__': unittest.main()
