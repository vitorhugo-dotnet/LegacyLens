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
type Event = { eventId: string; parentEventId?: string; kind: string; producerId: string; metadata?: Record<string, string> };
type Investigation = { events: { items: Event[]; total: number }; diagnostics?: { items: { code: string; message: string }[] }; relations: { items: { kind: string; layer: string; fromId: string; toId?: string }[] }; symbols: { items: { id: string; qualifiedName: string; kind: string }[] } };

function run(command: string, args: string[], timeout = 180_000, env = process.env): void {
  mkdirSync(cache, { recursive: true });
  const path = join(cache, `causality-${randomUUID()}.log`);
  const fd = openSync(path, 'w');
  let result;
  try { result = spawnSync(command, args, { cwd: root, env, timeout, stdio: ['ignore', fd, fd] }); }
  finally { closeSync(fd); }
  const output = readFileSync(path, 'utf8'); unlinkSync(path);
  if (result.status !== 0) throw new Error(`FIXTURE_SETUP: ${command} failed (${result.status}): ${(output || result.error?.message || 'no output').slice(-1000)}`);
}

test('keeps two selected clicks in separate causal trees, links timer and request effects, and leaves polling unlinked', async () => {
  let context: BrowserContext | undefined;
  let fixtureStarted = false;
  const profile = join(cache, `chrome-${randomUUID()}`);
  try {
    run(process.execPath, [process.env.npm_execpath!, 'run', 'build', '--workspace', 'apps/extension'], 60_000, { ...process.env, LEGACYLENS_FIXTURE_EXTENSION: '1' });
    run('pwsh', ['-NoProfile', '-File', join(root, 'scripts/fixtures/start-legacy.ps1'), '-ExtensionId', extensionId], 300_000);
    fixtureStarted = true;
    const state = JSON.parse(readFileSync(join(cache, 'state.json'), 'utf8')) as { projectId: string; httpPort: number };
    const discovery = JSON.parse(readFileSync(join(cache, 'LegacyLens/discovery.json'), 'utf8')) as { address: string; hostToken: string };
    const command = async (name: string, payload: Record<string, unknown>): Promise<any> => {
      const response = await fetch(`http://${discovery.address}/v1/commands`, { method: 'POST', headers: { Authorization: `Bearer ${discovery.hostToken}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ protocolVersion: 1, requestId: randomUUID(), command: name, payload }) });
      if (!response.ok) throw new Error(`FIXTURE_SETUP: ${name} returned ${response.status}`);
      return (await response.json()).result;
    };
    const investigation = (traceId: string) => command('investigation.get', { projectId: state.projectId, traceId, offset: 0, limit: 200 }) as Promise<Investigation>;
    await command('project.index', { projectId: state.projectId, paths: [], offset: 0, limit: 200 });
    context = await chromium.launchPersistentContext(profile, { channel: 'chromium', headless: true, env: { ...process.env, APPDATA: cache }, args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`] });
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker', { timeout: 15_000 });
    expect(new URL(worker.url()).hostname).toBe(extensionId);
    const page = await context.newPage();
    await page.goto(`http://127.0.0.1:${state.httpPort}/legacy-fixture/orders.xhtml`);
    const pageRequests: Array<{ url: string; traceparent?: string }> = [];
    page.on('request', (request) => { if (request.url().includes('/legacy-fixture/orders.xhtml')) pageRequests.push({ url: request.url(), traceparent: request.headers()['traceparent'] }); });
    await page.evaluate(() => {
      const save = document.getElementById('orderForm:saveOrder')!;
      (window as any).__captureBaseline = { fetch, handle: (window as any).PrimeFaces.ajax.Request.handle };
      (window as any).__selectedTimerRuns = 0;
      save.addEventListener('click', () => {
        setTimeout(() => { (window as any).__selectedTimerRuns++; void fetch('orders.xhtml?timer=selected'); }, 1_000);
      });
    });
    const activeTrace = () => worker.evaluate(async () => ((await chrome.storage.session.get('legacylens.activeCaptures.v1'))['legacylens.activeCaptures.v1'] ?? [])[0]?.session?.id as string | undefined);
    const traces: string[] = [];
    for (let index = 0; index < 2; index++) {
      await page.locator('#orderForm\\:message').evaluate((message) => { message.textContent = ''; });
      await page.locator('#orderForm\\:note').fill(`causality-${index}`);
      await startFixtureCapture(worker,page,state.projectId);
      await expect.poll(activeTrace, { timeout: 15_000 }).toMatch(/^[a-f0-9]{32}$/);
      await expect.poll(() => page.evaluate(() => ({
        fetch: fetch !== (window as any).__captureBaseline.fetch,
        primeFaces: (window as any).PrimeFaces.ajax.Request.handle !== (window as any).__captureBaseline.handle,
      })), { timeout: 5_000, message: 'page capture adapters must be installed before the selected click' }).toEqual({ fetch: true, primeFaces: true });
      const trace = await activeTrace();
      expect(trace).toBeTruthy(); traces.push(trace!);
      await page.locator('#orderForm\\:saveOrder').click();
      await expect(page.locator('#orderForm\\:message')).toHaveText('Order saved', { timeout: 15_000 });
      await expect(page.locator('#orderForm\\:count')).toHaveText(String(index + 1), { timeout: 15_000 });
      await expect.poll(() => page.evaluate(() => (window as any).__selectedTimerRuns), { timeout: 5_000,
        message: 'the selected click must schedule its timer callback' }).toBeGreaterThanOrEqual(index + 1);
      await expect.poll(() => pageRequests.some((request) => request.url.includes('timer=selected')), { timeout: 5_000,
        message: 'the scheduled timer fetch must reach the legacy fixture' }).toBe(true);
      await expect.poll(() => pageRequests.some((request) => request.url.includes('timer=selected')
        && new RegExp(`^00-${trace}-[a-f0-9]{16}-01$`).test(request.traceparent ?? '')), { timeout: 5_000,
        message: 'selected timer fetch must carry the active traceparent' }).toBe(true);
      await stopFixtureCapture(worker,page);
      await expect.poll(activeTrace, { timeout: 15_000 }).toBeUndefined();
    }
    expect(new Set(traces).size).toBe(2);
    for (const [index, trace] of traces.entries()) {
      let latest: Investigation | undefined;
      try { await expect.poll(async () => {
        latest = await investigation(trace);
        const result = latest;
        const events = result.events.items;
        const clickEvent = events.find((event) => event.kind === 'jsf.click');
        const effects = events.filter((event) => ['browser.network', 'primefaces.ajax'].includes(event.kind));
        return { result, clickEvent,
          clicks: events.filter((event) => event.kind === 'jsf.click').length,
          effects,
          timer: events.some((event) => event.kind === 'browser.network' && event.metadata?.stackGap?.includes('ASYNC_BOUNDARY')),
          httpServers: events.filter((event) => event.kind === 'http.server').length,
          dao: events.some((event) => event.kind === 'method.start' && event.metadata?.['code.class'] === 'io.legacylens.fixture.OrderDao' && event.metadata?.['code.method'] === 'insert') };
      }, { timeout: 40_000, message: `runtime evidence should be complete for capture ${index} (trace ${trace})` })
        .toMatchObject({ clicks: 1, timer: true, httpServers: 2, dao: true }); }
      catch (error) {
        const events = latest?.events.items ?? [];
        console.error('Causality sanitizer diagnostics:', JSON.stringify((latest?.diagnostics?.items ?? []).filter((item) => item.code === 'capture.metadata_removed').length));
        throw new Error(`${error instanceof Error ? error.message : String(error)}\nObserved methods: ${JSON.stringify(events.filter((event) => event.kind === 'method.start').map((event) => ({ class: event.metadata?.['code.class'], method: event.metadata?.['code.method'], thread: event.metadata?.['agent.thread_id'], parentEventId: event.parentEventId })))}\nMissing-context probes: ${JSON.stringify(events.filter((event) => event.kind === 'agent.method_context_probe').map((event) => ({ class: event.metadata?.['code.class'], method: event.metadata?.['code.method'], thread: event.metadata?.['agent.thread_id'], parentEventId: event.parentEventId })))}\nServer requests: ${JSON.stringify(events.filter((event) => event.kind === 'http.server').map((event) => ({ eventId: event.eventId, parentEventId: event.parentEventId, span: event.metadata?.['http.request_span'], method: event.metadata?.['http.method'], thread: event.metadata?.['agent.thread_id'] })))}\nObserved event kinds: ${JSON.stringify([...new Set(events.map((event) => event.kind))].sort())}\nAgent losses: ${JSON.stringify(events.filter((event) => event.kind === 'agent.loss').map((event) => event.metadata?.['agent.dropped_count']))}`);
      }
      const result = latest!;
      const click = result.events.items.find((event) => event.kind === 'jsf.click')!;
      const effects = result.events.items.filter((event) => ['browser.network', 'primefaces.ajax'].includes(event.kind));
      expect(effects.length, 'the selected action includes distinct fetch and Ajax requests').toBeGreaterThanOrEqual(2);
      expect(effects.every((event) => event.parentEventId === click.eventId)).toBe(true);
      expect(result.events.items.some((event) => event.kind === 'browser.network' && event.metadata?.stackGap?.includes('ASYNC_BOUNDARY'))).toBe(true);
      expect(result.relations.items.filter((relation) => relation.layer === 'static').length).toBeGreaterThan(0);
      expect(result.events.items.some((event) => event.kind === 'method.start' && event.metadata?.['code.class'] === 'io.legacylens.fixture.OrderServlet' && event.metadata?.['code.method'] === 'service')).toBe(false);
      expect(result.events.items.filter((event) => event.kind === 'browser.network').every((event) => event.parentEventId === click.eventId)).toBe(true);
    }
    await expect.poll(() => pageRequests.some((request) => request.url.includes('/orders.xhtml') && !request.url.includes('timer=selected') && !request.traceparent), { timeout: 5_000,
      message: 'the fixture polling request remains outside the selected causal trees' }).toBe(true);
  } finally {
    await context?.close();
    if (fixtureStarted) run('pwsh', ['-NoProfile', '-File', join(root, 'scripts/fixtures/stop.ps1')], 30_000);
    if (profile.startsWith(cache + '\\')) rmSync(profile, { recursive: true, force: true });
  }
});
