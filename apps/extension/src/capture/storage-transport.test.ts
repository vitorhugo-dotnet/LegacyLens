import { describe, expect, it } from 'vitest';
import { CAPTURE_TRANSPORT_STORAGE_PREFIX, captureStorageKey, parseCaptureStorageRequest } from './storage-transport.ts';

describe('capture storage transport envelope', () => {
  it('creates and parses a correlated request key', () => {
    const request = { requestId: 'a1b2c3d4e5f60718', tabId: 4, origin: 'http://127.0.0.1:18123',
      message: { type: 'capture.ready', sessionId: 'a'.repeat(32) } };
    expect(captureStorageKey(request.requestId)).toBe(`${CAPTURE_TRANSPORT_STORAGE_PREFIX}${request.requestId}`);
    expect(parseCaptureStorageRequest(captureStorageKey(request.requestId), request)).toEqual(request);
  });

  it.each([
    ['wrong prefix', 'other.key', { requestId: 'a1b2c3d4e5f60718', tabId: 1, origin: 'http://localhost', message: {} }],
    ['mismatched id', captureStorageKey('a1b2c3d4e5f60718'), { requestId: '1111111111111111', tabId: 1, origin: 'http://localhost', message: {} }],
    ['zero id', captureStorageKey('0000000000000000'), { requestId: '0000000000000000', tabId: 1, origin: 'http://localhost', message: {} }],
    ['invalid tab', captureStorageKey('a1b2c3d4e5f60718'), { requestId: 'a1b2c3d4e5f60718', tabId: -1, origin: 'http://localhost', message: {} }],
    ['invalid origin', captureStorageKey('a1b2c3d4e5f60718'), { requestId: 'a1b2c3d4e5f60718', tabId: 1, origin: 'x'.repeat(513), message: {} }],
    ['array message', captureStorageKey('a1b2c3d4e5f60718'), { requestId: 'a1b2c3d4e5f60718', tabId: 1, origin: 'http://localhost', message: [] }],
  ])('rejects %s', (_label, key, request) => expect(parseCaptureStorageRequest(key as string, request)).toBeUndefined());

  it('rejects arrays and null envelopes', () => {
    const key = captureStorageKey('a1b2c3d4e5f60718');
    expect(parseCaptureStorageRequest(key, null)).toBeUndefined();
    expect(parseCaptureStorageRequest(key, [])).toBeUndefined();
  });
});
