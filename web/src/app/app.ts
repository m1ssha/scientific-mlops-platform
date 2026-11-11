import { ChangeDetectorRef, Component, Inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { get, Obj, checkCounter } from './api';

@Component({selector:'app-root', standalone:true, imports:[CommonModule], templateUrl:'./app.html'})
export class App {
  repository: Obj | null = null; runs: Obj[] = []; cursor: number | null = null; view: number | null = null;
  selected = ''; left = ''; right = ''; detail: Obj | null = null; plan: Obj | null = null; diff: Obj | null = null;
  listState = 'loading'; detailState = ''; planState = ''; diffState = ''; error = '';
  private listGen=0; private detailGen=0; private planGen=0; private diffGen=0;
  constructor(@Inject(ChangeDetectorRef) private readonly cdr: ChangeDetectorRef) { void this.refresh(); }
  async refresh() {
    const gen=++this.listGen; ++this.detailGen; ++this.planGen; ++this.diffGen;
    this.runs=[]; this.cursor=null; this.view=null; this.selected=''; this.left=''; this.right=''; this.detail=null; this.plan=null; this.diff=null; this.detailState=''; this.planState=''; this.diffState=''; this.listState='loading'; this.error='';
    try { const [repo,page]=await Promise.all([get('/api/v1/repository'),get('/api/v1/runs?limit=50')]); if(gen!==this.listGen)return; checkCounter(repo['sequence']); this.repository=repo; this.applyPage(page, false); }
    catch { if(gen===this.listGen){this.listState='error';this.error='Не удалось загрузить историю.';} } finally { this.cdr.markForCheck(); }
  }
  private applyPage(page:Obj, append:boolean) {
    if (!Array.isArray(page['items'])) throw new Error('invalid page');
    const view=checkCounter(page['view_sequence']);
    if (append && view!==this.view) throw new Error('changed view');
    const next=page['next_after_sequence']; this.cursor=next == null ? null : checkCounter(next);
    this.view=view; this.runs=append ? [...this.runs,...page['items']] : page['items']; this.listState='ready';
  }
  async more() {
    if(this.cursor===null || this.view===null || this.listState==='loading')return;
    const gen=++this.listGen; const after=this.cursor, view=this.view; this.listState='loading';
    try {const page=await get(`/api/v1/runs?limit=50&after_sequence=${after}&view_sequence=${view}`);if(gen===this.listGen)this.applyPage(page,true);}
    catch {if(gen===this.listGen){this.listState='error';this.error='Не удалось загрузить следующую страницу.';}} finally { this.cdr.markForCheck(); }
  }
  async select(id:string) {
    this.selected=id; this.detail=null; this.plan=null; this.planState=''; ++this.planGen; const gen=++this.detailGen; this.detailState='loading';
    try { const data=await get('/api/v1/runs/'+encodeURIComponent(id)); if(gen===this.detailGen){this.detail=data;this.detailState='ready';} }
    catch {if(gen===this.detailGen)this.detailState='error';} finally { this.cdr.markForCheck(); }
  }
  async recovery() {
    const id=this.selected; if(!id)return; const gen=++this.planGen; this.plan=null; this.planState='loading';
    try {const data=await get('/api/v1/runs/'+encodeURIComponent(id)+'/recovery');if(gen===this.planGen){this.plan=data;this.planState='ready';}}
    catch {if(gen===this.planGen)this.planState='error';} finally { this.cdr.markForCheck(); }
  }
  setSide(side:'left'|'right', event:Event) {this[side]=(event.target as HTMLSelectElement).value; ++this.diffGen;this.diff=null;this.diffState='';}
  async compare() {
    if(!this.left||!this.right)return; const gen=++this.diffGen; this.diff=null;this.diffState='loading';
    try {const data=await get('/api/v1/runs/compare?left='+encodeURIComponent(this.left)+'&right='+encodeURIComponent(this.right));if(gen===this.diffGen){this.diff=data;this.diffState='ready';}}
    catch {if(gen===this.diffGen)this.diffState='error';} finally { this.cdr.markForCheck(); }
  }
  entries(value:unknown): [string, any][] {return value && typeof value==='object' && !Array.isArray(value) ? Object.entries(value) : [];}
  print(value:unknown):string {return value===undefined ? 'отсутствует' : JSON.stringify(value, null, 2) ?? 'null';}
}
