import type { ActiveCapture } from './session.ts';
import type { CaptureStageRecord } from './diagnostics.ts';
import type { CaptureEventReply } from './messages.ts';

export interface CaptureMessageSender { url?: string | undefined; tab?: { id?: number | undefined; url?: string | undefined } | undefined }
export interface CaptureEventController {
  restore(): Promise<void>;
  get(tabId: number): ActiveCapture | undefined;
  record(tabId: number, kind: string, metadata: Record<string, string>, identity?: { eventId?: string; parentEventId?: string }): Promise<void>;
}
export type CaptureBoundaryDiagnostics = (record: CaptureStageRecord) => void;

const eventKinds = new Set(['jsf.click', 'primefaces.ajax', 'primefaces.propagation_attempt', 'browser.network', 'browser.propagation_attempt', 'extension.diagnostic']);
const metadataKeys = ['source', 'code', 'spanId', 'transport', 'frameChain', 'stackGap'];
const eventIdPattern = /^[a-f0-9]{16}$/i;

function originOf(url?: string): string | undefined {
  if (!url) return undefined;
  try { const parsed = new URL(url); return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.origin : undefined; }
  catch { return undefined; }
}

function error(message: string, code: string): CaptureEventReply { return { error: message, code }; }

export async function handleCaptureEvent(
  message: unknown,
  sender: CaptureMessageSender,
  controller: CaptureEventController,
  diagnostics: CaptureBoundaryDiagnostics = () => {},
): Promise<CaptureEventReply | undefined> {
  if (!message || typeof message !== 'object' || (message as Record<string, unknown>).type !== 'capture.event') return undefined;
  const msg = message as Record<string, unknown>;
  const traceId = typeof msg.sessionId === 'string' && /^[a-f0-9]{32}$/i.test(msg.sessionId) ? msg.sessionId : '0'.repeat(32);
  const tabId = Number.isSafeInteger(sender.tab?.id) && (sender.tab?.id as number) >= 0 ? sender.tab!.id! : -1;
  const eventId = typeof msg.eventId === 'string' && eventIdPattern.test(msg.eventId) && !/^0+$/.test(msg.eventId) ? msg.eventId : undefined;
  const correlation = { traceId, tabId, ...(eventId ? { eventId } : {}) };
  diagnostics({ stage: 'background.receive', outcome: 'started', ...correlation });
  if (tabId < 0) {
    diagnostics({ stage: 'background.sender-validation', outcome: 'rejected', ...correlation, code: 'SENDER_TAB' });
    return error('Capture event sender has no tab', 'SENDER_TAB');
  }
  const senderOrigin = sender.url !== undefined ? originOf(sender.url) : originOf(sender.tab?.url);
  if (!senderOrigin) {
    diagnostics({ stage: 'background.sender-validation', outcome: 'rejected', ...correlation, code: 'SENDER_URL' });
    return error('Capture event sender URL is not HTTP(S)', 'SENDER_URL');
  }
  diagnostics({ stage: 'background.sender-validation', outcome: 'accepted', ...correlation });
  if (traceId === '0'.repeat(32) || typeof msg.kind !== 'string' || !eventKinds.has(msg.kind)
    || typeof msg.sessionId !== 'string') {
    diagnostics({ stage: 'background.session-validation', outcome: 'rejected', ...correlation, code: 'EVENT_INVALID' });
    return error('Capture event is malformed', 'EVENT_INVALID');
  }
  try { await controller.restore(); }
  catch {
    diagnostics({ stage: 'background.session-validation', outcome: 'rejected', ...correlation, code: 'SESSION_UNAVAILABLE' });
    return error('Capture session unavailable', 'SESSION_UNAVAILABLE');
  }
  const active = controller.get(tabId);
  if (!active || active.session.id !== traceId || Date.parse(active.session.expiresAt) <= Date.now()) {
    diagnostics({ stage: 'background.session-validation', outcome: 'rejected', ...correlation, code: 'SESSION_MISMATCH' });
    return error('Capture session is missing, expired, or mismatched', 'SESSION_MISMATCH');
  }
  if (active.request.origin !== senderOrigin) {
    diagnostics({ stage: 'background.session-validation', outcome: 'rejected', ...correlation, code: 'ORIGIN_MISMATCH' });
    return error('Capture session origin does not match sender', 'ORIGIN_MISMATCH');
  }
  diagnostics({ stage: 'background.session-validation', outcome: 'accepted', ...correlation });
  const rawMetadata = msg.metadata;
  if (!rawMetadata || typeof rawMetadata !== 'object' || Array.isArray(rawMetadata)) {
    return error('Capture event metadata is invalid', 'METADATA_INVALID');
  }
  const safe: Record<string, string> = {};
  for (const key of metadataKeys) {
    const value = (rawMetadata as Record<string, unknown>)[key];
    if (typeof value === 'string' && value.length <= 256) safe[key] = value;
  }
  const parentEventId = typeof msg.parentEventId === 'string' && eventIdPattern.test(msg.parentEventId) && !/^0+$/.test(msg.parentEventId) ? msg.parentEventId : undefined;
  if ((msg.eventId !== undefined && !eventId) || (msg.parentEventId !== undefined && !parentEventId)
    || (msg.kind === 'jsf.click' && !eventId)
    || (msg.kind !== 'jsf.click' && msg.kind !== 'extension.diagnostic' && !parentEventId)) {
    return error('Capture event identity is invalid', 'EVENT_ID');
  }
  if (safe.spanId && (!eventIdPattern.test(safe.spanId) || /^0+$/.test(safe.spanId))) return error('Capture span identity is invalid', 'SPAN_ID');
  if (msg.kind === 'browser.network' && (!safe.spanId || !['fetch', 'xhr'].includes(safe.transport ?? ''))) return error('Network effect is invalid', 'NETWORK_INVALID');
  if (msg.kind === 'browser.propagation_attempt' && safe.spanId) return error('Propagation attempt has an unverified span', 'SPAN_ID');
  diagnostics({ stage: 'background.record', outcome: 'started', ...correlation });
  try {
    await controller.record(tabId, msg.kind, safe, { ...(eventId ? { eventId } : {}), ...(parentEventId ? { parentEventId } : {}) });
  } catch {
    diagnostics({ stage: 'background.record', outcome: 'rejected', ...correlation, code: 'RECORD_FAILED' });
    diagnostics({ stage: 'background.receive', outcome: 'rejected', ...correlation, code: 'RECORD_FAILED' });
    return error('Capture event could not be recorded', 'RECORD_FAILED');
  }
  diagnostics({ stage: 'background.record', outcome: 'accepted', ...correlation });
  diagnostics({ stage: 'background.receive', outcome: 'accepted', ...correlation });
  return { accepted: true, gap: active.gap };
}
