"""Double-gated installed production entry smoke; no model credentials."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

if os.environ.get('MULTICA_RUN_REAL_AGENT_SMOKE') != '1':
    raise SystemExit('SKIP: real entry smoke disabled')
entry = Path(os.environ['MULTICA_DEEPAGENTS_TEST_ENTRY'])
if not entry.is_absolute():
    raise SystemExit('explicit installed entry required')
with tempfile.TemporaryDirectory() as tmp:
    root = Path(tmp)
    env = dict(HOME=tmp, PATH=str(entry.parent)+':/usr/bin:/bin', DEEPAGENTS_HOME=str(root/'profile'), XDG_CONFIG_HOME=tmp, XDG_CACHE_HOME=tmp, XDG_DATA_HOME=tmp, DO_NOT_TRACK='1')
    version = subprocess.run([entry, '--version'], cwd=tmp, env=env, capture_output=True, text=True, timeout=10)
    assert version.returncode == 0, version.returncode
    print('VERSION exit=0:', version.stdout.strip())
    policy=root/'policy.json';policy.write_text('{"required":{},"tools":{}}')
    request=root/'request.json'
    request.write_text(json.dumps(dict(schema='1',nonce='isolated-smoke',config='',policy=str(policy),receipt=str(root/'ready.json'),config_digest=hashlib.sha256(b'').hexdigest(),policy_digest=hashlib.sha256(policy.read_bytes()).hexdigest())))
    env['MULTICA_DEEPAGENTS_REQUEST']=str(request)
    run = subprocess.run([entry,'--acp'],input='',cwd=tmp,env=env,capture_output=True,text=True,timeout=20)
    assert run.returncode != 0
    assert 'credentials' in (run.stdout+run.stderr).lower(), 'expected missing model credentials'
    assert not (root/'ready.json').exists(), 'model creation must precede loader for pinned CLI'
    print('PRODUCTION --acp exit='+str(run.returncode)+': missing model credentials; no READY; model E2E BLOCKED')
