import { defineBackground } from 'wxt/utils/define-background';
import type { CaptureSession, ProjectListResult } from '@legacylens/contracts/src/protocol.ts';
import { NativeClient } from '../src/native/client.ts';
import { CaptureController, type ActiveCapture } from '../src/capture/session.ts';
import { handleCapturePort } from '../src/capture/background-port.ts';
import { handleCaptureEvent, handleCaptureHandshake } from '../src/capture/background-messages.ts';
import { CAPTURE_PORT_NAME } from '../src/capture/port-protocol.ts';
import { recordCaptureStage } from '../src/capture/diagnostics.ts';
import { CAPTURE_TRANSPORT_STORAGE_PREFIX, parseCaptureStorageRequest } from '../src/capture/storage-transport.ts';

const client = new NativeClient();
const stateKey = 'legacylens.activeCaptures.v1';
const selectedProjectsKey = 'legacylens.selectedProjects.v1';
const rootMenuId = 'legacylens.root';
const statusMenuId = 'legacylens.status';
const startMenuId = 'legacylens.capture.start';
const stopMenuId = 'legacylens.capture.stop';
const manageMenuId = 'legacylens.projects.manage';
const pagePatterns = ['http://*/*', 'https://*/*'];
const fixtureMode = (() => {
  try { return chrome.runtime.getManifest().host_permissions?.includes('http://127.0.0.1/*') === true; }
  catch { return false; }
})();
type FixtureDebugGlobal = typeof globalThis & {
  __legacylensFixturePhase?: string;
  __legacylensFixtureRuntimeConnectRegistered?: boolean;
  __legacylensFixtureRuntimeConnectCount?: number;
  __legacylensFixtureStorageRequestCount?: number;
  __legacylensFixtureStorageChangeCount?: number;
  __legacyLensCaptureDiagnostics?: ReturnType<typeof fixtureDiagnosticRecords>;
};
function fixtureDiagnosticRecords() { return [] as Array<import('../src/capture/diagnostics.ts').CaptureStageRecord>; }
function setFixturePhase(phase: string): void {
  (globalThis as FixtureDebugGlobal).__legacylensFixturePhase = phase;
}
function recordDiagnostic(record: import('../src/capture/diagnostics.ts').CaptureStageRecord): void {
  recordCaptureStage(record, chrome.runtime.getManifest() as { host_permissions?: string[] }, (safeRecord) => {
    console.info('LegacyLens capture stage', JSON.stringify(safeRecord));
    if (!fixtureMode) return;
    const target = globalThis as FixtureDebugGlobal;
    const ring = target.__legacyLensCaptureDiagnostics ?? (target.__legacyLensCaptureDiagnostics = fixtureDiagnosticRecords());
    ring.push(safeRecord);
    if (ring.length > 100) ring.splice(0, ring.length - 100);
  });
}
setFixturePhase(fixtureMode ? 'manifest-fixture-enabled' : 'manifest-fixture-disabled');
// @types/chrome currently omits the documented dynamic-menu lifecycle API.
const dynamicContextMenus = chrome.contextMenus as typeof chrome.contextMenus & {
  onShown?: { addListener(listener: (info: unknown, tab?: chrome.tabs.Tab) => void): void };
  refresh?: () => Promise<void> | void;
};
const controller = new CaptureController(client, {
  async load() { return ((await chrome.storage.session.get(stateKey))[stateKey] ?? []) as ActiveCapture[]; },
  async save(entries) { await chrome.storage.session.set({ [stateKey]: entries }); },
}, recordDiagnostic);
const capturePortsByTab = new Map<number, Set<() => void>>();
const CAPTURE_OUTBOX_POLL_INTERVAL_MS = 250;
let captureOutboxPollTimer: ReturnType<typeof setTimeout> | undefined;
let captureOutboxPollRunning = false;
let requestCaptureOutboxPoll = () => {};
function closeCapturePorts(tabId: number): void {
  for (const close of capturePortsByTab.get(tabId) ?? []) close();
  capturePortsByTab.delete(tabId);
}

