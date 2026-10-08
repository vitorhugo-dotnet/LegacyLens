import { describe, expect, it, vi } from 'vitest';
import { handleCapturePort } from './background-port.ts';
import { CAPTURE_PORT_NAME, CAPTURE_PORT_VERSION } from './port-protocol.ts';
import { CaptureRecordError } from './session.ts';

function setup({ tabId = 9, url = 'https://app.example/page', active = true } = {}) {
  const messages: unknown[] = [];
  let activeState = active;
  let incoming: ((message: unknown) => void) | undefined;
  let disconnected: (() => void) | undefined;
  const port = {
    name: CAPTURE_PORT_NAME,
    sender: { url, tab: tabId === undefined ? undefined : { id: tabId, url } },
    postMessage: vi.fn((message: unknown) => messages.push(message)),
    disconnect: vi.fn(),
    onMessage: { addListener: (fn: (message: unknown) => void) => { incoming = fn; }, removeListener: vi.fn() },
    onDisconnect: { addListener: (fn: () => void) => { disconnected = fn; }, removeListener: vi.fn() },
  };
  const record = vi.fn().mockResolvedValue({ gap: false, duplicate: false });
  const controller = { restore: vi.fn(), get: vi.fn(() => activeState ? {
    request: { origin: 'https://app.example' }, session: { id: 'a'.repeat(32), expiresAt: new Date(Date.now() + 60_000).toISOString() },
  } : undefined), record };
  const cleanup = handleCapturePort(port as never, controller as never);
  const send = async (message: unknown) => { incoming?.(message); await new Promise((resolve) => setTimeout(resolve, 0)); };
  return { port, messages, record, controller, send, disconnect: () => disconnected?.(), cleanup, setActive(value: boolean) { activeState = value; } };
}
const traceId = 'a'.repeat(32), eventId = 'b'.repeat(16);
const hello = { type: 'hello', version: CAPTURE_PORT_VERSION, traceId };
const event = { type: 'event', version: CAPTURE_PORT_VERSION, traceId, eventId, kind: 'jsf.click', metadata: { source: 'button' } };

describe('background capture Port', () => {
  it('acknowledges only after the host record resolves and preserves duplicate status', async () => {
    const deps = setup();
    await deps.send(hello);
    expect(deps.messages).toEqual([{ type: 'ready', version: 1, traceId }]);
    deps.record.mockResolvedValueOnce({ gap: false, duplicate: true });
    await deps.send(event);
    expect(deps.messages.at(-1)).toEqual({ type: 'ack', version: 1, traceId, eventId, gap: false, duplicate: true });
  });
  it('rejects invalid senders and sessions before record', async () => {
    const noTab = setup({ tabId: -1 });
    await noTab.send(hello);
    expect(noTab.messages.at(-1)).toMatchObject({ type: 'nack', code: 'SENDER_TAB', retryable: false });
    expect(noTab.record).not.toHaveBeenCalled();
    const wrongOrigin = setup({ url: 'https://other.example/page' });
    await wrongOrigin.send(hello);
    expect(wrongOrigin.messages.at(-1)).toMatchObject({ type: 'nack', code: 'ORIGIN_MISMATCH' });
    expect(wrongOrigin.record).not.toHaveBeenCalled();
    const expired = setup({ active: false });
    await expired.send(hello);
    expect(expired.messages.at(-1)).toMatchObject({ type: 'nack', code: 'SESSION_MISMATCH' });
    expect(expired.record).not.toHaveBeenCalled();
  });
  it('rejects malformed event messages and closes listener on disconnect', async () => {
    const deps = setup();
    await deps.send(hello);
    await deps.send({ ...event, metadata: { secret: 'x' } });
    expect(deps.messages.at(-1)).toMatchObject({ type: 'nack', code: 'INVALID_MESSAGE' });
    expect(deps.record).not.toHaveBeenCalled();
    deps.disconnect();
    expect(deps.port.onMessage.removeListener).toHaveBeenCalled();
    expect(deps.port.disconnect).not.toHaveBeenCalled();
    deps.cleanup();
    expect(deps.port.disconnect).toHaveBeenCalled();
  });
  it('records overflow as a gap event and acknowledges it', async () => {
    const deps = setup();
    await deps.send(hello);
    await deps.send({ type: 'overflow', version: 1, traceId, eventId });
    expect(deps.record).toHaveBeenCalledWith(9, 'extension.gap', { code: 'QUEUE_OVERFLOW' }, { eventId });
    expect(deps.messages.at(-1)).toMatchObject({ type: 'ack', eventId });
  });
  it('marks a native timeout retryable so the client can replay the same identity', async () => {
    const deps = setup();
    await deps.send(hello);
    deps.record.mockRejectedValueOnce(new CaptureRecordError('HOST_TIMEOUT', true));
    await deps.send(event);
    expect(deps.messages.at(-1)).toMatchObject({ type: 'nack', traceId, eventId, code: 'HOST_TIMEOUT', retryable: true });
  });
  it('rejects a stale port after its active tab session is stopped', async () => {
    const deps = setup();
    await deps.send(hello);
    deps.setActive(false);
    await deps.send(event);
    expect(deps.messages.at(-1)).toMatchObject({ type: 'nack', code: 'SESSION_MISMATCH', retryable: false });
    expect(deps.record).not.toHaveBeenCalled();
  });
});
