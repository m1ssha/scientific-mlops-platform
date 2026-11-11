// @vitest-environment jsdom
import '@angular/compiler';
import { readFileSync } from 'node:fs';
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { provideZonelessChangeDetection, ɵresolveComponentResources } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { BrowserTestingModule, platformBrowserTesting } from '@angular/platform-browser/testing';
import { App } from './app';
const template=readFileSync('src/app/app.html','utf8');
const response=(body:unknown)=>({ok:true,json:async()=>body}) as Response;
const page={view_sequence:3,next_after_sequence:null,items:[]};
beforeAll(async()=>{await ɵresolveComponentResources(async()=>template); TestBed.initTestEnvironment(BrowserTestingModule,platformBrowserTesting())});
afterEach(()=>{TestBed.resetTestingModule();vi.unstubAllGlobals()});
async function mount(fetcher:any) {
 vi.stubGlobal('fetch',fetcher);TestBed.configureTestingModule({imports:[App],providers:[provideZonelessChangeDetection()]});await TestBed.compileComponents();const fixture=TestBed.createComponent(App);fixture.autoDetectChanges();return fixture;
}
describe('zoneless DOM',()=>{
 it('renders async empty state without a manual detection after the response',async()=>{
  let resolve!:(r:Response)=>void;const pending=new Promise<Response>(r=>resolve=r);
  const f=await mount(vi.fn().mockResolvedValueOnce(response({sequence:3})).mockReturnValueOnce(pending));expect(f.nativeElement.textContent).toContain('Загрузка истории');resolve(response(page));await vi.waitFor(()=>expect(f.nativeElement.textContent).toContain('В истории пока нет запусков'));
 });
 it('renders unchecked and blockers as text and never interprets hostile content',async()=>{
  const attack='<img src=x onerror=alert(1)>';const fetcher=vi.fn().mockResolvedValueOnce(response({sequence:3})).mockResolvedValueOnce(response(page)).mockResolvedValueOnce(response({view_sequence:3,coverage:'declared_only',stored_materials_ready:false,runtime_ready:'unchecked',execution:'unchecked',numerical_agreement:'unchecked',blockers:[{material_key:'x',code:'missing',message:attack}],materials:[{path:attack,purpose:'input',required:true,check:'missing'}],actions:[]}));
  const f=await mount(fetcher);await vi.waitFor(()=>expect(f.componentInstance.listState).toBe('ready'));f.componentInstance.selected='x';await f.componentInstance.recovery();await vi.waitFor(()=>expect(f.nativeElement.textContent).toContain(attack));expect(f.nativeElement.textContent).toContain('unchecked');expect(f.nativeElement.querySelector('img')).toBeNull();
 });
 it('shows incomparable without inventing a zero delta and empty diff as structural only',async()=>{
  const fetcher=vi.fn().mockResolvedValueOnce(response({sequence:3})).mockResolvedValueOnce(response(page)).mockResolvedValueOnce(response({view_sequence:3,changes:[{path:'/metric',kind:'incomparable',left:1,right:2,delta:null}]})).mockResolvedValueOnce(response({view_sequence:3,changes:[]}));
  const f=await mount(fetcher);await vi.waitFor(()=>expect(f.componentInstance.listState).toBe('ready'));f.componentInstance.left='a';f.componentInstance.right='b';await f.componentInstance.compare();await vi.waitFor(()=>expect(f.nativeElement.textContent).toContain('incomparable'));expect(f.nativeElement.textContent).toContain('—');await f.componentInstance.compare();await vi.waitFor(()=>expect(f.nativeElement.textContent).toContain('Структурных изменений нет.'));
 });
 it('renders errors instead of empty results',async()=>{
  const f=await mount(vi.fn().mockResolvedValueOnce(response({sequence:3})).mockResolvedValueOnce({ok:false}));await vi.waitFor(()=>expect(f.nativeElement.textContent).toContain('Не удалось загрузить историю'));expect(f.nativeElement.textContent).not.toContain('В истории пока нет запусков');
 });
});