function originOf(url?: string): string | undefined {
  if (!url) return undefined;
  try { const parsed = new URL(url); return ['https:', 'http:'].includes(parsed.protocol) ? parsed.origin : undefined; } catch { return undefined; }
}

function createMenu(properties: chrome.contextMenus.CreateProperties): Promise<void> {
  return new Promise((resolve, reject) => {
    chrome.contextMenus.create(properties, () => {
      const error = chrome.runtime.lastError;
      if (error) reject(new Error(error.message));
      else resolve();
    });
  });
}

function removeAllMenus(): Promise<void> {
  return new Promise((resolve, reject) => {
    chrome.contextMenus.removeAll(() => {
      const error = chrome.runtime.lastError;
      if (error) reject(new Error(error.message));
      else resolve();
    });
  });
}

async function openInvestigation(projectId?: string, traceId?: string): Promise<void> {
  const url = new URL(chrome.runtime.getURL('investigation.html'));
  if (projectId) url.searchParams.set('projectId', projectId);
  if (traceId) url.searchParams.set('traceId', traceId);
  await chrome.tabs.create({ url: url.toString() });
}

async function selectedProjectFor(tabId: number): Promise<string | undefined> {
  const selected = ((await chrome.storage.session.get(selectedProjectsKey))[selectedProjectsKey] ?? {}) as Record<string, string>;
  return selected[String(tabId)];
}

async function renderMenu(tab?: chrome.tabs.Tab): Promise<void> {
  const tabId = tab?.id;
  const selectedId = tabId === undefined ? undefined : await selectedProjectFor(tabId);
  const active = tabId === undefined ? undefined : controller.get(tabId);
  let projects: ProjectListResult | undefined;
  let hostError = false;
  try {
    projects = await client.request<ProjectListResult>('project.list', { offset: 0, limit: 200 });
  } catch {
    hostError = true;
  }

  await removeAllMenus();
  const nextProjectMap = new Map<string, string>();
  await createMenu({ id: rootMenuId, title: 'LegacyLens', contexts: ['page'], documentUrlPatterns: pagePatterns });

  if (hostError) {
    await createMenu({ id: statusMenuId, parentId: rootMenuId, title: 'Host nativo desconectado', enabled: false, contexts: ['page'], documentUrlPatterns: pagePatterns });
  } else if (!projects?.items.length) {
    await createMenu({ id: statusMenuId, parentId: rootMenuId, title: 'Nenhum projeto registrado', enabled: false, contexts: ['page'], documentUrlPatterns: pagePatterns });
  } else {
    for (const [index, project] of projects.items.entries()) {
      const id = `legacylens.project.${index}`;
      nextProjectMap.set(id, project.id);
      await createMenu({ id, parentId: rootMenuId, type: 'radio', title: project.name, checked: project.id === selectedId,
        contexts: ['page'], documentUrlPatterns: pagePatterns });
    }
  }

  const selectedProjectExists = Boolean(projects?.items.some((project) => project.id === selectedId));
  await createMenu({ id: startMenuId, parentId: rootMenuId, title: 'Iniciar captura', enabled: selectedProjectExists && !active,
    contexts: ['page'], documentUrlPatterns: pagePatterns });
  await createMenu({ id: stopMenuId, parentId: rootMenuId, title: 'Parar captura', enabled: Boolean(active),
    contexts: ['page'], documentUrlPatterns: pagePatterns });
  await createMenu({ id: manageMenuId, parentId: rootMenuId, title: 'Gerenciar projetos', contexts: ['page'], documentUrlPatterns: pagePatterns });
  // `removeAll` clears stale items across extension reloads; this map resolves this render's radio entries.
  projectMenuMap.clear();
  for (const [menuId, projectId] of nextProjectMap) projectMenuMap.set(menuId, projectId);
  // Chrome added refresh() later than the base contextMenus API. The menu is
  // still usable on browsers without it; the next install/startup render will
  // populate the submenu, while supported browsers refresh it per tab.
  await dynamicContextMenus.refresh?.();
}

