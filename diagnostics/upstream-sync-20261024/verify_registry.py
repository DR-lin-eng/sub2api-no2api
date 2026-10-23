#!/usr/bin/env python3
"""Read public GHCR manifests/config labels without persisting registry tokens."""
import json,ssl,sys,urllib.request
from pathlib import Path
revision=sys.argv[1];out=Path(sys.argv[2]);out.parent.mkdir(parents=True,exist_ok=True)
repository='dr-lin-eng/sub2api-no2api';ctx=ssl.create_default_context(cafile='/etc/ssl/cert.pem')
def request(url,headers=None):
 with urllib.request.urlopen(urllib.request.Request(url,headers=headers or {}),context=ctx,timeout=40) as response:return json.load(response),dict(response.headers)
token,_=request('https://ghcr.io/token?service=ghcr.io&scope=repository:'+repository+':pull')
headers={'Authorization':'Bearer '+token['token'],'Accept':'application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json'}
def registry(kind,ref):return request('https://ghcr.io/v2/'+repository+'/'+kind+'/'+ref,headers)
report={'revision':revision,'tags':{}}
for tag in ['sha-'+revision[:7],'latest']:
 index,h=registry('manifests',tag);platforms={}
 for entry in index.get('manifests',[]):
  platform=entry.get('platform',{});arch=platform.get('architecture')
  if platform.get('os')!='linux' or arch not in ['amd64','arm64']:continue
  manifest,_=registry('manifests',entry['digest']);config,_=registry('blobs',manifest['config']['digest'])
  labels=config.get('config',{}).get('Labels',{});actual=labels.get('org.opencontainers.image.revision')
  assert actual==revision,(tag,arch,actual)
  platforms[arch]={'manifest_digest':entry['digest'],'config_digest':manifest['config']['digest'],'revision':actual}
 assert set(platforms)=={'amd64','arm64'},(tag,platforms)
 digest=next(v for k,v in h.items() if k.lower()=='docker-content-digest')
 report['tags'][tag]={'digest':digest,'platforms':platforms}
assert len({v['digest'] for v in report['tags'].values()})==1
out.write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))
