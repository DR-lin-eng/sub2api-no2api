#!/usr/bin/env python3
import json, pathlib, subprocess, sys
ROOT=pathlib.Path(__file__).resolve().parent.parent
E=ROOT/'evidence'; R=ROOT/'repo'
def run(name,command,cwd=R):
 record=E/'records'/(name+'.record.json')
 if '--resume' in sys.argv and record.exists() and json.loads(record.read_text())['exit_status']==0:
  print('REUSE '+name,flush=True);return
 print('START '+name,flush=True)
 status=subprocess.run([sys.executable,str(E/'run_record.py'),name,*command],cwd=cwd).returncode
 if status:raise SystemExit(status)
 print('PASS '+name,flush=True)
def go(tree):
 return ['docker','run','--rm','--cpus','2','--memory','3g','-e','GOMAXPROCS=2','-e','GOFLAGS=-p=1','-e','GOMEMLIMIT=1500MiB','-v',str(ROOT/tree/'backend')+':/workspace','-v','sub2api-gomod-20261009:/go/pkg/mod','-v','sub2api-gobuild-20261013:/root/.cache/go-build','-w','/workspace','sub2api-upstream-sync-test:20261009-docker']
run('baseline-behavior-bounded',go('baseline')+['go','test','-tags','unit','-run','^TestUpstream20261013ObservedBehavior$','-count=1','-v','./internal/application/service'],ROOT/'baseline')
run('modified-behavior',go('repo')+['go','test','-tags','unit','-run','^TestUpstream20261013ObservedBehavior$','-count=1','-v','./internal/application/service'])
run('rollback-behavior',go('rollback')+['go','test','-tags','unit','-run','^TestUpstream20261013ObservedBehavior$','-count=1','-v','./internal/application/service'],ROOT/'rollback')
run('backend-unit-bounded',go('repo')+['make','test-unit'])
cmd=go('repo');position=cmd.index('-w');cmd[position:position]=['-e','CI=true','-e','DOCKER_HOST=unix:///var/run/docker.sock','-e','TESTCONTAINERS_HOST_OVERRIDE=host.docker.internal','-v','/var/run/docker.sock:/var/run/docker.sock']
run('backend-integration',cmd+['make','test-integration'])
run('backend-race',go('repo')+['go','test','-race','-tags','unit','-run','Test(.*TempUnsched|.*StructuredInput|.*RawFallbackPreserves|.*ChatFallbackPreserves|.*AntigravityImageDoesNot|.*ToolResultImages|GetCodingPlanProvider)','-count=1','./internal/application/service'])
cmd=go('repo');position=cmd.index('-w');cmd[position:position]=['-v','sub2api-sync-tools-20261009:/tools']
run('backend-lint',cmd+['/tools/golangci-lint','run','--timeout=30m','./...'])
run('artifact-final-reconstruction',[sys.executable,str(E/'verify_roles.py')])
run('rollback-behavior-final',go('rollback')+['go','test','-tags','unit','-run','^TestUpstream20261013ObservedBehavior$','-count=1','-v','./internal/application/service'],ROOT/'rollback')
print('SERIAL_BACKEND_VALIDATION_COMPLETE',flush=True)