const projectMenuMap = new Map<string, string>();
let pendingMenuRender = Promise.resolve();

function queueMenuRender(tab?: chrome.tabs.Tab): Promise<void> {
  const next = pendingMenuRender.catch(() => {}).then(() => renderMenu(tab));
  pendingMenuRender = next;
  return next;
}

async function startCapture(tab?: chrome.tabs.Tab, options: { requestPermission?: boolean; projectId?: string; notifyContent?: boolean; injectScripts?: boolean } = {}): Promise<CaptureSession | undefined> {
  const tabId = tab?.id;
  const origin = originOf(tab?.url);
  if (tabId === undefined || !origin) return;

  try {
    if (options.requestPermission !== false) {
      // Invoke permissions.request synchronously in this menu click handler so the browser keeps the user gesture.
      const parsed = new URL(origin);
      const permissionRequest = chrome.permissions.request({ origins: [`${parsed.protocol}//${parsed.hostname}/*`] });
      if (!await permissionRequest) return;
    }
    const projectId = options.projectId ?? await selectedProjectFor(tabId);
    if (!projectId) throw new Error('Selecione um projeto LegacyLens no submenu.');
    setFixturePhase('project-list:pending');
    const projects = await client.request<ProjectListResult>('project.list', { offset: 0, limit: 200 });
    setFixturePhase('project-list:complete');
    if (fixtureMode) console.info('LegacyLens fixture capture: project.list completed');
    if (!projects.items.some((project) => project.id === projectId)) throw new Error('O projeto selecionado não está mais registrado.');

    if (options.injectScripts !== false) {
      await chrome.scripting.executeScript({ target: { tabId }, files: ['content-scripts/content.js'] });
      await chrome.scripting.executeScript({ target: { tabId }, files: ['content-scripts/page.js'], world: 'MAIN' });
    }
    setFixturePhase('capture-start:pending');
    const session = await controller.start({ projectId, tabId, origin });
    requestCaptureOutboxPoll();
    setFixturePhase('capture-start:complete');
    if (fixtureMode) console.info('LegacyLens fixture capture: capture.start completed');
    if (options.notifyContent !== false) {
      try {
        const reply = await chrome.tabs.sendMessage(tabId, { type: 'capture.begin', projectId, session, origin, tabId }) as { ok?: boolean; error?: string };
        if (reply?.error || !reply?.ok) throw new Error(reply?.error ?? 'Content script did not acknowledge capture start');
      } catch (error) {
        await controller.stop(tabId);
        throw error;
      }
    }
    await chrome.action.setBadgeText({ tabId, text: '' });
    return session;
  } catch (error) {
    console.error('LegacyLens could not start capture', error);
    await chrome.action.setBadgeText({ tabId, text: 'ERR' });
    return undefined;
  }
}

async function stopCapture(tab?: chrome.tabs.Tab, options: { notifyContent?: boolean; openInvestigation?: boolean } = {}): Promise<void> {
  const tabId = tab?.id;
  if (tabId === undefined) return;
  try {
    await controller.restore();
    const active = controller.get(tabId);
    if (!active) return;
    if (options.notifyContent !== false) {
      try {
        const reply = await chrome.tabs.sendMessage(tabId, { type: 'capture.end', traceId: active.session.id }) as { ok?: boolean; gap?: boolean };
        if (!reply?.ok || reply.gap) await controller.markGap(tabId);
      }
      catch (error) {
        await controller.markGap(tabId);
        console.info('LegacyLens stopped capture after the page content script became unavailable', error);
      }
    }
    await controller.stop(tabId);
    if (options.openInvestigation !== false) await openInvestigation(active.request.projectId, active.session.id);
    await chrome.action.setBadgeText({ tabId, text: '' });
  } catch (error) {
    console.error('LegacyLens could not stop capture', error);
    await chrome.action.setBadgeText({ tabId, text: 'ERR' });
  }
}

