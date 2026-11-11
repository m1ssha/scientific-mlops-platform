import json
import hashlib
import os
from pathlib import Path
import shutil
import tempfile
import unittest
import wine_demo as d

class SafetyTests(unittest.TestCase):
    def test_existing_overlap_and_symlink(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);existing=root/'exists';existing.mkdir()
            with self.assertRaises(ValueError): d.safe_new(existing)
            with self.assertRaises(ValueError): d.safe_new(existing/'child',[existing])
            link=root/'link';link.symlink_to(existing,target_is_directory=True)
            with self.assertRaises(ValueError): d.safe_new(link/'child')
    def test_bool_never_equals_number(self):
        self.assertFalse(d.strict_equal({'n':True},{'n':1}))
        self.assertTrue(d.strict_equal({'n':1},{'n':1.0}))
        self.assertTrue(d.strict_equal({'n':[1,2]},{'n':[1,2]}))
    def test_tampered_evidence(self):
        source=Path(os.environ.get('WINE_CORE_SOURCE',Path(__file__).resolve().parents[2]/'scientific-mlops-core'))/'examples/wine_catalog/evidence/issue20'
        if not source.exists(): self.skipTest('set up full demo for evidence test')
        with tempfile.TemporaryDirectory() as tmp:
            core=Path(tmp);dest=core/'examples/wine_catalog/evidence/issue20';dest.parent.mkdir(parents=True);shutil.copytree(source,dest)
            d.verify_evidence(core)
            with (dest/'repeat-output/model.json').open('ab') as f:f.write(b'tamper')
            with self.assertRaises(ValueError): d.verify_evidence(core)
            shutil.copyfile(source/'repeat-output/model.json',dest/'repeat-output/model.json')
            request=dest/'repeat-request.json'; value=json.loads(request.read_text()); value['body']['outcome']='failed'; request.write_text(json.dumps(value))
            manifest_path=dest/'sha256.json';manifest=json.loads(manifest_path.read_text());manifest['repeat-request.json']=hashlib.sha256(request.read_bytes()).hexdigest();manifest_path.write_text(json.dumps(manifest))
            with self.assertRaises(ValueError): d.verify_evidence(core)
if __name__=='__main__':unittest.main()
