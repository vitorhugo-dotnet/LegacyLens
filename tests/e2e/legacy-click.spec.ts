import { test, expect, chromium, type BrowserContext } from '@playwright/test';
import { readFileSync, rmSync, mkdirSync, openSync, closeSync, unlinkSync } from 'node:fs';
import { resolve, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { startFixtureCapture, stopFixtureCapture } from './support/capture.ts';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const cache = join(root, '.fixture-cache');
const extension = join(root, 'apps/extension/.output/chrome-mv3');
const extensionId = 'olmoddbhnipjpamkjngdefdfekjehbef';
type Event = { eventId: string; parentEventId?: string; kind: string; metadata?: Record<string,string>; producerId: string };
type Investigation = { events: {items: Event[]; total: number}; relations: {items: {kind:string;layer?:string;fromId:string;toId?:string}[]}; symbols: {items:{id:string;qualifiedName:string}[]}; agentStatus:{state:string;evidenceDiagnosticId?:string}; diagnostics:{items:{id:string;code:string}[]} };

function run(command: string, args: string[], timeout = 180_000, env = process.env): void {
  mkdirSync(cache,{recursive:true});
  const outputPath=join(cache,`setup-${randomUUID()}.log`);
  const output=openSync(outputPath,'w');
  let result;
  try { result=spawnSync(command,args,{cwd:root,env,timeout,stdio:['ignore',output,output]}); }
  finally { closeSync(output); }
  const message=readFileSync(outputPath,'utf8');
  unlinkSync(outputPath);
  if (result.status !== 0) throw new Error(`FIXTURE_SETUP: ${command} failed (exit ${result.status}): ${(message || result.error?.message || 'no output').slice(-1000)}`);
}

test('selected save traverses two exact request spans into JSF, bean, service, DAO and orders; DOM only has no Java events', async () => {
  let context: BrowserContext | undefined;
  const profile = join(cache,`chrome-${randomUUID()}`);
  let fixtureStarted = false;
  try {
    run(process.execPath,[process.env.npm_execpath!,'run','build','--workspace','apps/extension'],60_000,{...process.env,LEGACYLENS_FIXTURE_EXTENSION:'1'});
    run('pwsh',['-NoProfile','-File',join(root,'scripts/fixtures/start-legacy.ps1'),'-ExtensionId',extensionId]);
    fixtureStarted = true;
    const state = JSON.parse(readFileSync(join(cache,'state.json'),'utf8')) as {projectId:string;httpPort:number;wildflyVersion:string;platform:{name:string;architecture:string;version:string}};
    expect(state.wildflyVersion).toBe('10.0.0.Final');
    expect(state.platform.architecture).toBe('64-bit');
    const discovery = JSON.parse(readFileSync(join(cache,'LegacyLens/discovery.json'),'utf8')) as {address:string;hostToken:string};
    const command = async (name:string,payload:Record<string,unknown>):Promise<any> => {
      const reply = await fetch(`http://${discovery.address}/v1/commands`,{method:'POST',headers:{Authorization:`Bearer ${discovery.hostToken}`,'Content-Type':'application/json'},body:JSON.stringify({protocolVersion:1,requestId:randomUUID(),command:name,payload})});
      if (!reply.ok) throw new Error(`FIXTURE_SETUP: core command ${name} returned ${reply.status}`);
      return (await reply.json()).result;
    };
    const investigation = (traceId:string) => command('investigation.get',{projectId:state.projectId,traceId,offset:0,limit:200}) as Promise<Investigation>;
    await command('project.index',{projectId:state.projectId,paths:[],offset:0,limit:200});
    context = await chromium.launchPersistentContext(profile,{channel:'chromium',headless:true,env:{...process.env,APPDATA:cache},args:[`--disable-extensions-except=${extension}`,`--load-extension=${extension}`]});
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker',{timeout:15_000});
    expect(new URL(worker.url()).hostname,'FIXTURE_SETUP: built extension ID differs from pinned native host origin').toBe(extensionId);
    const page = await context.newPage();
    const url = `http://127.0.0.1:${state.httpPort}/legacy-fixture/orders.xhtml`;
    await page.goto(url);
    const runtime = await page.locator('#runtimeInfo').evaluate((node) => Object.fromEntries([...node.attributes].map((attribute) => [attribute.name, attribute.value])));
    expect(runtime).toMatchObject({ 'data-java': '1.8.0_462-b08', 'data-primefaces': '5.3', 'data-mysql': '5.7.44' });
    expect(runtime['data-connector-j']).toContain('5.1.49');
    expect(runtime['data-faces']).toMatch(/^2\.2\./);
    await page.evaluate(() => {
      const probe: unknown[]=[];
      (window as any).__legacyLensProbe=probe;
      for(const name of ['legacylens:start','legacylens:select','legacylens:ajax','legacylens:network'])
        window.addEventListener(name,(event)=>{const detail=(event as CustomEvent).detail??{};probe.push({name,source:detail.source,propagation:detail.propagation,code:detail.code,hasSpan:Boolean(detail.spanId),hasTraceparent:Boolean(detail.traceparent)});});
    });
    await expect(page.locator('#orderForm\\:saveOrder')).toBeVisible();
    const activeTrace = async ():Promise<string> => worker.evaluate(async () => {
      const stored = await chrome.storage.session.get('legacylens.activeCaptures.v1');
      return stored['legacylens.activeCaptures.v1']?.[0]?.session?.id as string;
    });

    await startFixtureCapture(worker,page,state.projectId);
    await expect.poll(activeTrace,{timeout:15_000,message:'FIXTURE_SETUP: native host did not start a capture'}).toMatch(/^[0-9a-f]{32}$/);
    const domTrace = await activeTrace();
    await page.locator('#domOnly').click();
    await expect(page.locator('#domOnly')).toHaveText('DOM changed');
    await expect.poll(async()=> (await investigation(domTrace)).events.total).toBeGreaterThanOrEqual(1);
    const dom = await investigation(domTrace);
    expect(dom.events.items.some((event)=>event.kind==='jsf.click')).toBe(true);
    expect(dom.events.items.filter((event)=>event.kind==='http.server'||event.kind.startsWith('method.')||event.kind.startsWith('db.')),'DOM-only action must have zero Java events').toHaveLength(0);
    await stopFixtureCapture(worker,page);
    await expect.poll(activeTrace,{timeout:15_000,message:'first capture did not stop'}).toBeUndefined();
    await page.evaluate(() => {
      (window as any).__legacyLensBaseline = { fetch, handle: (window as any).PrimeFaces.ajax.Request.handle };
    });

    await page.locator('#orderForm\\:note').fill('fixture-private-order-value');
    const observedRequests:{parent?:string}[]=[];
    page.on('request',(request)=>{if(request.url().includes('/legacy-fixture/orders.xhtml'))observedRequests.push({parent:request.headers()['traceparent']});});
    const captureStartedAt = Date.now();
    await startFixtureCapture(worker,page,state.projectId);
    await expect.poll(activeTrace,{timeout:15_000,message:'FIXTURE_SETUP: second native capture did not start'}).toMatch(/^[0-9a-f]{32}$/);
    const trace = await activeTrace();
    expect(trace,'second capture must have a new trace ID').not.toBe(domTrace);
    await expect.poll(() => page.evaluate(() => ({
      fetch: fetch !== (window as any).__legacyLensBaseline.fetch,
      primeFaces: (window as any).PrimeFaces.ajax.Request.handle !== (window as any).__legacyLensBaseline.handle,
    })),{timeout:5_000}).toEqual({fetch:true,primeFaces:true});
    await page.locator('#orderForm\\:saveOrder').click();
    await expect(page.locator('#orderForm\\:message')).toHaveText('Order saved',{timeout:15_000});
    await expect.poll(async()=> {
      const pending=await investigation(trace);
      const events=pending.events.items;
      return JSON.stringify({
        httpServer:events.filter((event)=>event.kind==='http.server').length,
        kinds:[...new Set(events.map((event)=>event.kind))].sort(),
        browserSpans:events.filter((event)=>event.kind==='browser.network'||event.kind==='primefaces.ajax').map((event)=>event.metadata?.spanId).filter(Boolean).sort(),
        serverSpans:events.filter((event)=>event.kind==='http.server').map((event)=>event.metadata?.['http.request_span']).filter(Boolean).sort(),
        agentLoss:events.filter((event)=>event.kind==='agent.loss').map((event)=>event.metadata?.['agent.dropped_count']),
        observedTraceparent:observedRequests.map((request)=>Boolean(request.parent)),
        pageEvents:await page.evaluate(()=>((window as any).__legacyLensProbe as unknown[]).slice(-12)),
      });
    },{timeout:15_000}).toMatch(/"httpServer":2/);
    await expect.poll(async()=> {
      const pending=await investigation(trace);
      const events=pending.events.items;
      return {
        methods:events.filter((event)=>event.kind==='method.start').map((event)=>`${event.metadata?.['code.class']}#${event.metadata?.['code.method']}`).sort(),
        insert:events.some((event)=>event.kind==='method.start'&&event.metadata?.['code.class']==='io.legacylens.fixture.OrderDao'&&event.metadata?.['code.method']==='insert'),
        update:events.some((event)=>event.kind==='db.update'&&event.metadata?.sql==='INSERT INTO orders (note) VALUES (?)'),
        losses:events.filter((event)=>event.kind==='agent.loss').map((event)=>event.metadata?.['agent.dropped_count']),
      };
    },{timeout:30_000}).toMatchObject({insert:true,update:true});
    const investigationPage = context.waitForEvent('page',{timeout:15_000});
    await stopFixtureCapture(worker,page);
    await expect.poll(activeTrace,{timeout:15_000,message:'second capture did not stop'}).toBeUndefined();
    const result = await investigation(trace);
    expect(result.agentStatus.state).toBe('online');
    expect(result.relations.items.some((relation)=>relation.layer==='static'),'the legacy project index should provide static search and impact relations').toBe(true);
    console.log(`compatibility-baseline ${JSON.stringify({ id: 'legacy-java8-wildfly10', captureMs: Date.now() - captureStartedAt, eventCount: result.events.total, methodEventCount: result.events.items.filter((event)=>event.kind==='method.start').length, staticRelationCount: result.relations.items.filter((relation)=>relation.layer==='static').length })}`);
    expect(result.diagnostics.items.some((item)=>item.id===result.agentStatus.evidenceDiagnosticId&&item.code==='agent.heartbeat')).toBe(true);
    const events=result.events.items;
    expect(events.filter((event)=>event.kind==='agent.loss'),'Java agent must deliver the captured flow without loss').toHaveLength(0);
    const requests=events.filter((event)=>event.kind==='browser.network'||event.kind==='primefaces.ajax');
    const servers=events.filter((event)=>event.kind==='http.server');
    const spans=new Set(requests.map((event)=>event.metadata?.spanId).filter(Boolean));
    expect(spans.size,'two requests under one click require distinct spans').toBeGreaterThanOrEqual(2);
    for(const span of spans) {expect(span).toMatch(/^[0-9a-f]{16}$/);expect(servers.filter((event)=>event.metadata?.['http.request_span']===span)).toHaveLength(1);}
    const edges=result.relations.items.filter((relation)=>relation.kind==='http.request');
    expect(edges.length).toBeGreaterThanOrEqual(2);
    expect(observedRequests.some((request)=>!request.parent),'unrelated poll must stay unlinked').toBe(true);
    const chain=(expected:{className:string;method:string}[])=>{
      const byId=new Map(events.map((event)=>[event.eventId,event]));
      const leaf=expected[expected.length-1]!;
      const last=events.find((event)=>event.kind==='method.start'&&event.metadata?.['code.class']===leaf.className&&event.metadata?.['code.method']===leaf.method);
      expect(last,`missing ${leaf.className}.${leaf.method}`).toBeDefined();
      const ancestors:Event[]=[];let current:Event|undefined=last;
      while(current){ancestors.push(current);current=current.parentEventId?byId.get(current.parentEventId):undefined;}
      let previousIndex=-1;
      for(const expectedMethod of [...expected].reverse()) {
        const index=ancestors.findIndex((event,position)=>position>previousIndex&&event.kind==='method.start'
          &&event.metadata?.['code.class']===expectedMethod.className&&event.metadata?.['code.method']===expectedMethod.method);
        expect(index,`missing ordered causal ancestor ${expectedMethod.className}.${expectedMethod.method}`).toBeGreaterThan(previousIndex);
        previousIndex=index;
      }
      expect(ancestors.some((event)=>event.kind==='http.server')).toBe(true);
    };
    chain([
      {className:'io.legacylens.fixture.OrderBean',method:'save'},
      {className:'io.legacylens.fixture.OrderService',method:'save'},
      {className:'io.legacylens.fixture.OrderDao',method:'insert'},
    ]);
    expect(events.some((event)=>event.kind==='db.update'&&event.metadata?.sql==='INSERT INTO orders (note) VALUES (?)')).toBe(true);
    expect(JSON.stringify(result)).not.toContain('fixture-private-order-value');
    expect(JSON.stringify(result)).not.toContain('traceparent');
    const panel = await investigationPage;
    await expect.poll(() => {
      try { return new URL(panel.url()).searchParams.get('traceId'); }
      catch { return null; }
    },{timeout:15_000,message:'the opened investigation must belong to the selected capture'}).toBe(trace);
    await expect(panel.getByRole('button',{name:'Relação observada: http.request'}).first()).toBeVisible({timeout:15_000});
    await panel.getByRole('button',{name:'Relação observada: http.request'}).first().click();
    await expect(panel.getByRole('complementary',{name:'Evidência'})).toContainText('Camada: observed; resolução: resolved');
  } finally {
    await context?.close();
    if (fixtureStarted) run('pwsh',['-NoProfile','-File',join(root,'scripts/fixtures/stop.ps1')],30_000);
    if (profile.startsWith(cache+'\\')) rmSync(profile,{recursive:true,force:true});
  }
});