async function selectProject(menuId: string, tab?: chrome.tabs.Tab): Promise<void> {
  const tabId = tab?.id;
  const projectId = projectMenuMap.get(menuId);
  if (tabId === undefined || !projectId) return;
  const selected = ((await chrome.storage.session.get(selectedProjectsKey))[selectedProjectsKey] ?? {}) as Record<string, string>;
  await chrome.storage.session.set({ [selectedProjectsKey]: { ...selected, [String(tabId)]: projectId } });
  await queueMenuRender(tab);
}

export default defineBackground(() => {
  // Capture transport is critical; register it before menu APIs that can fail independently.
  const processingStorageRequests = new Map<string, Promise<void>>();
  const processStorageRequest = (key: string, value: unknown): Promise<void> => {
    const existing = processingStorageRequests.get(key);
    if (existing) return existing;
    const processing = (async () => {
      const request = parseCaptureStorageRequest(key, value);
      if (!request) {
        await chrome.storage.local.remove(key);
        return;
      }
      if (fixtureMode) {
        const debug = globalThis as FixtureDebugGlobal;
        debug.__legacylensFixtureStorageRequestCount = (debug.__legacylensFixtureStorageRequestCount ?? 0) + 1;
        setFixturePhase(`storage:${String(request.message.type ?? 'unknown')}`);
      }
      let reply: unknown;
      try {
        const tab = await chrome.tabs.get(request.tabId);
        const actualOrigin = originOf(tab.url);
        if (!tab.url || !actualOrigin || actualOrigin !== request.origin) {
          reply = { ready: false, error: 'Capture tab origin changed', code: 'ORIGIN_MISMATCH' };
        } else {
          const verifiedSender = { url: tab.url, tab: { id: tab.id, url: tab.url } };
          if (request.message.type === 'capture.ready') {
            reply = await handleCaptureHandshake(request.message, verifiedSender, controller);
          } else if (request.message.type === 'capture.event') {
            reply = await handleCaptureEvent(request.message, verifiedSender, controller, recordDiagnostic);
          } else {
            reply = { error: 'Capture request type is invalid', code: 'EVENT_INVALID' };
          }
        }
      } catch (error) {
        reply = { error: error instanceof Error ? error.message : 'Capture request could not be processed', code: 'TRANSPORT_FAILED' };
      }
      await chrome.storage.local.remove(key);
      try {
        await chrome.tabs.sendMessage(request.tabId, { type: 'capture.transport.reply', requestId: request.requestId, reply });
      } catch {
        // The tab can close or navigate after the host accepted an event. The stable event ID makes retry safe.
      }
    })().finally(() => processingStorageRequests.delete(key));
    processingStorageRequests.set(key, processing);
    return processing;
  };
  const scheduleCaptureOutboxPoll = (delay = 0): void => {
    if (captureOutboxPollTimer || captureOutboxPollRunning) return;
    captureOutboxPollTimer = globalThis.setTimeout(() => {
      captureOutboxPollTimer = undefined;
      void pollCaptureOutbox();
    }, delay);
  };
  const pollCaptureOutbox = async (): Promise<void> => {
    if (captureOutboxPollRunning) return;
    captureOutboxPollRunning = true;
    try {
      if ((await controller.activeTabIds()).length) {
        const stored = await chrome.storage.local.get(null);
        await Promise.all(Object.entries(stored)
          .filter(([key]) => key.startsWith(CAPTURE_TRANSPORT_STORAGE_PREFIX))
          .map(([key, value]) => processStorageRequest(key, value)));
      }
    } catch (error) {
      console.warn('LegacyLens capture outbox polling failed', error);
    } finally {
      captureOutboxPollRunning = false;
      try { if ((await controller.activeTabIds()).length) scheduleCaptureOutboxPoll(CAPTURE_OUTBOX_POLL_INTERVAL_MS); }
      catch (error) { console.warn('LegacyLens could not refresh capture outbox polling state', error); }
    }
  };
  requestCaptureOutboxPoll = scheduleCaptureOutboxPoll;
  chrome.storage.onChanged.addListener((changes, areaName) => {
    if (areaName !== 'local') return;
    for (const [key, change] of Object.entries(changes)) {
      if (!key.startsWith(CAPTURE_TRANSPORT_STORAGE_PREFIX)) continue;
      if (fixtureMode) {
        const debug = globalThis as FixtureDebugGlobal;
        debug.__legacylensFixtureStorageChangeCount = (debug.__legacylensFixtureStorageChangeCount ?? 0) + 1;
        setFixturePhase('storage:change');
      }
      if (change.newValue !== undefined) void processStorageRequest(key, change.newValue)
        .catch((error) => console.warn('LegacyLens capture outbox processing failed', error));
    }
  });
  void chrome.storage.local.get(null).then((stored) => Promise.all(Object.entries(stored)
    .filter(([key]) => key.startsWith(CAPTURE_TRANSPORT_STORAGE_PREFIX))
    .map(([key, value]) => processStorageRequest(key, value))))
    .then(() => controller.activeTabIds())
    .then((tabIds) => { if (tabIds.length) scheduleCaptureOutboxPoll(CAPTURE_OUTBOX_POLL_INTERVAL_MS); })
    .catch((error) => console.warn('LegacyLens could not recover capture outbox', error));
  chrome.runtime.onConnect.addListener((port) => {
    if (fixtureMode) {
      const debug = globalThis as FixtureDebugGlobal;
      debug.__legacylensFixtureRuntimeConnectCount = (debug.__legacylensFixtureRuntimeConnectCount ?? 0) + 1;
      setFixturePhase(`port.connect:${port.name}`);
      console.info(`LegacyLens fixture port connected: ${port.name}`);
    }
    if (port.name !== CAPTURE_PORT_NAME) {
      try { port.disconnect(); } catch { /* already disconnected */ }
      return;
    }
    const tabId = port.sender?.tab?.id;
    let cleanup = () => {};
    cleanup = handleCapturePort(port, controller, recordDiagnostic, () => {
      const cleanups = tabId === undefined ? undefined : capturePortsByTab.get(tabId);
      cleanups?.delete(cleanup);
      if (tabId !== undefined && cleanups?.size === 0) capturePortsByTab.delete(tabId);
    });
    if (tabId !== undefined) {
      const cleanups = capturePortsByTab.get(tabId) ?? new Set<() => void>();
      cleanups.add(cleanup);
      capturePortsByTab.set(tabId, cleanups);
    }
  });
  if (fixtureMode) (globalThis as FixtureDebugGlobal).__legacylensFixtureRuntimeConnectRegistered = true;
  if (fixtureMode) {
    globalThis.addEventListener('error', (event) => setFixturePhase(`uncaught:${event.message.slice(0, 120)}`));
    globalThis.addEventListener('unhandledrejection', () => setFixturePhase('unhandled-rejection'));
  }
  if (fixtureMode) {
    (globalThis as FixtureDebugGlobal & {
      __legacyLensFixtureCapture?: (request: { action: 'start' | 'stop'; tabId: number; projectId?: string; openInvestigation?: boolean }) => Promise<{ ok: true; session?: CaptureSession }>;
    }).__legacyLensFixtureCapture = async ({ action, tabId, projectId, openInvestigation }) => {
      const tab = await chrome.tabs.get(tabId);
      if (action === 'start') {
        if (!projectId) throw new Error('Fixture capture needs a project ID');
        const selected = ((await chrome.storage.session.get(selectedProjectsKey))[selectedProjectsKey] ?? {}) as Record<string, string>;
        await chrome.storage.session.set({ [selectedProjectsKey]: { ...selected, [String(tabId)]: projectId } });
        const session = await startCapture(tab, { requestPermission: false, projectId, notifyContent: false });
        if (!session) throw new Error(`Fixture capture did not start (background phase: ${(globalThis as FixtureDebugGlobal).__legacylensFixturePhase ?? 'unknown'})`);
        const origin = originOf(tab.url);
        if (!origin) throw new Error('Fixture tab does not have an HTTP origin');
        const reply = await chrome.tabs.sendMessage(tabId, { type: 'capture.begin', projectId, session, origin, tabId }) as { ok?: boolean; error?: string };
        if (reply?.error || !reply?.ok) {
          await controller.stop(tabId);
          const debug = globalThis as FixtureDebugGlobal;
          const phase = debug.__legacylensFixturePhase ?? 'unknown';
          const listener = debug.__legacylensFixtureRuntimeConnectRegistered === true;
          const connections = debug.__legacylensFixtureRuntimeConnectCount ?? 0;
          const storageRequests = debug.__legacylensFixtureStorageRequestCount ?? 0;
          const storageChanges = debug.__legacylensFixtureStorageChangeCount ?? 0;
          const stored = await chrome.storage.local.get(null);
          const pendingStorageKeys = Object.keys(stored).filter((key) => key.startsWith(CAPTURE_TRANSPORT_STORAGE_PREFIX)).length;
          throw new Error(`${reply?.error ?? 'Content script did not acknowledge fixture capture'} (phase: ${phase}; onConnect registered: ${listener}; calls: ${connections}; storage changes: ${storageChanges}; valid storage requests: ${storageRequests}; pending outbox keys: ${pendingStorageKeys})`);
        }
        return { ok: true, session };
      }
      await stopCapture(tab, { openInvestigation: openInvestigation === true });
      return { ok: true };
    };
  }
  void controller.restore();
  const initializeMenu = () => {
    void queueMenuRender().catch((error) => console.error('LegacyLens could not initialize context menu', error));
  };
  chrome.runtime.onInstalled.addListener(initializeMenu);
  chrome.runtime.onStartup.addListener(initializeMenu);
  if (dynamicContextMenus.onShown?.addListener) {
    dynamicContextMenus.onShown.addListener((_info, tab) => {
      void controller.restore().then(() => queueMenuRender(tab)).catch((error) => console.error('LegacyLens could not refresh context menu', error));
    });
  } else {
    console.warn('LegacyLens dynamic context-menu updates are unavailable; using the startup menu');
  }
  chrome.action.onClicked.addListener(() => { void openInvestigation().catch((error) => console.error('LegacyLens could not open investigation', error)); });
  chrome.contextMenus.onClicked.addListener((info, tab) => {
    const menuId = String(info.menuItemId);
    if (projectMenuMap.has(menuId)) void selectProject(menuId, tab).catch((error) => console.error('LegacyLens project selection failed', error));
    else if (menuId === startMenuId) void startCapture(tab);
    else if (menuId === stopMenuId) void stopCapture(tab);
    else if (menuId === manageMenuId) void openInvestigation().catch((error) => console.error('LegacyLens could not open project management', error));
  });
  chrome.tabs.onRemoved.addListener((tabId) => {
    closeCapturePorts(tabId);
    void controller.markGap(tabId).then(() => controller.stop(tabId)).catch((error) => console.warn('LegacyLens could not stop capture for closed tab', error));
    void chrome.storage.session.get(selectedProjectsKey).then((stored) => {
      const selected = (stored[selectedProjectsKey] ?? {}) as Record<string, string>;
      delete selected[String(tabId)];
      return chrome.storage.session.set({ [selectedProjectsKey]: selected });
    }).catch((error) => console.warn('LegacyLens could not clear closed-tab project selection', error));
  });

  chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
    if (!changeInfo.url || !controller.get(tabId)) return;
    closeCapturePorts(tabId);
    void controller.markGap(tabId).then(() => stopCapture(tab, { notifyContent: false, openInvestigation: false }))
      .catch((error) => console.warn('LegacyLens could not stop capture after page navigation', error));
  });

});
