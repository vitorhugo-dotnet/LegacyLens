import { test, expect, chromium, type BrowserContext } from '@playwright/test';
import { readFileSync, rmSync, mkdirSync, openSync, closeSync, unlinkSync } from 'node:fs';
import { resolve, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { captureDiagnostics, startFixtureCapture } from './support/capture.ts';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const cache = join(root, '.fixture-cache');
const extension = join(root, 'apps/extension/.output/chrome-mv3');
const extensionId = 'olmoddbhnipjpamkjngdefdfekjehbef';
test.setTimeout(420_000);
type Event = { eventId: string; kind: string; metadata?: Record<string, string> };
type Investigation = { events: { items: Event[] }; relations: { items: { layer: string }[] } };

function run(command: string, args: string[], timeout = 180_000, env = process.env): void {
  mkdirSync(cache, { recursive: true });
  const path = join(cache, `modern-${randomUUID()}.log`);
  const fd = openSync(path, 'w');
  let result;
  try { result = spawnSync(command, args, { cwd: root, env, timeout, stdio: ['ignore', fd, fd] }); }
  finally { closeSync(fd); }
  const output = readFileSync(path, 'utf8'); unlinkSync(path);
  if (result.status !== 0) throw new Error(`FIXTURE_SETUP: ${command} failed (${result.status}): ${(output || result.error?.message || 'no output').slice(-1000)}`);
}

test('captures a Jakarta Faces action and reports exact runtime capabilities', async () => {
  let context: BrowserContext | undefined;
  let fixtureStarted = false;
  const profile = join(cache, `chrome-modern-${randomUUID()}`);
  try {
    run(process.execPath, [process.env.npm_execpath!, 'run', 'build', '--workspace', 'apps/extension'], 60_000, { ...process.env, LEGACYLENS_FIXTURE_EXTENSION: '1' });
    run('pwsh', ['-NoProfile', '-File', join(root, 'scripts/fixtures/start-modern.ps1'), '-ExtensionId', extensionId, '-Revision', 'modern-A'], 360_000);
    fixtureStarted = true;
    const state = JSON.parse(readFileSync(join(cache, 'state.json'), 'utf8')) as { projectId: string; httpPort: number; wildflyVersion: string; platform: { name: string; architecture: string; version: string } };
    expect(state.wildflyVersion).toBe('35.0.0.Final');
    expect(state.platform.architecture).toBe('64-bit');
    const discovery = JSON.parse(readFileSync(join(cache, 'LegacyLens/discovery.json'), 'utf8')) as { address: string; hostToken: string };
    const command = async (name: string, payload: Record<string, unknown>): Promise<any> => {
      const response = await fetch(`http://${discovery.address}/v1/commands`, { method: 'POST', headers: { Authorization: `Bearer ${discovery.hostToken}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ protocolVersion: 1, requestId: randomUUID(), command: name, payload }) });
      if (!response.ok) throw new Error(`FIXTURE_SETUP: ${name} returned ${response.status}`);
      return (await response.json()).result;
    };
    await command('project.index', { projectId: state.projectId, paths: [], offset: 0, limit: 200 });
    context = await chromium.launchPersistentContext(profile, { channel: 'chromium', headless: true, env: { ...process.env, APPDATA: cache }, args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`] });
    const worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker', { timeout: 15_000 });
    expect(new URL(worker.url()).hostname).toBe(extensionId);
    const page = await context.newPage();
    await page.goto(`http://127.0.0.1:${state.httpPort}/modern-fixture/orders.xhtml`);
    await page.locator('#orderForm\\:note').fill('modern-fixture');
    const captureStartedAt = Date.now();
    await startFixtureCapture(worker,page,state.projectId);
    const activeTrace = () => worker.evaluate(async () => ((await chrome.storage.session.get('legacylens.activeCaptures.v1'))['legacylens.activeCaptures.v1'] ?? [])[0]?.session?.id as string | undefined);
    await expect.poll(activeTrace, { timeout: 15_000 }).toMatch(/^[a-f0-9]{32}$/);
    const traceId = (await activeTrace())!;
    await page.locator('#orderForm\\:saveOrder').click();
    await expect(page.locator('#orderForm\\:message')).toHaveText('Order saved', { timeout: 15_000 });
    let result: Investigation | undefined;
    await expect.poll(async () => {
      result = await command('investigation.get', { projectId: state.projectId, traceId, offset: 0, limit: 200 });
      return result.events.items.some((event) => event.kind === 'method.start' && event.metadata?.['code.class'] === 'io.legacylens.fixture.OrderDao' && event.metadata?.['code.method'] === 'insert');
    }, { timeout: 30_000 }).toBe(true);
    const events = result!.events.items;
    console.log(`compatibility-baseline ${JSON.stringify({ id: 'jakarta-java21-wildfly35', captureMs: Date.now() - captureStartedAt, eventCount: result!.events.items.length, methodEventCount: events.filter((event) => event.kind === 'method.start').length, staticRelationCount: result!.relations.items.filter((relation) => relation.layer === 'static').length })}`);
    const click = events.find((event) => event.kind === 'jsf.click' && /^[a-f0-9]{16}$/i.test(event.eventId));
    expect(click, `jsf.click missing; stages=${JSON.stringify(await captureDiagnostics(worker))}`).toBeDefined();
    const clickStages = (await captureDiagnostics(worker)).filter((record) => record.traceId === traceId && record.eventId === click!.eventId);
    expect(clickStages.map((record) => `${record.stage}.${record.outcome}`), `click transport stages=${JSON.stringify(clickStages)}`).toEqual(
      expect.arrayContaining(['content.selection.accepted', 'content.send.accepted', 'background.receive.started', 'background.receive.accepted', 'background.record.accepted', 'host.ingest.accepted']));
    expect(events.some((event) => event.kind === 'method.start' && event.metadata?.['code.class'] === 'io.legacylens.fixture.OrderBean')).toBe(true);
    const methodEvents = events.filter((event) => event.kind === 'method.start');
    expect(methodEvents.every((event) => /^deployment@loader-[a-f0-9]+$/.test(event.metadata?.['code.deployment'] ?? ''))).toBe(true);
    expect(new Set(methodEvents.map((event) => event.metadata?.['code.deployment'])).size).toBe(1);
    expect(result!.relations.items.some((relation) => relation.layer === 'static')).toBe(true);
    const runtime = await page.locator('#runtimeInfo').evaluate((node) => Object.fromEntries([...node.attributes].map((attribute) => [attribute.name, attribute.value])));
    expect(runtime).toMatchObject({ 'data-java': '21.0.5+11-LTS', 'data-faces': '4.0.8', 'data-primefaces': '15.0.0', 'data-mysql': '8.4.0', 'data-build-revision': 'modern-A' });
    expect(runtime['data-connector-j']).toContain('8.4.0');
  } finally {
    await context?.close();
    if (fixtureStarted) run('pwsh', ['-NoProfile', '-File', join(root, 'scripts/fixtures/stop.ps1')], 30_000);
    if (profile.startsWith(cache + '\\')) rmSync(profile, { recursive: true, force: true });
  }
});
