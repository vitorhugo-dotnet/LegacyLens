import { describe, expect, it } from 'vitest';
import { parseCapturePortClientMessage, parseCapturePortServerMessage } from './port-protocol.ts';
const traceId = 'a'.repeat(32), eventId = 'b'.repeat(16);

describe('capture Port protocol', () => {
  it('parses the supported messages in both directions', () => {
    expect(parseCapturePortClientMessage({ type: 'hello', version: 1, traceId })).toMatchObject({ type: 'hello' });
    expect(parseCapturePortClientMessage({ type: 'event', version: 1, traceId, eventId, kind: 'jsf.click', metadata: { source: 'button' } })).toMatchObject({ eventId });
    expect(parseCapturePortClientMessage({ type: 'overflow', version: 1, traceId, eventId })).toMatchObject({ type: 'overflow' });
    expect(parseCapturePortServerMessage({ type: 'ready', version: 1, traceId })).toMatchObject({ type: 'ready' });
    expect(parseCapturePortServerMessage({ type: 'ack', version: 1, traceId, eventId, gap: false, duplicate: true })).toMatchObject({ duplicate: true });
    expect(parseCapturePortServerMessage({ type: 'nack', version: 1, traceId, eventId, code: 'RECORD_FAILED', retryable: false })).toMatchObject({ retryable: false });
  });
  it.each([
    { type: 'hello', version: 2, traceId }, { type: 'wat', version: 1, traceId },
    { type: 'hello', version: 1, traceId: '0'.repeat(32) }, { type: 'event', version: 1, traceId, eventId: 'bad', kind: 'jsf.click', metadata: {} },
    { type: 'event', version: 1, traceId, eventId, kind: 'jsf.click', metadata: { secret: 'x' } },
    { type: 'event', version: 1, traceId, eventId, kind: 'jsf.click', metadata: { source: 'x'.repeat(257) } },
    { type: 'hello', version: 1, traceId, extra: true },
  ])('rejects invalid client envelope %#', (message) => expect(parseCapturePortClientMessage(message)).toBeUndefined());
  it.each([
    { type: 'ack', version: 1, traceId, eventId, gap: false },
    { type: 'nack', version: 1, traceId, code: 'private-error', retryable: false },
    { type: 'ready', version: 1, traceId, extra: true },
  ])('rejects invalid server envelope %#', (message) => expect(parseCapturePortServerMessage(message)).toBeUndefined());
});
