import { afterEach, describe, expect, it, vi } from 'vitest';

const messagesModule = await import('./messages.ts').catch(() => ({} as typeof import('./messages.ts')));

const request = {
  type: 'capture.event' as const,
  sessionId: 'a'.repeat(32),
  kind: 'jsf.click',
  eventId: 'b'.repeat(16),
  metadata: { source: 'form:save' },
};
const context = { traceId: request.sessionId, tabId: 12 };

afterEach(() => vi.useRealTimers());

describe('capture event messaging', () => {
  it('accepts an explicit background acknowledgement', async () => {
    expect(typeof messagesModule.requestCaptureEvent).toBe('function');
    const diagnostics = vi.fn();
    const sendMessage = vi.fn().mockResolvedValue({ accepted: true, gap: false });

    await expect(messagesModule.requestCaptureEvent(sendMessage, request, context, diagnostics))
      .resolves.toEqual({ accepted: true, gap: false });
    expect(diagnostics).toHaveBeenCalledWith(expect.objectContaining({ stage: 'content.send', outcome: 'started', ...context, eventId: request.eventId }));
    expect(diagnostics).toHaveBeenCalledWith(expect.objectContaining({ stage: 'content.send', outcome: 'accepted', ...context, eventId: request.eventId }));
  });

  it('treats a missing response as an unacknowledged event', async () => {
    const sendMessage = vi.fn().mockResolvedValue(undefined);

    await expect(messagesModule.requestCaptureEvent(sendMessage, request, context, vi.fn()))
      .rejects.toMatchObject({ code: 'NO_ACK' });
  });

  it('rejects a malformed acknowledgement', async () => {
    const sendMessage = vi.fn().mockResolvedValue({ accepted: true, gap: 'no' });

    await expect(messagesModule.requestCaptureEvent(sendMessage, request, context, vi.fn()))
      .rejects.toMatchObject({ code: 'INVALID_ACK' });
  });

  it('reports an explicit background rejection', async () => {
    const sendMessage = vi.fn().mockResolvedValue({ error: 'Rejected', code: 'SESSION_MISMATCH' });

    await expect(messagesModule.requestCaptureEvent(sendMessage, request, context, vi.fn()))
      .rejects.toMatchObject({ code: 'BACKGROUND_REJECTED' });
  });

  it('reports a rejected send promise', async () => {
    const sendMessage = vi.fn().mockRejectedValue(new Error('port closed'));

    await expect(messagesModule.requestCaptureEvent(sendMessage, request, context, vi.fn()))
      .rejects.toMatchObject({ code: 'SEND_FAILED' });
  });

  it('bounds the wait for a background acknowledgement', async () => {
    vi.useFakeTimers();
    const sendMessage = vi.fn().mockReturnValue(new Promise(() => {}));
    const pending = messagesModule.requestCaptureEvent(sendMessage, request, context, vi.fn());
    const assertion = expect(pending).rejects.toMatchObject({ code: 'TIMEOUT' });

    await vi.advanceTimersByTimeAsync(35_000);
    await assertion;
  });
});
