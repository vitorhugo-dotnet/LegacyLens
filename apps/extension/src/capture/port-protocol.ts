export const CAPTURE_PORT_NAME = 'legacylens.capture.v1';
export const CAPTURE_PORT_VERSION = 1 as const;
export const MAX_PENDING_CAPTURE_EVENTS = 100;
export const CAPTURE_PORT_RECONNECT_DELAYS_MS = [250, 1_000, 2_000] as const;

export const CAPTURE_PORT_NACK_CODES = [
  'INVALID_MESSAGE', 'SENDER_TAB', 'SENDER_URL', 'SESSION_UNAVAILABLE', 'SESSION_MISMATCH',
  'ORIGIN_MISMATCH', 'EVENT_INVALID', 'METADATA_INVALID', 'EVENT_ID', 'SPAN_ID', 'NETWORK_INVALID',
  'RECORD_FAILED', 'HOST_TIMEOUT', 'CAPTURE_STOPPING', 'PORT_CLOSED', 'RECONNECT_EXHAUSTED', 'QUEUE_FULL', 'INTERNAL',
] as const;
export type CapturePortNackCode = typeof CAPTURE_PORT_NACK_CODES[number];

export type CapturePortClientMessage =
  | { type: 'hello'; version: 1; traceId: string }
  | { type: 'event'; version: 1; traceId: string; eventId: string; kind: string; parentEventId?: string; metadata: Record<string, string> }
  | { type: 'overflow'; version: 1; traceId: string; eventId: string };
export type CapturePortServerMessage =
  | { type: 'ready'; version: 1; traceId: string }
  | { type: 'ack'; version: 1; traceId: string; eventId: string; gap: boolean; duplicate: boolean }
  | { type: 'nack'; version: 1; traceId: string; eventId?: string; code: CapturePortNackCode; retryable: boolean };

const tracePattern = /^(?!0{32})[a-f0-9]{32}$/i;
const eventPattern = /^(?!0{16})[a-f0-9]{16}$/i;
const metadataKeys = new Set(['source', 'code', 'spanId', 'transport', 'frameChain', 'stackGap']);
const eventKinds = new Set(['jsf.click', 'primefaces.ajax', 'primefaces.propagation_attempt', 'browser.network', 'browser.propagation_attempt', 'extension.diagnostic']);
const isRecord = (v: unknown): v is Record<string, unknown> => Boolean(v && typeof v === 'object' && !Array.isArray(v));
const exact = (value: Record<string, unknown>, allowed: string[]) => Object.keys(value).every((key) => allowed.includes(key));
const validTrace = (value: unknown): value is string => typeof value === 'string' && tracePattern.test(value);
const validEvent = (value: unknown): value is string => typeof value === 'string' && eventPattern.test(value);

function validMetadata(value: unknown): value is Record<string, string> {
  if (!isRecord(value) || Object.keys(value).length > metadataKeys.size) return false;
  return Object.entries(value).every(([key, item]) => metadataKeys.has(key) && typeof item === 'string' && item.length <= 256);
}

export function parseCapturePortClientMessage(value: unknown): CapturePortClientMessage | undefined {
  if (!isRecord(value) || value.version !== CAPTURE_PORT_VERSION || !validTrace(value.traceId) || typeof value.type !== 'string') return;
  if (value.type === 'hello' && exact(value, ['type', 'version', 'traceId'])) return value as CapturePortClientMessage;
  if (value.type === 'overflow' && exact(value, ['type', 'version', 'traceId', 'eventId']) && validEvent(value.eventId)) return value as CapturePortClientMessage;
  if (value.type !== 'event' || !exact(value, ['type', 'version', 'traceId', 'eventId', 'kind', 'parentEventId', 'metadata'])
    || !validEvent(value.eventId) || (value.parentEventId !== undefined && !validEvent(value.parentEventId))
    || typeof value.kind !== 'string' || !eventKinds.has(value.kind) || !validMetadata(value.metadata)) return;
  return value as CapturePortClientMessage;
}

export function parseCapturePortServerMessage(value: unknown): CapturePortServerMessage | undefined {
  if (!isRecord(value) || value.version !== CAPTURE_PORT_VERSION || !validTrace(value.traceId) || typeof value.type !== 'string') return;
  if (value.type === 'ready' && exact(value, ['type', 'version', 'traceId'])) return value as CapturePortServerMessage;
  if (value.type === 'ack' && exact(value, ['type', 'version', 'traceId', 'eventId', 'gap', 'duplicate'])
    && validEvent(value.eventId) && typeof value.gap === 'boolean' && typeof value.duplicate === 'boolean') return value as CapturePortServerMessage;
  if (value.type === 'nack' && exact(value, ['type', 'version', 'traceId', 'eventId', 'code', 'retryable'])
    && (value.eventId === undefined || validEvent(value.eventId))
    && typeof value.code === 'string' && (CAPTURE_PORT_NACK_CODES as readonly string[]).includes(value.code)
    && typeof value.retryable === 'boolean') return value as CapturePortServerMessage;
}
