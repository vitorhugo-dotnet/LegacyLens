import { afterEach, describe, expect, it, vi } from 'vitest';
import { NativeClient, NativeRequestError } from './client.ts';

function installPort(postMessage: (message: unknown) => void = () => {}) {
  let onMessage!: (message: unknown) => void;
  let onDisconnect!: () => void;
  const port = {
    postMessage,
    onMessage: { addListener(listener: (message: unknown) => void) { onMessage = listener; } },
    onDisconnect: { addListener(listener: () => void) { onDisconnect = listener; } },
  };
  vi.stubGlobal('chrome', { runtime: { connectNative: vi.fn(() => port) } });
  return { port, message: (value: unknown) => onMessage(value), disconnect: () => onDisconnect() };
}
const responseBase = { protocolVersion: 1, requestId: 'request' };
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

describe('NativeClient typed failures', () => {
  it('maps a host error response', async () => {
    const fake = installPort((request) => queueMicrotask(() => fake.message({ ...responseBase,
      requestId: (request as { requestId: string }).requestId, error: { code: 'INVALID', message: 'private detail' } })));
    await expect(new NativeClient().request('trace.ingest', { events: [] })).rejects.toMatchObject({ name: 'NativeRequestError', code: 'host-rejected' });
  });
  it('maps a disconnect', async () => {
    const fake = installPort(() => queueMicrotask(fake.disconnect));
    await expect(new NativeClient().request('trace.ingest', { events: [] })).rejects.toMatchObject({ code: 'disconnected' });
  });
  it('maps postMessage failure', async () => {
    installPort(() => { throw new Error('private transport detail'); });
    await expect(new NativeClient().request('trace.ingest', { events: [] })).rejects.toMatchObject({ code: 'post-failed' });
  });
  it('maps timeout', async () => {
    vi.useFakeTimers(); installPort();
    const request = new NativeClient().request('trace.ingest', { events: [] });
    const rejected = expect(request).rejects.toMatchObject({ name: 'NativeRequestError', code: 'timeout' });
    await vi.advanceTimersByTimeAsync(30_000);
    await rejected;
  });
  it('exposes the safe code without putting raw text in it', () => {
    const error = new NativeRequestError('host-rejected', 'private details');
    expect(error.code).toBe('host-rejected');
    expect(error.message).toContain('private details');
  });
});
