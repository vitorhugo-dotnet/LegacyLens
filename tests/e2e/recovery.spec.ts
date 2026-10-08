import { test, expect, chromium, type BrowserContext, type Page } from '@playwright/test';
import { readFileSync, rmSync, mkdirSync, openSync, closeSync, unlinkSync } from 'node:fs';
import { resolve, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const cache = join(root, '.fixture-cache');
const extension = join(root, 'apps/extension/.output/chrome-mv3');
const extensionId = 'olmoddbhnipjpamkjngdefdfekjehbef';
type Event = { eventId: string; kind: string; producerId: string };
type Investigation = { trace: { incomplete: boolean }; events: { items: Event[]; total: number }; diagnostics: { items: { code: string }[] } };

function run(command: string, args: string[], timeout = 180_000, env = process.env): void {
  mkdirSync(cache, { recursive: true });
  const path = join(cache, `recovery-${randomUUID()}.log`);
  const fd = openSync(path, 'w');
  let result;
  try { result = spawnSync(command, args, { cwd: root, env, timeout, stdio: ['ignore', fd, fd] }); }
  finally { closeSync(fd); }
  const output = readFileSync(path, 'utf8'); unlinkSync(path);
  if (result.status !== 0) throw new Error(`FIXTURE_SETUP: ${command} failed (${result.status}): ${(output || result.error?.message || 'no output').slice(-1000)}`);
}

async function openPanel(worker: import('@playwright/test').Worker, page: Page): Promise<void> {
  await worker.evaluate(async (url) => {
    const tab = (await chrome.tabs.query({})).find((item) => item.url === url);
    if (!tab?.id) throw new Error('fixture tab is unavailable');
    await chrome.tabs.sendMessage(tab.id, { type: 'selection.open' });
  }, page.url());
  await expect(page.locator('[data-legacylens-ui]')).toBeVisible();
}

test('recovers after host loss and background restart, preserves overflow events, marks capture incomplete, and keeps the app responsive', async () => {
  let context: BrowserContext | undefined;
  let fixtureStarted = false;
  const profile = join(cache, `chrome-${randomUUID()}`);
  try {
    run(process.execPath, [process.env.npm_execpath!, 'run', 'build', '--workspace', 'apps/extension'], 60_000, { ...process.env, LEGACYLENS_FIXTURE_EXTENSION: '1' });
    run('pwsh', ['-NoProfile', '-File', join(root, 'scripts/fixtures/start-legacy.ps1'), '-ExtensionId', extensionId]);
    fixtureStarted = true;
    const state = JSON.parse(readFileSync(join(cache, 'state.json'), 'utf8')) as { projectId: string; httpPort: number };
    const discovery = JSON.parse(readFileSync(join(cache, 'LegacyLens/discovery.json'), 'utf8')) as { address: string; hostToken: string };
    const command = async (name: string, payload: Record<string, unknown>): Promise<any> => {
      const response = await fetch(`http://${discovery.address}/v1/commands`, { method: 'POST', headers: { Authorization: `Bearer ${discovery.hostToken}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ protocolVersion: 1, requestId: randomUUID(), command: name, payload }) });
      if (!response.ok) throw new Error(`FIXTURE_SETUP: ${name} returned ${response.status}`);
      return (await response.json()).result;
    };
    const investigation = (traceId: string) => command('investigation.get', { projectId: state.projectId, traceId, offset: 0, limit: 200 }) as Promise<Investigation>;
    context = await chromium.launchPersistentContext(profile, { channel: 'chromium', headless: true, env: { ...process.env, APPDATA: cache }, args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`] });
    let worker = context.serviceWorkers()[0] ?? await context.waitForEvent('serviceworker', { timeout: 15_000 });
    expect(new URL(worker.url()).hostname).toBe(extensionId);
    const page = await context.newPage();
    await page.goto(`http://127.0.0.1:${state.httpPort}/legacy-fixture/orders.xhtml`);
    const recoveryRequests: string[] = [];
    page.on('request', (request) => { if (request.url().includes('recovery=timer')) recoveryRequests.push(request.url()); });
    await page.evaluate(() => {
      (window as any).__recoveryEvents = [];
      for (const name of ['legacylens:start', 'legacylens:select', 'legacylens:ajax', 'legacylens:network'])
        window.addEventListener(name, (event) => (window as any).__recoveryEvents.push({ name, detail: (event as CustomEvent).detail }));
      const save = document.getElementById('orderForm:saveOrder') as HTMLButtonElement;
      const original = save.onclick;
      save.onclick = function (event) {
        setTimeout(() => { void fetch('orders.xhtml?recovery=timer'); }, 10_000);
        return original?.call(this, event);
      };
    });
    await openPanel(worker, page);
    await page.locator('[data-legacylens-ui] select').selectOption(state.projectId);
    await page.locator('#orderForm\\:note').fill('recovery-fixture');
    await page.getByRole('button', { name: 'Capture next interaction' }).click();
    const activeTrace = () => worker.evaluate(async () => ((await chrome.storage.session.get('legacylens.activeCaptures.v1'))['legacylens.activeCaptures.v1'] ?? [])[0]?.session?.id as string | undefined);
    await expect.poll(activeTrace, { timeout: 15_000 }).toMatch(/^[a-f0-9]{32}$/);
    const traceId = (await activeTrace())!;
    await page.locator('#orderForm\\:saveOrder').click();
    await expect(page.locator('#orderForm\\:message')).toHaveText('Order saved', { timeout: 15_000 });
    await expect.poll(async () => (await investigation(traceId)).events.total, { timeout: 20_000 }).toBeGreaterThan(1);

    const session = await context.newCDPSession(page);
    const versions = new Map<string, { versionId: string; scriptURL: string; runningStatus: string }>();
    session.on('ServiceWorker.workerVersionUpdated', (event: { versions: { versionId: string; scriptURL: string; runningStatus: string }[] }) => {
      for (const version of event.versions) versions.set(version.versionId, version);
    });
    await session.send('ServiceWorker.enable');
    await expect.poll(() => [...versions.values()].some((version) => version.scriptURL.startsWith(`chrome-extension://${extensionId}/`) && version.runningStatus === 'running'),
      { timeout: 15_000, message: 'Chrome should expose the running extension service worker version' }).toBe(true);
    const runningVersion = [...versions.values()].find((version) => version.scriptURL.startsWith(`chrome-extension://${extensionId}/`) && version.runningStatus === 'running');
    expect(runningVersion, 'fixture extension service worker should be running').toBeDefined();
    await session.send('ServiceWorker.stopWorker', { versionId: runningVersion!.versionId });
    await expect.poll(() => versions.get(runningVersion!.versionId)?.runningStatus,
      { timeout: 15_000, message: 'Chrome should stop the fixture extension service worker before exercising restart' }).toBe('stopped');
    await expect.poll(() => recoveryRequests.length, { timeout: 15_000, message: 'the selected click timer should fetch after the worker stops' }).toBe(1);
    let restartState: { running: boolean; contextWorkers: string[]; pageEvents: unknown[] } | undefined;
    try { await expect.poll(async () => {
      const workerVersions = [...versions.values()].filter((version) => version.scriptURL.startsWith(`chrome-extension://${extensionId}/`));
      restartState = { running: workerVersions.some((version) => version.runningStatus === 'running'),
        contextWorkers: context!.serviceWorkers().map((candidate) => candidate.url()).filter((url) => url.startsWith(`chrome-extension://${extensionId}/`)),
        pageEvents: await page.evaluate(() => (window as any).__recoveryEvents) };
      return { restarted: restartState.running && restartState.contextWorkers.length > 0, ...restartState };
    }, { timeout: 30_000, message: 'the page action should restart the extension service worker and retain capture state' })
      .toMatchObject({ restarted: true }); }
    catch (error) { throw new Error(`${error instanceof Error ? error.message : String(error)}\nRecovery evidence: ${JSON.stringify(restartState)}`); }
    worker = context.serviceWorkers().find((candidate) => candidate.url().startsWith(`chrome-extension://${extensionId}/`))!;
    expect(new URL(worker.url()).hostname).toBe(extensionId);
    await expect.poll(() => context!.serviceWorkers().find((candidate) => candidate.url().startsWith(`chrome-extension://${extensionId}/`)),
      { timeout: 15_000, message: 'Playwright should expose the restarted extension background' }).toBeDefined();
    worker = context.serviceWorkers().find((candidate) => candidate.url().startsWith(`chrome-extension://${extensionId}/`))!;
    await expect.poll(() => worker.evaluate(async () => ((await chrome.storage.session.get('legacylens.activeCaptures.v1'))['legacylens.activeCaptures.v1'] ?? []).length), { timeout: 10_000 }).toBe(1);
    await expect.poll(async () => (await investigation(traceId)).events.items.some((event) => event.kind === 'extension.gap'), { timeout: 20_000 }).toBe(true);

    const hostKill = `$fixtureCache = '${cache.replace(/'/g, "''")}'; Get-CimInstance Win32_Process -Filter "Name='legacylens-host.exe'" | Where-Object { $_.ExecutablePath -like ($fixtureCache + '*') } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }`;
    run('pwsh', ['-NoProfile', '-Command', hostKill], 15_000);
    await expect(page.locator('#orderForm\\:saveOrder')).toBeEnabled();
    await page.locator('#domOnly').click();
    await expect(page.locator('#domOnly')).toHaveText('DOM changed');

    for (let offset = 0; offset < 11_000; offset += 500) {
      const batch = Array.from({ length: Math.min(500, 11_000 - offset) }, (_, index) => ({
        projectId: state.projectId, traceId, producerId: 'overflow-probe', sequence: offset + index + 1,
        eventId: (offset + index + 1).toString(16).padStart(16, '0'), kind: 'fixture.overflow', occurredAt: new Date().toISOString(), metadata: {},
      }));
      const result = await command('trace.ingest', { projectId: state.projectId, events: batch }) as { diagnostics?: { code: string }[] };
      if (result.diagnostics?.some((item) => item.code === 'capture.event_limit')) break;
    }
    const result = await investigation(traceId);
    expect(result.trace.incomplete).toBe(true);
    expect(result.diagnostics.items.some((item) => item.code === 'capture.event_limit')).toBe(true);
    expect(result.events.items.some((event) => event.kind === 'jsf.click'), 'events accepted before overflow must remain available').toBe(true);
    const investigationPage = await context.newPage();
    await investigationPage.goto(`chrome-extension://${extensionId}/investigation.html?projectId=${state.projectId}&traceId=${traceId}`);
    await expect(investigationPage.getByText('Captura incompleta')).toBeVisible({ timeout: 15_000 });
    expect(result.events.total).toBeGreaterThanOrEqual(9_000);
    await session.detach();
  } finally {
    await context?.close();
    if (fixtureStarted) run('pwsh', ['-NoProfile', '-File', join(root, 'scripts/fixtures/stop.ps1')], 30_000);
    if (profile.startsWith(cache + '\\')) rmSync(profile, { recursive: true, force: true });
  }
});
