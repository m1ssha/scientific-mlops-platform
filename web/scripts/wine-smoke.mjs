import { chromium } from '@playwright/test';
import { spawn } from 'node:child_process';
import { readFile, mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
const work=process.env.WINE_WORK, binary=process.env.WINE_PLATFORM, evidence=process.env.UI_EVIDENCE_DIR;
if(!work||!binary||!evidence)throw Error('WINE_WORK, WINE_PLATFORM and UI_EVIDENCE_DIR required');
const meta=JSON.parse(await readFile(resolve(work,'demo-metadata.json'),'utf8'));
const ids=process.env.WINE_HISTORICAL_STORE ? meta.historical_catalog : meta.reconstructed_catalog;
const expected=JSON.parse(await readFile(resolve(work,'core/examples/wine_catalog/evidence/issue20/repeat-record.json'),'utf8'));
await mkdir(evidence,{recursive:true});
const browser=await chromium.launch({headless:true,...(process.env.UI_CHROMIUM?{executablePath:process.env.UI_CHROMIUM}:{})});
async function run(store,mode){
 const proc=spawn(binary,['--store',store,'--listen','127.0.0.1:0','--web-dir',resolve('dist/browser')]);let logs='';
 let page;
 try {
  const base=await new Promise((ok,fail)=>{const timer=setTimeout(()=>fail(Error('server startup timeout '+logs)),10000);proc.stderr.on('data',buf=>{logs+=buf;const match=logs.match(/listening on (http:\/\/127\.0\.0\.1:\d+)/);if(match){clearTimeout(timer);ok(match[1]);}});proc.once('error',e=>{clearTimeout(timer);fail(e)});proc.once('exit',()=>{clearTimeout(timer);fail(Error('server exited '+logs))});});
  page=await browser.newPage({viewport:{width:1440,height:1000}});const errs=[];page.on('pageerror',e=>errs.push(e.message));
  const api=async path=>{const r=await page.request.get(base+path);if(!r.ok())throw Error(`${path}: ${r.status()}`);return r.json()};
  const runs=await api('/api/v1/runs');await page.goto(base,{waitUntil:'networkidle'});
  if(mode==='empty'){if(runs.items.length!==0)throw Error('catalog not empty');await page.getByText('В истории пока нет запусков.').waitFor();return;}
  for(const id of [ids.source_run_id,ids.repeat_run_id])if(!runs.items.some(x=>x.id===id))throw Error('run missing '+id);
  const source=await api('/api/v1/runs/'+ids.source_run_id);const repeat=await api('/api/v1/runs/'+ids.repeat_run_id);
  if(source.body.snapshot_id!==repeat.body.snapshot_id || repeat.body.source_run_id!==ids.source_run_id)throw Error('history links mismatch');
  const metrics=['dummy.accuracy','dummy.balanced_accuracy','model.accuracy','model.balanced_accuracy'];for(const m of metrics)if(!repeat.body.metrics[m] || ['value','unit','definition'].some(k=>repeat.body.metrics[m][k]!==expected.body.metrics[m][k]))throw Error('metric mismatch '+m);
  const diff=await api(`/api/v1/runs/compare?left=${ids.source_run_id}&right=${ids.repeat_run_id}`);if(diff.left_id!==ids.source_run_id||diff.right_id!==ids.repeat_run_id||diff.kind!=='run')throw Error('diff mismatch');
  const plan=await api(`/api/v1/runs/${ids.source_run_id}/recovery`);for(const k of ['runtime_ready','execution','numerical_agreement'])if(plan[k]!=='unchecked')throw Error('plan '+k);
  if(mode==='historical' && (!plan.stored_materials_ready || !plan.materials.some(x=>x.purpose==='input' && x.required && x.check==='verified') || plan.materials.some(x=>x.required && x.check!=='verified')))throw Error('historical input readiness mismatch');
  if(mode==='missing' && (!plan.blockers.some(x=>x.code==='missing'&&x.material_key===meta.missing_material_key)||plan.stored_materials_ready))throw Error('missing blocker absent');
  if(mode==='demo'&&!meta.wheelhouse_supplied && (plan.stored_materials_ready||plan.materials.filter(x=>x.source.kind==='external'&&x.check==='unchecked').length!==7))throw Error('unchecked wheels mismatch');
  await page.locator('tr').filter({hasText:ids.repeat_run_id}).getByRole('button',{name:'Открыть'}).click();await page.getByRole('heading',{name:'Метрики'}).waitFor();for(const m of metrics){const row=page.locator('tr').filter({hasText:m});await row.getByText(m,{exact:true}).waitFor();await row.getByText(String(expected.body.metrics[m].value),{exact:true}).waitFor();}
  const detail=page.locator('section[aria-labelledby="detail"]');await detail.getByText(ids.source_run_id,{exact:true}).waitFor();await detail.getByText(ids.snapshot_id,{exact:true}).waitFor();
  await page.locator('tr').filter({hasText:ids.source_run_id}).getByRole('button',{name:'Открыть'}).click();await page.getByRole('button',{name:'Показать план восстановления'}).click();await page.getByRole('heading',{name:'План восстановления'}).waitFor();if(await detail.getByText('unchecked',{exact:true}).count()<3)throw Error('unchecked statuses not visible');
  if(mode==='missing')await page.getByText(new RegExp(meta.missing_material_key+': missing')).waitFor();
  await page.getByLabel('Первый запуск').selectOption(ids.source_run_id);await page.getByLabel('Второй запуск').selectOption(ids.repeat_run_id);await page.getByRole('button',{name:'Сравнить',exact:true}).click();await page.getByText('Сравнение сохранённых данных, не проверка численного совпадения.').waitFor();if(diff.changes.length)await page.getByText(diff.changes[0].path,{exact:true}).waitFor();
  await page.screenshot({path:resolve(evidence,mode+'.png'),fullPage:true});if(errs.length)throw Error(errs.join('\n'));
 } finally {
  try {if(page)await page.close();} finally {
   if(proc.pid && proc.exitCode===null && proc.signalCode===null){
    const exited=new Promise(resolve=>proc.once('exit',resolve));proc.kill('SIGTERM');
    const timer=setTimeout(()=>proc.kill('SIGKILL'),2000);try {await exited} finally {clearTimeout(timer)}
   }
  }
 }
}
try {if(process.env.WINE_HISTORICAL_STORE){await run(process.env.WINE_HISTORICAL_STORE,'historical');}else{await run(resolve(work,'store'),'demo');await run(resolve(work,'missing-store'),'missing');await run(resolve(work,'empty-store'),'empty');}console.log('Wine browser smoke passed');}finally{await browser.close()}
