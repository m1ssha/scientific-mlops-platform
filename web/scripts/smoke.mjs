import { chromium } from '@playwright/test';
import { mkdir } from 'node:fs/promises';
const base = process.env.UI_BASE_URL;
const evidence = process.env.UI_EVIDENCE_DIR;
if (!base || !evidence) throw new Error('UI_BASE_URL and UI_EVIDENCE_DIR are required');
await mkdir(evidence,{recursive:true});
const browser = await chromium.launch({headless:true, ...(process.env.UI_CHROMIUM ? {executablePath:process.env.UI_CHROMIUM}: {})});
try {
 const context=await browser.newContext({viewport:{width:1440,height:1000}}); const page=await context.newPage(); const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(base,{waitUntil:'networkidle'});await page.getByRole('heading',{name:'История экспериментов'}).waitFor();
 const open=page.getByRole('button',{name:'Открыть'});if(await open.count()<2)throw new Error('At least two real runs required');
 await open.first().click();await page.getByRole('heading',{name:'Метрики'}).waitFor();await page.getByRole('button',{name:'Показать план восстановления'}).click();await page.getByRole('heading',{name:'План восстановления'}).waitFor();
 const selectors=page.locator('select');await selectors.nth(0).selectOption({index:1});await selectors.nth(1).selectOption({index:2});await page.getByRole('button',{name:'Сравнить',exact:true}).click();await page.getByText('Сравнение сохранённых данных, не проверка численного совпадения.').waitFor();
 await page.evaluate(()=>window.scrollTo(0,0)); await page.screenshot({path:evidence+'/desktop.png',fullPage:true}); await page.screenshot({path:evidence+'/desktop-viewport.png'});await page.setViewportSize({width:390,height:844}); await page.evaluate(()=>window.scrollTo(0,0)); await page.screenshot({path:evidence+'/mobile.png',fullPage:true}); await page.screenshot({path:evidence+'/mobile-viewport.png'});
 const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth);if(overflow)throw new Error('mobile horizontal overflow');
 await page.goto(base,{waitUntil:'networkidle'}); await page.keyboard.press('Tab');if(!await page.evaluate(()=>document.activeElement !== document.body))throw new Error('keyboard focus missing');
 if(errors.length)throw new Error(errors.join('\n')); console.log('Real API browser smoke passed; screenshots:',evidence);
} finally {await browser.close()}
