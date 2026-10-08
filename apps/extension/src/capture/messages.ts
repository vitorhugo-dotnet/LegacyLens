import type { CaptureStageRecord } from './diagnostics.ts';

export interface CaptureEventRequest {
  type: 'capture.event';
  sessionId: string;
  kind: string;
  eventId?: string;
  parentEventId?: string;
  metadata: Record<string, string>;
}

export interface CaptureEventAcknowledgement { accepted: true; gap: boolean }
export type CaptureEventReply = CaptureEventAcknowledgement | { error: string; code?: string };
export type CaptureMessageErrorCode = 'NO_ACK' | 'INVALID_ACK' | 'BACKGROUND_REJECTED' | 'SEND_FAILED' | 'TIMEOUT';
export type CaptureMessageDiagnostics = (record: CaptureStageRecord) => void;
export type CaptureMessageSender = (message: CaptureEventRequest) => Promise<unknown>;

export class CaptureMessageError extends Error {
  constructor(readonly code: CaptureMessageErrorCode) {
    super(code);
    this.name = 'CaptureMessageError';
  }
}

export const CAPTURE_EVENT_ACK_TIMEOUT_MS = 35_000;

export async function requestCaptureEvent(
  sendMessage: CaptureMessageSender,
  message: CaptureEventRequest,
  context: { traceId: string; tabId: number },
  diagnostics: CaptureMessageDiagnostics,
): Promise<CaptureEventAcknowledgement> {
  const startedAt = Date.now();
  const correlation = { ...context, ...(message.eventId ? { eventId: message.eventId } : {}) };
  diagnostics({ stage: 'content.send', outcome: 'started', ...correlation });

  let timeout: ReturnType<typeof setTimeout> | undefined;
  try {
    const reply = await Promise.race([
      Promise.resolve().then(() => sendMessage(message)),
      new Promise<never>((_resolve, reject) => {
        timeout = setTimeout(() => reject(new CaptureMessageError('TIMEOUT')), CAPTURE_EVENT_ACK_TIMEOUT_MS);
      }),
    ]);

    if (reply === undefined || reply === null) throw new CaptureMessageError('NO_ACK');
    if (typeof reply !== 'object' || reply === null || (!('accepted' in reply) && !('error' in reply))) {
      throw new CaptureMessageError('INVALID_ACK');
    }
    const acknowledgement = reply as Record<string, unknown>;
    if ('error' in acknowledgement) throw new CaptureMessageError('BACKGROUND_REJECTED');
    if (acknowledgement.accepted !== true || typeof acknowledgement.gap !== 'boolean') throw new CaptureMessageError('INVALID_ACK');

    diagnostics({ stage: 'content.send', outcome: 'accepted', ...correlation, durationMs: Date.now() - startedAt });
    return { accepted: true, gap: acknowledgement.gap };
  } catch (error) {
    const failure = error instanceof CaptureMessageError ? error : new CaptureMessageError('SEND_FAILED');
    diagnostics({ stage: 'content.send', outcome: failure.code === 'TIMEOUT' ? 'timeout' : 'rejected',
      ...correlation, durationMs: Date.now() - startedAt, code: failure.code });
    throw failure;
  } finally {
    if (timeout !== undefined) clearTimeout(timeout);
  }
}
