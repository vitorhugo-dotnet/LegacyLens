export type CaptureStage =
  | 'content.selection'
  | 'content.send'
  | 'background.receive'
  | 'background.sender-validation'
  | 'background.session-validation'
  | 'background.record'
  | 'host.ingest'
  | 'investigation.query';

export type CaptureStageOutcome = 'started' | 'accepted' | 'rejected' | 'timeout';

export interface CaptureStageRecord {
  stage: CaptureStage;
  outcome: CaptureStageOutcome;
  traceId: string;
  tabId: number;
  eventId?: string;
  durationMs?: number;
  code?: string;
}

export interface CaptureDiagnosticsManifest { host_permissions?: string[] }
export type CaptureDiagnosticsSink = (record: CaptureStageRecord) => void;

const stages = new Set<CaptureStage>([
  'content.selection', 'content.send', 'background.receive', 'background.sender-validation',
  'background.session-validation', 'background.record', 'host.ingest', 'investigation.query',
]);
const outcomes = new Set<CaptureStageOutcome>(['started', 'accepted', 'rejected', 'timeout']);
const safeCodes = new Set([
  'NO_ACK', 'INVALID_ACK', 'BACKGROUND_REJECTED', 'SEND_FAILED', 'TIMEOUT',
  'SENDER_TAB', 'SENDER_URL', 'ORIGIN_MISMATCH', 'EVENT_INVALID', 'SESSION_UNAVAILABLE', 'SESSION_MISMATCH',
  'METADATA_INVALID', 'EVENT_ID', 'SPAN_ID', 'NETWORK_INVALID', 'RECORD_FAILED', 'INTERNAL',
  'DISCONNECTED', 'POST_FAILED', 'HOST_REJECTED', 'NATIVE_ERROR',
]);

export function recordCaptureStage(
  record: CaptureStageRecord,
  manifest: CaptureDiagnosticsManifest,
  sink: CaptureDiagnosticsSink = (safeRecord) => console.info('LegacyLens capture stage', JSON.stringify(safeRecord)),
): void {
  if (!manifest.host_permissions?.includes('http://127.0.0.1/*')) return;
  if (!stages.has(record.stage) || !outcomes.has(record.outcome)) return;
  if (!/^[a-f0-9]{32}$/i.test(record.traceId) || !Number.isSafeInteger(record.tabId) || record.tabId < 0) return;
  if (record.eventId !== undefined && !/^[a-f0-9]{16}$/i.test(record.eventId)) return;
  if (record.durationMs !== undefined && (!Number.isFinite(record.durationMs) || record.durationMs < 0)) return;
  if (record.code !== undefined && !safeCodes.has(record.code)) return;

  sink({
    stage: record.stage,
    outcome: record.outcome,
    traceId: record.traceId,
    tabId: record.tabId,
    ...(record.eventId ? { eventId: record.eventId } : {}),
    ...(record.durationMs !== undefined ? { durationMs: Math.round(record.durationMs) } : {}),
    ...(record.code ? { code: record.code } : {}),
  });
}
