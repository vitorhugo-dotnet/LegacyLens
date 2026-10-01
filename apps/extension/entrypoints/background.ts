import { defineBackground } from 'wxt/utils/define-background';
import type { ProjectListResult } from '@legacylens/contracts/src/protocol.ts';
import { NativeClient } from '../src/native/client.ts';
import { CaptureController, type ActiveCapture } from '../src/capture/session.ts';

const client = new NativeClient();
const stateKey = 'legacylens.activeCaptures.v1';
const controller = new CaptureController(client, {
  async load() { return ((await chrome.storage.session.get(stateKey))[stateKey] ?? []) as ActiveCapture[]; },
  async save(entries) { await chrome.storage.session.set({ [stateKey]: entries }); },
});

function originOf(url?: string): string | undefined {
  if (!url) return undefined;
  try { const parsed = new URL(url); return ['https:', 'http:'].includes(parsed.protocol) ? parsed.origin : undefined; } catch { return undefined; }
}

export default defineBackground(() => {
  void controller.restore();
  chrome.action.onClicked.addListener(async (tab) => {
    if (tab.id === undefined) return;
    const origin = originOf(tab.url);
    if (!origin) { await chrome.action.setBadgeText({ tabId: tab.id, text: 'N/A' }); return; }
    const selected = new URL(origin);
    const permission = { origins: [`${selected.protocol}//${selected.hostname}/*`] };
    if (!(await chrome.permissions.contains(permission)) && !(await chrome.permissions.request(permission))) return;
    try { await chrome.tabs.sendMessage(tab.id, { type: 'selection.open' }); }
    catch { await chrome.action.setBadgeText({ tabId: tab.id, text: 'ERR' }); }
  });

  chrome.runtime.onMessage.addListener((message: unknown, sender, respond) => {
    const msg = message as Record<string, unknown>;
    const tabId = sender.tab?.id;
    const senderOrigin = originOf(sender.url);
    if (tabId === undefined || !senderOrigin || !msg || typeof msg.type !== 'string') return;
    const run = async () => {
      if (msg.type === 'projects.list') return client.request<ProjectListResult>('project.list', { offset: typeof msg.offset === 'number' ? msg.offset : 0, limit: 200 });
      if (msg.type === 'projects.open') {
        await chrome.tabs.create({ url: chrome.runtime.getURL('investigation.html') });
        return { opened: true };
      }
      if (msg.type === 'investigation.open' && typeof msg.projectId === 'string' && /^[a-f0-9]{32}$/i.test(String(msg.traceId))) {
        const url = new URL(chrome.runtime.getURL('investigation.html'));
        url.searchParams.set('projectId', msg.projectId);
        url.searchParams.set('traceId', String(msg.traceId));
        await chrome.tabs.create({ url: url.toString() });
        return { opened: true };
      }
      if (msg.type === 'capture.start' && typeof msg.projectId === 'string') {
        const session = await controller.start({ projectId: msg.projectId, tabId, origin: senderOrigin });
        return { session, gap: false };
      }
      if (msg.type === 'capture.stop') { await controller.stop(tabId); return { stopped: true }; }
      if (msg.type === 'capture.event' && typeof msg.kind === 'string' && typeof msg.sessionId === 'string') {
        await controller.restore();
        const active = controller.get(tabId);
        if (!active || active.session.id !== msg.sessionId || active.request.origin !== senderOrigin) throw new Error('Unmatched capture event');
        if (!['jsf.click', 'primefaces.ajax', 'primefaces.propagation_attempt', 'browser.network', 'browser.propagation_attempt', 'extension.diagnostic'].includes(msg.kind)) throw new Error('Unsupported event kind');
        const metadata = msg.metadata;
        if (!metadata || typeof metadata !== 'object' || Array.isArray(metadata)) throw new Error('Invalid event metadata');
        const safe: Record<string, string> = {};
        for (const key of ['source', 'code', 'spanId', 'transport', 'frameChain', 'stackGap']) {
          const value = (metadata as Record<string, unknown>)[key];
          if (typeof value === 'string' && value.length <= 256) safe[key] = value;
        }
        const eventId = typeof msg.eventId === 'string' ? msg.eventId : undefined;
        const parentEventId = typeof msg.parentEventId === 'string' ? msg.parentEventId : undefined;
        if (msg.kind === 'jsf.click' && !eventId) throw new Error('Missing click identity');
        if (msg.kind !== 'jsf.click' && msg.kind !== 'extension.diagnostic' && !parentEventId) throw new Error('Missing interaction parent');
        if (safe.spanId && (!/^[a-f0-9]{16}$/i.test(safe.spanId) || /^0+$/.test(safe.spanId))) throw new Error('Invalid span identity');
        if (msg.kind === 'browser.network' && (!safe.spanId || !['fetch', 'xhr'].includes(safe.transport ?? ''))) throw new Error('Invalid network effect');
        if (msg.kind === 'browser.propagation_attempt' && safe.spanId) throw new Error('Unverified span identity');
        await controller.record(tabId, msg.kind, safe, { ...(eventId ? { eventId } : {}), ...(parentEventId ? { parentEventId } : {}) });
        return { accepted: true, gap: active.gap };
      }
      throw new Error('Unsupported capture message');
    };
    void run().then(respond, (error: unknown) => respond({ error: error instanceof Error ? error.message : 'Capture failed' }));
    return true;
  });
});
