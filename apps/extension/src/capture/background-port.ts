import type { CaptureStageRecord } from './diagnostics.ts';
import { handleCaptureEvent, type CaptureEventController, type CaptureMessageSender } from './background-messages.ts';
import { CAPTURE_PORT_NAME, CAPTURE_PORT_VERSION, parseCapturePortClientMessage, type CapturePortNackCode, type CapturePortServerMessage } from './port-protocol.ts';

export type CapturePortDiagnostics = (record: CaptureStageRecord) => void;
const originOf = (url?: string): string | undefined => {
  if (!url) return;
  try { const parsed = new URL(url); return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.origin : undefined; }
  catch { return; }
};

export function handleCapturePort(port: chrome.runtime.Port, controller: CaptureEventController, diagnostics: CapturePortDiagnostics = () => {}, onClosed: () => void = () => {}): () => void {
  const sender = port.sender as CaptureMessageSender | undefined;
  const tabId = sender?.tab?.id;
  const traceFallback = '0'.repeat(32);
  let traceId = traceFallback;
  let ready = false;
  let closed = false;
  let queue = Promise.resolve();
  const correlation = (eventId?: string) => ({ traceId, tabId: Number.isInteger(tabId) ? tabId! : -1, ...(eventId ? { eventId } : {}) });
  const post = (message: CapturePortServerMessage) => { if (!closed) { try { port.postMessage(message); } catch { dispose(); } } };
  const nack = (code: CapturePortNackCode, eventId?: string, retryable = false) => post({ type: 'nack', version: CAPTURE_PORT_VERSION, traceId, ...(eventId ? { eventId } : {}), code, retryable });
  const close = () => { if (closed) return; closed = true; port.onMessage.removeListener(onMessage); port.onDisconnect.removeListener(onDisconnect); onClosed(); };
  const dispose = () => { close(); try { port.disconnect(); } catch { /* already disconnected */ } };
  const onDisconnect = () => { close(); };
  const onMessage = (value: unknown) => {
    queue = queue.then(async () => {
      const message = parseCapturePortClientMessage(value);
      if (!message) {
        if (ready) nack('INVALID_MESSAGE');
        else dispose();
        return;
      }
      if (message.type === 'hello') {
        traceId = message.traceId;
        diagnostics({ stage: 'background.port', outcome: 'started', ...correlation() });
        const validTab = Number.isSafeInteger(tabId) && (tabId as number) >= 0;
        if (ready || !validTab || !originOf(sender?.url) && !originOf(sender?.tab?.url)) { nack(!validTab ? 'SENDER_TAB' : 'SENDER_URL'); return; }
        try { await controller.restore(); } catch { nack('SESSION_UNAVAILABLE'); return; }
        const active = controller.get(tabId as number);
        const senderOrigin = originOf(sender?.url) ?? originOf(sender?.tab?.url);
        if (!active || active.session.id !== message.traceId || Date.parse(active.session.expiresAt) <= Date.now()) { nack('SESSION_MISMATCH'); return; }
        if (!senderOrigin || active.request.origin !== senderOrigin) { nack('ORIGIN_MISMATCH'); return; }
        ready = true;
        diagnostics({ stage: 'background.port', outcome: 'accepted', ...correlation() });
        post({ type: 'ready', version: CAPTURE_PORT_VERSION, traceId });
        return;
      }
      const eventId = message.eventId;
      if (!ready || message.traceId !== traceId || tabId === undefined) { nack('SESSION_MISMATCH', eventId); return; }
      const active = controller.get(tabId);
      if (!active || active.session.id !== traceId || Date.parse(active.session.expiresAt) <= Date.now()) { nack('SESSION_MISMATCH', eventId); return; }
      const senderOrigin = originOf(sender?.url) ?? originOf(sender?.tab?.url);
      if (!senderOrigin || senderOrigin !== active.request.origin) { nack('ORIGIN_MISMATCH', eventId); return; }
      if (message.type === 'overflow') {
        try {
          const result = await controller.record(tabId, 'extension.gap', { code: 'QUEUE_OVERFLOW' }, { eventId });
          post({ type: 'ack', version: CAPTURE_PORT_VERSION, traceId, eventId, gap: result.gap, duplicate: result.duplicate });
        } catch { nack('RECORD_FAILED', eventId); }
        return;
      }
      const result = await handleCaptureEvent({ type: 'capture.event', sessionId: traceId, eventId,
        kind: message.kind, ...(message.parentEventId ? { parentEventId: message.parentEventId } : {}), metadata: message.metadata },
      sender as CaptureMessageSender, controller, diagnostics);
      if (result && 'accepted' in result) post({ type: 'ack', version: CAPTURE_PORT_VERSION, traceId, eventId, gap: result.gap, duplicate: result.duplicate === true });
      else if (result && 'error' in result) nack((result.code as CapturePortNackCode) ?? 'RECORD_FAILED', eventId, result.retryable === true);
      else nack('INTERNAL', eventId);
    }).catch(() => { nack('INTERNAL'); });
  };
  if (port.name !== CAPTURE_PORT_NAME) { nack('INVALID_MESSAGE'); dispose(); }
  else { port.onMessage.addListener(onMessage); port.onDisconnect.addListener(onDisconnect); }
  return dispose;
}
