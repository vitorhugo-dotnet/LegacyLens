import { defineContentScript } from 'wxt/utils/define-content-script';
import type { CaptureSession } from '@legacylens/contracts/src/protocol.ts';
import { chooseElement } from '../src/capture/selection.ts';
import { toStackMetadata } from '../src/capture/stack.ts';

type Reply<T> = T | { error: string };
async function ask<T>(message: object): Promise<T> {
  const reply = await chrome.runtime.sendMessage(message) as Reply<T>;
  if (reply && typeof reply === 'object' && 'error' in reply) throw new Error(reply.error);
  return reply as T;
}

export default defineContentScript({
  matches: ['http://*/*', 'https://*/*'],
  runAt: 'document_start',
  main() {
    const global = globalThis as typeof globalThis & { __legacylensContentScriptInstalled?: boolean };
    if (global.__legacylensContentScriptInstalled) return;
    global.__legacylensContentScriptInstalled = true;
    const fixtureMode = chrome.runtime.getManifest().host_permissions?.includes('http://127.0.0.1/*') === true;

    let session: CaptureSession | undefined;
    let nonce = '';
    let selectedSource = '';
    let clickEventId = '';
    let stopChoosing: (() => void) | undefined;
    const resetCapture = () => {
      stopChoosing?.(); stopChoosing = undefined;
      if (nonce) window.dispatchEvent(new CustomEvent('legacylens:stop', { detail: { nonce } }));
      session = undefined; selectedSource = ''; clickEventId = ''; nonce = '';
    };
    const beginCapture = (projectId: string, nextSession: CaptureSession, expectedOrigin: string) => {
      const expiresAt = Date.parse(nextSession.expiresAt);
      if (nextSession.projectId !== projectId || !/^[a-f0-9]{32}$/i.test(nextSession.id)
        || !Number.isFinite(expiresAt) || expiresAt <= Date.now() || expectedOrigin !== location.origin) {
        throw new Error('Invalid, expired, or mismatched capture session');
      }
      if (session?.id === nextSession.id) return;
      resetCapture();
      session = nextSession;
      nonce = [...crypto.getRandomValues(new Uint8Array(16))].map((n) => n.toString(16).padStart(2, '0')).join('');
      window.dispatchEvent(new CustomEvent('legacylens:start', { detail: { nonce, traceId: session.id, origin: location.origin } }));
      stopChoosing = chooseElement(document, (id) => {
        selectedSource = id;
        do { clickEventId = [...crypto.getRandomValues(new Uint8Array(8))].map((byte) => byte.toString(16).padStart(2, '0')).join(''); }
        while (/^0+$/.test(clickEventId));
        window.dispatchEvent(new CustomEvent('legacylens:select', { detail: { nonce, source: id } }));
        void ask({ type: 'capture.event', sessionId: session!.id, kind: 'jsf.click', eventId: clickEventId, metadata: { source: id } })
          .catch((error) => console.warn('LegacyLens capture event failed', error));
      });
    };
    chrome.runtime.onMessage.addListener((message: unknown, _sender, respond) => {
      const msg = message as { type?: unknown; projectId?: unknown; session?: unknown; origin?: unknown; traceId?: unknown };
      if (fixtureMode && msg?.type === 'fixture.capture.start' && typeof msg.projectId === 'string') {
        void ask<{ ok: boolean; session: CaptureSession }>({ type: 'fixture.capture.start', projectId: msg.projectId })
          .then((reply) => {
            try {
              if (!reply?.session) throw new Error('Background did not return a fixture capture session');
              beginCapture(msg.projectId as string, reply.session, location.origin);
              respond({ ok: true, session: reply.session });
            } catch (error) {
              respond({ error: error instanceof Error ? error.message : 'Fixture capture setup failed' });
            }
          }, (error: unknown) => respond({ error: error instanceof Error ? error.message : 'Fixture capture start failed' }));
        return true;
      }
      if (fixtureMode && msg?.type === 'fixture.capture.stop') {
        void ask<{ ok: boolean }>({ type: 'fixture.capture.stop' })
          .then((reply) => { resetCapture(); respond(reply); }, (error: unknown) => respond({ error: error instanceof Error ? error.message : 'Fixture capture stop failed' }));
        return true;
      }
      try {
        if (msg?.type === 'capture.begin' && typeof msg.projectId === 'string' && typeof msg.origin === 'string'
          && msg.session && typeof msg.session === 'object') {
          beginCapture(msg.projectId, msg.session as CaptureSession, msg.origin);
          respond({ ok: true });
        } else if (msg?.type === 'capture.end' && typeof msg.traceId === 'string') {
          if (!session || session.id === msg.traceId) resetCapture();
          respond({ ok: true });
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
        void ask({ type: 'capture.event', sessionId: session.id, kind: 'extension.diagnostic', metadata: { code: detail.code } }).catch(() => {});
        console.warn('LegacyLens: page does not expose the supported PrimeFaces Ajax API.');
        return;
      }
      if (typeof detail.source !== 'string' || detail.source.length > 256) return;
      if (detail.source !== selectedSource || !clickEventId) return;
      if (detail.propagation === 'attempted' && !detail.spanId && !detail.traceparent) {
        void ask({ type: 'capture.event', sessionId: session.id, kind: 'primefaces.propagation_attempt', parentEventId: clickEventId, metadata: { source: detail.source } }).catch((error) => console.warn('LegacyLens capture transport interrupted', error));
        return;
      }
      if (detail.propagation !== 'propagated' || !detail.traceparent || !detail.spanId) return;
      if (detail.traceparent !== `00-${session.id}-${detail.spanId}-01` || !/^[a-f0-9]{16}$/i.test(detail.spanId)) return;
      void ask({ type: 'capture.event', sessionId: session.id, kind: 'primefaces.ajax', parentEventId: clickEventId, metadata: { source: detail.source, spanId: detail.spanId } }).catch((error) => console.warn('LegacyLens capture transport interrupted', error));
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
      void ask({ type: 'capture.event', sessionId: session.id, kind: propagated ? 'browser.network' : 'browser.propagation_attempt',
        parentEventId: clickEventId, metadata }).catch((error) => console.warn('LegacyLens capture transport interrupted', error));
    });
  },
});
