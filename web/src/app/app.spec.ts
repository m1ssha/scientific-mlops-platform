import '@angular/compiler';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { App } from './app';
import { checkCounter } from './api';
const response = (body:unknown) => ({ok:true,json:async()=>body}) as Response;
const page = {view_sequence:3,next_after_sequence:null,items:[]};
afterEach(()=>vi.unstubAllGlobals());
describe('local API and view state',()=>{
 it('rejects counters outside safe integer range',()=>{expect(()=>checkCounter(9007199254740992)).toThrow();expect(()=>checkCounter(1.5)).toThrow();expect(checkCounter(3)).toBe(3)});
 it('keeps view sequence on pagination and distinguishes empty history',async()=>{
  const fetcher=vi.fn().mockResolvedValueOnce(response({sequence:3,repository_id:'repo'})).mockResolvedValueOnce(response({...page,next_after_sequence:1,items:[{id:'a',sequence:1}]})).mockResolvedValueOnce(response({...page,items:[{id:'b',sequence:2}]}));vi.stubGlobal('fetch',fetcher);
  const app=new App({markForCheck:()=>{}} as any);await vi.waitFor(()=>expect(app.listState).toBe('ready'));await app.more();expect(fetcher.mock.calls[2][0]).toContain('after_sequence=1&view_sequence=3');expect(app.runs.map(r=>r['id'])).toEqual(['a','b']);
 });
 it('clears prior repository after a failed refresh',async()=>{
  const fetcher=vi.fn().mockResolvedValueOnce(response({sequence:3,repository_id:'old'})).mockResolvedValueOnce(response(page)).mockResolvedValueOnce({ok:false}).mockResolvedValueOnce(response(page));vi.stubGlobal('fetch',fetcher);
  const app=new App({markForCheck:()=>{}} as any);await vi.waitFor(()=>expect(app.listState).toBe('ready'));expect(app.repository?.['repository_id']).toBe('old');await app.refresh();expect(app.listState).toBe('error');expect(app.repository).toBeNull();
 });
 it('discards obsolete detail responses',async()=>{
  let resolveA:(r:Response)=>void=()=>{};const slow=new Promise<Response>(r=>resolveA=r);
  const fetcher=vi.fn().mockResolvedValueOnce(response({sequence:3})).mockResolvedValueOnce(response(page)).mockReturnValueOnce(slow).mockResolvedValueOnce(response({id:'b',sequence:2}));vi.stubGlobal('fetch',fetcher);
  const app=new App({markForCheck:()=>{}} as any);await vi.waitFor(()=>expect(app.listState).toBe('ready'));const old=app.select('a');await app.select('b');resolveA(response({id:'a',sequence:1}));await old;expect(app.detail?.['id']).toBe('b');
 });
 it('keeps failed recovery separate from a successful empty result',async()=>{
  const fetcher=vi.fn().mockResolvedValueOnce(response({sequence:3})).mockResolvedValueOnce(response(page)).mockResolvedValueOnce({ok:false});vi.stubGlobal('fetch',fetcher);
  const app=new App({markForCheck:()=>{}} as any);await vi.waitFor(()=>expect(app.listState).toBe('ready'));app.selected='a';await app.recovery();expect(app.planState).toBe('error');expect(app.plan).toBeNull();
 });
});
