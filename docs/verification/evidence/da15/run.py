import pathlib,tempfile,subprocess,tarfile,io,json,shutil
repo=pathlib.Path.cwd();out=repo/'docs/verification/evidence/da15';out.mkdir(parents=True,exist_ok=True)
with tempfile.TemporaryDirectory(prefix='da15-',dir='/tmp') as d:
 root=pathlib.Path(d)
 archive=subprocess.check_output(['git','archive','HEAD'])
 with tarfile.open(fileobj=io.BytesIO(archive)) as t:t.extractall(root,filter='data')
 for p in ['server/internal/cli/config.go','server/internal/cli/config_json.go','server/internal/cli/config_test.go','server/cmd/multica/cmd_runtime_profile_test.go']:shutil.copyfile(repo/p,root/p)
 env={'PATH':'/usr/bin:/bin:/usr/sbin:/sbin','GOTOOLCHAIN':'local','GOMODCACHE':'/Users/citrine/go/pkg/mod','GOPROXY':'off','GOSUMDB':'off'}
 for k in ['HOME','USERPROFILE','XDG_CONFIG_HOME','XDG_CACHE_HOME','XDG_DATA_HOME','TMPDIR','GOTMPDIR','GOPATH','GOCACHE']:
  p=root/k;p.mkdir();env[k]=str(p)
 go='/Users/citrine/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.6.darwin-arm64/bin/go'
 commands=[('version',[go,'version']),('config',[go,'test','-race','./internal/cli','-count=1','-v','-timeout=120s']),('paths',[go,'test','-race','./cmd/multica','-run','RuntimeProfile|ConfigSet','-count=1','-v','-timeout=120s']),('vet',[go,'vet','./internal/cli','./cmd/multica'])]
 (out/'commands.json').write_text(json.dumps({'commands':commands,'env':env,'cwd':str(root/'server')},indent=2))
 for name,cmd in commands:
  with (out/(name+'.log')).open('w') as f:
   r=subprocess.run(cmd,cwd=root/'server',env=env,stdout=f,stderr=subprocess.STDOUT);f.write('\nEXIT_CODE='+str(r.returncode)+'\n')
  print(name,r.returncode,flush=True)
  if r.returncode:break
print('private source/state/cache removed',flush=True)
