#!/usr/bin/env python3
"""Reconstruct a read-only Wine history from pinned public evidence; never trains."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import uuid

COMMIT = '5c37e250896fbb67670e7a1897853f78baec144d'
REMOTE = 'https://github.com/m1ssha/scientific-mlops-core.git'

def require(ok, message):
    if not ok:
        raise ValueError(message)

def call(*args, cwd=None):
    return subprocess.run([str(x) for x in args], cwd=cwd, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE).stdout

def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def tree_hash(root):
    return {str(p.relative_to(root)): digest(p) for p in sorted(root.rglob('*')) if p.is_file()}

def no_symlinks(path):
    path = Path(os.path.abspath(path))
    for p in (path, *path.parents):
        require(not p.is_symlink(), 'symlink in path')
    return path

def safe_new(path, other_paths=()):
    path = Path(os.path.abspath(path))
    require(not path.exists() and not path.is_symlink(), 'work path already exists')
    no_symlinks(path.parent)
    require(path.parent.is_dir(), 'work parent must exist')
    for other in other_paths:
        other = no_symlinks(other).resolve(strict=True)
        require(path != other and path not in other.parents and other not in path.parents, 'overlapping paths')
    return path

def read(p):
    return json.loads(p.read_text())

def strict_equal(a, b):
    if type(a) in (int, float) and type(b) in (int, float):
        return a == b
    if type(a) is not type(b):
        return False
    if isinstance(a, dict):
        return a.keys() == b.keys() and all(strict_equal(a[k], b[k]) for k in a)
    if isinstance(a, list):
        return len(a) == len(b) and all(strict_equal(x, y) for x, y in zip(a, b))
    return a == b

def verify_evidence(core):
    e = core / 'examples/wine_catalog/evidence/issue20'
    manifest = read(e/'sha256.json')
    require(len(manifest)==27, 'unexpected evidence manifest')
    for name, h in manifest.items():
        require(not Path(name).is_absolute() and '..' not in Path(name).parts and re.fullmatch('[0-9a-f]{64}',h), 'unsafe manifest')
        require(digest(e/name)==h, 'evidence hash mismatch: '+name)
    imp, orig, receipts, req, repeat, report = [read(e/n) for n in ('import.json','original-records.json','original-receipts.json','repeat-request.json','repeat-record.json','repeat-report.json')]
    request_body = dict(req['body']); record_body = dict(repeat['body'])
    for body in (request_body, record_body):
        require(len({x['path'] for x in body['outputs']}) == len(body['outputs']), 'duplicate output path')
        body['outputs'] = sorted(body['outputs'], key=lambda x: x['path'])
    require(strict_equal(request_body, record_body) and req['repository_id']==imp['repository_id'], 'repeat request mismatch after core output normalization')
    for k in ('dataset','snapshot','run'):
        require(orig[k]['id']==receipts[k]['record_id']==imp[k]['record_id'], 'original receipt mismatch')
    require(orig['run']['body']['snapshot_id']==orig['snapshot']['id'] and req['body']['snapshot_id']==orig['snapshot']['id'] and req['body']['source_run_id']==orig['run']['id'], 'broken history link')
    require(report['run_id']==repeat['id'] and report['source_run_id']==orig['run']['id'] and report['snapshot_id']==orig['snapshot']['id'], 'repeat report mismatch')
    require(len(req['body']['outputs'])==5 and req['body']['evidence_source']=='caller_reported', 'unexpected repeat evidence')
    for output in req['body']['outputs']:
        ref=output['object']; name=Path(output['path']).name; data=e/'repeat-output'/name
        require(ref['algorithm']=='sha256' and digest(data)==ref['digest'] and data.stat().st_size==ref['size'], 'repeat output mismatch')
    return e, imp, orig, req, repeat

def setup(work, source=REMOTE, wheelhouse=None):
    other=[Path(__file__).resolve().parents[1]]
    if Path(source).exists(): other.append(Path(source))
    if wheelhouse: other.append(Path(wheelhouse))
    work=safe_new(work,other);work.mkdir(mode=0o700)
    core=work/'core';call('git','clone','--no-local','--no-checkout',source,core)
    call('git','checkout','--detach',COMMIT,cwd=core)
    require(call('git','rev-parse','HEAD',cwd=core).decode().strip()==COMMIT, 'wrong core commit')
    # Verify every staged file against the Git object, before executing any core code.
    files=call('git','ls-tree','-r','--name-only','-z',COMMIT,cwd=core).decode().split('\0')
    for name in filter(None,files):
        p=core/name;require(p.is_file() and not p.is_symlink() and p.read_bytes()==call('git','show',COMMIT+':'+name,cwd=core), 'staged core differs from Git object: '+name)
    e, imp, orig, req, repeat=verify_evidence(core)
    binary=work/'smlops';call('go','build','-o',binary,'./cmd/smlops',cwd=core)
    store=work/'store';store.mkdir();repo=uuid.uuid4().hex
    command=[sys.executable,core/'examples/wine_catalog/import_reference.py','--smlops',binary,'--store',store,'--repository-id',repo]
    if wheelhouse: command += ['--wheelhouse',Path(wheelhouse).resolve(strict=True)]
    imported=json.loads(call(*command,cwd=core)); snapshot=imported['snapshot']['record_id']; source_id=imported['run']['record_id']
    def cli(*args):return json.loads(call(binary,*args))
    new_snapshot=cli('snapshot','show','--store',store,'--id',snapshot)['record']['body']
    old_snapshot=orig['snapshot']['body']
    require(new_snapshot['profile_id']==old_snapshot['profile_id'] and new_snapshot['parameters']==old_snapshot['parameters'] and new_snapshot['environment']['conditions']==old_snapshot['environment']['conditions'] and new_snapshot['code']==old_snapshot['code'], 'imported snapshot differs from historical conditions')
    for output in req['body']['outputs']:
        ref=output['object'];path=e/'repeat-output'/Path(output['path']).name
        cli('object','put','--store',store,'--file',path,'--sha256',ref['digest'],'--size',ref['size'])
    body=req['body'];body['snapshot_id']=snapshot;body['source_run_id']=source_id
    for observation in body['observations'].values():
        observation['evidence']='Historical caller-reported observation imported from pinned Wine evidence; not a current verification.'
    request={'schema_version':1,'repository_id':repo,'operation_id':uuid.uuid4().hex,'type':'register_run','body':body,'ref_update':None}
    request_file=work/'repeat-import.json';request_file.write_text(json.dumps(request))
    receipt=cli('run','register','--store',store,'--request',request_file);repeat_id=receipt['record_id']
    plan=cli('recovery','plan','--store',store,'--run',source_id)
    require(all(plan[x]=='unchecked' for x in ('runtime_ready','execution','numerical_agreement')), 'unexpected plan status')
    if not wheelhouse:
        external=[x for x in plan['materials'] if x['source']['kind']=='external']
        require(len(external)==7 and all(x['check']=='unchecked' for x in external) and not plan['stored_materials_ready'], 'external dependencies mismatch')
    before=tree_hash(store);missing=work/'missing-store';shutil.copytree(store,missing,copy_function=shutil.copy2)
    item=next(x for x in plan['materials'] if x['purpose']=='input' and x['required'] and x['source']['kind']=='object')
    obj=item['source']['object'];d=obj['digest'];require(obj['algorithm']=='sha256' and re.fullmatch('[0-9a-f]{64}',d), 'invalid input digest')
    target=missing/'objects'/'sha256'/d;require(target.is_file() and not target.is_symlink() and digest(target)==d and target.stat().st_size==obj['size'],'copied input mismatch');target.unlink()
    missing_plan=cli('recovery','plan','--store',missing,'--run',source_id);require(any(x['material_key']==item['key'] and x['code']=='missing' for x in missing_plan['blockers']), 'missing blocker absent')
    require(before==tree_hash(store), 'source store changed')
    empty=work/'empty-store';empty.mkdir();cli('repository','init','--store',empty,'--repository-id',uuid.uuid4().hex)
    metadata={'core_commit':COMMIT,'evidence_manifest_sha256':digest(e/'sha256.json'),'historical_catalog':{'repository_id':imp['repository_id'],'source_run_id':orig['run']['id'],'snapshot_id':orig['snapshot']['id'],'repeat_run_id':repeat['id']},'pipeline_uuids':{'source':imp['provenance']['original_run_id'],'repeat':read(e/'repeat-output/result.json')['run_id']},'reconstructed_catalog':{'repository_id':repo,'source_run_id':source_id,'snapshot_id':snapshot,'repeat_run_id':repeat_id},'modified_fields':['repository_id','operation_id','body.snapshot_id','body.source_run_id','body.observations.*.evidence'],'normalization':'core orders outputs by path', 'meaning':'historical caller-reported import; no current training or runtime verification','wheelhouse_supplied':bool(wheelhouse),'missing_material_key':item['key'],'source_store_sha256':before}
    (work/'demo-metadata.json').write_text(json.dumps(metadata,indent=2,sort_keys=True)+'\n');return metadata

def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--work',required=True);parser.add_argument('--core-source',default=REMOTE);parser.add_argument('--wheelhouse');args=parser.parse_args()
    try: print(json.dumps(setup(args.work,args.core_source,args.wheelhouse),sort_keys=True))
    except (ValueError,OSError,subprocess.CalledProcessError) as exc:
        print('demo setup failed: '+str(exc),file=sys.stderr);return 1
    return 0
if __name__=='__main__':sys.exit(main())
