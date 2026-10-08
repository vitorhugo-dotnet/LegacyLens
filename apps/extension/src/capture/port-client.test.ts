import { describe, expect, it, vi } from 'vitest';
import { CapturePortClient, type CapturePortLike } from './port-client.ts';

function fakePort() {
  let onMessage: ((message: unknown) => void) | undefined;
  let onDisconnect: (() => void) | undefined;
  const sent: unknown[] = [];
  const port: CapturePortLike = {
    postMessage(message) { sent.push(message); },
    disconnect() {},
    onMessage: { addListener(listener) { onMessage = listener; }, removeListener() {} },
    onDisconnect: { addListener(listener) { onDisconnect = listener; }, removeListener() {} },
  };
  return { port, sent, emit(message: unknown) { onMessage?.(message); }, drop() { onDisconnect?.(); } };
}
const traceId = 'a'.repeat(32);
const ready = { type: 'ready', version: 1, traceId };
const ack = (eventId: string) => ({ type: 'ack', version: 1, traceId, eventId, gap: false, duplicate: false });
const event = (eventId: string) => ({ eventId, kind: 'jsf.click', metadata: { source: 'button' } });

describe('CapturePortClient', () => {
  it('waits for ready, sends one event at a time, and resolves matching ACK', async () => {
    const conn = fakePort();
    const client = new CapturePortClient(() => conn.port);
    const started = client.start(traceId);
    conn.emit(ready);
    await started;
    const first = client.send(event('1'.repeat(16)));
    const second = client.send(event('2'.repeat(16)));
    expect(conn.sent.filter((message) => (message as { type: string }).type === 'event')).toHaveLength(1);
    conn.emit(ack('1'.repeat(16)));
    await expect(first).resolves.toMatchObject({ accepted: true, duplicate: false });
    expect(conn.sent.filter((message) => (message as { type: string }).type === 'event')).toHaveLength(2);
    conn.emit(ack('2'.repeat(16)));
    await expect(second).resolves.toMatchObject({ accepted: true });
    await client.stop();
  });

  it('reconnects after disconnect and replays the same event identity', async () => {
    vi.useFakeTimers();
    const ports = [fakePort(), fakePort()];
    let index = 0;
    const client = new CapturePortClient(() => ports[index++]!.port);
    const started = client.start(traceId);
    ports[0]!.emit(ready);
    await started;
    const pending = client.send(event('3'.repeat(16)));
    ports[0]!.drop();
    await vi.advanceTimersByTimeAsync(250);
    ports[1]!.emit(ready);
    await Promise.resolve();
    expect(ports[1]!.sent.at(-1)).toMatchObject({ type: 'event', eventId: '3'.repeat(16) });
    ports[1]!.emit(ack('3'.repeat(16)));
    await expect(pending).resolves.toMatchObject({ accepted: true });
    await client.stop();
    vi.useRealTimers();
  });

  it('rejects permanent NACK and stops accepting new sends', async () => {
    const conn = fakePort();
    const client = new CapturePortClient(() => conn.port);
    const started = client.start(traceId);
    conn.emit(ready);
    await started;
    const pending = client.send(event('4'.repeat(16)));
    conn.emit({ type: 'nack', version: 1, traceId, eventId: '4'.repeat(16), code: 'EVENT_INVALID', retryable: false });
    await expect(pending).rejects.toThrow('EVENT_INVALID');
    await client.stop();
    await expect(client.send(event('5'.repeat(16)))).rejects.toThrow();
  });

  it('drains an acknowledged event before closing on stop', async () => {
    const conn = fakePort();
    const client = new CapturePortClient(() => conn.port);
    const started = client.start(traceId);
    conn.emit(ready);
    await started;
    const pending = client.send(event('6'.repeat(16)));
    const stopping = client.stop();
    conn.emit(ack('6'.repeat(16)));
    await expect(pending).resolves.toMatchObject({ accepted: true });
    await expect(stopping).resolves.toBe(false);
  });

  it('uses the three bounded reconnect delays and rejects after exhaustion', async () => {
    vi.useFakeTimers();
    const ports = [fakePort(), fakePort(), fakePort(), fakePort()];
    let index = 0;
    const client = new CapturePortClient(() => ports[index++]!.port);
    const started = client.start(traceId);
    ports[0]!.emit(ready);
    await started;
    const pending = client.send(event('7'.repeat(16)));
    ports[0]!.drop();
    await vi.advanceTimersByTimeAsync(249);
    expect(index).toBe(1);
    await vi.advanceTimersByTimeAsync(1);
    ports[1]!.emit(ready);
    await Promise.resolve();
    ports[1]!.drop();
    await vi.advanceTimersByTimeAsync(999);
    expect(index).toBe(2);
    await vi.advanceTimersByTimeAsync(1);
    ports[2]!.emit(ready);
    await Promise.resolve();
    ports[2]!.drop();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(index).toBe(4);
    ports[3]!.emit(ready);
    await Promise.resolve();
    ports[3]!.drop();
    await expect(pending).rejects.toThrow('reconnect limit exhausted');
    await client.stop();
    vi.useRealTimers();
  });

  it('reopens when the connection handshake gets no ready response', async () => {
    vi.useFakeTimers();
    const ports = [fakePort(), fakePort()];
    let index = 0;
    const client = new CapturePortClient(() => ports[index++]!.port);
    const started = client.start(traceId);
    await vi.advanceTimersByTimeAsync(5_000);
    await vi.advanceTimersByTimeAsync(250);
    expect(index).toBe(2);
    ports[1]!.emit(ready);
    await expect(started).resolves.toBeUndefined();
    await client.stop();
    vi.useRealTimers();
  });
});
