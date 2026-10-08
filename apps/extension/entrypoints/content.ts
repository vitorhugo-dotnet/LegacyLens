import { defineContentScript } from 'wxt/utils/define-content-script';
import type { CaptureSession } from '@legacylens/contracts/src/protocol.ts';
import { chooseElement } from '../src/capture/selection.ts';
import { toStackMetadata } from '../src/capture/stack.ts';
import { recordCaptureStage } from '../src/capture/diagnostics.ts';
import { CapturePortClient } from '../src/capture/port-client.ts';

export default defineContentScript({
  matches: ['http://*/*', 'https://*/*'],
  runAt: 'document_start',
  main() {
    const global = globalThis as typeof globalThis & { __legacylensContentScriptInstalled?: boolean };
    if (global.__legacylensContentScriptInstalled) return;
    global.__legacylensContentScriptInstalled = true;
    const diagnosticsManifest = chrome.runtime.getManifest() as { host_permissions?: string[] };
    const fixtureDiagnostics = diagnosticsManifest.host_permissions?.includes('http://127.0.0.1/*') === true;
    const diagnosticsKey = 'legacylens.captureDiagnostics.v1';
    let diagnosticsWrite = Promise.resolve();
    const recordContentStage = (record: Parameters<typeof recordCaptureStage>[0]) => recordCaptureStage(record, diagnosticsManifest, (safeRecord) => {
      console.info('LegacyLens capture stage', JSON.stringify(safeRecord));
      if (!fixtureDiagnostics) return;
      diagnosticsWrite = diagnosticsWrite.then(async () => {
        const existing = ((await chrome.storage.local.get(diagnosticsKey))[diagnosticsKey] ?? []) as typeof safeRecord[];
        existing.push(safeRecord);
        if (existing.length > 100) existing.splice(0, existing.length - 100);
        await chrome.storage.local.set({ [diagnosticsKey]: existing });
      }).catch(() => console.warn('LegacyLens could not persist fixture diagnostics'));
    });

    let session: CaptureSession | undefined;
    let capturePort: CapturePortClient | undefined;
    let captureTabId = -1;
    let nonce = '';
    let selectedSource = '';
    let clickEventId = '';
    let stopChoosing: (() => void) | undefined;
    const resetCapture = () => {
      stopChoosing?.(); stopChoosing = undefined;
      const previousPort = capturePort; capturePort = undefined;
      if (previousPort) void previousPort.stop();
      if (nonce) window.dispatchEvent(new CustomEvent('legacylens:stop', { detail: { nonce } }));
      session = undefined; captureTabId = -1; selectedSource = ''; clickEventId = ''; nonce = '';
    };
    const randomEventId = () => {
      let id = '';
      do { id = [...crypto.getRandomValues(new Uint8Array(8))].map((byte) => byte.toString(16).padStart(2, '0')).join(''); } while (/^0+$/.test(id));
      return id;
    };
    const sendCaptureEvent = async (message: { type: 'capture.event'; sessionId: string; kind: string; eventId?: string; parentEventId?: string; metadata: Record<string, string> }) => {
      const eventId = message.eventId ?? randomEventId();
      const client = capturePort;
      if (!client) throw new Error('PORT_CLOSED');
      const result = await client.send({ eventId, kind: message.kind, ...(message.parentEventId ? { parentEventId: message.parentEventId } : {}), metadata: message.metadata });
      recordContentStage({ stage: 'content.port', outcome: 'accepted', traceId: message.sessionId, tabId: captureTabId, eventId });
      return result;
    };
    const beginCapture = async (projectId: string, nextSession: CaptureSession, expectedOrigin: string, tabId: number) => {
      const expiresAt = Date.parse(nextSession.expiresAt);
      if (nextSession.projectId !== projectId || !/^[a-f0-9]{32}$/i.test(nextSession.id)
        || !Number.isFinite(expiresAt) || expiresAt <= Date.now() || expectedOrigin !== location.origin) {
        throw new Error('Invalid, expired, or mismatched capture session');
      }
      if (session?.id === nextSession.id) return;
      resetCapture();
      session = nextSession;
      captureTabId = tabId;
      capturePort = new CapturePortClient((name) => chrome.runtime.connect({ name }), (stage, eventId, code) => {
        const outcome = stage.endsWith('ack') || stage.endsWith('ready') ? 'accepted'
          : stage.includes('timeout') ? 'timeout' : stage.includes('nack') || stage.includes('overflow') || stage.includes('invalid') || stage.includes('exhausted') ? 'rejected' : 'started';
        recordContentStage({ stage: 'content.port', outcome, traceId: nextSession.id, tabId,
          ...(eventId ? { eventId } : {}), ...(code ? { code } : {}) });
      }, undefined, (message) => new Promise((resolve, reject) => {
        chrome.runtime.sendMessage(message, (reply) => {
          const error = chrome.runtime.lastError;
          if (error) reject(new Error(error.message));
          else resolve(reply);
        });
      }));
      try { await capturePort.start(nextSession.id); }
      catch (error) { resetCapture(); throw error; }
      nonce = [...crypto.getRandomValues(new Uint8Array(16))].map((n) => n.toString(16).padStart(2, '0')).join('');
      window.dispatchEvent(new CustomEvent('legacylens:start', { detail: { nonce, traceId: session.id, origin: location.origin } }));
      stopChoosing = chooseElement(document, (id) => {
        selectedSource = id;
        clickEventId = randomEventId();
        recordContentStage({ stage: 'content.selection', outcome: 'accepted', traceId: session!.id, tabId: captureTabId, eventId: clickEventId });
        window.dispatchEvent(new CustomEvent('legacylens:select', { detail: { nonce, source: id } }));
        void sendCaptureEvent({ type: 'capture.event', sessionId: session!.id, kind: 'jsf.click', eventId: clickEventId, metadata: { source: id } })
          .catch((error) => console.warn('LegacyLens capture event failed', error));
      });
    };
    chrome.runtime.onMessage.addListener((message: unknown, _sender, respond) => {
      const msg = message as { type?: unknown; projectId?: unknown; session?: unknown; origin?: unknown; tabId?: unknown; traceId?: unknown };
      try {
        if (msg?.type === 'capture.begin' && typeof msg.projectId === 'string' && typeof msg.origin === 'string'
          && Number.isSafeInteger(msg.tabId) && (msg.tabId as number) >= 0 && msg.session && typeof msg.session === 'object') {
          void beginCapture(msg.projectId, msg.session as CaptureSession, msg.origin, msg.tabId as number)
            .then(() => respond({ ok: true }), (error: unknown) => respond({ error: error instanceof Error ? error.message : 'Capture setup failed' }));
          return true;
        } else if (msg?.type === 'capture.end' && typeof msg.traceId === 'string') {
          const stop = async () => {
            if (!session || session.id === msg.traceId) {
              const current = capturePort; capturePort = undefined;
              const gap = await current?.stop();
              resetCapture();
              respond({ ok: true, gap: gap === true });
              return;
            }
            respond({ ok: true });
          };
          void stop();
          return true;
        }
      } catch (error) {
        console.error('LegacyLens could not update capture state', error);
        respond({ error: error instanceof Error ? error.message : 'Capture setup failed' });
      }
      return false;
    });
    window.addEventListener('legacylens:ajax', (event) => {
      const detail = (event as CustomEvent).detail as { nonce?: string; source?: string; propagation?: string; traceparent?: string; spanId?: string; code?: string };
      if (!session || !detail || detail.nonce !== nonce) return;
      if (typeof detail.code === 'string' && /^UNSUPPORTED_[A-Z_]+$/.test(detail.code)) {
        void sendCaptureEvent({ type: 'capture.event', sessionId: session.id, kind: 'extension.diagnostic', metadata: { code: detail.code } }).catch(() => {});
        console.warn('LegacyLens: page does not expose the supported PrimeFaces Ajax API.');
        return;
      }
      if (typeof detail.source !== 'string' || detail.source.length > 256) return;
      if (detail.source !== selectedSource || !clickEventId) return;
      if (detail.propagation === 'attempted' && !detail.spanId && !detail.traceparent) {
        void sendCaptureEvent({ type: 'capture.event', sessionId: session.id, kind: 'primefaces.propagation_attempt', parentEventId: clickEventId, metadata: { source: detail.source } }).catch((error) => console.warn('LegacyLens capture transport interrupted', error));
        return;
      }
      if (detail.propagation !== 'propagated' || !detail.traceparent || !detail.spanId) return;
      if (detail.traceparent !== `00-${session.id}-${detail.spanId}-01` || !/^[a-f0-9]{16}$/i.test(detail.spanId)) return;
      void sendCaptureEvent({ type: 'capture.event', sessionId: session.id, kind: 'primefaces.ajax', parentEventId: clickEventId, metadata: { source: detail.source, spanId: detail.spanId } }).catch((error) => console.warn('LegacyLens capture transport interrupted', error));
    });
    window.addEventListener('legacylens:network', (event) => {
      const detail = (event as CustomEvent).detail as { nonce?: string; source?: string; transport?: string; propagation?: string;
        traceparent?: string; spanId?: string; frames?: unknown; stackGap?: string };
      if (!session || !clickEventId || detail?.nonce !== nonce || detail.source !== selectedSource) return;
      if (detail.transport !== 'fetch' && detail.transport !== 'xhr') return;
      const propagated = detail.propagation === 'propagated' && typeof detail.spanId === 'string'
        && /^[a-f0-9]{16}$/i.test(detail.spanId) && !/^0+$/.test(detail.spanId)
        && detail.traceparent === `00-${session.id}-${detail.spanId}-01`;
      if (!propagated && detail.propagation !== 'attempted') return;
      const stack = toStackMetadata(detail.frames, detail.stackGap === 'ASYNC_BOUNDARY' ? 'ASYNC_BOUNDARY' : undefined);
      const metadata = { source: detail.source, transport: detail.transport,
        ...(propagated ? { spanId: detail.spanId } : {}),
        ...stack };
      void sendCaptureEvent({ type: 'capture.event', sessionId: session.id, kind: propagated ? 'browser.network' : 'browser.propagation_attempt',
        parentEventId: clickEventId, metadata }).catch((error) => console.warn('LegacyLens capture transport interrupted', error));
    });
  },
});
