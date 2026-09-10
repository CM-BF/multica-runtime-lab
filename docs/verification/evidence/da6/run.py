import os,pathlib,subprocess,sys,tempfile
root=pathlib.Path.cwd()
with tempfile.TemporaryDirectory(prefix='private-',dir=root/'.oa2-tools') as home:
 env={'PATH':os.environ['PATH'],'HOME':home,'USERPROFILE':home,'TMPDIR':home,'XDG_CONFIG_HOME':home,'XDG_CACHE_HOME':home,'XDG_DATA_HOME':home,'OPENAI_AGENTS_DISABLE_TRACING':'1','GOPATH':'/Users/citrine/go','GOMODCACHE':'/Users/citrine/go/pkg/mod','GOCACHE':'/Users/citrine/Library/Caches/go-build','GOTOOLCHAIN':'auto'}
 with open(sys.argv[1],'w') as log:
  result=subprocess.run(sys.argv[2:],env=env,stdout=log,stderr=subprocess.STDOUT)
  log.write('\nEXIT_CODE='+str(result.returncode)+'\n')
 print(sys.argv[1], 'exit',result.returncode)
 sys.exit(result.returncode)
