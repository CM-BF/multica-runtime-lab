import pathlib, tempfile, subprocess, tarfile, io, json
out=pathlib.Path.cwd(); repo=pathlib.Path('/Users/citrine/Projects/multica-runtime-lab')
with tempfile.TemporaryDirectory(prefix='da14-review-',dir='/tmp') as d:
 root=pathlib.Path(d)
 env={'PATH':'/usr/bin:/bin:/usr/sbin:/sbin','GOTOOLCHAIN':'local','GOMODCACHE':'/Users/citrine/go/pkg/mod','GOPROXY':'off','GOSUMDB':'off'}
 for key in ['HOME','XDG_CONFIG_HOME','XDG_DATA_HOME','XDG_CACHE_HOME','TMPDIR','GOTMPDIR','GOPATH','GOCACHE']:
  p=root/key;p.mkdir();env[key]=str(p)
 archive=subprocess.check_output(['/usr/bin/git','-C',str(repo),'archive','c97eb88269ac6c737df8a4da887b3d3d9bb1ccce'])
 with tarfile.open(fileobj=io.BytesIO(archive)) as t:t.extractall(root,filter='data')
 (root/'server/internal/cli/da14_review_test.go').write_text('''package cli
import("encoding/json";"os";"path/filepath";"testing")
func TestDA14UnknownPreservation(t *testing.T){
 h:=t.TempDir();t.Setenv("HOME",h)
 p:=filepath.Join(h,".multica","config.json");if e:=os.MkdirAll(filepath.Dir(p),0700);e!=nil{t.Fatal(e)}
 if e:=os.WriteFile(p,[]byte(`{"server_url":"https://synthetic.invalid","future_top":{"canary":1},"backends":{"future":{"canary":2}}}`),0600);e!=nil{t.Fatal(e)}
 c,e:=LoadCLIConfig();if e!=nil{t.Fatal(e)}
 c.ProfileCommandOverrides=map[string]string{"deepagents":"/synthetic/dcode"}
 if e=SaveCLIConfig(c);e!=nil{t.Fatal(e)}
 b,e:=os.ReadFile(p);if e!=nil{t.Fatal(e)};var m map[string]json.RawMessage;if e=json.Unmarshal(b,&m);e!=nil{t.Fatal(e)}
 t.Logf("synthetic saved config: %s",b)
 for _,k:=range []string{"future_top","backends"}{if _,ok:=m[k];!ok{t.Errorf("unknown key lost: %s",k)}}
}
''')
 cmd=['/Users/citrine/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.6.darwin-arm64/bin/go','-C',str(root/'server'),'test','./internal/cli','-run','^TestDA14UnknownPreservation$','-count=1','-v','-timeout=60s']
 (out/'da14-command.json').write_text(json.dumps({'cmd':cmd,'env':env},indent=2))
 with (out/'da14-config.log').open('w') as log:r=subprocess.run(cmd,cwd=root,env=env,stdout=log,stderr=subprocess.STDOUT,timeout=300)
 (out/'da14-config.exit').write_text(str(r.returncode));print('config preservation test exit',r.returncode)
